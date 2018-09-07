// Only IPv4, Only tunnel, Only ESP, Only AES-128-CBC
package main

import "github.com/intel-go/nff-go/packet"
import "github.com/intel-go/nff-go/flow"
import "github.com/intel-go/nff-go/common"
import "bytes"
import "unsafe"
import "crypto/aes"

const esp = 0x32
const mode1234 = 1234
const espHeadLen = 24
const authLen = 12
const espTailLen = authLen + 2
const etherLen = common.EtherLen
const outerIPLen = common.IPv4MinLen

type espHeader struct {
	SPI uint32
	SEQ uint32
	IV  [16]byte
}

type espTail struct {
	paddingLen uint8
	nextIP     uint8
	Auth       [authLen]byte
}

// General decapsulation
func decapsulation(currentPacket *packet.Packet, context flow.UserContext) bool {
	length := currentPacket.GetPacketLen()
	currentESPHeader := (*espHeader)(currentPacket.StartAtOffset(etherLen + outerIPLen))
	currentESPTail := (*espTail)(unsafe.Pointer(currentPacket.StartAtOffset(uintptr(length) - espTailLen)))
	// Security Association
	switch packet.SwapBytesUint32(currentESPHeader.SPI) {
	case mode1234:
		encryptionPart := (*[common.MaxLength]byte)(unsafe.Pointer(currentPacket.StartAtOffset(0)))[etherLen+outerIPLen+espHeadLen : length-authLen]
		authPart := (*[common.MaxLength]byte)(unsafe.Pointer(currentPacket.StartAtOffset(0)))[etherLen+outerIPLen : length-authLen]
		if decapsulationSPI123(authPart, currentESPTail.Auth, currentESPHeader.IV, encryptionPart, context) == false {
			return false
		}
	default:
		return false
	}
	// Decapsulate
	currentPacket.DecapsulateHead(etherLen, outerIPLen+espHeadLen)
	currentPacket.DecapsulateTail(length-espTailLen-uint(currentESPTail.paddingLen), uint(currentESPTail.paddingLen)+espTailLen)

	return true
}

// Specific decapsulation
func decapsulationSPI123(currentAuth []byte, Auth [authLen]byte, iv [16]byte, ciphertext []byte, context0 flow.UserContext) bool {
	context := (context0).(*sContext)

	context.mac123.Reset()
	context.mac123.Write(currentAuth)
	if bytes.Equal(context.mac123.Sum(nil)[0:12], Auth[:]) == false {
		return false
	}

	// Decryption
	if len(ciphertext) < aes.BlockSize || len(ciphertext)%aes.BlockSize != 0 {
		return false
	}
	context.modeDec.(SetIVer).SetIV(iv[:])
	context.modeDec.CryptBlocks(ciphertext, ciphertext)
	return true
}

// General encapsulation
func vectorEncapsulation(currentPackets []*packet.Packet, mask *[32]bool, notDrop *[32]bool, context flow.UserContext) {
	n := uint(0)
	for i := uint(0); i < 32; i++ {
		if (*mask)[i] == true {
			currentPackets[i].EncapsulateHead(etherLen, outerIPLen+espHeadLen)
			currentPackets[i].ParseL3()
			ipv4 := currentPackets[i].GetIPv4NoCheck()
			ipv4.SrcAddr = packet.BytesToIPv4(111, 22, 3, 0)
			ipv4.DstAddr = packet.BytesToIPv4(3, 22, 111, 0)
			ipv4.VersionIhl = 0x45
			ipv4.NextProtoID = esp
			ipv4.TotalLength = uint16(currentPackets[i].GetPacketLen() - etherLen)
			notDrop[i] = true
			n++
		}
	}
	// TODO All packets will be encapsulated as 1234
	vectorEncapsulationSPI123(currentPackets, n, context)
}

