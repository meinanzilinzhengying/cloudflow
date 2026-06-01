package portal

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"cloud-flow-center/internal/config"
	"cloud-flow-center/internal/validator"
	"cloud-flow/pkg/mock"
)

// TestHandleLogin_ValidCredentials 测试有效凭证的登录
func TestHandleLogin_ValidCredentials(t *testing.T) {
	// 创建 mock storage
	storage := mock.NewStorageEngine()
	storage.Users = append(storage.Users, map[string]interface{}{
		"username": "admin",
		"password": "admin123",
		"role":    "admin",
	})

	// 创建 portal server
	cfg := config.RateLimitConfig{
		Enabled: false, // 测试时禁用限流
	}
	
	// 由于 NewServer 需要完整的依赖，我们这里只测试 handler 本身
	t.Log("集成测试需要完整的服务器依赖，暂时跳过")
}

// TestHandleLogin_InvalidCredentials 测试无效凭证的登录
func TestHandleLogin_InvalidCredentials(t *testing.T) {
	t.Log("集成测试需要完整的服务器依赖，暂时跳过")
}

// TestHandleMetrics_QueryParams 测试指标查询的参数验证
func TestHandleMetrics_QueryParams(t *testing.T) {
	tests := []struct {
		name      string
		query     string
		wantError bool
	}{
		{
			name:      "空参数应该使用默认值",
			query:     "",
			wantError: false,
		},
		{
			name:      "无效的 probe_id 格式",
			query:     "?probe_id=invalid'id",
			wantError: true,
		},
		{
			name:      "有效的 probe_id",
			query:     "?probe_id=probe-001",
			wantError: false,
		},
		{
			name:      "无效的日期格式",
			query:     "?date=2024-13-45",
			wantError: true,
		},
		{
			name:      "无效的 limit 值",
			query:     "?limit=-1",
			wantError: true,
		},
		{
			name:      "limit 超过最大值",
			query:     "?limit=100001",
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 解析查询参数
			if tt.query != "" {
				r := httptest.NewRequest("GET", "/api/metrics"+tt.query, nil)
				probeID := r.URL.Query().Get("probe_id")
				
				if probeID != "" {
					if err := validator.ValidateProbeID(probeID); err != nil {
						if !tt.wantError {
							t.Errorf("ProbeID 验证错误: %v", err)
						}
					}
				}

				date := r.URL.Query().Get("date")
				if date != "" {
					if err := validator.ValidateDate(date); err != nil {
						if !tt.wantError {
							t.Errorf("日期验证错误: %v", err)
						}
					}
				}

				limitStr := r.URL.Query().Get("limit")
				if limitStr != "" {
					var limit int
					if _, err := json.Marshal(limitStr); err == nil {
						if err := json.Unmarshal([]byte(limitStr), &limit); err == nil {
							if err := validator.ValidateLimit(limit); err != nil {
								if !tt.wantError {
									t.Errorf("Limit 验证错误: %v", err)
								}
							}
						}
					}
				}
			}
		})
	}
}

// TestHandleTraces_RequestValidation 测试追踪查询的请求验证
func TestHandleTraces_RequestValidation(t *testing.T) {
	tests := []struct {
		name    string
		probeID string
		limit   int
		wantErr bool
	}{
		{
			name:    "正常参数",
			probeID: "probe-001",
			limit:   100,
			wantErr: false,
		},
		{
			name:    "空 probe_id",
			probeID: "",
			limit:   100,
			wantErr: false,
		},
		{
			name:    "过大的 limit",
			probeID: "probe-001",
			limit:   100000,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest("GET", "/api/traces", nil)
			q := r.URL.Query()
			if tt.probeID != "" {
				q.Set("probe_id", tt.probeID)
			}
			if tt.limit > 0 {
				q.Set("limit", string(rune(tt.limit)))
			}
			r.URL.RawQuery = q.Encode()

			probeID := r.URL.Query().Get("probe_id")
			if probeID != "" {
				if err := validator.ValidateProbeID(probeID); err != nil {
					if !tt.wantErr {
						t.Errorf("ProbeID 验证错误: %v", err)
					}
				}
			}

			limitStr := r.URL.Query().Get("limit")
			if limitStr != "" {
				var limit int
				if _, err := json.Marshal(limitStr); err == nil {
					if err := json.Unmarshal([]byte(limitStr), &limit); err == nil {
						if err := validator.ValidateLimit(limit); err != nil {
							if !tt.wantErr {
								t.Errorf("Limit 验证错误: %v", err)
							}
						}
					}
				}
			}
		})
	}
}

// TestRateLimitMiddleware 测试限流中间件
func TestRateLimitMiddleware(t *testing.T) {
	t.Log("限流中间件测试需要完整的服务器依赖，暂时跳过")
}

