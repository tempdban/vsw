//
// Copyright 2017 Nippon Telegraph and Telephone Corporation.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//   http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//

package rif

/*
#cgo CFLAGS: -I${SRCDIR}/../../include -I/usr/local/include/dpdk -m64 -pthread -O3 -msse4.2
#cgo LDFLAGS: -Wl,-unresolved-symbols=ignore-all -L/usr/local/lib -ldpdk
#include "rif.h"
*/
import "C"

import (
	"errors"
	"fmt"
	"net"
	"sync"
	"unsafe"

	"github.com/lagopus/vsw/dpdk"
	"github.com/lagopus/vsw/utils/notifier"
	"github.com/lagopus/vsw/vswitch"
)

const (
	moduleName = "rif"
	mbufLen    = C.RIF_MBUF_LEN
)

type RIFInstance struct {
	base     *vswitch.BaseInstance
	service  *rifService
	instance *vswitch.RuntimeInstance
	param    *C.struct_rif_instance
	mac      net.HardwareAddr
	mtu      vswitch.MTU
	mode     vswitch.VLANMode
	enabled  bool
}

type RIFVIFInstance struct {
	vif     *vswitch.VIF
	rif     *RIFInstance
	noti    *notifier.Notifier
	notiCh  chan notifier.Notification
	running bool
}

type rifService struct {
	runtime *vswitch.Runtime
	rifs    map[uint32]*RIFInstance
	refcnt  int
	coreid  uint
}

var rifServices = make(map[uint]*rifService)

var log = vswitch.Logger
var mutex sync.Mutex

// TOML Config
type rifConfigSection struct {
	RIF rifConfig
}

type rifConfig struct {
	Core uint
}

var config rifConfig

var defaultConfig = rifConfig{
	Core: 3,
}

// RIF Instance
func getRIFService(coreID uint) (*rifService, error) {
	mutex.Lock()
	defer mutex.Unlock()

	if rs, ok := rifServices[coreID]; ok {
		rs.refcnt++
		return rs, nil
	}

	param := C.struct_rif_runtime_param{}
	ops := vswitch.LagopusRuntimeOps(unsafe.Pointer(&C.rif_runtime_ops))
	rt, err := vswitch.NewRuntime(coreID, moduleName, ops, unsafe.Pointer(&param))
	if err != nil {
		return nil, err
	}
	if rt.Enable(); err != nil {
		return nil, err
	}

	rs := &rifService{
		runtime: rt,
		rifs:    make(map[uint32]*RIFInstance),
		refcnt:  1,
		coreid:  coreID,
	}
	rifServices[coreID] = rs
	return rs, nil
}

func (rs *rifService) free() {
	mutex.Lock()
	defer mutex.Unlock()

	rs.refcnt--
	if rs.refcnt == 0 {
		for _, rif := range rs.rifs {
			rif.instance.Unregister()
		}
		rs.runtime.Terminate()
		delete(rifServices, rs.coreid)
	}
}

func loadConfig() {
	c := rifConfigSection{defaultConfig}
	vswitch.GetConfig().Decode(&c)
	config = c.RIF
}

var once sync.Once

func newRIFInstance(base *vswitch.BaseInstance, priv interface{}) (vswitch.Instance, error) {
	once.Do(loadConfig)

	rs, err := getRIFService(config.Core)
	if err != nil {
		return nil, err
	}

	// Craete & register RIF Instance
	r := &RIFInstance{
		base:    base,
		service: rs,
		param:   (*C.struct_rif_instance)(C.malloc(C.sizeof_struct_rif_instance)),
		mode:    vswitch.AccessMode,
		mtu:     vswitch.DefaultMTU,
		enabled: false,
	}

	// set rif_instance parameter
	r.param.base.name = C.CString(base.Name())
	r.param.base.input = (*C.struct_rte_ring)(unsafe.Pointer(base.Input()))
	r.param.base.outputs = &r.param.o[0]
	r.param.mtu = C.int(r.mtu)

	// instantiate
	ri, err := vswitch.NewRuntimeInstance((vswitch.LagopusInstance)(unsafe.Pointer(r.param)))
	if err != nil {
		r.Free()
		return nil, fmt.Errorf("Creating new runtime instance failed: %v", err)
	}

	if err := rs.runtime.Register(ri); err != nil {
		r.Free()
		return nil, fmt.Errorf("Regiteration of a runtime instance failed: %v", err)
	}

	r.instance = ri

	return r, nil
}

