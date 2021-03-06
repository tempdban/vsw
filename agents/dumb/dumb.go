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

package dumb

import "github.com/lagopus/vsw/vswitch"

var log = vswitch.Logger

type dumb struct{}

const name = "dumb"

func (d *dumb) Start() bool {
	log.Println("Start dumb agent.")
	return true
}

func (d *dumb) Stop() {
	log.Println("Stops dumb agent.")
}

func (d *dumb) String() string {
	return name
}

func init() {
	vswitch.RegisterAgent(&dumb{})
}
