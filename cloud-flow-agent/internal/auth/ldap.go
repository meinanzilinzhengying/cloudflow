//go:build linux

package auth

import (
	"crypto/tls"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/meinanzilinzhengying/cloudflow/agent/internal/tenant"
)

// ============================================================================
// 一、LDAP 配置
// ============================================================================

// LDAPConfig LDAP 配置
type LDAPConfig struct {
	URL                string        `json:"url"`                 // LDAP 服务器地址 ldap://host:port 或 ldaps://host:port
	BindDN             string        `json:"bind_dn"`             // 绑定 DN
	BindPassword       string        `json:"bind_password"`       // 绑定密码
	BaseDN             string        `json:"base_dn"`             // 搜索基础 DN
	UserSearchFilter   string        `json:"user_search_filter"`  // 用户搜索过滤器 (uid=%s)
	GroupSearchFilter  string        `json:"group_search_filter"` // 组搜索过滤器
	UserAttrMap        UserAttrMap   `json:"user_attr_map"`       // 属性映射
	TLSConfig          *tls.Config   `json:"-"`                   // TLS 配置
	Timeout            time.Duration `json:"timeout"`             // 超时
	SkipTLSVerify      bool          `json:"skip_tls_verify"`     // 跳过 TLS 验证（仅开发环境）
}

// UserAttrMap LDAP 属性到用户属性的映射
type UserAttrMap struct {
	Username string `json:"username"` // 默认: uid
	Email    string `json:"email"`    // 默认: mail
	Phone    string `json:"phone"`    // 默认: telephoneNumber
	RealName string `json:"real_name"` // 默认: cn
	Department string `json:"department"` // 默认: ou
}

// DefaultLDAPConfig 默认 LDAP 配置
func DefaultLDAPConfig() *LDAPConfig {
	return &LDAPConfig{
		UserSearchFilter:  "(uid=%s)",
		GroupSearchFilter: "(member=%s)",
		UserAttrMap: UserAttrMap{
			Username: "uid",
			Email:    "mail",
			Phone:    "telephoneNumber",
			RealName: "cn",
			Department: "ou",
		},
		Timeout:       10 * time.Second,
		SkipTLSVerify: false,
	}
}

// Validate 验证配置
func (c *LDAPConfig) Validate() error {
	if c.URL == "" {
		return fmt.Errorf("LDAP URL is required")
	}
	if c.BindDN == "" {
		return fmt.Errorf("LDAP BindDN is required")
	}
	if c.BaseDN == "" {
		return fmt.Errorf("LDAP BaseDN is required")
	}
	u, err := url.Parse(c.URL)
	if err != nil {
		return fmt.Errorf("invalid LDAP URL: %v", err)
	}
	if u.Scheme != "ldap" && u.Scheme != "ldaps" {
		return fmt.Errorf("LDAP URL scheme must be ldap or ldaps")
	}
	return nil
}

// ============================================================================
// 二、LDAP 用户和组
// ============================================================================

// LDAPUser LDAP 用户
type LDAPUser struct {
	DN         string            `json:"dn"`
	Username   string            `json:"username"`
	Email      string            `json:"email"`
	Phone      string            `json:"phone,omitempty"`
	RealName   string            `json:"real_name,omitempty"`
	Department string            `json:"department,omitempty"`
	Attributes map[string][]string `json:"attributes,omitempty"`
}

// LDAPGroup LDAP 组
type LDAPGroup struct {
	DN          string   `json:"dn"`
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	Members     []string `json:"members,omitempty"`
}

// ============================================================================
// 三、LDAP Provider 接口
// ============================================================================

// LDAPProvider LDAP 认证提供者接口
type LDAPProvider interface {
	// Authenticate 使用用户名和密码认证
	Authenticate(username, password string) (*LDAPUser, error)

	// GetUser 获取用户信息
	GetUser(username string) (*LDAPUser, error)

	// GetUserGroups 获取用户所属组
	GetUserGroups(username string) ([]*LDAPGroup, error)

	// SearchUsers 搜索用户
	SearchUsers(filter string) ([]*LDAPUser, error)

	// Close 关闭连接
	Close() error
}

// ============================================================================
// 四、LDAP 认证结果
// ============================================================================

// LDAPAuthResult LDAP 认证结果
type LDAPAuthResult struct {
	Success      bool        `json:"success"`
	User         *LDAPUser   `json:"user,omitempty"`
	Groups       []*LDAPGroup `json:"groups,omitempty"`
	ErrorMessage string      `json:"error_message,omitempty"`
	IsNewUser    bool        `json:"is_new_user"` // 是否首次登录
}

