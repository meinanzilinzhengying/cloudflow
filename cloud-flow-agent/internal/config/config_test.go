package config

import (
	"strings"
	"testing"
)

func TestConfigSummary(t *testing.T) {
	cfg := &Config{
		ProbeID:         "probe-1",
		EdgeAddr:        "edge:50051",
		CollectInterval: 10,
		BatchSize:       100,
		APIKey:          "my-secret-key",
		Collect: CollectConfig{
			CPU:     true,
			Memory:  true,
			Network: true,
		},
	}
	summary := cfg.Summary()
	if summary == "" {
		t.Fatal("Summary() should not return empty")
	}
	// API Key should be masked
	if strings.Contains(summary, "my-secret-key") {
		t.Error("Summary() should not contain plaintext API Key")
	}
}
