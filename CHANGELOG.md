# Changelog

All notable changes to CloudFlow will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- Webhook HMAC-SHA256 signature support for secure alert notifications
- Environment variable support for Kafka SASL password (`CLOUD_FLOW_KAFKA_SASL_PASS`)
- TLS certificate verification warning when `SkipTLSVerify` is enabled
- Unit tests for alert, circuitbreaker, and reliable modules
- CONTRIBUTING.md guide for contributors
- Apache 2.0 LICENSE file

### Changed
- Refactored main.go using Provider pattern for better dependency injection
- Updated Dockerfile to clarify CGO configuration (CGO_ENABLED=1 for eBPF support)
- Improved type assertion safety in dashboard API to prevent panics
- Enhanced Dockerfile comments with detailed capability requirements

### Fixed
- Fixed unsafe type assertion in dashboard tenant stats that could cause panic
- Corrected Dockerfile comments (scratch → alpine:3.20)
- Added proper error handling for JSON encoding in dashboard API

### Security
- Implemented proper HMAC-SHA256 signing for webhook notifications (replaced fake signature)
- Added security warnings for TLS verification bypass
- Supported environment variables for sensitive credentials

---

## [v0.1.0] - 2024-01-XX

### Added
- Initial release of CloudFlow
- eBPF-based network traffic collection (L3-L7)
- UnifiedFlow data model for metrics/logs/traces integration
- Three-tier architecture: Agent → Edge → Center
- ClickHouse and TiDB storage backends
- Kafka message queue for data streaming
- Prometheus metrics export
- Grafana dashboards for visualization
- Alert engine with multiple notification channels (Webhook, Kafka, API)
- Circuit breaker for resource protection
- Self-monitoring and health checks
- Docker Compose deployment for development and production
- RESTful APIs for management and querying

### Features
- **Agent**: Lightweight eBPF collector with <1% CPU overhead
- **Edge**: Distributed aggregation with consistent hashing
- **Center**: Scalable storage and query engine
- **Dashboard**: Real-time monitoring and analytics
- **Alerts**: Configurable rules with multi-channel notifications
- **Topology**: Automatic service dependency mapping

### Performance
- Supports 100K flows/sec collection
- 1M rows/sec write throughput to ClickHouse
- Sub-second query latency for common queries
- <50MB memory footprint per agent instance

### Documentation
- Comprehensive README with quick start guide
- Architecture overview and design decisions
- API documentation
- Deployment guides for Docker and Kubernetes

---

## Types of Changes

- **Added** for new features.
- **Changed** for changes in existing functionality.
- **Deprecated** for soon-to-be removed features.
- **Removed** for now removed features.
- **Fixed** for any bug fixes.
- **Security** in case of vulnerabilities.

---

## Release Process

1. Update version in relevant files
2. Update this CHANGELOG.md
3. Create git tag: `git tag v0.x.x`
4. Push tag: `git push origin v0.x.x`
5. Create GitHub Release with changelog notes
6. Build and publish Docker images
7. Update documentation

---

For more information about releases, see our [Release Guide](docs/release-guide.md).
