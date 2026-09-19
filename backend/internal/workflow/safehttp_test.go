package workflow

import (
	"context"
	"testing"
	"time"
)

func TestValidatePublicURL(t *testing.T) {
	ctx := context.Background()

	// Disallow private hosts
	blockedURLs := []string{
		"http://127.0.0.1:8080/secret",
		"http://localhost:5432",
		"http://169.254.169.254/latest/meta-data/",
		"http://10.0.0.1/admin",
		"http://192.168.1.1/",
	}

	for _, raw := range blockedURLs {
		_, err := ValidatePublicURL(ctx, raw, false)
		if err == nil {
			t.Fatalf("expected blocked url for %s, got nil", raw)
		}
	}

	allowedURLs := []string{
		"https://api.github.com/repos",
		"https://hooks.slack.com/services/xxx",
		"http://example.com/api",
	}

	for _, raw := range allowedURLs {
		u, err := ValidatePublicURL(ctx, raw, false)
		if err != nil {
			t.Fatalf("expected allowed url for %s, got error: %v", raw, err)
		}
		if u == nil {
			t.Fatalf("expected non-nil url for %s", raw)
		}
	}
}

func TestValidateSlackWebhookURL(t *testing.T) {
	valid := "https://hooks.slack.com/services/T00000000/B00000000/XXXXXXXXXXXXXXXXXXXXXXXX"
	if err := ValidateSlackWebhookURL(valid); err != nil {
		t.Fatalf("expected valid slack webhook, got: %v", err)
	}

	invalid := []string{
		"http://hooks.slack.com/services/xxx", // not https
		"https://attacker.com/hooks.slack.com",
		"https://localhost:8080/slack",
		"https://slack.com.attacker.com",
	}

	for _, url := range invalid {
		if err := ValidateSlackWebhookURL(url); err == nil {
			t.Fatalf("expected error for invalid slack url %s, got nil", url)
		}
	}
}

func TestValidateDiscordWebhookURL(t *testing.T) {
	valid := "https://discord.com/api/webhooks/123456789/abcdef"
	if err := ValidateDiscordWebhookURL(valid); err != nil {
		t.Fatalf("expected valid discord webhook, got: %v", err)
	}

	invalid := []string{
		"http://discord.com/api/webhooks/123", // not https
		"https://attacker.com/discord.com",
		"https://localhost:8080/discord",
	}

	for _, url := range invalid {
		if err := ValidateDiscordWebhookURL(url); err == nil {
			t.Fatalf("expected error for invalid discord url %s, got nil", url)
		}
	}
}

func TestSafeHTTPClientBlocksPrivate(t *testing.T) {
	client := NewSafeHTTPClient(false, 2*time.Second)

	// Attempt request to loopback
	_, err := client.Get("http://127.0.0.1:8080")
	if err == nil {
		t.Fatal("expected request to 127.0.0.1 to be blocked by dialer, got nil")
	}
}
