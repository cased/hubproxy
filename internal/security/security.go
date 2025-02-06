package security

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
)

// GenerateSignature generates a SHA256 HMAC signature for the given payload and secret
func GenerateSignature(payload []byte, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

// VerifySignature verifies that the provided signature matches the payload and secret
func VerifySignature(signature string, payload []byte, secret string) bool {
	expectedSignature := GenerateSignature(payload, secret)
	return hmac.Equal([]byte(signature), []byte(expectedSignature))
}
