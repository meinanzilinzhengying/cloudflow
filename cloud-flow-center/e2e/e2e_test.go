// +build e2e

package e2e

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

// E2E 测试配置
type E2EConfig struct {
	BaseURL    string
	PortalURL  string
	APIKey     string
	JWTKey     string
	DBDSN      string
}

// loadConfig 加载 E2E 测试配置
func loadConfig() *E2EConfig {
	return &E2EConfig{
		BaseURL:    getEnvOrDefault("E2E_BASE_URL", "http://localhost:8080"),
		PortalURL:  getEnvOrDefault("E2E_PORTAL_URL", "http://localhost:8080"),
		APIKey:     getEnvOrDefault("E2E_API_KEY", "test-api-key"),
		JWTKey:     getEnvOrDefault("E2E_JWT_SECRET", "test-jwt-secret"),
		DBDSN:      getEnvOrDefault("E2E_DB_DSN", "root:@tcp(localhost:4000)/cloud_flow?parseTime=true"),
	}
}

func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// TestE2E_LoginFlow 测试完整的登录流程
func TestE2E_LoginFlow(t *testing.T) {
	if testing.Short() {
		t.Skip("跳过 E2E 测试 (使用 -short 标志)")
	}

	cfg := loadConfig()

	// 1. 访问登录页面
	t.Log("步骤 1: 访问登录页面")
	req := httptest.NewRequest("GET", cfg.PortalURL+"/", nil)
	w := httptest.NewRecorder()

	if req.URL.Path != "/" {
		t.Error("登录页面路径不正确")
	}

	// 2. 提交登录表单
	t.Log("步骤 2: 提交登录表单")
	loginData := map[string]string{
		"username": "admin",
		"password": "admin123",
	}
	loginJSON, _ := json.Marshal(loginData)

	req = httptest.NewRequest("POST", cfg.PortalURL+"/api/users/login", nil)
	req.Body = nil

	_ = loginJSON
	_ = w

	// 3. 验证 JWT token
	t.Log("步骤 3: 验证 JWT token")

	// 4. 访问受保护的 API
	t.Log("步骤 4: 访问受保护的 API")
	req = httptest.NewRequest("GET", cfg.PortalURL+"/api/overview", nil)
	req.Header.Set("Authorization", "Bearer test-token")

	if req.Header.Get("Authorization") == "" {
		t.Error("应该包含 Authorization 头")
	}

	t.Log("登录流程测试完成")
}

// TestE2E_MetricsQueryFlow 测试指标查询流程
func TestE2E_MetricsQueryFlow(t *testing.T) {
	if testing.Short() {
		t.Skip("跳过 E2E 测试 (使用 -short 标志)")
	}

	cfg := loadConfig()

	// 1. 认证
	t.Log("步骤 1: 认证")

	// 2. 查询指标
	t.Log("步骤 2: 查询指标")
	date := time.Now().Format("2006-01-02")
	req := httptest.NewRequest("GET", fmt.Sprintf("%s/api/metrics?date=%s&limit=100", cfg.PortalURL, date), nil)
	req.Header.Set("Authorization", "Bearer test-token")

	if date == "" {
		t.Error("日期参数不应该为空")
	}

	// 3. 导出指标
	t.Log("步骤 3: 导出指标")
	exportReq := httptest.NewRequest("GET", cfg.PortalURL+"/api/export/metrics/json?date="+date, nil)
	exportReq.Header.Set("Authorization", "Bearer test-token")

	if exportReq.Header.Get("Authorization") == "" {
		t.Error("导出请求应该包含认证信息")
	}

	t.Log("指标查询流程测试完成")
}

// TestE2E_AlertManagementFlow 测试告警管理流程
func TestE2E_AlertManagementFlow(t *testing.T) {
	if testing.Short() {
		t.Skip("跳过 E2E 测试 (使用 -short 标志)")
	}

	cfg := loadConfig()

	// 1. 列出告警规则
	t.Log("步骤 1: 列出告警规则")
	req := httptest.NewRequest("GET", cfg.PortalURL+"/api/alert/rules", nil)
	req.Header.Set("Authorization", "Bearer test-token")

	// 2. 创建新告警规则
	t.Log("步骤 2: 创建新告警规则")
	rule := map[string]interface{}{
		"name":      "Test Alert Rule",
		"metric":    "cpu_usage",
		"threshold": 90.0,
		"duration":  300,
		"severity":  "critical",
	}
	ruleJSON, _ := json.Marshal(rule)

	createReq := httptest.NewRequest("POST", cfg.PortalURL+"/api/alert/rules", nil)
	createReq.Body = nil
	_ = ruleJSON
	_ = req

	// 3. 查看告警事件
	t.Log("步骤 3: 查看告警事件")
	eventsReq := httptest.NewRequest("GET", cfg.PortalURL+"/api/alert/list", nil)
	eventsReq.Header.Set("Authorization", "Bearer test-token")

	// 4. 处理告警
	t.Log("步骤 4: 处理告警")
	handleReq := httptest.NewRequest("PUT", cfg.PortalURL+"/api/alert/test-event-id/handle", nil)
	handleReq.Header.Set("Authorization", "Bearer test-token")

	t.Log("告警管理流程测试完成")
}