func (r *RIFInstance) Free() {
	if r.instance != nil {
		r.instance.Unregister()
	}

	C.free(unsafe.Pointer(r.param.base.name))
	C.free(unsafe.Pointer(r.param))

	r.param = nil
	r.service.free()
}

func (r *RIFInstance) Enable() error {
	if !r.enabled {
		if err := r.instance.Enable(); err != nil {
			return err
		}
		r.enabled = true
	}
	return nil
}

func (r *RIFInstance) Disable() {
	if r.enabled {
		r.instance.Disable()
		r.enabled = false
	}
}

type devcmd int

const (
	RIF_CMD_ADD_VID              = devcmd(C.RIF_CMD_ADD_VID)
	RIF_CMD_DELETE_VID           = devcmd(C.RIF_CMD_DELETE_VID)
	RIF_CMD_SET_MTU              = devcmd(C.RIF_CMD_SET_MTU)
	RIF_CMD_SET_MAC              = devcmd(C.RIF_CMD_SET_MAC)
	RIF_CMD_SET_TRUNK_MODE       = devcmd(C.RIF_CMD_SET_TRUNK_MODE)
	RIF_CMD_SET_ACCESS_MODE      = devcmd(C.RIF_CMD_SET_ACCESS_MODE)
	RIF_CMD_SET_DST_SELF_FORWARD = devcmd(C.RIF_CMD_SET_DST_SELF_FORWARD)
	RIF_CMD_SET_DST_BC_FORWARD   = devcmd(C.RIF_CMD_SET_DST_BC_FORWARD)
	RIF_CMD_SET_DST_MC_FORWARD   = devcmd(C.RIF_CMD_SET_DST_MC_FORWARD)
)

func (c devcmd) String() string {
	var cmdstr = map[devcmd]string{
		RIF_CMD_ADD_VID:              "Add VID",
		RIF_CMD_DELETE_VID:           "Delete VID",
		RIF_CMD_SET_MTU:              "Set MTU",
		RIF_CMD_SET_MAC:              "Set MAC Address",
		RIF_CMD_SET_TRUNK_MODE:       "Set to TRUNK",
		RIF_CMD_SET_ACCESS_MODE:      "Set to ACCESS",
		RIF_CMD_SET_DST_SELF_FORWARD: "Set Dst Self Forward",
		RIF_CMD_SET_DST_BC_FORWARD:   "Set Dst Broadcast Forward",
		RIF_CMD_SET_DST_MC_FORWARD:   "Set Dst Multicast Forward",
	}
	return cmdstr[c]
}

func (r *RIFInstance) control(cmd devcmd, vif *vswitch.VIF, out *dpdk.Ring, mtu vswitch.MTU) error {
	p := &C.struct_rif_control_param{cmd: C.rif_cmd_t(cmd)}

	switch cmd {
	case RIF_CMD_ADD_VID:
		// VID, VIF Index, and output ring
		p.vid = C.int(vif.VID())
		p.index = C.vifindex_t(vif.Index())
		p.output = (*C.struct_rte_ring)(unsafe.Pointer(out))

	case RIF_CMD_SET_MTU:
		// MTU
		p.mtu = C.int(mtu + 14)

	case RIF_CMD_SET_MAC:
		// MAC Address
		p.mac = (*C.struct_ether_addr)(unsafe.Pointer(&r.mac[0]))

	case RIF_CMD_DELETE_VID,
		RIF_CMD_SET_DST_SELF_FORWARD,
		RIF_CMD_SET_DST_BC_FORWARD,
		RIF_CMD_SET_DST_MC_FORWARD:
		// VID, and output ring
		p.vid = C.int(vif.VID())
		p.output = (*C.struct_rte_ring)(unsafe.Pointer(out))

	case RIF_CMD_SET_TRUNK_MODE, RIF_CMD_SET_ACCESS_MODE:
		// None
	}

	rc, err := r.instance.Control(unsafe.Pointer(p))
	if rc == false || err != nil {
		return fmt.Errorf("%v Failed: %v", cmd, err)
	}
	return nil
}

