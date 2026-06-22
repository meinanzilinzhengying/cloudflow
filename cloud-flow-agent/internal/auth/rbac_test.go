//go:build linux

package auth_test

import (
	"testing"
	"time"

	"github.com/meinanzilinzhengying/cloudflow/agent/internal/auth"
	"github.com/meinanzilinzhengying/cloudflow/agent/internal/tenant"
)

// ============================================================================
// 一、LDAP 配置测试
// ============================================================================

func TestLDAPConfig(t *testing.T) {
	t.Run("Default config", func(t *testing.T) {
		cfg := auth.DefaultLDAPConfig()
		if cfg.UserSearchFilter == "" {
			t.Error("expected default user search filter")
		}
		if cfg.UserAttrMap.Username == "" {
			t.Error("expected default username attribute")
		}
	})

	t.Run("Validate empty URL", func(t *testing.T) {
		cfg := &auth.LDAPConfig{}
		if err := cfg.Validate(); err == nil {
			t.Error("expected error for empty URL")
		}
	})

	t.Run("Validate invalid scheme", func(t *testing.T) {
		cfg := &auth.LDAPConfig{
			URL:      "http://localhost:389",
			BindDN:   "cn=admin,dc=test",
			BaseDN:   "dc=test",
		}
		if err := cfg.Validate(); err == nil {
			t.Error("expected error for invalid scheme")
		}
	})

	t.Run("Validate valid config", func(t *testing.T) {
		cfg := &auth.LDAPConfig{
			URL:      "ldaps://localhost:636",
			BindDN:   "cn=admin,dc=test",
			BaseDN:   "dc=test",
		}
		if err := cfg.Validate(); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})
}

// ============================================================================
// 二、LDAP 组映射测试
// ============================================================================

func TestLDAPGroupMapper(t *testing.T) {
	mapper := auth.NewLDAPGroupMapper()
	mapper.AddMapping("cn=admins,ou=groups,dc=test", "role-admin")
	mapper.AddMapping("cn=operators,ou=groups,dc=test", "role-operator")

	groups := []*auth.LDAPGroup{
		{DN: "cn=admins,ou=groups,dc=test", Name: "admins"},
		{DN: "cn=users,ou=groups,dc=test", Name: "users"},
	}

	roles := mapper.MapGroupsToRoles(groups)
	if len(roles) != 1 || roles[0] != "role-admin" {
		t.Errorf("expected role-admin, got %v", roles)
	}
}

// ============================================================================
// 三、LDAP 模拟 Provider 测试
// ============================================================================

func TestMockLDAPProvider(t *testing.T) {
	provider := auth.NewMockLDAPProvider()
	provider.AddMockUser(&auth.LDAPUser{
		DN:       "uid=john,dc=test",
		Username: "john",
		Email:    "john@test.com",
	})
	provider.AddMockGroup(&auth.LDAPGroup{
		DN:      "cn=admins,ou=groups,dc=test",
		Name:    "admins",
		Members: []string{"john"},
	})

	t.Run("Authenticate success", func(t *testing.T) {
		user, err := provider.Authenticate("john", "password")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if user.Username != "john" {
			t.Errorf("unexpected username: %s", user.Username)
		}
	})

	t.Run("Authenticate not found", func(t *testing.T) {
		_, err := provider.Authenticate("jane", "password")
		if err == nil {
			t.Error("expected error for non-existent user")
		}
	})

	t.Run("Authenticate empty password", func(t *testing.T) {
		_, err := provider.Authenticate("john", "")
		if err == nil {
			t.Error("expected error for empty password")
		}
	})

	t.Run("GetUser", func(t *testing.T) {
		user, err := provider.GetUser("john")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if user.Email != "john@test.com" {
			t.Errorf("unexpected email: %s", user.Email)
		}
	})

	t.Run("GetUserGroups", func(t *testing.T) {
		groups, err := provider.GetUserGroups("john")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(groups) != 1 {
			t.Errorf("expected 1 group, got %d", len(groups))
		}
	})

	t.Run("SearchUsers", func(t *testing.T) {
		users, err := provider.SearchUsers("john")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(users) != 1 {
			t.Errorf("expected 1 user, got %d", len(users))
		}
	})

	t.Run("Close", func(t *testing.T) {
		if err := provider.Close(); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})
}

