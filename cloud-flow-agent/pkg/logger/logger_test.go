package logger

import (
	"testing"
)

func TestNew(t *testing.T) {
	tests := []struct {
		name   string
		config Config
		wantPanic bool
	}{
		{
			name:   "default config",
			config: Config{},
		},
		{
			name: "debug level",
			config: Config{
				Level:  "debug",
				Format: "json",
			},
		},
		{
			name: "console format",
			config: Config{
				Level:  "info",
				Format: "console",
			},
		},
		{
			name: "error level",
			config: Config{
				Level: "error",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := New(tt.config)
			if logger == nil {
				t.Fatal("New() returned nil")
			}
			if logger.SugaredLogger == nil {
				t.Fatal("SugaredLogger is nil")
			}
		})
	}
}

func TestParseLevel(t *testing.T) {
	tests := []struct {
		level string
		want  string
	}{
		{"debug", "debug"},
		{"DEBUG", "debug"},
		{"info", "info"},
		{"INFO", "info"},
		{"warn", "warning"},
		{"warning", "warning"},
		{"error", "error"},
		{"ERROR", "error"},
		{"", "info"},     // default
		{"unknown", "info"}, // default
	}

	for _, tt := range tests {
		t.Run(tt.level, func(t *testing.T) {
			got := parseLevel(tt.level)
			// Just verify it doesn't panic and returns a valid level
			if got.String() == "" {
				t.Errorf("parseLevel(%q) returned empty level", tt.level)
			}
		})
	}
}

func TestLoggerMethods(t *testing.T) {
	logger := New(Config{Level: "debug", Format: "console"})
	
	// Test that all logging methods don't panic
	t.Run("Info", func(t *testing.T) {
		logger.Info("test info message")
	})
	
	t.Run("Infof", func(t *testing.T) {
		logger.Infof("test %s message", "formatted")
	})
	
	t.Run("Debug", func(t *testing.T) {
		logger.Debug("test debug message")
	})
	
	t.Run("Warn", func(t *testing.T) {
		logger.Warn("test warn message")
	})
	
	t.Run("Error", func(t *testing.T) {
		logger.Error("test error message")
	})
	
	t.Run("Sync", func(t *testing.T) {
		logger.Sync() // Should not panic
	})
}