// Specific encapsulation
func vectorEncapsulationSPI123(currentPackets []*packet.Packet, n uint, context0 flow.UserContext) {
	context := context0.(*vContext)
	s := VECTOR * (n/VECTOR + 1)
	var Z uint

	// TODO Only for equal length
	length := currentPackets[0].GetPacketLen()
	paddingLength := uint8((16 - (length-(etherLen+outerIPLen+espHeadLen)-espTailLen)%16) % 16)
	new_length := length + uint(paddingLength) + espTailLen

	for i := uint(0); i < s; i += VECTOR {
		if i == s-VECTOR {
			Z = n % VECTOR
		} else {
			Z = VECTOR
		}
		for t := uint(0); t < Z; t++ {
			currentPackets[i+t].EncapsulateTail(length, uint(paddingLength)+espTailLen)
			currentESPHeader := (*espHeader)(unsafe.Pointer(currentPackets[i+t].StartAtOffset(etherLen + outerIPLen)))
			currentESPHeader.SPI = packet.SwapBytesUint32(mode1234)
			// TODO should be random
			currentESPHeader.IV = [16]byte{0x90, 0x9d, 0x78, 0xa8, 0x72, 0x70, 0x68, 0x00, 0x8f, 0xdc, 0x55, 0x73, 0xa3, 0x75, 0xb5, 0xa7}
			currentESPTail := (*espTail)(unsafe.Pointer(currentPackets[i+t].StartAtOffset(uintptr(new_length) - espTailLen)))
			currentESPTail.paddingLen = paddingLength
			currentESPTail.nextIP = common.IPNumber

			context.vectorEncryptionPart[t] = (*[common.MaxLength]byte)(unsafe.Pointer(currentPackets[i+t].StartAtOffset(0)))[etherLen+outerIPLen+espHeadLen : new_length-authLen]
			context.vectorIV[t] = currentESPHeader.IV[:]
			context.vectorAuthPart[t] = (*[common.MaxLength]byte)(unsafe.Pointer(currentPackets[i+t].StartAtOffset(0)))[etherLen+outerIPLen : new_length-authLen]
			context.vectorAuthPlace[t] = currentESPTail.Auth[:]
		}
		Encrypt(context.vectorEncryptionPart, context.vectorEncryptionPart, context.vectorIV, Z, context)
		Authenticate(context.vectorAuthPart, context.vectorAuthPlace, Z, context)
	}
}

// General encapsulation
func scalarEncapsulation(currentPacket *packet.Packet, context flow.UserContext) bool {
	currentPacket.EncapsulateHead(etherLen, outerIPLen+espHeadLen)

	currentPacket.ParseL3()
	ipv4 := currentPacket.GetIPv4NoCheck()
	ipv4.SrcAddr = packet.BytesToIPv4(111, 22, 3, 0)
	ipv4.DstAddr = packet.BytesToIPv4(3, 22, 111, 0)
	ipv4.VersionIhl = 0x45
	ipv4.NextProtoID = esp
	ipv4.TotalLength = uint16(currentPacket.GetPacketLen() - etherLen)

	// TODO All packets will be encapsulated as 1234
	scalarEncapsulationSPI123(currentPacket, context)
	return true
}

// Specific encapsulation
func scalarEncapsulationSPI123(currentPacket *packet.Packet, context0 flow.UserContext) {
	context := (context0).(*sContext)
	length := currentPacket.GetPacketLen()
	paddingLength := uint8((16 - (length-(etherLen+outerIPLen+espHeadLen)-espTailLen)%16) % 16)
	newLength := length + uint(paddingLength) + espTailLen
	currentPacket.EncapsulateTail(length, uint(paddingLength)+espTailLen)

	currentESPHeader := (*espHeader)(currentPacket.StartAtOffset(etherLen + outerIPLen))
	currentESPHeader.SPI = packet.SwapBytesUint32(mode1234)
	// TODO should be random
	currentESPHeader.IV = [16]byte{0x90, 0x9d, 0x78, 0xa8, 0x72, 0x70, 0x68, 0x00, 0x8f, 0xdc, 0x55, 0x73, 0xa3, 0x75, 0xb5, 0xa7}

	currentESPTail := (*espTail)(currentPacket.StartAtOffset(uintptr(newLength) - espTailLen))
	currentESPTail.paddingLen = paddingLength
	currentESPTail.nextIP = common.IPNumber

	// Encryption
	EncryptionPart := (*[common.MaxLength]byte)(currentPacket.StartAtOffset(0))[etherLen+outerIPLen+espHeadLen : newLength-authLen]
	context.modeEnc.(SetIVer).SetIV(currentESPHeader.IV[:])
	context.modeEnc.CryptBlocks(EncryptionPart, EncryptionPart)

	// Authentication
	context.mac123.Reset()
	AuthPart := (*[common.MaxLength]byte)(currentPacket.StartAtOffset(0))[etherLen+outerIPLen : newLength-authLen]
	context.mac123.Write(AuthPart)
	copy(currentESPTail.Auth[:], context.mac123.Sum(nil))
}
