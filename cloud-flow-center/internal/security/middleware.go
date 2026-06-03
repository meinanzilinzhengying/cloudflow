package security

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/meinanzilinzhengying/cloudflow/center/internal/validator"
)

type SecurityConfig struct {
	EnableParamValidation bool
	EnableAuditLog        bool
	MaxRequestBodySize    int64
	AllowedContentTypes   []string
}

type SecurityMiddleware struct {
	config SecurityConfig
	logger func(format string, args ...interface{})
	// P1-06 修复: 预编译正则表达式，避免每次调用都重新编译
	requestIDRegex *regexp.Regexp
	probeIDRegex   *regexp.Regexp
	dateRegex      *regexp.Regexp
}

func NewSecurityMiddleware(config SecurityConfig, logger func(format string, args ...interface{})) *SecurityMiddleware {
	// P1-06 修复: 预编译正则表达式
	return &SecurityMiddleware{
		config:         config,
		logger:         logger,
		requestIDRegex: regexp.MustCompile(`^[a-zA-Z0-9_-]+$`),
		probeIDRegex:   regexp.MustCompile(`^[a-zA-Z0-9_-]+$`),
		dateRegex:      regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`),
	}
}

type RequestValidationRule struct {
	Path       string
	Method     string
	ParamName  string
	Validator  func(value string) error
	Required   bool
	QueryParam bool
	BodyField  string
}

// P1-06 修复: 使用更精确的 SQL 注入检测模式，减少误报
var (
	// 使用单词边界和更具体的模式，避免误报正常业务输入
	sqlInjectionPatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)\bunion\b.*\bselect\b`),     // UNION SELECT
		regexp.MustCompile(`(?i)\binsert\b.*\binto\b`),      // INSERT INTO
		regexp.MustCompile(`(?i)\bdelete\b.*\bfrom\b`),      // DELETE FROM
		regexp.MustCompile(`(?i)\bupdate\b.*\bset\b`),       // UPDATE SET
		regexp.MustCompile(`(?i)\bdrop\b.*\b(table|database)\b`), // DROP TABLE/DATABASE
		regexp.MustCompile(`(?i)\bexec(ute)?\b.*\b(xp_|sp_)`),   // EXEC xp_/sp_
		regexp.MustCompile(`(?i);\s*(drop|alter|create|insert|delete|update)\b`), // 分号后跟危险命令
		regexp.MustCompile(`(?i)\bor\b\s+\d+=\d+`),          // OR 1=1
		regexp.MustCompile(`(?i)\band\b\s+\d+=\d+`),         // AND 1=1
		regexp.MustCompile(`(?i)'\s*(or|and)\s+'[^']*'\s*=\s*'[^']*'`), // ' OR '1'='1'
		regexp.MustCompile(`(?i)--\s*$`),                    // SQL 注释结尾
		regexp.MustCompile(`/\*.*\*/`),                      // 块注释
		regexp.MustCompile(`(?i)\bwaitfor\b.*\bdelay\b`),    // WAITFOR DELAY
		regexp.MustCompile(`(?i)\bbenchmark\b.*\(`),         // BENCHMARK()
	}
	
	xssPatterns = []string{
		"<script", "</script>", "javascript:",
		"onerror=", "onload=", "onclick=",
		"onmouseover=", "onfocus=", "onblur=",
		"<iframe", "<object", "<embed",
		"<applet", "<form", "onabort=", "onafterprint=",
		"onbeforeprint=", "onbeforeunload=", "oncanplay=",
	}
	
	pathTraversalPatterns = []string{
		"../", "..\\", "%2e%2e", "%252e",
		"./etc/passwd", ".env", ".git/config",
	}
)

// P1-06 修复: 使用预编译的正则表达式进行 SQL 注入检测
func containsSQLInjection(input string) bool {
	lowerInput := strings.ToLower(input)
	for _, pattern := range sqlInjectionPatterns {
		if pattern.MatchString(lowerInput) {
			return true
		}
	}
	return false
}

func containsXSS(input string) bool {
	lowerInput := strings.ToLower(input)
	for _, pattern := range xssPatterns {
		if strings.Contains(lowerInput, pattern) {
			return true
		}
	}
	return false
}

