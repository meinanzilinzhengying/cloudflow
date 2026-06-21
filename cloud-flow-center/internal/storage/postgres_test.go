//go:build linux

package storage_test

import (
	"testing"
	"time"

	"github.com/meinanzilinzhengying/cloudflow/center/internal/storage"
	"github.com/meinanzilinzhengying/cloudflow/center/pkg/logger"
)

func TestPostgresConfig(t *testing.T) {
	cfg := storage.DefaultPostgresConfig()
	if cfg.Database != "cloudflow" {
		t.Errorf("expected database 'cloudflow', got %s", cfg.Database)
	}
	if cfg.MaxOpenConn != 25 {
		t.Errorf("expected MaxOpenConn 25, got %d", cfg.MaxOpenConn)
	}
	if cfg.MaxIdleConn != 5 {
		t.Errorf("expected MaxIdleConn 5, got %d", cfg.MaxIdleConn)
	}
	if cfg.ConnMaxLife != 5*time.Minute {
		t.Errorf("expected ConnMaxLife 5m, got %v", cfg.ConnMaxLife)
	}
}

func TestNewPostgresEngineNilDB(t *testing.T) {
	cfg := storage.DefaultPostgresConfig()
	log := logger.New(logger.Config{})
	_, err := storage.NewPostgresEngine(nil, cfg, log)
	if err == nil {
		t.Fatal("expected error for nil db")
	}
	if err.Error() != "db connection required" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestPostgresConfigFields(t *testing.T) {
	cfg := storage.DefaultPostgresConfig()
	if cfg.DSN != "" {
		t.Errorf("expected empty DSN by default, got %s", cfg.DSN)
	}
	if cfg.Database == "" {
		t.Error("expected non-empty Database")
	}
}