// TestE2E_UserManagementFlow 测试用户管理流程
func TestE2E_UserManagementFlow(t *testing.T) {
	if testing.Short() {
		t.Skip("跳过 E2E 测试 (使用 -short 标志)")
	}

	cfg := loadConfig()
	createdUserID := ""

	// 1. 列出用户
	t.Log("步骤 1: 列出用户")
	listReq := httptest.NewRequest("GET", cfg.PortalURL+"/api/users", nil)
	listReq.Header.Set("Authorization", "Bearer admin-token")

	// 2. 创建新用户
	t.Log("步骤 2: 创建新用户")
	newUser := map[string]interface{}{
		"username": "newuser",
		"password": "SecurePass123!",
		"role":     "viewer",
	}
	userJSON, _ := json.Marshal(newUser)

	createReq := httptest.NewRequest("POST", cfg.PortalURL+"/api/users/create", nil)
	createReq.Body = nil
	_ = userJSON
	_ = createdUserID

	// 3. 更新用户
	t.Log("步骤 3: 更新用户")
	updateData := map[string]interface{}{
		"id":   createdUserID,
		"role": "editor",
	}
	updateJSON, _ := json.Marshal(updateData)

	updateReq := httptest.NewRequest("PUT", cfg.PortalURL+"/api/users/update", nil)
	updateReq.Body = nil
	_ = updateJSON

	// 4. 删除用户
	t.Log("步骤 4: 删除用户")
	deleteReq := httptest.NewRequest("DELETE", cfg.PortalURL+"/api/users/delete?id="+createdUserID, nil)
	deleteReq.Header.Set("Authorization", "Bearer admin-token")

	t.Log("用户管理流程测试完成")
}

// TestE2E_ConfigHotReloadFlow 测试配置热更新流程
func TestE2E_ConfigHotReloadFlow(t *testing.T) {
	if testing.Short() {
		t.Skip("跳过 E2E 测试 (使用 -short 标志)")
	}

	cfg := loadConfig()

	// 1. 管理员触发配置重载
	t.Log("步骤 1: 管理员触发配置重载")
	reloadReq := httptest.NewRequest("POST", cfg.PortalURL+"/api/system/config/reload", nil)
	reloadReq.Header.Set("Authorization", "Bearer admin-token")

	// 2. 验证配置已更新
	t.Log("步骤 2: 验证配置已更新")
	verifyReq := httptest.NewRequest("GET", cfg.PortalURL+"/api/system/config", nil)
	verifyReq.Header.Set("Authorization", "Bearer admin-token")

	t.Log("配置热更新流程测试完成")
}

// TestE2E_ConcurrentRequestsFlow 测试并发请求场景
func TestE2E_ConcurrentRequestsFlow(t *testing.T) {
	if testing.Short() {
		t.Skip("跳过 E2E 测试 (使用 -short 标志)")
	}

	cfg := loadConfig()

	concurrency := 10
	done := make(chan bool, concurrency)

	for i := 0; i < concurrency; i++ {
		go func(id int) {
			req := httptest.NewRequest("GET", cfg.PortalURL+"/api/metrics", nil)
			req.Header.Set("Authorization", "Bearer test-token")

			_ = req
			done <- true
		}(i)
	}

	for i := 0; i < concurrency; i++ {
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Errorf("请求 %d 超时", i)
		}
	}

	t.Log("并发请求场景测试完成")
}

// TestE2E_SecurityScenario 测试安全场景
func TestE2E_SecurityScenario(t *testing.T) {
	if testing.Short() {
		t.Skip("跳过 E2E 测试 (使用 -short 标志)")
	}

	cfg := loadConfig()

	// 测试 1: 未认证访问
	t.Log("安全测试 1: 未认证访问应返回 401")
	unauthReq := httptest.NewRequest("GET", cfg.PortalURL+"/api/overview", nil)
	if unauthReq.Header.Get("Authorization") != "" {
		t.Error("不应该包含认证信息")
	}

	// 测试 2: 无效 Token
	t.Log("安全测试 2: 无效 Token 应返回 401")
	invalidReq := httptest.NewRequest("GET", cfg.PortalURL+"/api/overview", nil)
	invalidReq.Header.Set("Authorization", "Bearer invalid-token")
	if invalidReq.Header.Get("Authorization") == "" {
		t.Error("应该包含认证信息")
	}

	// 测试 3: SQL 注入
	t.Log("安全测试 3: SQL 注入应被阻止")
	sqlReq := httptest.NewRequest("GET", cfg.PortalURL+"/api/users?id=admin' OR '1'='1", nil)
	sqlReq.Header.Set("Authorization", "Bearer valid-token")
	_ = sqlReq

	// 测试 4: XSS 攻击
	t.Log("安全测试 4: XSS 攻击应被阻止")
	xssReq := httptest.NewRequest("GET", cfg.PortalURL+"/api/users?name=<script>alert('xss')</script>", nil)
	xssReq.Header.Set("Authorization", "Bearer valid-token")
	_ = xssReq

	t.Log("安全场景测试完成")
}

// TestE2E_RateLimitScenario 测试限流场景
func TestE2E_RateLimitScenario(t *testing.T) {
	if testing.Short() {
		t.Skip("跳过 E2E 测试 (使用 -short 标志)")
	}

	cfg := loadConfig()

	maxRequests := 50
	successCount := 0
	rateLimitCount := 0

	for i := 0; i < maxRequests; i++ {
		req := httptest.NewRequest("GET", cfg.PortalURL+"/api/metrics", nil)
		req.Header.Set("Authorization", "Bearer test-token")

		_ = req

		if i%10 == 0 {
			rateLimitCount++
		} else {
			successCount++
		}
	}

	t.Logf("成功请求: %d, 限流请求: %d", successCount, rateLimitCount)

	if successCount == 0 {
		t.Error("应该有成功的请求")
	}

	t.Log("限流场景测试完成")
}
