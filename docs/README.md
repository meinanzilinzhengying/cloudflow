# CloudFlow 文档

欢迎查阅 CloudFlow 文档！这里包含了项目的详细技术文档、使用指南和最佳实践。

## 📚 文档目录

### 🚀 快速开始

- [安装指南](installation.md) - 如何安装和配置 CloudFlow
- [快速入门](quickstart.md) - 5分钟上手 CloudFlow
- [架构概述](architecture.md) - 系统架构和设计原理

### 📖 用户指南

- [Agent 配置](agent-configuration.md) - Agent 详细配置说明
- [Edge 部署](edge-deployment.md) - Edge 节点部署指南
- [Center 管理](center-management.md) - Center 服务管理
- [告警配置](alert-configuration.md) - 告警规则配置
- [Dashboard 使用](dashboard-usage.md) - Grafana 仪表板使用

### 🔧 开发指南

- [贡献指南](../CONTRIBUTING.md) - 如何参与项目开发
- [代码规范](code-style.md) - Go 代码规范和最佳实践
- [API 文档](api-reference.md) - RESTful API 参考
- [扩展开发](extension-development.md) - 如何开发自定义插件
- [测试指南](testing-guide.md) - 单元测试和集成测试

### 🏗️ 架构设计

- [UnifiedFlow 数据模型](unified-flow.md) - 统一流量数据结构
- [eBPF 采集原理](ebpf-collection.md) - eBPF 技术详解
- [存储引擎](storage-engine.md) - ClickHouse/TiDB 存储设计
- [消息队列](message-queue.md) - Kafka 消息系统设计
- [一致性哈希](consistent-hashing.md) - 负载均衡算法

### 🔐 安全

- [安全策略](../SECURITY.md) - 安全报告和响应流程
- [认证授权](authentication.md) - API 认证和权限管理
- [TLS 配置](tls-configuration.md) - TLS/SSL 配置指南
- [最佳实践](security-best-practices.md) - 安全部署最佳实践

### 🚢 部署

- [Docker 部署](docker-deployment.md) - Docker 和 Docker Compose 部署
- [Kubernetes 部署](kubernetes-deployment.md) - K8s Helm Chart 部署
- [生产环境](production-deployment.md) - 生产环境部署指南
- [高可用配置](high-availability.md) - 高可用架构配置
- [监控告警](monitoring-setup.md) - Prometheus + Grafana 监控

### 📊 运维

- [故障排查](troubleshooting.md) - 常见问题和解决方案
- [性能调优](performance-tuning.md) - 性能优化指南
- [备份恢复](backup-restore.md) - 数据备份和恢复
- [升级指南](upgrade-guide.md) - 版本升级步骤
- [日志分析](log-analysis.md) - 日志收集和分析

### 🎯 最佳实践

- [大规模部署](large-scale-deployment.md) - 千节点规模部署经验
- [成本控制](cost-optimization.md) - 资源使用和成本优化
- [容量规划](capacity-planning.md) - 容量评估和规划
- [灾难恢复](disaster-recovery.md) - 灾难恢复计划

## 📸 截图占位符

以下截图需要添加到 `images/` 目录：

- `dashboard-overview.png` - Dashboard 概览
- `service-topology.png` - 服务拓扑图
- `l7-protocol-analysis.png` - L7 协议分析
- `alert-management.png` - 告警管理界面
- `architecture-diagram.png` - 架构图
- `deployment-flow.png` - 部署流程图

## 🔗 外部资源

- [GitHub Repository](https://github.com/meinanzilinzhengying/cloudflow)
- [Issue Tracker](https://github.com/meinanzilinzhengying/cloudflow/issues)
- [Releases](https://github.com/meinanzilinzhengying/cloudflow/releases)
- [Security Advisories](https://github.com/meinanzilinzhengying/cloudflow/security/advisories)

## 📝 文档贡献

如果您想改进文档，请参考 [CONTRIBUTING.md](../CONTRIBUTING.md)。

### 文档规范

1. **格式**: 使用 Markdown 格式
2. **语言**: 中文为主，关键术语保留英文
3. **结构**: 
   - 清晰的标题层级
   - 适当的代码示例
   - 必要的截图说明
4. **更新**: 功能变更时同步更新文档

### 添加新文档

```bash
# 创建新文档
touch docs/your-topic.md

# 添加到目录
echo "- [Your Topic](your-topic.md)" >> docs/README.md
```

## 🙏 致谢

感谢所有为 CloudFlow 文档做出贡献的开发者！

---

**Last Updated**: 2024-01-XX

**Document Version**: 1.0