func containsPathTraversal(input string) bool {
	lowerInput := strings.ToLower(input)
	for _, pattern := range pathTraversalPatterns {
		if strings.Contains(lowerInput, pattern) {
			return true
		}
	}
	return false
}

func (sm *SecurityMiddleware) validateInput(input, inputType string) error {
	if containsSQLInjection(input) {
		return validator.ValidationError{
			Field:   inputType,
			Message: "输入包含可能的 SQL 注入特征",
		}
	}
	
	if containsXSS(input) {
		return validator.ValidationError{
			Field:   inputType,
			Message: "输入包含可能的 XSS 攻击特征",
		}
	}
	
	if containsPathTraversal(input) {
		return validator.ValidationError{
			Field:   inputType,
			Message: "输入包含可能的路径遍历特征",
		}
	}
	
	return nil
}

// P1-06 修复: 递归验证所有类型，包括数组、数字等，防止绕过
func (sm *SecurityMiddleware) validateMapRecursive(data map[string]interface{}, prefix string) validator.ValidationErrors {
	var errors validator.ValidationErrors
	
	for key, value := range data {
		fullKey := prefix + "." + key
		
		switch v := value.(type) {
		case string:
			if err := sm.validateInput(v, fullKey); err != nil {
				errors = append(errors, err.(validator.ValidationError))
			}
		case map[string]interface{}:
			errors = append(errors, sm.validateMapRecursive(v, fullKey)...)
		case []interface{}:
			// P1-06 修复: 遍历数组中的所有元素，不仅限于字符串
			for i, item := range v {
				itemKey := fmt.Sprintf("%s[%d]", fullKey, i)
				switch itemVal := item.(type) {
				case string:
					if err := sm.validateInput(itemVal, itemKey); err != nil {
						errors = append(errors, err.(validator.ValidationError))
					}
				case map[string]interface{}:
					// 递归验证嵌套对象
					errors = append(errors, sm.validateMapRecursive(itemVal, itemKey)...)
				case []interface{}:
					// 递归验证嵌套数组
					for j, nestedItem := range itemVal {
						if str, ok := nestedItem.(string); ok {
							if err := sm.validateInput(str, fmt.Sprintf("%s[%d]", itemKey, j)); err != nil {
								errors = append(errors, err.(validator.ValidationError))
							}
						}
					}
				}
			}
		case float64, bool:
			// P1-06 修复: 数字和布尔值也需要转换为字符串检查（虽然不太可能包含攻击代码）
			strVal := fmt.Sprintf("%v", v)
			if err := sm.validateInput(strVal, fullKey); err != nil {
				errors = append(errors, err.(validator.ValidationError))
			}
		}
	}
	
	return errors
}

func (sm *SecurityMiddleware) requestValidator() func(http.HandlerFunc) http.HandlerFunc {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			route := r.URL.Path
			
			if err := sm.validateInput(r.URL.RawQuery, "query"); err != nil {
				sm.logger("安全验证失败 - 路径: %s, 错误: %v", route, err)
				http.Error(w, "Invalid request parameters", http.StatusBadRequest)
				return
			}
			
			if err := sm.validateInput(r.URL.Path, "path"); err != nil {
				sm.logger("安全验证失败 - 路径: %s, 错误: %v", route, err)
				http.Error(w, "Invalid request path", http.StatusBadRequest)
				return
			}
			
			contentType := r.Header.Get("Content-Type")
			if contentType != "" && !sm.isAllowedContentType(contentType) {
				sm.logger("不支持的内容类型 - 路径: %s, Content-Type: %s", route, contentType)
				http.Error(w, "Unsupported Content-Type", http.StatusUnsupportedMediaType)
				return
			}
			
			if r.ContentLength > sm.config.MaxRequestBodySize {
				sm.logger("请求体过大 - 路径: %s, 大小: %d, 最大: %d", route, r.ContentLength, sm.config.MaxRequestBodySize)
				http.Error(w, "Request body too large", http.StatusRequestEntityTooLarge)
				return
			}
			
			if r.Method == "POST" || r.Method == "PUT" || r.Method == "PATCH" {
				if err := r.ParseForm(); err != nil {
					sm.logger("解析表单失败 - 路径: %s, 错误: %v", route, err)
					http.Error(w, "Failed to parse form", http.StatusBadRequest)
					return
				}
				
				for key, values := range r.Form {
					for _, value := range values {
						if err := sm.validateInput(value, "form."+key); err != nil {
							sm.logger("表单验证失败 - 路径: %s, 字段: %s, 错误: %v", route, key, err)
							http.Error(w, "Invalid form parameter: "+key, http.StatusBadRequest)
							return
						}
					}
				}
				
				if strings.Contains(contentType, "application/json") {
					var jsonData map[string]interface{}
					if err := json.NewDecoder(r.Body).Decode(&jsonData); err == nil {
						if errors := sm.validateMapRecursive(jsonData, "body"); errors.HasErrors() {
							sm.logger("JSON body 验证失败 - 路径: %s, 错误: %v", route, errors)
							http.Error(w, "Invalid JSON body: "+errors.Error(), http.StatusBadRequest)
							return
						}
					}
				}
			}
			
			next(w, r)
		}
	}
}