func (r *RIFInstance) SetMACAddress(mac net.HardwareAddr) error {
	oldmac := r.mac
	r.mac = mac
	if err := r.control(RIF_CMD_SET_MAC, nil, nil, 0); err != nil {
		r.mac = oldmac
		return fmt.Errorf("Can't set MAC to %v: %v", r.base.Name(), err)
	}
	return nil
}

func (r *RIFInstance) MACAddress() net.HardwareAddr {
	return r.mac
}

func (r *RIFInstance) MTU() vswitch.MTU {
	return r.mtu
}

func (r *RIFInstance) SetMTU(mtu vswitch.MTU) error {
	if err := r.control(RIF_CMD_SET_MTU, nil, nil, mtu); err != nil {
		return err
	}
	r.mtu = mtu
	return nil
}

func (r *RIFInstance) InterfaceMode() vswitch.VLANMode {
	return r.mode
}

func (r *RIFInstance) SetInterfaceMode(mode vswitch.VLANMode) error {
	cmd := RIF_CMD_SET_TRUNK_MODE
	if mode == vswitch.AccessMode {
		cmd = RIF_CMD_SET_ACCESS_MODE
	}

	if err := r.control(cmd, nil, nil, 0); err != nil {
		return err
	}
	r.mode = mode
	return nil
}

func (r *RIFInstance) AddVID(vid vswitch.VID) error {
	return nil
}

func (r *RIFInstance) DeleteVID(vid vswitch.VID) error {
	return nil
}

func (r *RIFInstance) SetNativeVID(vid vswitch.VID) error {
	return errors.New("Not supported")
}

func (r *RIFInstance) NewVIF(vif *vswitch.VIF) (vswitch.VIFInstance, error) {
	rv := &RIFVIFInstance{
		vif:  vif,
		rif:  r,
		noti: vif.Rules().Notifier(),
	}

	rv.notiCh = rv.noti.Listen()
	go rv.listener()

	return rv, nil
}

func (rv *RIFVIFInstance) Free() {
	rv.noti.Close(rv.notiCh)
	rv.noti = nil
}

func (rv *RIFVIFInstance) SetVRF(vrf *vswitch.VRF) {
}

func (rv *RIFVIFInstance) listener() {
	for n := range rv.notiCh {
		rule, ok := n.Value.(vswitch.Rule)
		if !ok {
			continue
		}

		switch rule.Match {
		case vswitch.MATCH_ANY:
			// Default Output (The same as Output())

		case vswitch.MATCH_ETH_DST_SELF:
			if err := rv.rif.control(RIF_CMD_SET_DST_SELF_FORWARD, rv.vif, rule.Ring, 0); err != nil {
				log.Printf("Setting dst self Forward failed: %v", err)
			}

		case vswitch.MATCH_ETH_DST_BC:
			if err := rv.rif.control(RIF_CMD_SET_DST_BC_FORWARD, rv.vif, rule.Ring, 0); err != nil {
				log.Printf("Setting dst broadcast Forward failed: %v", err)
			}

		case vswitch.MATCH_ETH_DST_MC:
			if err := rv.rif.control(RIF_CMD_SET_DST_MC_FORWARD, rv.vif, rule.Ring, 0); err != nil {
				log.Printf("Setting dst multicast Forward failed: %v", err)
			}
		}
	}
}

func (rv *RIFVIFInstance) Enable() error {
	return rv.rif.control(RIF_CMD_ADD_VID, rv.vif, rv.vif.Output(), 0)
}

func (rv *RIFVIFInstance) Disable() {
	rv.rif.control(RIF_CMD_DELETE_VID, rv.vif, nil, 0)
}

func init() {
	rp := &vswitch.RingParam{
		Count:    mbufLen,
		SocketId: dpdk.SOCKET_ID_ANY, // XXX: This should be the same as socket running the runtime.
	}

	if err := vswitch.RegisterModule(moduleName, newRIFInstance, rp, vswitch.TypeInterface); err != nil {
		log.Fatalf("Failed to register a module '%s': %v", moduleName, err)
	}
}
