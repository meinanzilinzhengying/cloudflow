//go:build linux

package auth

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/meinanzilinzhengying/cloudflow/agent/internal/tenant"
)

// ============================================================================
// 一、SSO 配置
// ============================================================================

// SSOProviderType SSO 提供者类型
type SSOProviderType string

const (
	SSOTypeOAuth2 SSOProviderType = "oauth2" // OAuth2 / OpenID Connect
	SSOTypeSAML   SSOProviderType = "saml"   // SAML 2.0
	SSOTypeOIDC   SSOProviderType = "oidc"   // OpenID Connect (专用)
)

// OAuth2Config OAuth2 配置
type OAuth2Config struct {
	ProviderName     string   `json:"provider_name"`     // 提供者名称 (google, github, azure, okta)
	ClientID         string   `json:"client_id"`
	ClientSecret     string   `json:"client_secret"`
	AuthURL          string   `json:"auth_url"`          // 授权端点
	TokenURL         string   `json:"token_url"`          // Token 端点
	UserInfoURL      string   `json:"user_info_url"`      // 用户信息端点
	RedirectURL      string   `json:"redirect_url"`       // 回调地址
	Scopes           []string `json:"scopes"`             // 请求范围
	LogoutURL        string   `json:"logout_url,omitempty"` // 登出端点
	AllowedDomains   []string `json:"allowed_domains,omitempty"` // 允许的域名
}

// Validate 验证 OAuth2 配置
func (c *OAuth2Config) Validate() error {
	if c.ClientID == "" {
		return fmt.Errorf("client_id is required")
	}
	if c.ClientSecret == "" {
		return fmt.Errorf("client_secret is required")
	}
	if c.AuthURL == "" {
		return fmt.Errorf("auth_url is required")
	}
	if c.TokenURL == "" {
		return fmt.Errorf("token_url is required")
	}
	if c.RedirectURL == "" {
		return fmt.Errorf("redirect_url is required")
	}
	return nil
}

// DefaultOAuth2Scopes 默认 OAuth2 范围
var DefaultOAuth2Scopes = []string{"openid", "profile", "email"}

// ============================================================================
// 二、OIDC 配置
// ============================================================================

// OIDCConfig OpenID Connect 配置
type OIDCConfig struct {
	Issuer       string   `json:"issuer"`        // 发行者 URL
	ClientID     string   `json:"client_id"`
	ClientSecret string   `json:"client_secret"`
	RedirectURL  string   `json:"redirect_url"`
	Scopes       []string `json:"scopes"`

	// 自动发现
	DiscoveryURL string `json:"discovery_url,omitempty"` // .well-known/openid-configuration

	// 验证配置
	VerifyIssuer   bool `json:"verify_issuer"`   // 验证发行者
	VerifyAudience bool `json:"verify_audience"` // 验证受众
}

// OIDCDiscovery OIDC 发现文档
type OIDCDiscovery struct {
	Issuer                string   `json:"issuer"`
	AuthorizationEndpoint string   `json:"authorization_endpoint"`
	TokenEndpoint         string   `json:"token_endpoint"`
	UserInfoEndpoint      string   `json:"userinfo_endpoint"`
	EndSessionEndpoint    string   `json:"end_session_endpoint,omitempty"`
	JWKSURI               string   `json:"jwks_uri"`
	ScopesSupported       []string `json:"scopes_supported"`
}

// ============================================================================
// 三、SSO 用户和 Token
// ============================================================================

// SSOUser SSO 用户信息
type SSOUser struct {
	ID            string `json:"id"`
	Username      string `json:"username,omitempty"`
	Email         string `json:"email"`
	Name          string `json:"name,omitempty"`
	Avatar        string `json:"avatar,omitempty"`
	Provider      string `json:"provider"`
	ProviderID    string `json:"provider_id"`
	EmailVerified bool   `json:"email_verified"`
	Groups        []string `json:"groups,omitempty"`
	Attributes    map[string]interface{} `json:"attributes,omitempty"`
}

// OAuth2Token OAuth2 Token 响应
type OAuth2Token struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	RefreshToken string `json:"refresh_token,omitempty"`
	ExpiresIn    int    `json:"expires_in"`
	IDToken      string `json:"id_token,omitempty"` // OIDC ID Token
}

// Expiry 计算过期时间
func (t *OAuth2Token) Expiry() time.Time {
	return time.Now().Add(time.Duration(t.ExpiresIn) * time.Second)
}

// IsExpired 检查是否过期
func (t *OAuth2Token) IsExpired() bool {
	return time.Now().After(t.Expiry())
}

// ============================================================================
// 四、SSO 认证流程
// ============================================================================