// ============================================================================
// 四、LDAP 认证服务测试
// ============================================================================

type mockUserStore struct {
	users map[string]*tenant.User
}

func newMockUserStore() *mockUserStore {
	return &mockUserStore{
		users: make(map[string]*tenant.User),
	}
}

func (s *mockUserStore) GetUserByUsername(username string) (*tenant.User, error) {
	for _, user := range s.users {
		if user.Username == username {
			return user, nil
		}
	}
	return nil, nil
}

func (s *mockUserStore) CreateUser(user *tenant.User) error {
	s.users[user.ID] = user
	return nil
}

func (s *mockUserStore) UpdateUser(user *tenant.User) error {
	s.users[user.ID] = user
	return nil
}

func TestLDAPAuthService(t *testing.T) {
	store := newMockUserStore()
	cfg := &auth.LDAPConfig{
		URL:    "ldaps://localhost:636",
		BindDN: "cn=admin,dc=test",
		BaseDN: "dc=test",
	}
	service := auth.NewLDAPAuthService(cfg, store)

	t.Run("Authenticate", func(t *testing.T) {
		result, err := service.Authenticate("john", "password")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.Success {
			t.Error("expected success")
		}
		if result.User.Username != "john" {
			t.Errorf("unexpected username: %s", result.User.Username)
		}
		if !result.IsNewUser {
			t.Error("expected new user")
		}
	})

	t.Run("Authenticate no config", func(t *testing.T) {
		svc := auth.NewLDAPAuthService(nil, store)
		_, err := svc.Authenticate("john", "password")
		if err == nil {
			t.Error("expected error for nil config")
		}
	})

	t.Run("SyncUser new", func(t *testing.T) {
		ldapUser := &auth.LDAPUser{
			Username: "jane",
			Email:    "jane@test.com",
		}
		user, err := service.SyncUser(ldapUser)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if user.Username != "jane" {
			t.Errorf("unexpected username: %s", user.Username)
		}
	})

	t.Run("SyncUser existing", func(t *testing.T) {
		store.CreateUser(&tenant.User{
			ID:       "existing",
			Username: "existing",
			Email:    "old@test.com",
		})
		ldapUser := &auth.LDAPUser{
			Username: "existing",
			Email:    "new@test.com",
		}
		user, err := service.SyncUser(ldapUser)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if user.Email != "new@test.com" {
			t.Errorf("expected email updated, got %s", user.Email)
		}
	})

	t.Run("SyncUser no store", func(t *testing.T) {
		svc := auth.NewLDAPAuthService(cfg, nil)
		_, err := svc.SyncUser(&auth.LDAPUser{Username: "test"})
		if err == nil {
			t.Error("expected error for nil store")
		}
	})

	t.Run("SyncUserGroups", func(t *testing.T) {
		mapper := auth.NewLDAPGroupMapper()
		mapper.AddMapping("cn=admins,ou=groups,dc=test", "role-admin")
		service.SetGroupMapper(mapper)

		groups := []*auth.LDAPGroup{
			{DN: "cn=admins,ou=groups,dc=test", Name: "admins"},
		}
		roles := service.SyncUserGroups(&auth.LDAPUser{Username: "john"}, groups)
		if len(roles) != 1 || roles[0] != "role-admin" {
			t.Errorf("expected role-admin, got %v", roles)
		}
	})

	t.Run("Default mappings", func(t *testing.T) {
		service.DefaultMappings()
		groups := []*auth.LDAPGroup{
			{DN: "cn=admins,ou=groups,dc=example,dc=com", Name: "admins"},
		}
		roles := service.SyncUserGroups(&auth.LDAPUser{Username: "john"}, groups)
		if len(roles) != 1 || roles[0] != "role-admin" {
			t.Errorf("expected role-admin from default mapping, got %v", roles)
		}
	})
}

// ============================================================================
// 五、OAuth2 配置测试
// ============================================================================

