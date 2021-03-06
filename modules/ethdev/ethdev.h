/*
 * Copyright 2017 Nippon Telegraph and Telephone Corporation.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *   http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

#ifndef _LAGOPUS_MODULES_ETHDEV_H
#define _LAGOPUS_MODULES_ETHDEV_H

#include <stdint.h>
#include <stdlib.h>

#include <rte_ring.h>

#include "runtime.h"
#include "packet.h"

#define MAX_VID 4096

//
// ETHDEV Instances
//
struct ethdev_tx_instance;
struct ethdev_rx_instance;

struct ethdev_instance {
	struct lagopus_instance base;
	struct rte_ring *o[MAX_VID];	// Internal buffers for output rings (connected to base.outputs)
	unsigned port_id;		// Port ID of the ether device
	bool trunk;			// TRUNK port or not
	uint16_t vid;			// VID for NATIVE or ACCESS VLAN (-1 = disabled)
	vifindex_t index[MAX_VID];	// VID to VIF index

	void (*tx)(struct ethdev_tx_instance*, struct rte_mbuf**, int);
	void (*rx)(struct ethdev_rx_instance*, struct rte_mbuf**, int);
};

struct ethdev_tx_instance {
	struct ethdev_instance common;
	uint16_t nb_tx_desc;

	// Filled by Runtime
	struct ethdev_runtime *r;

	unsigned tx_count;		// TX packet counts
	unsigned tx_dropped;		// TX dropped packet counts
};

typedef enum {
	ETHDEV_FWD_TYPE_NONE = 0x0,
	ETHDEV_FWD_TYPE_SELF = 0x1,
	ETHDEV_FWD_TYPE_BC   = 0x2,
	ETHDEV_FWD_TYPE_MC   = 0x4
} fwd_type_t;

struct ethdev_rx_instance {
	struct ethdev_instance common;
	uint16_t nb_rx_desc;

	// Filled by Runtime
	struct ether_addr self_addr;	// MAC Address of the port
	struct rte_ring *fwd[MAX_VID];	// Output rings for forwarding packets
	fwd_type_t fwd_type[MAX_VID];	// Packet types to forward

	unsigned rx_count;		// RX packet counts
	unsigned rx_dropped;		// RX dropped packet counts
};

//
// For control
//
typedef enum {
	ETHDEV_CMD_ADD_VID,
	ETHDEV_CMD_DELETE_VID,
	ETHDEV_CMD_SET_TRUNK_MODE,
	ETHDEV_CMD_SET_ACCESS_MODE,
	ETHDEV_CMD_SET_NATIVE_VID,
	ETHDEV_CMD_SET_DST_SELF_FORWARD,
	ETHDEV_CMD_SET_DST_BC_FORWARD,
	ETHDEV_CMD_SET_DST_MC_FORWARD,
	ETHDEV_CMD_UPDATE_MAC,
} ethdev_cmd_t;

struct ethdev_control_param {
	ethdev_cmd_t cmd;
	int vid;
	vifindex_t index;
	struct rte_ring *output;
};

struct ethdev_runtime_param {
	struct rte_mempool *pool;	// Mempool to use for RX
};

// A length of input ring
#define ETHDEV_MBUF_LEN 1024	// XXX: Must be configurable

// Runtime OPs
extern struct lagopus_runtime_ops ethdev_tx_runtime_ops;
extern struct lagopus_runtime_ops ethdev_rx_runtime_ops;

#endif // _LAGOPUS_MODULES_ETHDEV_H
