// Only IPv4, Only tunnel, Only ESP, Only AES-128-CBC
package main

import "github.com/intel-go/nff-go/examples/ipsec/crypto_mb"

import "crypto/aes"
import "crypto/cipher"
import "crypto/hmac"
import "crypto/sha1"
import "hash"

const VECTOR = 8

type sContext struct {
	mac123  hash.Hash
	modeEnc cipher.BlockMode
	modeDec cipher.BlockMode
}

type vContext struct {
	mac123   crypto_mb.MultiHash
	modeEnc  crypto_mb.MultiBlockMode
	modeDec  crypto_mb.MultiBlockMode

	vectorEncryptionPart [][]byte
	vectorIV             [][]byte
	vectorAuthPart       [][]byte
	vectorAuthPlace      [][]byte

	s sContext
}

type SetIVerM interface {
	SetIV([][]byte)
}

type SetIVer interface {
	SetIV([]byte)
}

func InitSContext() interface{} {
	var auth123Key = []byte("qqqqqqqqqqqqqqqqqqqq")
	var crypt123Key = []byte("AES128Key-16Char")
	block123, _ := aes.NewCipher(crypt123Key)

	tempScalarIV := make([]byte, 16)

	n := new(sContext)
	n.mac123 = hmac.New(sha1.New, auth123Key)
	n.modeEnc = cipher.NewCBCEncrypter(block123, tempScalarIV)
	n.modeDec = cipher.NewCBCDecrypter(block123, tempScalarIV)
	return n
}

func InitVContext() interface{} {
	var auth123Key = []byte("qqqqqqqqqqqqqqqqqqqq")
	var crypt123Key = []byte("AES128Key-16Char")
	block123 := crypto_mb.NewAESMultiBlock(crypt123Key)

	tempVectorIV := make([][]byte, VECTOR, VECTOR)
	for i := 0; i < VECTOR; i++ {
		tempVectorIV[i] = make([]byte, 16)
	}

	n := new(vContext)
	n.mac123 = crypto_mb.NewHmac(crypto_mb.New, auth123Key)
	n.modeEnc = crypto_mb.NewMultiCBCEncrypter(block123, tempVectorIV)
	n.modeDec = crypto_mb.NewMultiCBCDecrypter(block123, tempVectorIV)
	n.vectorEncryptionPart = make([][]byte, VECTOR, VECTOR)
	n.vectorIV = make([][]byte, VECTOR, VECTOR)
	n.vectorAuthPart = make([][]byte, VECTOR, VECTOR)
	n.vectorAuthPlace = make([][]byte, VECTOR, VECTOR)
	n.s = *InitSContext().(*sContext)
	return n
}

func (c sContext) Copy() interface{} {
	return InitSContext()
}

func (c vContext) Copy() interface{} {
        return InitVContext()
}

func (c sContext) Delete() {
}

func (c vContext) Delete() {
}

func Encrypt(EncryptionPart [][]byte, where [][]byte, IV [][]byte, Z uint, context *vContext) {
	if Z != VECTOR {
		for t := uint(0); t < Z; t++ {
			context.s.modeEnc.(SetIVer).SetIV(IV[t])
			context.s.modeEnc.CryptBlocks(EncryptionPart[t], where[t])
		}
	} else {
		context.modeEnc.(SetIVerM).SetIV(IV[:])
		context.modeEnc.CryptManyBlocks(EncryptionPart, where)
	}
}

func Authenticate(AuthenticationPart [][]byte, where [][]byte, Z uint, context *vContext) {
	if Z != VECTOR {
		for t := uint(0); t < Z; t++ {
			context.s.mac123.Reset()
			context.s.mac123.Write(where[t])
			copy(where[t], context.s.mac123.Sum(nil))
		}
	} else {
		context.mac123.Reset()
		context.mac123.Write(context.vectorAuthPart)
		temp := context.mac123.Sum(nil)
		for t := uint(0); t < VECTOR; t++ {
			copy(where[t], temp[t])
		}
	}
}
