package main

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"time"
)

var sampleEvents = []struct {
	Type    string
	Payload interface{}
	Invalid bool // Flag to indicate if this should be sent with invalid signature
}{
	{
		Type: "push",
		Payload: map[string]interface{}{
			"ref": "refs/heads/main",
			"repository": map[string]interface{}{
				"name":      "test-repo",
				"full_name": "user/test-repo",
				"private":   false,
			},
			"sender": map[string]interface{}{
				"login": "test-user",
				"type":  "User",
			},
			"commits": []map[string]interface{}{
				{
					"id":        "abc123",
					"message":   "Test commit",
					"timestamp": time.Now().Format(time.RFC3339),
				},
			},
		},
	},
	{
		Type: "pull_request",
		Payload: map[string]interface{}{
			"action": "opened",
			"number": 1,
			"repository": map[string]interface{}{
				"name":      "test-repo",
				"full_name": "user/test-repo",
				"private":   false,
			},
			"sender": map[string]interface{}{
				"login": "test-user",
				"type":  "User",
			},
			"pull_request": map[string]interface{}{
				"title": "Test PR",
				"body":  "This is a test pull request",
				"head": map[string]interface{}{
					"ref": "feature-branch",
					"sha": "def456",
				},
			},
		},
	},
	{
		Type: "issues",
		Payload: map[string]interface{}{
			"action": "opened",
			"repository": map[string]interface{}{
				"name":      "test-repo",
				"full_name": "user/test-repo",
				"private":   false,
			},
			"sender": map[string]interface{}{
				"login": "test-user",
				"type":  "User",
			},
			"issue": map[string]interface{}{
				"number": 123,
				"title":  "Test Issue",
				"body":   "This is a test issue",
				"state":  "open",
			},
		},
	},
	{
		Type: "push",
		Payload: map[string]interface{}{
			"ref": "refs/heads/main",
			"repository": map[string]interface{}{
				"name":      "test-repo",
				"full_name": "user/test-repo",
				"private":   false,
			},
			"sender": map[string]interface{}{
				"login": "test-user",
				"type":  "User",
			},
			"commits": []map[string]interface{}{
				{
					"id":        "xyz789",
					"message":   "Invalid signature test",
					"timestamp": time.Now().Format(time.RFC3339),
				},
			},
		},
		Invalid: true,
	},
}

func generateSignature(payload []byte, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	sig := "sha256=" + hex.EncodeToString(mac.Sum(nil))
	log.Printf("Generated signature with secret '%s': %s", secret, sig)
	return sig
}

func main() {
	var (
		targetURL = flag.String("url", "http://localhost:8080", "Target URL for webhooks")
		secret    = flag.String("secret", "dev-secret", "Webhook secret")
		delay     = flag.Duration("delay", 2*time.Second, "Delay between webhooks")
	)
	flag.Parse()

	log.Printf("Starting webhook simulation")
	log.Printf("Target URL: %s/webhook", *targetURL)
	log.Printf("Using secret: %q", *secret)
	log.Printf("Delay between webhooks: %v", *delay)

	client := &http.Client{}

	for _, event := range sampleEvents {
		if event.Invalid {
			log.Printf("Sending %s event with INVALID signature...", event.Type)
		} else {
			log.Printf("Sending %s event with valid signature...", event.Type)
		}

		payload, err := json.Marshal(event.Payload)
		if err != nil {
			log.Fatalf("Error marshaling payload: %v", err)
		}
		log.Printf("Payload: %s", string(payload))

		req, err := http.NewRequest("POST", *targetURL+"/webhook", bytes.NewReader(payload))
		if err != nil {
			log.Fatalf("Error creating request: %v", err)
		}

		// Add headers
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-GitHub-Event", event.Type)
		req.Header.Set("X-GitHub-Delivery", fmt.Sprintf("test-%d", time.Now().UnixNano()))

		// Add signature
		if event.Invalid {
			// Use a valid hex string but with wrong secret
			mac := hmac.New(sha256.New, []byte("wrong-secret"))
			mac.Write(payload)
			req.Header.Set("X-Hub-Signature-256", "sha256="+hex.EncodeToString(mac.Sum(nil)))
		} else {
			req.Header.Set("X-Hub-Signature-256", generateSignature(payload, *secret))
		}

		log.Printf("Sending request to %s with headers: %+v", req.URL, req.Header)

		start := time.Now()
		resp, err := client.Do(req)
		if err != nil {
			log.Printf("Error sending webhook: %v", err)
			continue
		}
		resp.Body.Close()

		log.Printf("Response: HTTP %d (%v)", resp.StatusCode, time.Since(start))

		if !event.Invalid {
			time.Sleep(*delay)
		}
	}

	log.Printf("Simulation complete")
}
