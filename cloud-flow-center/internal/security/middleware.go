package security

import (
	"encoding/json"
	"net/http"
	"regexp"
	"strings"
	"time"

	"cloud-flow-center/internal/validator"
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
}

func NewSecurityMiddleware(config SecurityConfig, logger func(format string, args ...interface{})) *SecurityMiddleware {
	return &SecurityMiddleware{
		config: config,
		logger: logger,
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

var (
	sqlInjectionPatterns = []string{
		"'", "\"", ";", "--", "/*", "*/", "@@", "@",
		"char(", "nchar(", "varchar(", "nvarchar(",
		"alter ", "begin ", "cast(", "create ", "cursor ",
		"declare ", "delete ", "drop ", "end ", "exec(",
		"execute(", "fetch(", "insert(", "kill(",
		"select ", "sys(", "sysobjects", "syscolumns",
		"table(", "update(", "xp_", "0x",
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

func containsSQLInjection(input string) bool {
	lowerInput := strings.ToLower(input)
	for _, pattern := range sqlInjectionPatterns {
		if strings.Contains(lowerInput, pattern) {
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
			for i, item := range v {
				if str, ok := item.(string); ok {
					if err := sm.validateInput(str, fullKey); err != nil {
						errors = append(errors, err.(validator.ValidationError))
					}
				}
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

func (sm *SecurityMiddleware) AuditLog() func(http.HandlerFunc) http.HandlerFunc {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			userID, _ := r.Context().Value(userContextKey).(string)
			role, _ := r.Context().Value(roleContextKey).(string)
			
			sm.logger("审计日志 - 用户: %s, 角色: %s, 方法: %s, 路径: %s, IP: %s, User-Agent: %s",
				userID, role, r.Method, r.URL.Path, r.RemoteAddr, r.UserAgent())
			
			next(w, r)
			
			sm.logger("请求完成 - 路径: %s, 耗时: %v", r.URL.Path, time.Since(start))
		}
	}
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

func ValidateRequestID(id string) error {
	if id == "" {
		return validator.ValidationError{Field: "id", Message: "ID 不能为空"}
	}
	if len(id) > 64 {
		return validator.ValidationError{Field: "id", Message: "ID 长度不能超过 64 个字符"}
	}
	
	idRegex := regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)
	if !idRegex.MatchString(id) {
		return validator.ValidationError{Field: "id", Message: "ID 只能包含字母、数字、下划线和连字符"}
	}
	return nil
}

func ValidateProbeID(probeID string) error {
	if probeID != "" && len(probeID) > 64 {
		return validator.ValidationError{Field: "probe_id", Message: "Probe ID 长度不能超过 64 个字符"}
	}
	
	if probeID != "" {
		probeIDRegex := regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)
		if !probeIDRegex.MatchString(probeID) {
			return validator.ValidationError{Field: "probe_id", Message: "Probe ID 格式无效"}
		}
	}
	return nil
}

func ValidateDate(date string) error {
	if date == "" {
		return nil
	}
	dateRegex := regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)
	if !dateRegex.MatchString(date) {
		return validator.ValidationError{Field: "date", Message: "日期格式必须是 YYYY-MM-DD"}
	}
	return nil
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
