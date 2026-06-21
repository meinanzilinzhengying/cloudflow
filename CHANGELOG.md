# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

---

## [Unreleased]

### Added

- **CI/CD 增强**：GitHub Actions 新增 services 模块测试、goimports 检查、测试覆盖率门禁
- **代码质量**：新增本地 pre-commit 代码质量检查脚本
- **测试覆盖**：新增 4 个核心服务模块单元测试（告警引擎/数据平面/控制平面/查询服务），1252 行测试代码
- **生产部署**：新增生产环境部署检查清单
- **可视化增强**：前端 Dashboard/Alerts/Topology 三个页面从硬编码 Mock 数据改为调用后端 API

### Fixed

- **前端 Mock 数据修复**：Topology.vue、Dashboard.vue、Alerts.vue 改为从后端 API 获取真实数据
- **后端拓扑数据**：handleTopology 从 s.store.GetNodes() 获取真实节点构建 Gateway->LB->Node 拓扑
- **安全加固**：添加 OWASP 安全检查步骤到 CI 工作流
- **监控增强**：CI 流程增加测试覆盖率报告和构建产物缓存

### Changed

- **CI 测试范围**：从仅测试 `pkg/` 扩展为测试 `pkg/`、`services/`、`cloud-flow-center/` 等核心目录
- **代码风格**：CI 新增 `gofmt -d` 和 `goimports` 检查，未通过代码风格检查的 PR 将被拒绝

---

## [1.0.0] - 2024-01-15

### Added

- 初始版本发布
- Agent 模块：eBPF 流量采集
- Edge 模块：数据处理与聚合
- Center 模块：API 服务与可视化
- 支持 ClickHouse 存储
- 支持 Redis 缓存
- 支持 VictoriaMetrics 指标存储
- gRPC 通信协议
- RESTful API 接口
- JWT 认证
