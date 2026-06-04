# CloudFlow 版本管理规范

## 概述

本文档定义了 CloudFlow 项目的版本管理规范，包括语义化版本控制、CHANGELOG 管理、版本发布流程、升级与回滚策略。

---

## 目录

1. [语义化版本控制](#1-语义化版本控制)
2. [CHANGELOG 管理](#2-changelog-管理)
3. [版本发布流程](#3-版本发布流程)
4. [升级与回滚策略](#4-升级与回滚策略)
5. [分支管理策略](#5-分支管理策略)
6. [标签管理](#6-标签管理)

---

## 1. 语义化版本控制

### 1.1 版本格式

CloudFlow 遵循 [Semantic Versioning 2.0.0](https://semver.org/spec/v2.0.0.html) 规范：

```
MAJOR.MINOR.PATCH
```

### 1.2 版本号含义

| 部分 | 含义 | 更新场景 |
|------|------|----------|
| **MAJOR** | 主版本号 | 不兼容的 API 变更 |
| **MINOR** | 次版本号 | 向后兼容的功能新增 |
| **PATCH** | 修订版本号 | 向后兼容的问题修复 |

### 1.3 版本递增规则

#### MAJOR 版本递增

当进行**不兼容的 API 变更**时，递增主版本号：

- 删除已公开的 API
- 修改 API 的签名或返回值格式
- 移除或重命名配置项
- 数据库 schema 不兼容变更
- 协议变更（如 gRPC 接口变更）

**示例**：从 `1.x.x` 升级到 `2.0.0`

#### MINOR 版本递增

当进行**向后兼容的功能新增**时，递增次版本号：

- 新增 API 端点
- 添加新功能模块
- 扩展现有 API（向后兼容）
- 新增配置项（非必填）
- 新增文档

**示例**：从 `1.0.x` 升级到 `1.1.0`

#### PATCH 版本递增

当进行**向后兼容的问题修复**时，递增修订版本号：

- 修复 bug
- 性能优化（无 API 变更）
- 安全补丁
- 文档更新（非功能变更）

**示例**：从 `1.0.0` 升级到 `1.0.1`

### 1.4 预发布版本

对于不稳定的开发版本，可使用预发布版本号：

```
MAJOR.MINOR.PATCH-PRERELEASE
```

**预发布标识符**：

| 标识符 | 含义 |
|--------|------|
| `alpha` | 内部测试版本 |
| `beta` | 公开测试版本 |
| `rc` | 候选发布版本（Release Candidate） |

**示例**：`1.0.0-alpha.1`、`2.0.0-beta.3`、`3.1.0-rc.2`

---

## 2. CHANGELOG 管理

### 2.1 CHANGELOG 格式

项目使用 [Keep a Changelog](https://keepachangelog.com/en/1.0.0/) 格式：

```markdown
# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- 新增功能描述

### Changed
- 修改内容描述

### Deprecated
- 即将移除的功能描述

### Removed
- 已移除的功能描述

### Fixed
- 修复内容描述

### Security
- 安全修复描述

## [1.0.0] - 2024-01-15
...
```

### 2.2 变更分类

| 分类 | 说明 | 示例 |
|------|------|------|
| **Added** | 新增功能、API、配置 | 新增多维度筛选 API |
| **Changed** | 现有功能变更（向后兼容） | 修改 API 响应格式 |
| **Deprecated** | 标记为弃用的功能 | 标记 `/v1/old-api` 即将移除 |
| **Removed** | 已移除的功能 | 移除 `/v1/old-api` |
| **Fixed** | Bug 修复 | 修复流量统计计算错误 |
| **Security** | 安全修复 | 修复认证绕过漏洞 |

### 2.3 自动化工具配置

使用 `standard-version` 工具自动生成 CHANGELOG：

**安装依赖**：

```bash
npm install -g standard-version
```

**配置文件**（`.versionrc`）：

```json
{
  "types": [
    { "type": "feat", "section": "Added" },
    { "type": "feature", "section": "Added" },
    { "type": "fix", "section": "Fixed" },
    { "type": "refactor", "section": "Changed" },
    { "type": "perf", "section": "Changed" },
    { "type": "docs", "section": "Added" },
    { "type": "chore", "section": "Changed" },
    { "type": "test", "section": "Changed" },
    { "type": "security", "section": "Security" },
    { "type": "deprecate", "section": "Deprecated" },
    { "type": "remove", "section": "Removed" }
  ],
  "commitUrlFormat": "https://github.com/meinanzilinzhengying/cloudflow/commit/{{hash}}",
  "compareUrlFormat": "https://github.com/meinanzilinzhengying/cloudflow/compare/{{previousTag}}...{{currentTag}}",
  "issueUrlFormat": "https://github.com/meinanzilinzhengying/cloudflow/issues/{{issue}}"
}
```

**使用命令**：

```bash
# 发布新版本（自动更新 CHANGELOG 和版本号）
standard-version

# 发布预发布版本
standard-version --prerelease alpha

# 发布指定类型的版本
standard-version --release-as minor
```

### 2.4 Commit 消息规范

项目使用 [Conventional Commits](https://www.conventionalcommits.org/en/v1.0.0/)：

```
<type>(<scope>): <description>

[optional body]

[optional footer(s)]
```

**类型说明**：

| 类型 | 说明 |
|------|------|
| `feat` | 新增功能 |
| `fix` | 修复 bug |
| `docs` | 文档更新 |
| `style` | 代码格式调整（不影响功能） |
| `refactor` | 代码重构（不改变功能） |
| `perf` | 性能优化 |
| `test` | 测试相关 |
| `chore` | 构建/工具更新 |
| `security` | 安全修复 |

**示例**：

```
feat(center): 新增多维度流量筛选 API

- 添加 /api/traffic/advanced 端点
- 支持 K8s 资源/协议/时间筛选
- 添加分页支持

fix(agent): 修复 eBPF 程序加载失败问题

修复内核版本检测逻辑错误
```

---

## 3. 版本发布流程

### 3.1 发布前检查清单

| 检查项 | 说明 | 责任人 |
|--------|------|--------|
| 代码质量 | 通过所有测试 | 开发人员 |
| 文档更新 | API 文档同步更新 | 开发人员 |
| CHANGELOG | 更新未发布变更 | 发布负责人 |
| 依赖检查 | 确认依赖版本 | 发布负责人 |
| 兼容性检查 | 确认向后兼容性 | 架构师 |
| 安全审计 | 完成安全扫描 | 安全团队 |

### 3.2 发布流程

```
开发完成 → 代码审查 → 测试通过 → 更新 CHANGELOG → 更新版本号 → 创建标签 → 推送到远程
```

**详细步骤**：

1. **完成开发**：功能开发和 bug 修复完成
2. **代码审查**：提交 PR，通过代码审查
3. **测试验证**：通过所有自动化测试和人工验证
4. **更新文档**：更新相关文档
5. **生成 CHANGELOG**：运行 `standard-version` 生成 CHANGELOG
6. **提交变更**：提交版本更新到 `main` 分支
7. **创建标签**：`git tag -a v1.0.0 -m "Release v1.0.0"`
8. **推送标签**：`git push origin v1.0.0`
9. **发布通知**：发送版本发布通知

### 3.3 版本发布模板

**发布说明模板**：

```markdown
## CloudFlow v1.0.0 发布

### 新增功能

- 新增多维度流量筛选 API
- 新增数据导出功能（JSON/CSV）
- 新增自定义仪表盘功能

### 问题修复

- 修复 Agent 编译错误
- 修复拓扑统计计算问题

### 文档更新

- 添加 K8s 部署指南
- 添加故障排查手册
- 添加 OpenAPI 接口文档

### 升级方式

```bash
# Helm 升级
helm upgrade cloudflow cloudflow/cloudflow --version 1.0.0

# 手动升级
kubectl apply -f deploy/
```

### 兼容性

- 与 v0.9.x 版本完全兼容
- 数据库 schema 无变更
```

---

## 4. 升级与回滚策略

### 4.1 升级策略

#### 升级前准备

1. **备份数据**：
   ```bash
   # 备份 ClickHouse 数据
   clickhouse-client -q "BACKUP DATABASE cloudflow TO 's3://backup/cloudflow-v1.0.0'"
   
   # 备份 Redis 数据
   redis-cli SAVE
   kubectl cp <redis-pod>:/data/dump.rdb ./backup/dump.rdb
   ```

2. **检查版本兼容性**：查看 CHANGELOG 中是否有不兼容变更

3. **制定升级计划**：选择低峰期进行升级

#### 升级步骤

**滚动升级（推荐）**：

```bash
# 升级 Center 服务
kubectl rollout restart deployment/cloudflow-center

# 等待升级完成
kubectl rollout status deployment/cloudflow-center

# 升级 Edge 服务
kubectl rollout restart deployment/cloudflow-edge
kubectl rollout status deployment/cloudflow-edge

# 升级 Agent
kubectl rollout restart daemonset/cloudflow-agent
kubectl rollout status daemonset/cloudflow-agent
```

**蓝绿部署**：

```bash
# 创建新版本部署
kubectl apply -f deploy/cloudflow-center-v2.yaml

# 切换流量
kubectl patch service cloudflow-center -p '{"spec":{"selector":{"version":"v2"}}}'

# 验证成功后删除旧版本
kubectl delete deployment cloudflow-center-v1
```

### 4.2 回滚策略

#### 回滚触发条件

| 条件 | 说明 |
|------|------|
| 服务不可用 | 升级后服务无法正常启动 |
| 数据异常 | 数据处理出现错误 |
| 性能下降 | 性能指标严重下降 |
| API 故障 | 关键 API 无法正常响应 |
| 安全漏洞 | 发现严重安全问题 |

#### 回滚步骤

**快速回滚**：

```bash
# 使用 Helm 回滚
helm rollback cloudflow <revision>

# 使用 kubectl 回滚
kubectl rollout undo deployment/cloudflow-center
kubectl rollout undo deployment/cloudflow-edge
kubectl rollout undo daemonset/cloudflow-agent
```

**数据恢复**：

```bash
# 恢复 ClickHouse 数据
clickhouse-client -q "RESTORE DATABASE cloudflow FROM 's3://backup/cloudflow-v1.0.0'"

# 恢复 Redis 数据
kubectl cp ./backup/dump.rdb <redis-pod>:/data/dump.rdb
kubectl exec -it <redis-pod> -- redis-cli SHUTDOWN NOSAVE
kubectl exec -it <redis-pod> -- redis-server /etc/redis/redis.conf
```

### 4.3 版本兼容矩阵

| 当前版本 | 可升级版本 | 是否需要数据迁移 |
|----------|------------|------------------|
| 0.9.x | 1.0.x | 否 |
| 1.0.x | 1.1.x | 否 |
| 1.1.x | 1.2.x | 否 |
| 1.x.x | 2.0.x | 是 |

---

## 5. 分支管理策略

### 5.1 分支类型

| 分支 | 用途 | 生命周期 |
|------|------|----------|
| `main` | 主分支，生产就绪代码 | 永久 |
| `develop` | 开发分支，集成功能 | 永久 |
| `feature/*` | 功能开发分支 | 临时 |
| `bugfix/*` | Bug 修复分支 | 临时 |
| `release/*` | 发布准备分支 | 临时 |
| `hotfix/*` | 紧急修复分支 | 临时 |

### 5.2 分支流转

```
feature/* → develop → release/* → main
                          ↓
                     hotfix/*
```

### 5.3 分支命名规范

| 类型 | 命名格式 | 示例 |
|------|----------|------|
| 功能分支 | `feature/feature-name` | `feature/multi-dimensional-filter` |
| Bug 修复 | `bugfix/bug-description` | `bugfix/traffic-calculation-error` |
| 发布分支 | `release/x.y.z` | `release/1.0.0` |
| 紧急修复 | `hotfix/issue-description` | `hotfix/api-timeout` |

---

## 6. 标签管理

### 6.1 标签格式

```
v<MAJOR>.<MINOR>.<PATCH>
v<MAJOR>.<MINOR>.<PATCH>-<PRERELEASE>
```

**示例**：

- `v1.0.0`
- `v1.1.0-beta.1`
- `v2.0.0-rc.2`

### 6.2 标签创建流程

```bash
# 创建带注释的标签
git tag -a v1.0.0 -m "Release v1.0.0"

# 推送到远程
git push origin v1.0.0

# 删除标签
git tag -d v1.0.0
git push origin :v1.0.0
```

### 6.3 标签管理规范

- 每个正式发布版本必须创建标签
- 标签必须与 CHANGELOG 中的版本对应
- 标签命名必须遵循 `vX.Y.Z` 格式
- 删除标签需要团队确认

---

**文档版本**: v1.0  
**最后更新**: 2024-01-15  
**适用版本**: CloudFlow v1.0+