// ============================================================================
// 五、LDAP 组到角色映射
// ============================================================================

// LDAPGroupMapping LDAP 组到角色映射
type LDAPGroupMapping struct {
	LDAPGroupDN string `json:"ldap_group_dn"` // LDAP 组 DN 或名称
	RoleID      string `json:"role_id"`       // 本地角色 ID
}

// LDAPGroupMapper LDAP 组映射器
type LDAPGroupMapper struct {
	mappings []LDAPGroupMapping
}

// NewLDAPGroupMapper 创建组映射器
func NewLDAPGroupMapper() *LDAPGroupMapper {
	return &LDAPGroupMapper{
		mappings: make([]LDAPGroupMapping, 0),
	}
}

// AddMapping 添加映射
func (m *LDAPGroupMapper) AddMapping(ldapGroupDN, roleID string) {
	m.mappings = append(m.mappings, LDAPGroupMapping{
		LDAPGroupDN: ldapGroupDN,
		RoleID:      roleID,
	})
}

// MapGroupsToRoles 将 LDAP 组映射到角色
func (m *LDAPGroupMapper) MapGroupsToRoles(groups []*LDAPGroup) []string {
	var roles []string
	seen := make(map[string]bool)

	for _, group := range groups {
		for _, mapping := range m.mappings {
			if mapping.LDAPGroupDN == group.DN || mapping.LDAPGroupDN == group.Name {
				if !seen[mapping.RoleID] {
					roles = append(roles, mapping.RoleID)
					seen[mapping.RoleID] = true
				}
			}
		}
	}

	return roles
}

// ============================================================================
// 六、LDAP 认证服务
// ============================================================================

// LDAPAuthService LDAP 认证服务
type LDAPAuthService struct {
	config    *LDAPConfig
	mapper    *LDAPGroupMapper
	userStore UserStore // 用户存储接口
}

// UserStore 用户存储接口
type UserStore interface {
	GetUserByUsername(username string) (*tenant.User, error)
	CreateUser(user *tenant.User) error
	UpdateUser(user *tenant.User) error
}

// NewLDAPAuthService 创建 LDAP 认证服务
func NewLDAPAuthService(config *LDAPConfig, userStore UserStore) *LDAPAuthService {
	return &LDAPAuthService{
		config:    config,
		mapper:    NewLDAPGroupMapper(),
		userStore: userStore,
	}
}

// SetGroupMapper 设置组映射器
func (s *LDAPAuthService) SetGroupMapper(mapper *LDAPGroupMapper) {
	s.mapper = mapper
}

// AddGroupMapping 添加组映射
func (s *LDAPAuthService) AddGroupMapping(ldapGroup, roleID string) {
	if s.mapper == nil {
		s.mapper = NewLDAPGroupMapper()
	}
	s.mapper.AddMapping(ldapGroup, roleID)
}

// DefaultMappings 设置默认的 LDAP 组映射
func (s *LDAPAuthService) DefaultMappings() {
	if s.mapper == nil {
		s.mapper = NewLDAPGroupMapper()
	}
	// 常见的 LDAP 组映射
	s.mapper.AddMapping("cn=admins,ou=groups,dc=example,dc=com", "role-admin")
	s.mapper.AddMapping("cn=operators,ou=groups,dc=example,dc=com", "role-operator")
	s.mapper.AddMapping("cn=auditors,ou=groups,dc=example,dc=com", "role-auditor")
	s.mapper.AddMapping("cn=users,ou=groups,dc=example,dc=com", "role-viewer")
}

// Authenticate LDAP 认证（模拟实现，无真实 LDAP 服务器）
func (s *LDAPAuthService) Authenticate(username, password string) (*LDAPAuthResult, error) {
	if s.config == nil {
		return nil, fmt.Errorf("LDAP config not set")
	}
	if err := s.config.Validate(); err != nil {
		return nil, err
	}

	// 模拟 LDAP 认证结果
	// 实际实现需要连接 LDAP 服务器
	user := &LDAPUser{
		DN:       fmt.Sprintf("uid=%s,%s", username, s.config.BaseDN),
		Username: username,
		Email:    fmt.Sprintf("%s@example.com", username),
		RealName: username,
	}

	result := &LDAPAuthResult{
		Success: true,
		User:    user,
		Groups: []*LDAPGroup{
			{DN: "cn=users,ou=groups,dc=example,dc=com", Name: "users"},
		},
		IsNewUser: true,
	}

	return result, nil
}