// SSOAuthFlow SSO 认证流程
type SSOAuthFlow struct {
	State       string    `json:"state"`
	Provider    string    `json:"provider"`
	RedirectURL string    `json:"redirect_url"`
	CreatedAt   time.Time `json:"created_at"`
	Used        bool      `json:"used"`
}

// SSOAuthManager SSO 认证管理器
type SSOAuthManager struct {
	mu sync.RWMutex

	// 配置
	oauth2Configs map[string]*OAuth2Config
	oidcConfigs   map[string]*OIDCConfig

	// 认证流程（state 管理）
	flows map[string]*SSOAuthFlow

	// 用户存储
	userStore UserStore

	// HTTP 客户端
	httpClient *http.Client
}

// NewSSOAuthManager 创建 SSO 认证管理器
func NewSSOAuthManager(userStore UserStore) *SSOAuthManager {
	return &SSOAuthManager{
		oauth2Configs: make(map[string]*OAuth2Config),
		oidcConfigs:   make(map[string]*OIDCConfig),
		flows:          make(map[string]*SSOAuthFlow),
		userStore:      userStore,
		httpClient:     &http.Client{Timeout: 30 * time.Second},
	}
}

// RegisterOAuth2Provider 注册 OAuth2 提供者
func (m *SSOAuthManager) RegisterOAuth2Provider(name string, config *OAuth2Config) error {
	if err := config.Validate(); err != nil {
		return err
	}
	if len(config.Scopes) == 0 {
		config.Scopes = DefaultOAuth2Scopes
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.oauth2Configs[name] = config
	return nil
}

// RegisterOIDCProvider 注册 OIDC 提供者
func (m *SSOAuthManager) RegisterOIDCProvider(name string, config *OIDCConfig) error {
	if config.Issuer == "" && config.DiscoveryURL == "" {
		return fmt.Errorf("issuer or discovery_url is required")
	}
	if config.ClientID == "" {
		return fmt.Errorf("client_id is required")
	}
	if config.RedirectURL == "" {
		return fmt.Errorf("redirect_url is required")
	}
	if len(config.Scopes) == 0 {
		config.Scopes = DefaultOAuth2Scopes
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.oidcConfigs[name] = config
	return nil
}

// GetAuthURL 获取授权 URL
func (m *SSOAuthManager) GetAuthURL(providerName string) (string, string, error) {
	m.mu.RLock()
	config, ok := m.oauth2Configs[providerName]
	m.mu.RUnlock()

	if !ok {
		return "", "", fmt.Errorf("provider not found: %s", providerName)
	}

	// 生成 state
	state := generateState()

	// 保存认证流程
	flow := &SSOAuthFlow{
		State:       state,
		Provider:    providerName,
		RedirectURL: config.RedirectURL,
		CreatedAt:   time.Now(),
		Used:        false,
	}
	m.mu.Lock()
	m.flows[state] = flow
	m.mu.Unlock()

	// 构建授权 URL
	authURL, err := url.Parse(config.AuthURL)
	if err != nil {
		return "", "", err
	}

	q := authURL.Query()
	q.Set("client_id", config.ClientID)
	q.Set("redirect_uri", config.RedirectURL)
	q.Set("response_type", "code")
	q.Set("scope", strings.Join(config.Scopes, " "))
	q.Set("state", state)
	authURL.RawQuery = q.Encode()

	return authURL.String(), state, nil
}

// HandleCallback 处理 OAuth2 回调
func (m *SSOAuthManager) HandleCallback(providerName, code, state string) (*SSOAuthResult, error) {
	// 验证 state
	m.mu.Lock()
	flow, ok := m.flows[state]
	if !ok || flow.Used {
		m.mu.Unlock()
		return nil, fmt.Errorf("invalid or expired state")
	}
	flow.Used = true
	m.mu.Unlock()

	// 检查配置
	m.mu.RLock()
	config, ok := m.oauth2Configs[providerName]
	m.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("provider not found: %s", providerName)
	}

	// 1. 交换 code 获取 token
	token, err := m.exchangeCode(config, code)
	if err != nil {
		return nil, fmt.Errorf("token exchange failed: %v", err)
	}

	// 2. 获取用户信息
	ssoUser, err := m.getUserInfo(config, token)
	if err != nil {
		return nil, fmt.Errorf("get user info failed: %v", err)
	}

	// 3. 同步用户到本地
	localUser, isNew, err := m.syncUser(ssoUser)
	if err != nil {
		return nil, fmt.Errorf("sync user failed: %v", err)
	}

	return &SSOAuthResult{
		Success:   true,
		User:      ssoUser,
		LocalUser: localUser,
		Token:     token,
		IsNewUser: isNew,
	}, nil
}

// exchangeCode 交换 code 获取 token
func (m *SSOAuthManager) exchangeCode(config *OAuth2Config, code string) (*OAuth2Token, error) {
	data := url.Values{}
	data.Set("grant_type", "authorization_code")
	data.Set("code", code)
	data.Set("redirect_uri", config.RedirectURL)
	data.Set("client_id", config.ClientID)
	data.Set("client_secret", config.ClientSecret)

	resp, err := m.httpClient.PostForm(config.TokenURL, data)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("token endpoint returned %d: %s", resp.StatusCode, string(body))
	}

	var token OAuth2Token
	if err := json.NewDecoder(resp.Body).Decode(&token); err != nil {
		return nil, err
	}

	return &token, nil
}

