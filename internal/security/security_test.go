package security_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"hubproxy/internal/security"
)

func TestIPValidation(t *testing.T) {
	validator := security.NewIPValidator(1*time.Hour, true) // Skip updates
	require.NotNil(t, validator)

	// Set test CIDRs that include GitHub's documented webhook ranges
	err := validator.SetWebhookCIDRs([]string{
		"192.30.252.0/22",  // Example GitHub webhook range
		"185.199.108.0/22", // Example GitHub range
		"140.82.112.0/20",  // Example GitHub range
		"2a0a:a440::/29",   // Example GitHub IPv6 range
	})
	require.NoError(t, err)

	tests := []struct {
		name     string
		ip       string
		expected bool
	}{
		{
			name:     "Valid GitHub IP",
			ip:       "192.30.252.1",
			expected: true,
		},
		{
			name:     "Invalid GitHub IP",
			ip:       "1.1.1.1",
			expected: false,
		},
		{
			name:     "Invalid IP format",
			ip:       "invalid-ip",
			expected: false,
		},
		{
			name:     "Non-GitHub IPv6 address",
			ip:       "2001:db8::",
			expected: false,
		},
		{
			name:     "Private IP",
			ip:       "192.168.1.1",
			expected: false,
		},
		{
			name:     "Localhost",
			ip:       "127.0.0.1",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := validator.IsGitHubIP(tt.ip)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestSignatureVerification(t *testing.T) {
	secret := "test-secret"
	payload := []byte(`{"test": "payload"}`)

	tests := []struct {
		name        string
		signature   string
		payload     []byte
		secret      string
		expectValid bool
	}{
		{
			name:        "Valid SHA256 signature",
			signature:   security.GenerateSignature(payload, secret),
			payload:     payload,
			secret:      secret,
			expectValid: true,
		},
		{
			name:        "Invalid signature",
			signature:   "sha256=invalid",
			payload:     payload,
			secret:      secret,
			expectValid: false,
		},
		{
			name:        "Empty signature",
			signature:   "",
			payload:     payload,
			secret:      secret,
			expectValid: false,
		},
		{
			name:        "Missing sha256= prefix",
			signature:   "invalid",
			payload:     payload,
			secret:      secret,
			expectValid: false,
		},
		{
			name:        "Different payload",
			signature:   security.GenerateSignature(payload, secret),
			payload:     []byte(`{"different": "payload"}`),
			secret:      secret,
			expectValid: false,
		},
		{
			name:        "Different secret",
			signature:   security.GenerateSignature(payload, secret),
			payload:     payload,
			secret:      "different-secret",
			expectValid: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			valid := security.VerifySignature(tt.signature, tt.payload, tt.secret)
			assert.Equal(t, tt.expectValid, valid)
		})
	}
}
