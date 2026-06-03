# Security Policy

## Supported Versions

我们积极维护 CloudFlow 的安全性。以下是当前受支持的版本：

| Version | Supported          |
| ------- | ------------------ |
| v0.1.x  | :white_check_mark: |
| < v0.1  | :x:                |

## Reporting a Vulnerability

我们非常重视安全问题。如果您发现了安全漏洞，请按照以下步骤报告：

### 🚨 紧急安全问题

**不要**通过公开 Issue 报告安全漏洞！

请立即发送邮件至：**cloudflow-security@meinanzilinzhengying.com**

邮件中请包含：
- 漏洞的详细描述
- 复现步骤
- 潜在影响评估
- 您的联系方式（可选）

我们将在 **24 小时内** 确认收到，并在 **72 小时内** 提供初步响应。

### 📧 非紧急安全问题

对于非紧急的安全问题或建议，您可以通过以下方式联系：

1. **GitHub Security Advisories**: 
   - 访问 https://github.com/meinanzilinzhengying/cloudflow/security/advisories
   - 点击 "Report a vulnerability"
   - 填写详细信息

2. **邮件**: cloudflow-security@meinanzilinzhengying.com

## Security Measures

CloudFlow 实施了多层安全措施：

### 🔐 数据安全

- **Webhook 签名**: 使用 HMAC-SHA256 确保消息真实性
- **TLS 加密**: 支持 HTTPS/TLS 通信
- **敏感信息保护**: 
  - Kafka SASL 密码支持环境变量
  - API Keys 不在日志中明文显示
  - 配置文件中的敏感字段有警告提示

### 🛡️ 运行时安全

- **eBPF 权限最小化**: 仅请求必要的 Linux capabilities
- **资源限制**: 
  - CPU/内存使用监控
  - 熔断器防止资源耗尽
  - Cgroup 支持资源隔离
- **输入验证**: 
  - SQL 注入防护（白名单校验）
  - Protobuf schema 验证
  - API 参数类型检查

### 🔍 审计与监控

- **操作日志**: 所有关键操作都有审计日志
- **自监控**: 
  - CPU/内存使用率监控
  - 丢包率监控
  - 健康检查端点
- **告警系统**: 异常行为自动告警

### 🏗️ 架构安全

- **微服务隔离**: 各组件独立部署，故障隔离
- **最小权限原则**: 每个服务只拥有必要的权限
- **网络分段**: Edge 和 Center 之间通过 Kafka 解耦

## Best Practices for Users

### 生产环境部署建议

1. **启用 TLS**
   ```yaml
   tls:
     enabled: true
     ca_cert: /path/to/ca.crt
     server_name: cloudflow.example.com
   ```

2. **使用环境变量管理敏感信息**
   ```bash
   export CLOUD_FLOW_KAFKA_SASL_PASS="your-password"
   export CLOUD_FLOW_API_KEY="your-api-key"
   ```

3. **配置防火墙规则**
   ```bash
   # 仅允许必要的端口
   iptables -A INPUT -p tcp --dport 8080 -j ACCEPT  # gRPC
   iptables -A INPUT -p tcp --dport 9090 -j ACCEPT  # Prometheus
   iptables -A INPUT -p tcp --dport 3000 -j ACCEPT  # Grafana
   ```

4. **定期更新**
   ```bash
   # 订阅 Release 通知
   # 定期检查安全更新
   git pull origin main
   ```

5. **启用认证**
   ```yaml
   auth:
     enabled: true
     api_keys:
       - key: "${API_KEY}"
         permissions: ["read", "write"]
   ```

6. **备份配置和数据**
   ```bash
   # 定期备份 ClickHouse 数据
   clickhouse-client --query "BACKUP DATABASE cloudflow TO Disk('backups', 'backup-$(date +%Y%m%d)')"
   
   # 备份 TiDB
   mydumper --host tidb-host --outputdir ./backup
   ```

### 开发环境安全

1. **不要提交敏感信息到 Git**
   - 使用 `.gitignore` 排除配置文件
   - 使用 `.secretsignore` 跟踪敏感文件
   - 使用环境变量或密钥管理服务

2. **代码审查**
   - 所有 PR 必须经过安全审查
   - 使用静态分析工具（gosec）
   - 依赖漏洞扫描（trivy）

3. **测试安全功能**
   ```bash
   # 运行安全测试
   go test ./... -run TestSecurity
   
   # 运行静态分析
   gosec ./...
   
   # 扫描依赖漏洞
   trivy fs .
   ```

## Known Security Issues

我们会在 GitHub Security Advisories 中公开已知的安全问题：
https://github.com/meinanzilinzhengying/cloudflow/security/advisories

## Security Updates

安全更新将通过以下方式发布：

1. **GitHub Releases**: 标记为 "Security Release"
2. **Security Advisories**: 详细的安全公告
3. **邮件通知**: 订阅者将收到安全更新通知
4. **CHANGELOG.md**: 在 "Security" 部分记录

## Responsible Disclosure

我们遵循负责任的披露原则：

1. **发现漏洞** → 私下报告给我们
2. **确认漏洞** → 我们进行验证
3. **修复漏洞** → 开发补丁
4. **协调发布** → 确定公开时间
5. **公开披露** → 发布安全公告
6. **给予信用** → 感谢报告者（如愿意）

## Security Contacts

- **Email**: cloudflow-security@meinanzilinzhengying.com
- **PGP Key**: [下载 PGP 公钥](https://cloudflow.io/pgp-key.asc)
- **GitHub**: @cloudflow-security-team

## Acknowledgments

我们感谢以下安全研究人员和组织的贡献：

- [GitHub Security Lab](https://securitylab.github.com/)
- [OWASP](https://owasp.org/)
- 所有负责任地报告漏洞的研究人员

---

**Last Updated**: 2024-01-XX

**Policy Version**: 1.0