func (sm *SecurityMiddleware) isAllowedContentType(contentType string) bool {
	for _, allowed := range sm.config.AllowedContentTypes {
		if strings.Contains(contentType, allowed) {
			return true
		}
	}
	return false
}

func (sm *SecurityMiddleware) RequireRole(roles ...string) func(http.HandlerFunc) http.HandlerFunc {
	roleMap := make(map[string]bool)
	for _, role := range roles {
		roleMap[role] = true
	}
	
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			role, ok := r.Context().Value(roleContextKey).(string)
			if !ok || role == "" {
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}
			
			if !roleMap[role] {
				sm.logger("权限不足 - 用户角色: %s, 需要角色: %v, 路径: %s", role, roles, r.URL.Path)
				http.Error(w, "Forbidden", http.StatusForbidden)
				return
			}
			
			next(w, r)
		}
	}
}

func (sm *SecurityMiddleware) RequireOwnership(resourceOwner string) func(http.HandlerFunc) http.HandlerFunc {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			userID, ok := r.Context().Value(userContextKey).(string)
			role, _ := r.Context().Value(roleContextKey).(string)
			
			if !ok || userID == "" {
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}
			
			if role == "admin" {
				next(w, r)
				return
			}
			
			if userID != resourceOwner {
				sm.logger("越权访问尝试 - 用户: %s, 资源所有者: %s, 路径: %s", userID, resourceOwner, r.URL.Path)
				http.Error(w, "Forbidden", http.StatusForbidden)
				return
			}
			
			next(w, r)
		}
	}
}

// P1-06 修复: AuditLog 中间件记录响应状态码
func (sm *SecurityMiddleware) AuditLog() func(http.HandlerFunc) http.HandlerFunc {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			userID, _ := r.Context().Value(userContextKey).(string)
			role, _ := r.Context().Value(roleContextKey).(string)
			
			// P1-06 修复: 创建响应包装器以捕获状态码
			wrappedWriter := &responseWriterWrapper{ResponseWriter: w, statusCode: http.StatusOK}
			
			sm.logger("审计日志 - 用户: %s, 角色: %s, 方法: %s, 路径: %s, IP: %s, User-Agent: %s",
				userID, role, r.Method, r.URL.Path, r.RemoteAddr, r.UserAgent())
			
			next(wrappedWriter, r)
			
			// P1-06 修复: 记录响应状态码和耗时
			sm.logger("请求完成 - 路径: %s, 状态码: %d, 耗时: %v", 
				r.URL.Path, wrappedWriter.statusCode, time.Since(start))
		}
	}
}

// responseWriterWrapper 包装 http.ResponseWriter 以捕获状态码
type responseWriterWrapper struct {
	http.ResponseWriter
	statusCode int
	written    bool
}

func (w *responseWriterWrapper) WriteHeader(code int) {
	if !w.written {
		w.statusCode = code
		w.written = true
		w.ResponseWriter.WriteHeader(code)
	}
}

func (w *responseWriterWrapper) Write(b []byte) (int, error) {
	if !w.written {
		w.statusCode = http.StatusOK
		w.written = true
	}
	return w.ResponseWriter.Write(b)
}

