# CloudFlow Security OWASP Top 10 Checklist

This checklist covers the OWASP Top 10 security risks for the CloudFlow platform.

---

## A01: Broken Access Control

- [ ] **RBAC Enforcement**: All gRPC and HTTP handlers enforce tenant-scoped access control via `services/shared/auth` and `services/shared/tenant` middleware.
  - Reference: `services/alert-engine/service.go` (validateRuleOwnership), `services/tenant-service/service.go`
  - Verify: Non-admin users cannot access cross-tenant resources.
- [ ] **Least Privilege**: Default roles (admin, user, viewer) are enforced at the API gateway and service layers.
  - Reference: `services/auth-service/rbac/casbin.go`
- [ ] **API Endpoint Protection**: Admin endpoints (e.g., `/api/configs`, rule CRUD) reject unauthenticated requests.
  - Reference: `services/control-plane/service.go` (authMiddleware)
- [ ] **Resource ID Enumeration**: APIs do not expose sequential IDs; use UUIDs or nanosecond-based IDs.

## A02: Cryptographic Failures

- [ ] **TLS 1.2+**: All inter-service gRPC and HTTP communications use TLS 1.2 or higher.
  - Reference: `services/shared/tlsutil/tlsutil.go`, `services/*/service.go` (TLSEnabled, TLSCertFile, TLSKeyFile)
- [ ] **Certificate Validation**: `InsecureSkipVerify` is disabled in production (TLSInsecureSkip = false).
  - Reference: `services/*/service.go` Config structs
- [ ] **mTLS Support**: Client certificates are enforced where sensitive data flows (control-plane to agent).
  - Reference: `services/control-plane/service.go` (TLSClientAuth)
- [ ] **Password/Secret Storage**: Database passwords are not logged in plaintext; use environment variables or secret managers.
  - Reference: `services/*/service.go` Config structs (RelationalDBPassword, ClickHousePassword)
- [ ] **Sensitive Data Masking**: Passwords and API keys are masked in logs.
  - Reference: `services/shared/logger/middleware.go` (LogSanitizer)

## A03: Injection

- [ ] **SQL Injection Prevention**: All SQL queries use parameterized statements (prepared statements) via the storage abstraction layer.
  - Reference: `services/alert-engine/service.go` (db.Exec with `?` placeholders), `services/tenant-service/service.go`
  - Verify: No string-concatenated SQL queries exist in service code.
- [ ] **gRPC Injection**: gRPC message sizes are bounded (MaxRecvMsgSize / MaxSendMsgSize).
  - Reference: `services/control-plane/service.go`, `services/data-plane/service.go` (grpc.MaxRecvMsgSize)
- [ ] **NoSQL/ClickHouse Injection**: ClickHouse queries use parameterized inputs where possible.
  - Reference: `services/alert-engine/service.go` (getLatestMetrics), `services/data-plane/service.go`
- [ ] **Command Injection**: System command execution (e.g., docker stats) uses fixed argument lists, not shell interpolation.
  - Reference: `services/control-plane/service.go` (exec.Command with discrete arguments)

## A05: Security Misconfiguration

- [ ] **Default Credentials**: No hardcoded default passwords in production configs.
  - Reference: `services/*/service.go` DefaultConfig()
- [ ] **Error Handling**: Services do not leak stack traces or internal paths to clients in production.
  - Reference: `services/*/service.go` HTTP handlers (use generic error messages)
- [ ] **Feature Disabling**: Unused features (MockMetricsEnabled, debug endpoints) are disabled in production.
  - Reference: `services/alert-engine/service.go` (MockMetricsEnabled)
- [ ] **Security Headers**: HTTP responses include appropriate security headers (CSP, HSTS, X-Frame-Options).
  - Reference: `services/*/service.go` HTTP server configuration
- [ ] **Dependency Scanning**: Run `scripts/security-scan.sh` periodically to detect vulnerable dependencies.
  - Reference: `scripts/security-scan.sh`

## A07: Identification and Authentication Failures

- [ ] **JWT Validation**: Tokens are validated for signature, expiration, and issuer claims.
  - Reference: `services/auth-service/auth/jwt.go`, `services/shared/auth/auth.go`
- [ ] **Session Management**: JWT blacklist is enforced for logout/revocation.
  - Reference: `services/auth-service/internal/blacklist/blacklist.go`
- [ ] **Rate Limiting**: Authentication endpoints are rate-limited to prevent brute force.
  - Reference: `services/shared/ratelimit/middleware.go`
- [ ] **Multi-Factor Authentication**: Platform admin accounts require MFA where applicable.
  - Reference: `services/auth-service/`
- [ ] **Credential Recovery**: Secure password reset flow with time-limited tokens.

## A09: Security Logging and Monitoring Failures

- [ ] **Audit Logging**: All tenant-scoped mutations (create, update, delete) are logged with actor, action, and outcome.
  - Reference: `services/shared/audit/audit.go`
- [ ] **Log Sanitization**: Sensitive fields (passwords, tokens, API keys) are masked before writing to logs.
  - Reference: `services/shared/logger/middleware.go` (LogSanitizer)
- [ ] **Failed Authentication Logging**: Failed login attempts and token validation failures are logged with client IP.
  - Reference: `services/auth-service/service.go`, `services/shared/auth/auth.go`
- [ ] **Anomaly Detection**: Alert engine rules can detect abnormal access patterns (e.g., high error rates).
  - Reference: `services/alert-engine/service.go`
- [ ] **Centralized Log Shipping**: Service logs are forwarded to Loki for retention and forensics.
  - Reference: `services/data-plane/service.go` (Loki integration)

---

## Verification Commands

```bash
# Run vulnerability scan
bash scripts/security-scan.sh

# Check for hardcoded secrets
grep -r -E "password|secret|token|api_key" --include="*.go" services/ || true

# Verify TLS config in all services
grep -r "TLSEnabled" --include="*.go" services/
```
