// Copyright 2018 Intel Corporation.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package packet

import (
	"fmt"
	"sync"

	"github.com/intel-go/nff-go/common"
)

type NeighboursLookupTable struct {
	portIndex         uint16
	ipv4Table         sync.Map
	ipv6Table         sync.Map
	interfaceMAC      MACAddress
	ipv4InterfaceAddr IPv4Address
	ipv6InterfaceAddr IPv6Address
}

func NewNeighbourTable(index uint16, mac MACAddress, ipv4 IPv4Address, ipv6 IPv6Address) *NeighboursLookupTable {
	return &NeighboursLookupTable{
		portIndex:         index,
		interfaceMAC:      mac,
		ipv4InterfaceAddr: ipv4,
		ipv6InterfaceAddr: ipv6,
	}
}

// HandleIPv4ARPRequest processes IPv4 ARP request and reply packets
// and sends an ARP response (if needed) to the same interface. Packet
// has to have L3 parsed. If ARP request packet has VLAN tag, VLAN tag
// is copied into reply packet.
func (table *NeighboursLookupTable) HandleIPv4ARPPacket(pkt *Packet) error {
	arp := pkt.GetARPNoCheck()

	if SwapBytesUint16(arp.Operation) != ARPRequest {
		// Handle ARP reply and record information in lookup table
		if SwapBytesUint16(arp.Operation) == ARPReply {
			ipv4 := ArrayToIPv4(arp.SPA)
			table.ipv4Table.Store(ipv4, arp.SHA)
		}
		return nil
	}

	// Check that someone is asking about MAC of my IP address and HW
	// address is blank in request
	if BytesToIPv4(arp.TPA[0], arp.TPA[1], arp.TPA[2], arp.TPA[3]) != table.ipv4InterfaceAddr {
		return fmt.Errorf("Warning! Got an ARP packet with target IPv4 address %s different from IPv4 address on interface. Should be %s. ARP request ignored.", IPv4ArrayToString(arp.TPA), table.ipv4InterfaceAddr.String())
	}
	if arp.THA != (MACAddress{}) {
		return fmt.Errorf("Warning! Got an ARP packet with non-zero MAC address %s. ARP request ignored.", MACToString(arp.THA))
	}

	// Prepare an answer to this request
	answerPacket, err := NewPacket()
	if err != nil {
		common.LogFatal(common.Debug, err)
	}

	InitARPReplyPacket(answerPacket, table.interfaceMAC, arp.SHA, ArrayToIPv4(arp.TPA), ArrayToIPv4(arp.SPA))
	vlan := pkt.GetVLAN()
	if vlan != nil {
		answerPacket.AddVLANTag(SwapBytesUint16(vlan.TCI))
	}

	answerPacket.SendPacket(table.portIndex)
	return nil
}

// LookupMACForIPv4 tries to find MAC address for specified IPv4
// address.
func (table *NeighboursLookupTable) LookupMACForIPv4(ipv4 IPv4Address) (MACAddress, bool) {
	v, found := table.ipv4Table.Load(ipv4)
	if found {
		return v.(MACAddress), true
	}
	return [common.EtherAddrLen]byte{}, false
}

// SendARPRequestForIPv4 sends an ARP request for specified IPv4
// address. If specified vlan tag is not zero, ARP request packet gets
// VLAN tag assigned to it.
func (table *NeighboursLookupTable) SendARPRequestForIPv4(ipv4 IPv4Address, vlan uint16) {
	requestPacket, err := NewPacket()
	if err != nil {
		common.LogFatal(common.Debug, err)
	}

	InitARPRequestPacket(requestPacket, table.interfaceMAC, table.ipv4InterfaceAddr, ipv4)

	if vlan != 0 {
		requestPacket.AddVLANTag(vlan)
	}

	requestPacket.SendPacket(table.portIndex)
}