func TestOAuth2Config(t *testing.T) {
	t.Run("Validate empty client_id", func(t *testing.T) {
		cfg := &auth.OAuth2Config{
			AuthURL: "https://auth.example.com/auth",
		}
		if err := cfg.Validate(); err == nil {
			t.Error("expected error for empty client_id")
		}
	})

	t.Run("Validate valid", func(t *testing.T) {
		cfg := &auth.OAuth2Config{
			ClientID:     "client123",
			ClientSecret: "secret",
			AuthURL:      "https://auth.example.com/auth",
			TokenURL:     "https://auth.example.com/token",
			RedirectURL:  "https://app.example.com/callback",
		}
		if err := cfg.Validate(); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("Default scopes", func(t *testing.T) {
		cfg := &auth.OAuth2Config{
			ClientID:     "client123",
			ClientSecret: "secret",
			AuthURL:      "https://auth.example.com/auth",
			TokenURL:     "https://auth.example.com/token",
			RedirectURL:  "https://app.example.com/callback",
		}
		if len(cfg.Scopes) != 0 {
			t.Errorf("expected empty scopes initially, got %d", len(cfg.Scopes))
		}
	})
}

// ============================================================================
// 六、SSO 认证管理器测试
// ============================================================================

func TestSSOAuthManager(t *testing.T) {
	store := newMockUserStore()
	manager := auth.NewSSOAuthManager(store)

	cfg := &auth.OAuth2Config{
		ProviderName: "test-provider",
		ClientID:     "client123",
		ClientSecret: "secret",
		AuthURL:      "https://auth.example.com/auth",
		TokenURL:     "https://auth.example.com/token",
		RedirectURL:  "https://app.example.com/callback",
		UserInfoURL:  "https://auth.example.com/userinfo",
	}
	if err := manager.RegisterOAuth2Provider("test", cfg); err != nil {
		t.Fatalf("failed to register provider: %v", err)
	}

	t.Run("Register provider", func(t *testing.T) {
		providers := manager.GetProviders()
		found := false
		for _, p := range providers {
			if p == "test" {
				found = true
				break
			}
		}
		if !found {
			t.Error("expected provider 'test' to be registered")
		}
	})

	t.Run("Register invalid provider", func(t *testing.T) {
		invalidCfg := &auth.OAuth2Config{}
		if err := manager.RegisterOAuth2Provider("invalid", invalidCfg); err == nil {
			t.Error("expected error for invalid config")
		}
	})

	t.Run("GetAuthURL", func(t *testing.T) {
		authURL, state, err := manager.GetAuthURL("test")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if authURL == "" {
			t.Error("expected auth URL")
		}
		if state == "" {
			t.Error("expected state")
		}
		if !contains(authURL, "client_id=client123") {
			t.Error("expected auth URL to contain client_id")
		}
	})

	t.Run("GetAuthURL unknown provider", func(t *testing.T) {
		_, _, err := manager.GetAuthURL("unknown")
		if err == nil {
			t.Error("expected error for unknown provider")
		}
	})

	t.Run("Stats", func(t *testing.T) {
		stats := manager.GetStats()
		if stats["oauth2_providers"] != 1 {
			t.Errorf("expected 1 provider, got %v", stats["oauth2_providers"])
		}
	})

	t.Run("Cleanup flows", func(t *testing.T) {
		manager.CleanupFlows()
		_, state, _ := manager.GetAuthURL("test")
		if state == "" {
			t.Fatal("expected state")
		}
		manager.CleanupFlows()
	})
}

func TestSSOAuthManagerOIDC(t *testing.T) {
	store := newMockUserStore()
	manager := auth.NewSSOAuthManager(store)

	oidcCfg := &auth.OIDCConfig{
		Issuer:       "https://auth.example.com",
		ClientID:     "client123",
		ClientSecret: "secret",
		RedirectURL:  "https://app.example.com/callback",
	}
	if err := manager.RegisterOIDCProvider("oidc", oidcCfg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	providers := manager.GetProviders()
	found := false
	for _, p := range providers {
		if p == "oidc" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected OIDC provider to be registered")
	}
}

func TestSSOAuthManagerLogout(t *testing.T) {
	store := newMockUserStore()
	manager := auth.NewSSOAuthManager(store)
	cfg := &auth.OAuth2Config{
		ProviderName: "test",
		ClientID:     "client123",
		ClientSecret: "secret",
		AuthURL:      "https://auth.example.com/auth",
		TokenURL:     "https://auth.example.com/token",
		RedirectURL:  "https://app.example.com/callback",
		LogoutURL:    "https://auth.example.com/logout",
	}
	manager.RegisterOAuth2Provider("test", cfg)

	user := &tenant.User{ID: "user-1", Username: "test"}
	result := manager.Logout("test", user)
	if !result.Success {
		t.Error("expected logout success")
	}
	if !result.RemoteLogout {
		t.Error("expected remote logout")
	}
	if result.RedirectURL == "" {
		t.Error("expected redirect URL")
	}
}

func TestSSOAuthManagerLogoutNoProvider(t *testing.T) {
	store := newMockUserStore()
	manager := auth.NewSSOAuthManager(store)
	user := &tenant.User{ID: "user-1", Username: "test"}
	result := manager.Logout("unknown", user)
	if !result.Success {
		t.Error("expected local logout success")
	}
	if result.RemoteLogout {
		t.Error("expected no remote logout for unknown provider")
	}
}

// ============================================================================
// 七、SSO 组映射测试
// ============================================================================

func TestSSOGroupMapper(t *testing.T) {
	mapper := auth.NewSSOGroupMapper()
	mapper.AddMapping("admins", "role-admin")
	mapper.AddMapping("operators", "role-operator")
	mapper.AddMapping("users", "role-viewer")

	roles := mapper.MapGroupsToRoles([]string{"admins", "users"})
	if len(roles) != 2 {
		t.Errorf("expected 2 roles, got %d", len(roles))
	}

	roles2 := mapper.MapGroupsToRoles([]string{"admins", "admins"})
	if len(roles2) != 1 {
		t.Errorf("expected 1 unique role, got %d", len(roles2))
	}
}

// ============================================================================
// 八、Token 测试
// ============================================================================

func TestOAuth2Token(t *testing.T) {
	token := &auth.OAuth2Token{
		AccessToken:  "abc123",
		TokenType:    "Bearer",
		RefreshToken: "refresh456",
		ExpiresIn:    3600,
	}

	if token.IsExpired() {
		t.Error("expected token not expired")
	}
	if token.Expiry().Before(token.Expiry().Add(-3601 * time.Second)) {
		t.Error("expected expiry about 1 hour from now")
	}
}

func TestSSOUser(t *testing.T) {
	user := &auth.SSOUser{
		ID:            "sso-123",
		Username:      "john",
		Email:         "john@example.com",
		Name:          "John Doe",
		Provider:      "test",
		ProviderID:    "12345",
		EmailVerified: true,
		Groups:        []string{"admins", "users"},
	}
	if user.Email != "john@example.com" {
		t.Errorf("unexpected email: %s", user.Email)
	}
	if len(user.Groups) != 2 {
		t.Errorf("expected 2 groups, got %d", len(user.Groups))
	}
}

// ============================================================================
// 九、SAML 测试
// ============================================================================

func TestSAMLAuthService(t *testing.T) {
	store := newMockUserStore()
	cfg := &auth.SAMLConfig{
		ProviderName:             "saml-test",
		IDPMetadataURL:             "https://idp.example.com/metadata",
		EntityID:                 "https://app.example.com",
		AssertionConsumerService: "https://app.example.com/acs",
	}
	service := auth.NewSAMLAuthService(cfg, store)

	_, _, err := service.CreateAuthRequest()
	if err == nil {
		t.Fatal("expected SAML not fully implemented error")
	}

	_, err = service.ParseResponse("encoded-response")
	if err == nil {
		t.Fatal("expected SAML not fully implemented error")
	}
}

// ============================================================================
// 辅助函数
// ============================================================================

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) > 0 && containsSubstr(s, substr))
}

func containsSubstr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