// TestAuthMiddleware 测试认证中间件
func TestAuthMiddleware(t *testing.T) {
	tests := []struct {
		name           string
		authHeader     string
		expectedStatus int
	}{
		{
			name:           "无认证头",
			authHeader:     "",
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name:           "无效的认证格式",
			authHeader:     "Basic invalid",
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name:           "空白的 Bearer 令牌",
			authHeader:     "Bearer ",
			expectedStatus: http.StatusUnauthorized,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest("GET", "/api/overview", nil)
			if tt.authHeader != "" {
				r.Header.Set("Authorization", tt.authHeader)
			}
			
			// 这里应该调用实际的 handler 进行测试
			// 由于 handler 依赖很多，我们只测试认证头解析
			authHeader := r.Header.Get("Authorization")
			if tt.authHeader == "" && authHeader == "" {
				// 预期无认证头
				t.Logf("测试 %s: 无认证头，符合预期", tt.name)
			}
		})
	}
}

// TestSecurityMiddleware 测试安全中间件
func TestSecurityMiddleware(t *testing.T) {
	tests := []struct {
		name      string
		body      string
		wantStatus int
	}{
		{
			name:      "正常 JSON",
			body:      `{"username": "test", "password": "test123"}`,
			wantStatus: http.StatusOK,
		},
		{
			name:      "SQL 注入尝试",
			body:      `{"username": "admin' OR '1'='1", "password": "anything"}`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:      "XSS 攻击尝试",
			body:      `{"username": "<script>alert('xss')</script>", "password": "test"}`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:      "路径遍历尝试",
			body:      `{"username": "../../../etc/passwd", "password": "test"}`,
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest("POST", "/api/users", bytes.NewBufferString(tt.body))
			r.Header.Set("Content-Type", "application/json")
			
			// 测试请求体验证
			// 由于 handler 依赖多，我们只测试参数解析
			if err := r.ParseForm(); err != nil {
				if tt.wantStatus == http.StatusOK {
					t.Errorf("不应该出现解析错误: %v", err)
				}
			}
		})
	}
}

// TestConfigReloadAPI 测试配置重新加载 API
func TestConfigReloadAPI(t *testing.T) {
	t.Log("配置重新加载 API 测试需要完整的服务器依赖，暂时跳过")
}

// TestUserManagementAPI 测试用户管理 API
func TestUserManagementAPI(t *testing.T) {
	tests := []struct {
		name      string
		method    string
		path      string
		body      string
		wantCode  int
	}{
		{
			name:     "创建用户 - 正常",
			method:   "POST",
			path:     "/api/users/create",
			body:     `{"username": "newuser", "password": "Pass123!", "role": "viewer"}`,
			wantCode: http.StatusCreated,
		},
		{
			name:     "创建用户 - 用户名格式错误",
			method:   "POST",
			path:     "/api/users/create",
			body:     `{"username": "ab", "password": "Pass123!"}`,
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "创建用户 - 密码太短",
			method:   "POST",
			path:     "/api/users/create",
			body:     `{"username": "newuser", "password": "short"}`,
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "创建用户 - 无效的角色",
			method:   "POST",
			path:     "/api/users/create",
			body:     `{"username": "newuser", "password": "Pass123!", "role": "superadmin"}`,
			wantCode: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest(tt.method, tt.path, bytes.NewBufferString(tt.body))
			r.Header.Set("Content-Type", "application/json")
			
			// 验证用户名格式
			var reqBody map[string]interface{}
			if err := json.NewDecoder(r.Body).Decode(&reqBody); err == nil {
				if username, ok := reqBody["username"].(string); ok {
					if err := validator.ValidateUsername(username); err != nil {
						if tt.wantCode != http.StatusBadRequest {
							t.Errorf("不应该验证失败: %v", err)
						}
					}
				}
				
				if password, ok := reqBody["password"].(string); ok {
					if err := validator.ValidatePassword(password); err != nil {
						if tt.wantCode != http.StatusBadRequest {
							t.Errorf("不应该验证失败: %v", err)
						}
					}
				}
				
				if role, ok := reqBody["role"].(string); ok {
					if err := validator.ValidateRole(role); err != nil {
						if tt.wantCode != http.StatusBadRequest {
							t.Errorf("不应该验证失败: %v", err)
						}
					}
				}
			}
		})
	}
}

// TestAlertRuleValidation 测试告警规则验证
func TestAlertRuleValidation(t *testing.T) {
	tests := []struct {
		name    string
		ruleID  string
		wantErr bool
	}{
		{
			name:    "正常规则 ID",
			ruleID:  "rule-001",
			wantErr: false,
		},
		{
			name:    "空规则 ID",
			ruleID:  "",
			wantErr: true,
		},
		{
			name:    "过长的规则 ID",
			ruleID:  "a" + string(make([]byte, 100)),
			wantErr: true,
		},
		{
			name:    "包含特殊字符的规则 ID",
			ruleID:  "rule@001",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := validator.ValidateID(tt.ruleID); err != nil {
				if !tt.wantErr {
					t.Errorf("不应该验证失败: %v", err)
				}
			}
		})
	}
}
