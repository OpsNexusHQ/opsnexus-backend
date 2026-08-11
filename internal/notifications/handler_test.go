package notifications

import (
	"testing"
)

func TestCreateChannelRequestNormalizeDefaults(t *testing.T) {
	req := createChannelRequest{
		Name: "Webhook Channel",
		URL:  "https://example.com/webhook",
	}

	channel, err := req.normalize()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if channel.Type != "webhook" {
		t.Fatalf("expected default type webhook, got %q", channel.Type)
	}
	if !channel.Enabled {
		t.Fatal("expected enabled to default to true")
	}
}

func TestCreateChannelRequestNormalizeInvalidURL(t *testing.T) {
	req := createChannelRequest{
		Name: "Webhook Channel",
		URL:  "ftp://example.com/webhook",
	}

	_, err := req.normalize()
	if err == nil {
		t.Fatal("expected error for invalid URL scheme")
	}
}

func TestCreateChannelRequestNormalizeInvalidType(t *testing.T) {
	req := createChannelRequest{
		Name: "Webhook Channel",
		URL:  "https://example.com/webhook",
		Type: "unsupported",
	}

	_, err := req.normalize()
	if err == nil {
		t.Fatal("expected error for unsupported channel type")
	}
}
