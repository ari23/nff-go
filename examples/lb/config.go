// Copyright 2018 Intel Corporation.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package lb

import (
	"encoding/json"
	"os"

	"github.com/intel-go/nff-go/flow"
	"github.com/intel-go/nff-go/packet"
)

type NetworkSubnet struct {
	IPv4 packet.IPv4Subnet `json:"ipv4"`
	IPv6 packet.IPv6Subnet `json:"ipv6"`
}

type IpPort struct {
	Index      uint16        `json:"index"`
	Subnet     NetworkSubnet `json:"subnet"`
	neighCache *packet.NeighboursLookupTable
	macAddress packet.MACAddress
}

type LoadBalancerConfig struct {
	InputPort       IpPort               `json:"input-port"`
	TunnelPort      IpPort               `json:"tunnel-port"`
	TunnelSubnet    NetworkSubnet        `json:"tunnel-subnet"`
	WorkerAddresses []packet.IPv4Address `json:"worker-addresses"`
}

var LBConfig LoadBalancerConfig

func ReadConfig(fileName string) error {
	file, err := os.Open(fileName)
	if err != nil {
		return err
	}
	decoder := json.NewDecoder(file)

	err = decoder.Decode(&LBConfig)
	if err != nil {
		return err
	}

	return nil
}

func InitFlows() {
	ioFlow, err := flow.SetReceiver(LBConfig.InputPort.Index)
	flow.CheckFatal(err)
	flow.CheckFatal(flow.SetHandlerDrop(ioFlow, balancer, nil))
	flow.CheckFatal(flow.SetSender(ioFlow, LBConfig.TunnelPort.Index))
	ioFlow, err = flow.SetReceiver(LBConfig.TunnelPort.Index)
	flow.CheckFatal(flow.SetHandlerDrop(ioFlow, arpHandler, nil))
	flow.CheckFatal(flow.SetStopper(ioFlow))

	LBConfig.InputPort.initPort()
	LBConfig.TunnelPort.initPort()
}

func (port *IpPort) initPort() {
	port.macAddress = flow.GetPortMACAddress(port.Index)
	port.neighCache = packet.NewNeighbourTable(port.Index, port.macAddress, port.Subnet.IPv4.Addr, port.Subnet.IPv6.Addr)
}