// SyncUser 同步 LDAP 用户到本地
func (s *LDAPAuthService) SyncUser(ldapUser *LDAPUser) (*tenant.User, error) {
	if s.userStore == nil {
		return nil, fmt.Errorf("user store not set")
	}

	// 查找现有用户
	user, err := s.userStore.GetUserByUsername(ldapUser.Username)
	if err != nil {
		return nil, err
	}

	if user == nil {
		// 创建新用户
		user = &tenant.User{
			ID:       fmt.Sprintf("ldap-%s", ldapUser.Username),
			Username: ldapUser.Username,
			Email:    ldapUser.Email,
			Phone:    ldapUser.Phone,
			Status:   tenant.UserStatusActive,
		}
		if err := s.userStore.CreateUser(user); err != nil {
			return nil, err
		}
	} else {
		// 更新用户信息
		user.Email = ldapUser.Email
		user.Phone = ldapUser.Phone
		if err := s.userStore.UpdateUser(user); err != nil {
			return nil, err
		}
	}

	return user, nil
}

// SyncUserGroups 同步用户组并映射角色
func (s *LDAPAuthService) SyncUserGroups(ldapUser *LDAPUser, groups []*LDAPGroup) []string {
	if s.mapper == nil {
		return nil
	}
	return s.mapper.MapGroupsToRoles(groups)
}

// ============================================================================
// 七、LDAP 连接工厂（模拟）
// ============================================================================

// LDAPConnection LDAP 连接接口
type LDAPConnection interface {
	Bind(username, password string) error
	Search(baseDN, filter string, attrs []string) ([]map[string][]string, error)
	Close() error
}

// MockLDAPProvider 模拟 LDAP Provider（用于测试）
type MockLDAPProvider struct {
	users  map[string]*LDAPUser
	groups map[string]*LDAPGroup
}

// NewMockLDAPProvider 创建模拟 LDAP Provider
func NewMockLDAPProvider() *MockLDAPProvider {
	return &MockLDAPProvider{
		users:  make(map[string]*LDAPUser),
		groups: make(map[string]*LDAPGroup),
	}
}

// AddMockUser 添加模拟用户
func (m *MockLDAPProvider) AddMockUser(user *LDAPUser) {
	m.users[user.Username] = user
}

// AddMockGroup 添加模拟组
func (m *MockLDAPProvider) AddMockGroup(group *LDAPGroup) {
	m.groups[group.DN] = group
}

// Authenticate 模拟认证
func (m *MockLDAPProvider) Authenticate(username, password string) (*LDAPUser, error) {
	user, ok := m.users[username]
	if !ok {
		return nil, fmt.Errorf("user not found: %s", username)
	}
	// 简化：密码非空即可
	if password == "" {
		return nil, fmt.Errorf("invalid password")
	}
	return user, nil
}

// GetUser 模拟获取用户
func (m *MockLDAPProvider) GetUser(username string) (*LDAPUser, error) {
	user, ok := m.users[username]
	if !ok {
		return nil, fmt.Errorf("user not found: %s", username)
	}
	return user, nil
}

// GetUserGroups 模拟获取用户组
func (m *MockLDAPProvider) GetUserGroups(username string) ([]*LDAPGroup, error) {
	var result []*LDAPGroup
	for _, group := range m.groups {
		for _, member := range group.Members {
			if member == username {
				result = append(result, group)
				break
			}
		}
	}
	return result, nil
}

// SearchUsers 模拟搜索用户
func (m *MockLDAPProvider) SearchUsers(filter string) ([]*LDAPUser, error) {
	var result []*LDAPUser
	for _, user := range m.users {
		if strings.Contains(user.Username, filter) ||
			strings.Contains(user.Email, filter) ||
			strings.Contains(user.RealName, filter) {
			result = append(result, user)
		}
	}
	return result, nil
}

// Close 关闭连接
func (m *MockLDAPProvider) Close() error {
	return nil
}

// Ensure MockLDAPProvider implements LDAPProvider
var _ LDAPProvider = (*MockLDAPProvider)(nil)

// ============================================================================
// 八、需要导入 tenant 包
// ============================================================================
// 注意: 文件头部已经通过 auth 包导入 tenant 包
// 这里需要使用 tenant.User 类型，确保在 manager.go 中正确使用
