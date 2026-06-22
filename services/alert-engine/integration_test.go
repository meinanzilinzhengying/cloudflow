//go:build integration
// +build integration

package alertengine

import (
	"context"
	"testing"
	"time"

	svcproto "github.com/meinanzilinzhengying/cloudflow/services/proto"
	"github.com/meinanzilinzhengying/cloudflow/services/alert-engine/notifier"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIntegration_AlertEngine_FullWorkflow(t *testing.T) {
	if testing.Short() {
		t.Skip("skip integration in short mode")
	}

	cfg := DefaultConfig()
	cfg.RelationalDBHost = ""
	cfg.ClickHouseHost = ""
	cfg.AuthAddr = ""

	service, err := New(cfg)
	require.NoError(t, err)
	require.NotNil(t, service)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	t.Run("HealthCheck", func(t *testing.T) {
		status, err := service.health.Check(ctx, nil)
		require.NoError(t, err)
		assert.NotNil(t, status)
		assert.Equal(t, "UNKNOWN", status.Status.String())
	})

	t.Run("AlertRuleCRUD", func(t *testing.T) {
		resp, err := service.CreateRule(ctx, &svcproto.CreateAlertRuleRequest{
			TenantId:       "tenant-1",
			ProjectId:      "project-1",
			Name:           "test-rule",
			DisplayName:    "Test Rule",
			Description:    "Integration test alert rule",
			Severity:       "warning",
			Expression:     "cpu_usage > 80",
			Enabled:        true,
			NotifyChannels: "console",
			NotifyInterval: 60,
		})
		require.NoError(t, err)
		assert.NotNil(t, resp)

		listResp, err := service.ListRules(ctx, &svcproto.ListAlertRulesRequest{TenantId: "tenant-1"})
		require.NoError(t, err)
		assert.GreaterOrEqual(t, len(listResp.Rules), 1)

		updateResp, err := service.UpdateRule(ctx, &svcproto.UpdateAlertRuleRequest{
			RuleId:         resp.RuleId,
			DisplayName:    "Updated Test Rule",
			Description:    "Updated description",
			Severity:       "critical",
			Expression:     "cpu_usage > 90",
			Enabled:        true,
			NotifyChannels: "console,email",
			NotifyInterval: 120,
		})
		require.NoError(t, err)
		assert.NotNil(t, updateResp)

		delResp, err := service.DeleteRule(ctx, &svcproto.DeleteAlertRuleRequest{
			RuleId: resp.RuleId,
		})
		require.NoError(t, err)
		assert.NotNil(t, delResp)
	})

	t.Run("NotificationChannels", func(t *testing.T) {
		require.NotNil(t, service.notifierFactory)
		notifiers, errs, err := service.notifierFactory.CreateMulti([]notifier.ChannelConfig{{Type: "console"}})
		require.NoError(t, err)
		assert.Empty(t, errs)
		assert.Len(t, notifiers, 1)
		assert.Equal(t, "console", notifiers[0].Name())
	})

	t.Run("LeaderElection", func(t *testing.T) {
		require.NotNil(t, service.leaderElection)
		assert.True(t, service.leaderElection.IsLeader())
	})

	t.Run("RateLimitMiddleware", func(t *testing.T) {
		require.NotNil(t, service.rateLimiter)
	})

	t.Run("Configurability", func(t *testing.T) {
		assert.Equal(t, 15*time.Second, service.config.EvalInterval)
		assert.Equal(t, 30*time.Second, service.config.HTTPReadTimeout)
		assert.Equal(t, 30*time.Second, service.config.HTTPWriteTimeout)
	})
}