// getUserInfo 获取用户信息
func (m *SSOAuthManager) getUserInfo(config *OAuth2Config, token *OAuth2Token) (*SSOUser, error) {
	if config.UserInfoURL == "" {
		return nil, fmt.Errorf("user_info_url not configured")
	}

	req, err := http.NewRequest("GET", config.UserInfoURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token.AccessToken)

	resp, err := m.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("userinfo endpoint returned %d: %s", resp.StatusCode, string(body))
	}

	var userInfo map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&userInfo); err != nil {
		return nil, err
	}

	// 解析用户信息（标准 OIDC 字段）
	ssoUser := &SSOUser{
		Provider:   config.ProviderName,
		Attributes: userInfo,
	}

	if id, ok := userInfo["sub"].(string); ok {
		ssoUser.ProviderID = id
		ssoUser.ID = id
	}
	if email, ok := userInfo["email"].(string); ok {
		ssoUser.Email = email
	}
	if name, ok := userInfo["name"].(string); ok {
		ssoUser.Name = name
	} else if name, ok := userInfo["display_name"].(string); ok {
		ssoUser.Name = name
	}
	if username, ok := userInfo["preferred_username"].(string); ok {
		ssoUser.Username = username
	}
	if verified, ok := userInfo["email_verified"].(bool); ok {
		ssoUser.EmailVerified = verified
	}
	if picture, ok := userInfo["picture"].(string); ok {
		ssoUser.Avatar = picture
	}
	if groups, ok := userInfo["groups"].([]interface{}); ok {
		for _, g := range groups {
			if gs, ok := g.(string); ok {
				ssoUser.Groups = append(ssoUser.Groups, gs)
			}
		}
	}

	return ssoUser, nil
}

// syncUser 同步 SSO 用户到本地
func (m *SSOAuthManager) syncUser(ssoUser *SSOUser) (*tenant.User, bool, error) {
	if m.userStore == nil {
		return nil, false, fmt.Errorf("user store not configured")
	}

	username := ssoUser.Username
	if username == "" {
		username = ssoUser.Email
	}

	user, err := m.userStore.GetUserByUsername(username)
	if err != nil {
		return nil, false, err
	}

	isNew := false
	if user == nil {
		// 创建新用户
		isNew = true
		user = &tenant.User{
			ID:       fmt.Sprintf("sso-%s-%s", ssoUser.Provider, ssoUser.ProviderID),
			Username: username,
			Email:    ssoUser.Email,
			Status:   tenant.UserStatusActive,
		}
		if err := m.userStore.CreateUser(user); err != nil {
			return nil, false, err
		}
	} else {
		// 更新用户信息
		user.Email = ssoUser.Email
		if err := m.userStore.UpdateUser(user); err != nil {
			return nil, false, err
		}
	}

	return user, isNew, nil
}

// ============================================================================
// 五、SSO 认证结果
// ============================================================================

// SSOAuthResult SSO 认证结果
type SSOAuthResult struct {
	Success   bool         `json:"success"`
	User      *SSOUser     `json:"user,omitempty"`
	LocalUser *tenant.User `json:"local_user,omitempty"`
	Token     *OAuth2Token `json:"token,omitempty"`
	Error     string       `json:"error,omitempty"`
	IsNewUser bool         `json:"is_new_user"`
}

// ============================================================================
// 六、登出
// ============================================================================

// LogoutResult 登出结果
type LogoutResult struct {
	Success       bool   `json:"success"`
	LocalLogout   bool   `json:"local_logout"`   // 本地登出成功
	RemoteLogout  bool   `json:"remote_logout"`  // 远程登出成功
	RedirectURL   string `json:"redirect_url,omitempty"` // 远程登出重定向 URL
}

// Logout 执行 SSO 登出
func (m *SSOAuthManager) Logout(providerName string, localUser *tenant.User) *LogoutResult {
	result := &LogoutResult{
		Success:     true,
		LocalLogout: true,
	}

	m.mu.RLock()
	config, ok := m.oauth2Configs[providerName]
	m.mu.RUnlock()

	if ok && config.LogoutURL != "" {
		logoutURL, _ := url.Parse(config.LogoutURL)
		q := logoutURL.Query()
		q.Set("redirect_uri", config.RedirectURL)
		logoutURL.RawQuery = q.Encode()
		result.RemoteLogout = true
		result.RedirectURL = logoutURL.String()
	}

	return result
}

