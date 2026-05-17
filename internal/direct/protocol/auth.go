package protocol

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"io"
)

func NewNonce() ([]byte, error) {
	nonce := make([]byte, 32)
	_, err := io.ReadFull(rand.Reader, nonce)
	return nonce, err
}

func ComputeProof(secret, fromDevice, toDevice string, nonceA, nonceB []byte) []byte {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte("portshare-direct-v1"))
	mac.Write([]byte(fromDevice))
	mac.Write([]byte{0})
	mac.Write([]byte(toDevice))
	mac.Write([]byte{0})
	mac.Write(nonceA)
	mac.Write(nonceB)
	return mac.Sum(nil)
}

func VerifyProof(secret, fromDevice, toDevice string, nonceA, nonceB, proof []byte) bool {
	expected := ComputeProof(secret, fromDevice, toDevice, nonceA, nonceB)
	return hmac.Equal(expected, proof)
}
