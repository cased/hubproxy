package security

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"
)

// GitHubMeta represents the response from GitHub's /meta API
type GitHubMeta struct {
	Hooks []string `json:"hooks"`
}

// IPValidator validates if IP addresses are from GitHub's webhook range
type IPValidator struct {
	mu          sync.RWMutex
	webhookCIDR []*net.IPNet
	lastUpdate  time.Time
	updateFreq  time.Duration
}

// NewIPValidator creates a new IP validator that updates GitHub's IP ranges
// at the specified frequency. If skipUpdates is true, it will not perform the
// initial update or start background updates (useful for testing).
func NewIPValidator(updateFreq time.Duration, skipUpdates bool) *IPValidator {
	v := &IPValidator{
		updateFreq: updateFreq,
	}
	if !skipUpdates {
		// Initial update
		if err := v.Update(); err != nil {
			// Log error but continue - we'll retry later
			fmt.Printf("Initial GitHub IP range update failed: %v\n", err)
		}
		// Start background updater
		go v.backgroundUpdate()
	}
	return v
}

// Update fetches the latest IP ranges from GitHub
func (v *IPValidator) Update() error {
	resp, err := http.Get("https://api.github.com/meta")
	if err != nil {
		return fmt.Errorf("fetching GitHub meta: %w", err)
	}
	defer resp.Body.Close()

	var meta GitHubMeta
	if err := json.NewDecoder(resp.Body).Decode(&meta); err != nil {
		return fmt.Errorf("decoding GitHub meta: %w", err)
	}

	cidrs := make([]*net.IPNet, 0, len(meta.Hooks))
	for _, cidr := range meta.Hooks {
		_, ipNet, err := net.ParseCIDR(cidr)
		if err != nil {
			return fmt.Errorf("parsing CIDR %q: %w", cidr, err)
		}
		cidrs = append(cidrs, ipNet)
	}

	v.mu.Lock()
	v.webhookCIDR = cidrs
	v.lastUpdate = time.Now()
	v.mu.Unlock()

	return nil
}

// IsGitHubIP checks if the given IP is in GitHub's webhook range
func (v *IPValidator) IsGitHubIP(ipStr string) bool {
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return false
	}

	v.mu.RLock()
	defer v.mu.RUnlock()

	for _, cidr := range v.webhookCIDR {
		if cidr.Contains(ip) {
			return true
		}
	}
	return false
}

// LastUpdate returns when the IP ranges were last updated
func (v *IPValidator) LastUpdate() time.Time {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return v.lastUpdate
}

// SetWebhookCIDRs sets the webhook CIDRs directly - only used for testing
func (v *IPValidator) SetWebhookCIDRs(cidrs []string) error {
	parsedCIDRs := make([]*net.IPNet, 0, len(cidrs))
	for _, cidr := range cidrs {
		_, ipNet, err := net.ParseCIDR(cidr)
		if err != nil {
			return fmt.Errorf("parsing CIDR %q: %w", cidr, err)
		}
		parsedCIDRs = append(parsedCIDRs, ipNet)
	}

	v.mu.Lock()
	v.webhookCIDR = parsedCIDRs
	v.lastUpdate = time.Now()
	v.mu.Unlock()

	return nil
}

func (v *IPValidator) backgroundUpdate() {
	ticker := time.NewTicker(v.updateFreq)
	defer ticker.Stop()

	for range ticker.C {
		if err := v.Update(); err != nil {
			fmt.Printf("GitHub IP range update failed: %v\n", err)
		}
	}
}
