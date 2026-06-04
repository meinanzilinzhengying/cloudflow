# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

---

## [Unreleased]

### Added

- 可视化增强：新增多维度筛选 API（K8s 资源/协议/时间筛选）
- 可视化增强：新增高级数据导出功能（JSON/CSV 格式）
- 可视化增强：新增自定义仪表盘 CRUD API
- 可视化增强：新增大屏展示 API
- 文档：添加 K8s 部署指南
- 文档：添加故障排查手册
- 文档：添加用户操作手册
- 文档：添加 OpenAPI 接口文档

### Fixed

- 修复 Agent 模块编译错误（缺失 `time` 包导入）
- 修复拓扑统计中 `TotalBytes` 未计算问题

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