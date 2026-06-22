// console.go 控制台通知器（系统内通知，仅记录不发送）
package notifier

import (
	"context"
	"encoding/json"
	"fmt"
)

// consoleNotifier 控制台通知器
type consoleNotifier struct{}

func newConsoleNotifier(_ json.RawMessage) (Notifier, error) {
	return &consoleNotifier{}, nil
}

func (c *consoleNotifier) Name() string {
	return "console"
}

func (c *consoleNotifier) Send(ctx context.Context, msg *Message) error {
	fmt.Printf("[Console Alert] %s | Severity: %s | Tenant: %s | Rule: %s\n  %s\n",
		msg.Title, msg.Severity, msg.TenantID, msg.RuleID, msg.Body)
	return nil
}

func (c *consoleNotifier) Close() error {
	return nil
}
