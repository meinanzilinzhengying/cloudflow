# Contributing to CloudFlow

Thank you for your interest in contributing to CloudFlow! This document provides guidelines for contributing to the project.

## Table of Contents

- [Development Setup](#development-setup)
- [Running Tests](#running-tests)
- [Code Review Process](#code-review-process)
- [Release Process](#release-process)
- [Security Policy](#security-policy)
- [Code of Conduct](#code-of-conduct)

---

## Development Setup

### Prerequisites

- **Go 1.21+** — [Download](https://go.dev/dl/)
- **Docker 24.0+** and **Docker Compose 2.20+**
- **Make** (optional, for convenience scripts)
- **Protocol Buffers compiler** (`protoc`) v3.20+ — [Installation guide](https://grpc.io/docs/protoc-installation/)
- **Node.js 18+** (for frontend components, if applicable)

### Clone and Build

```bash
# Clone the repository
git clone https://github.com/your-org/cloudflow.git
cd cloudflow

# Set up Go workspace (if using go.work)
export GOWORK=/opt/cloudflow/go.work
export GOROOT=/usr/local/go
export PATH=/usr/local/go/bin:$PATH

# Build all services
make build
# or manually:
go build ./cmd/alert-engine
# ... repeat for each service
```

### Local Environment Setup

```bash
# Start infrastructure dependencies
docker-compose -f docker-compose.staging.yml up -d zookeeper kafka mysql clickhouse redis etcd

# Run migrations (if applicable)
make migrate

# Start individual services for development
./bin/auth-service
./bin/tenant-service
# ... etc
```

### IDE Configuration

- **VS Code:** Recommended extensions are in `.vscode/extensions.json`
- **GoLand / IntelliJ:** Import the project as a Go module
- **Linting:** We use `golangci-lint`. Run `make lint` before committing.

---

## Running Tests

### Unit Tests

```bash
# Run all unit tests
make test

# Run tests for a specific service
go test ./services/alert-engine/...

# Run with coverage
make test-coverage
```

### Integration Tests

```bash
# Integration tests require a full stack running
make integration-test

# Or manually:
docker-compose -f docker-compose.staging.yml up -d
# wait for services to be healthy, then:
go test -tags=integration ./tests/integration/...
```

### Load Tests

```bash
# Run the built-in load test tool
go run scripts/load-test.go -target http://localhost:8009 -concurrency 100 -duration 5m

# For custom scenarios, see scripts/load-test.go configuration flags
```

### Test Guidelines

- All new code must include unit tests with >70% coverage.
- Integration tests are required for API changes and cross-service features.
- Mock external dependencies (databases, message queues) in unit tests.
- Table-driven tests are preferred for Go code.

---

## Code Review Process

### Branching Strategy

- `main` — Production-ready code. Protected branch.
- `staging` — Pre-production validation. Protected branch.
- `feature/*` — Feature branches.
- `bugfix/*` — Bug fix branches.
- `hotfix/*` — Production hotfix branches.

### Submitting Changes

1. **Create an Issue** (or link to an existing one) describing the problem or feature.
2. **Fork / Branch** from the latest `staging` branch.
3. **Develop** your changes following our coding standards.
4. **Write tests** covering your changes.
5. **Run the full test suite** locally and ensure it passes.
6. **Commit** with clear, descriptive messages following [Conventional Commits](https://www.conventionalcommits.org/):
   - `feat:` — New feature
   - `fix:` — Bug fix
   - `docs:` — Documentation only
   - `test:` — Tests only
   - `refactor:` — Code refactoring
   - `perf:` — Performance improvement
   - `chore:` — Maintenance / tooling
7. **Open a Pull Request** targeting the `staging` branch.
8. **Fill out the PR template** completely.

### Review Criteria

- At least **2 approvals** from maintainers are required.
- All CI checks must pass (build, test, lint, security scan).
- No merge conflicts with the target branch.
- Code must follow the project's style guide (enforced by `golangci-lint`).
- Documentation must be updated for any API or configuration changes.

### PR Template Checklist

- [ ] Issue linked in the PR description
- [ ] Tests added or updated
- [ ] Documentation updated (README, API docs, deployment guides)
- [ ] CHANGELOG.md updated (if applicable)
- [ ] No breaking changes without approval
- [ ] Security implications reviewed (see [Security Policy](#security-policy))

---

## Release Process

### Versioning

CloudFlow follows [Semantic Versioning](https://semver.org/):
- `MAJOR.MINOR.PATCH` (e.g., `v1.2.3`)

### Release Cadence

- **Patch releases:** As needed for critical bug fixes and security patches.
- **Minor releases:** Bi-weekly sprints, on the 2nd and 4th Friday.
- **Major releases:** Quarterly, with a 4-week beta period on `staging`.

### Release Steps

1. **Freeze:** Merge window closes for the target branch.
2. **Validation:** Full regression testing on `staging`.
3. **Version Bump:** Update version constants in `cmd/*/main.go` and `VERSION` file.
4. **Changelog:** Finalize `CHANGELOG.md` with all changes since the last release.
5. **Tag:** Create a signed Git tag (`git tag -s v1.2.3 -m "Release v1.2.3"`).
6. **Build:** CI builds Docker images for all services and pushes to the registry.
7. **Deploy:** Deploy to production using the blue-green deployment procedure.
8. **Announce:** Post release notes in the project discussion forum.

### Emergency Hotfix

For critical production issues:

1. Branch from `main` into `hotfix/<description>`.
2. Apply the minimal fix with tests.
3. Fast-track review (1 approval from a senior maintainer).
4. Merge to `main` and `staging` simultaneously.
5. Deploy immediately and monitor.

---

## Security Policy

### Reporting Security Vulnerabilities

**DO NOT** open a public issue for security vulnerabilities.

Instead, email security@cloudflow.example.com with:
- A description of the vulnerability
- Steps to reproduce
- Potential impact assessment
- Suggested mitigation (if any)

You will receive an acknowledgment within 24 hours and a detailed response within 72 hours. We follow a 90-day disclosure timeline.

### Security Best Practices for Contributors

- **Secrets:** Never commit passwords, API keys, or tokens. Use environment variables and secret management tools.
- **Dependencies:** Keep dependencies updated. Run `make audit` to check for known vulnerabilities.
- **Input Validation:** Validate all external inputs at service boundaries.
- **SQL Injection:** Use parameterized queries / ORM; never concatenate SQL strings.
- **XSS / CSRF:** Sanitize output and use proper CSRF tokens for web endpoints.
- **gRPC Security:** Ensure TLS is configured for production gRPC channels.
- **Docker:** Use non-root containers and scan images with `trivy` or `snyk`.

### Security Checks in CI

- `govulncheck` — Go vulnerability scanner
- `trivy` — Container image vulnerability scanner
- `gosec` — Go security checker
- `dependabot` — Automated dependency updates

---

## Code of Conduct

We are committed to providing a welcoming and inclusive experience for everyone. Please be respectful and constructive in all interactions.

- Use inclusive language.
- Be respectful of differing viewpoints and experiences.
- Focus on what is best for the community and the project.
- Show empathy towards other contributors.

Violations of the code of conduct may result in temporary or permanent exclusion from the project.

---

## Questions?

If you have questions not covered by this guide, please:
- Open a [GitHub Discussion](https://github.com/your-org/cloudflow/discussions)
- Join our community Slack: [cloudflow.slack.com](https://cloudflow.slack.com)
- Contact the maintainers: maintainers@cloudflow.example.com