// ============================================================================
// 七、辅助函数
// ============================================================================

// generateState 生成随机 state
func generateState() string {
	b := make([]byte, 32)
	rand.Read(b)
	return base64.URLEncoding.EncodeToString(b)
}

// GetProviders 获取所有注册的提供者
func (m *SSOAuthManager) GetProviders() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var providers []string
	seen := make(map[string]bool)
	for name := range m.oauth2Configs {
		providers = append(providers, name)
		seen[name] = true
	}
	for name := range m.oidcConfigs {
		if !seen[name] {
			providers = append(providers, name)
		}
	}
	return providers
}

// CleanupFlows 清理过期的认证流程
func (m *SSOAuthManager) CleanupFlows() {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	for id, flow := range m.flows {
		if flow.Used || now.Sub(flow.CreatedAt) > 10*time.Minute {
			delete(m.flows, id)
		}
	}
}

// GetStats 获取统计信息
func (m *SSOAuthManager) GetStats() map[string]interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return map[string]interface{}{
		"oauth2_providers": len(m.oauth2Configs),
		"oidc_providers":   len(m.oidcConfigs),
		"active_flows":     len(m.flows),
	}
}

// ============================================================================
// 八、SAML 支持（结构定义）
// ============================================================================

// SAMLConfig SAML 配置
type SAMLConfig struct {
	ProviderName           string `json:"provider_name"`
	IDPMetadataURL         string `json:"idp_metadata_url"`         // IDP Metadata URL
	EntityID               string `json:"entity_id"`                // SP Entity ID
	AssertionConsumerService string `json:"assertion_consumer_service"` // ACS URL
	Certificate            string `json:"certificate"`                // SP 证书
	PrivateKey             string `json:"private_key"`                // SP 私钥
}

// SAMLAssertion SAML 断言
type SAMLAssertion struct {
	NameID       string            `json:"name_id"`
	Email        string            `json:"email"`
	Attributes   map[string][]string `json:"attributes"`
	SessionIndex string            `json:"session_index,omitempty"`
}

// SAMLAuthService SAML 认证服务（结构定义，完整实现需引入 SAML 库）
type SAMLAuthService struct {
	config    *SAMLConfig
	userStore UserStore
}

// NewSAMLAuthService 创建 SAML 认证服务
func NewSAMLAuthService(config *SAMLConfig, userStore UserStore) *SAMLAuthService {
	return &SAMLAuthService{
		config:    config,
		userStore: userStore,
	}
}

// CreateAuthRequest 创建 SAML 认证请求
func (s *SAMLAuthService) CreateAuthRequest() (string, string, error) {
	// 实际实现需要使用 SAML 库生成 AuthnRequest
	requestID := generateState()
	return "", requestID, fmt.Errorf("SAML not fully implemented")
}

// ParseResponse 解析 SAML 响应
func (s *SAMLAuthService) ParseResponse(encodedResponse string) (*SAMLAssertion, error) {
	return nil, fmt.Errorf("SAML not fully implemented")
}

// ============================================================================
// 九、组映射（SSO 组到本地角色）
// ============================================================================

// SSOGroupMapping SSO 组到角色映射
type SSOGroupMapping struct {
	SSOGroup  string `json:"sso_group"`
	RoleID    string `json:"role_id"`
	Provider  string `json:"provider,omitempty"`
}

// SSOGroupMapper SSO 组映射器
type SSOGroupMapper struct {
	mu       sync.RWMutex
	mappings []SSOGroupMapping
}

// NewSSOGroupMapper 创建 SSO 组映射器
func NewSSOGroupMapper() *SSOGroupMapper {
	return &SSOGroupMapper{
		mappings: make([]SSOGroupMapping, 0),
	}
}

// AddMapping 添加映射
func (m *SSOGroupMapper) AddMapping(ssoGroup, roleID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.mappings = append(m.mappings, SSOGroupMapping{
		SSOGroup: ssoGroup,
		RoleID:   roleID,
	})
}

// MapGroupsToRoles 映射 SSO 组到角色
func (m *SSOGroupMapper) MapGroupsToRoles(groups []string) []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var roles []string
	seen := make(map[string]bool)
	for _, group := range groups {
		for _, mapping := range m.mappings {
			if mapping.SSOGroup == group {
				if !seen[mapping.RoleID] {
					roles = append(roles, mapping.RoleID)
					seen[mapping.RoleID] = true
				}
			}
		}
	}
	return roles
}