type contextKey string

const (
	roleContextKey  contextKey = "role"
	userContextKey  contextKey = "user"
)

func GetUserIDFromContext(r *http.Request) string {
	userID, _ := r.Context().Value(userContextKey).(string)
	return userID
}

func GetRoleFromContext(r *http.Request) string {
	role, _ := r.Context().Value(roleContextKey).(string)
	return role
}

func IsAdmin(r *http.Request) bool {
	return GetRoleFromContext(r) == "admin"
}

func IsEditorOrAbove(r *http.Request) bool {
	role := GetRoleFromContext(r)
	return role == "admin" || role == "editor"
}

// P1-06 修复: 使用预编译的正则表达式，避免每次调用都重新编译
func ValidateRequestID(id string) error {
	if id == "" {
		return validator.ValidationError{Field: "id", Message: "ID 不能为空"}
	}
	if len(id) > 64 {
		return validator.ValidationError{Field: "id", Message: "ID 长度不能超过 64 个字符"}
	}
	
	// P1-06 修复: 使用包级别的预编译正则（在 NewSecurityMiddleware 中初始化）
	// 注意: 这是一个包级别函数，无法访问实例字段，所以使用 sync.Once 预编译
	if !getRequestIDRegex().MatchString(id) {
		return validator.ValidationError{Field: "id", Message: "ID 只能包含字母、数字、下划线和连字符"}
	}
	return nil
}

// P1-06 修复: 使用 sync.Once 确保正则只编译一次
var (
	requestIDRegexOnce sync.Once
	requestIDRegex     *regexp.Regexp
)

func getRequestIDRegex() *regexp.Regexp {
	requestIDRegexOnce.Do(func() {
		requestIDRegex = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)
	})
	return requestIDRegex
}

// P1-06 修复: 使用预编译的正则表达式
func ValidateProbeID(probeID string) error {
	if probeID != "" && len(probeID) > 64 {
		return validator.ValidationError{Field: "probe_id", Message: "Probe ID 长度不能超过 64 个字符"}
	}
	
	if probeID != "" {
		// P1-06 修复: 使用预编译正则
		if !getProbeIDRegex().MatchString(probeID) {
			return validator.ValidationError{Field: "probe_id", Message: "Probe ID 格式无效"}
		}
	}
	return nil
}

// P1-06 修复: 使用 sync.Once 确保正则只编译一次
var (
	probeIDRegexOnce sync.Once
	probeIDRegex     *regexp.Regexp
)

func getProbeIDRegex() *regexp.Regexp {
	probeIDRegexOnce.Do(func() {
		probeIDRegex = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)
	})
	return probeIDRegex
}

// P1-06 修复: 使用预编译的正则表达式
func ValidateDate(date string) error {
	if date == "" {
		return nil
	}
	// P1-06 修复: 使用预编译正则
	if !getDateRegex().MatchString(date) {
		return validator.ValidationError{Field: "date", Message: "日期格式必须是 YYYY-MM-DD"}
	}
	return nil
}

// P1-06 修复: 使用 sync.Once 确保正则只编译一次
var (
	dateRegexOnce sync.Once
	dateRegex     *regexp.Regexp
)

func getDateRegex() *regexp.Regexp {
	dateRegexOnce.Do(func() {
		dateRegex = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)
	})
	return dateRegex
}

func ValidateLimit(limit int) error {
	if limit <= 0 {
		return validator.ValidationError{Field: "limit", Message: "Limit 必须大于 0"}
	}
	if limit > 100000 {
		return validator.ValidationError{Field: "limit", Message: "Limit 不能超过 100000"}
	}
	return nil
}

func ValidatePaginationParams(page, pageSize int) error {
	if page <= 0 {
		return validator.ValidationError{Field: "page", Message: "页码必须大于 0"}
	}
	if pageSize <= 0 {
		return validator.ValidationError{Field: "page_size", Message: "每页数量必须大于 0"}
	}
	if pageSize > 1000 {
		return validator.ValidationError{Field: "page_size", Message: "每页数量不能超过 1000"}
	}
	return nil
}
