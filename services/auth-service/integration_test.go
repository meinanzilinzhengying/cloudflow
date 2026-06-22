//go:build integration
// +build integration

package authservice

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIntegration_AuthService_FullWorkflow(t *testing.T) {
	if testing.Short() {
		t.Skip("skip integration in short mode")
	}

	cfg := DefaultConfig()
	cfg.RelationalDBHost = "" // skip real DB

	service, err := New(cfg)
	require.NoError(t, err)
	require.NotNil(t, service)

	t.Run("RateLimitMiddleware", func(t *testing.T) {
		require.NotNil(t, service.rateLimiter)
	})

	t.Run("ConnectionLimiter", func(t *testing.T) {
		require.NotNil(t, service.connLimiter)
	})

	t.Run("AuditLog", func(t *testing.T) {
		require.NotNil(t, service.auditor)
	})

	t.Run("HealthCheck", func(t *testing.T) {
		assert.NotNil(t, service.health)
	})

	t.Run("Configurability", func(t *testing.T) {
		assert.Equal(t, "auth-service", service.config.ServiceName)
		assert.Equal(t, "1.0.0", service.config.Version)
	})
}
