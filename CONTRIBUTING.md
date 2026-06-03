# Contributing to CloudFlow

感谢你对 CloudFlow 项目的关注！我们欢迎各种形式的贡献，包括 bug 修复、功能增强、文档改进等。

## 📋 目录

- [行为准则](#行为准则)
- [如何贡献](#如何贡献)
- [开发环境设置](#开发环境设置)
- [提交代码流程](#提交代码流程)
- [代码规范](#代码规范)
- [测试要求](#测试要求)
- [Commit 规范](#commit-规范)
- [Issue 报告](#issue-报告)
- [Pull Request 流程](#pull-request-流程)

## 行为准则

本项目采用 [Contributor Covenant](https://www.contributor-covenant.org/) 行为准则。请尊重所有参与者，营造友好、包容的社区环境。

## 如何贡献

### 1. 报告 Bug

如果你发现了 bug，请：

1. 搜索 [Issues](https://github.com/meinanzilinzhengying/cloudflow/issues) 确认是否已有人报告
2. 如果没有，创建一个新的 Issue，包含：
   - 清晰的标题和描述
   - 复现步骤
   - 预期行为和实际行为
   - 环境信息（OS、Go 版本、eBPF 版本等）
   - 相关日志或截图

### 2. 提出新功能

在实现新功能之前：

1. 先创建 Issue 讨论功能的必要性和设计方案
2. 等待维护者的反馈和批准
3. 避免重复工作或方向偏差

### 3. 改进文档

文档改进随时欢迎！包括：

- README 更新
- API 文档补充
- 示例代码添加
- 翻译工作

## 开发环境设置

### 前置要求

- Go 1.22+
- Docker & Docker Compose
- Linux 内核 5.8+（用于 eBPF）
- clang/llvm（编译 eBPF 程序）

### 快速开始

```bash
# 克隆仓库
git clone https://github.com/meinanzilinzhengying/cloudflow.git
cd cloudflow

# 初始化子模块（如果有）
git submodule update --init --recursive

# 安装依赖
go mod download

# 运行测试
make test

# 本地启动开发环境
docker-compose up -d
```

## 提交代码流程

### 1. Fork 仓库

点击 GitHub 页面右上角的 "Fork" 按钮

### 2. 创建分支

```bash
git checkout -b feature/your-feature-name
# 或
git checkout -b fix/issue-description
```

分支命名规范：
- `feature/xxx` - 新功能
- `fix/xxx` - Bug 修复
- `docs/xxx` - 文档改进
- `refactor/xxx` - 代码重构
- `test/xxx` - 测试相关

### 3. 编写代码

遵循项目的代码规范和最佳实践

### 4. 运行测试

```bash
# 运行所有测试
make test

# 运行特定模块测试
go test ./internal/alert/... -v

# 检查代码格式
gofmt -s -w .

# 运行 linter
golangci-lint run
```

### 5. 提交更改

```bash
git add .
git commit -m "feat: add webhook HMAC signature support"
```

### 6. 推送到你的 Fork

```bash
git push origin feature/your-feature-name
```

### 7. 创建 Pull Request

在 GitHub 上创建从你的分支到主仓库 `main` 分支的 PR

## 代码规范

### Go 代码风格

- 遵循 [Effective Go](https://go.dev/doc/effective_go)
- 使用 `gofmt` 格式化代码
- 变量命名使用 camelCase
- 公开 API 使用 PascalCase
- 错误处理：不要忽略 error，使用有意义的错误消息

### 注释规范

- 所有公开函数、类型必须有文档注释
- 复杂逻辑需要添加行内注释说明
- 使用中文或英文均可，但保持文件内一致

### 示例

```go
// NewWebhookNotifier 创建 Webhook 通知器
// config: Webhook 配置
// log: 日志实例
func NewWebhookNotifier(config WebhookConfig, log *logger.Logger) *WebhookNotifier {
    // 实现...
}
```

## 测试要求

### 单元测试

- **新增代码必须有对应的单元测试**
- 测试覆盖率目标：核心模块 ≥ 70%
- 使用 `testify` 断言库

```go
func TestWebhookNotifierSign(t *testing.T) {
    // Given
    notifier := NewWebhookNotifier(config, log)
    
    // When
    signature := notifier.sign(event)
    
    // Then
    assert.NotEmpty(t, signature)
    assert.Equal(t, 64, len(signature))
}
```

### 集成测试

- 涉及外部依赖（数据库、Kafka 等）的功能需要集成测试
- 使用 Docker Compose 启动测试环境

### 运行测试

```bash
# 运行所有测试
make test

# 带覆盖率报告
make test-coverage

# 查看覆盖率 HTML
go tool cover -html=coverage.out
```

## Commit 规范

我们使用 [Conventional Commits](https://www.conventionalcommits.org/) 规范：

### 格式

```
<type>(<scope>): <description>

[optional body]

[optional footer(s)]
```

### Type 类型

- `feat`: 新功能
- `fix`: Bug 修复
- `docs`: 文档变更
- `style`: 代码格式（不影响功能）
- `refactor`: 重构
- `test`: 测试相关
- `chore`: 构建过程或辅助工具变动
- `perf`: 性能优化

### 示例

```bash
# 好的 commit message
feat(alert): add HMAC-SHA256 signature for webhooks
fix(dashboard): prevent panic on invalid type assertion
docs(readme): add deployment guide for Kubernetes
test(circuitbreaker): add unit tests for state transitions

# 不好的 commit message
update code
fix bug
changes
```

### Scope（可选）

指明影响的模块：
- `agent`: cloud-flow-agent
- `edge`: cloud-flow-edge
- `center`: cloud-flow-center
- `dashboard`: dashboard 相关
- `alert`: 告警模块
- `ebpf`: eBPF 采集器

## Issue 报告

### Bug Report 模板

```markdown
**Describe the bug**
清晰简洁地描述 bug

**To Reproduce**
复现步骤：
1. ...
2. ...
3. ...

**Expected behavior**
期望的行为

**Screenshots**
如果适用，添加截图

**Environment:**
- OS: [e.g. Ubuntu 22.04]
- Go Version: [e.g. 1.22]
- Kernel: [e.g. 5.15.0]
- CloudFlow Version: [e.g. v0.1.0]

**Additional context**
其他上下文
```

### Feature Request 模板

```markdown
**Is your feature request related to a problem?**
描述问题，例如："I'm always frustrated when..."

**Describe the solution you'd like**
描述期望的解决方案

**Describe alternatives you've considered**
描述考虑过的替代方案

**Additional context**
其他上下文或截图
```

## Pull Request 流程

### PR 检查清单

创建 PR 前，请确认：

- [ ] 代码遵循项目规范
- [ ] 添加了必要的测试
- [ ] 所有测试通过 (`make test`)
- [ ] 代码已通过 linter (`golangci-lint run`)
- [ ] 更新了相关文档
- [ ] Commit message 符合规范
- [ ] PR 描述清晰，说明改动内容和原因

### PR 描述模板

```markdown
## What this PR does / why we need it

简要说明 PR 的目的和背景

## Which issue(s) this PR fixes

Fixes #<issue number>

## Special notes for your reviewer

给审查者的特别说明

## Types of changes

- [ ] Bug fix (non-breaking change which fixes an issue)
- [ ] New feature (non-breaking change which adds functionality)
- [ ] Breaking change (fix or feature that would cause existing functionality to not work as expected)
- [ ] Documentation update

## Testing

描述如何测试这些更改：

```bash
# 测试命令
go test ./...
```

## Checklist

- [ ] Code follows project style guidelines
- [ ] Tests have been added/updated
- [ ] Documentation has been updated
- [ ] All tests pass locally
```

### Review 流程

1. **自动检查**：CI 会自动运行测试和 lint
2. **代码审查**：至少需要 1 名维护者批准
3. **修改建议**：根据 review 意见进行修改
4. **合并**：审查通过后由维护者合并

## 常见问题

### Q: 我的 PR 为什么被关闭？

A: 可能原因：
- 长时间无响应
- 与项目方向不符
- 已有类似实现
- 需要重大重构

### Q: 如何成为维护者？

A: 持续贡献高质量的代码和 review，积极参与社区讨论

### Q: 可以提小的拼写错误修复吗？

A: 可以！欢迎任何改进，无论大小

## 联系方式

- GitHub Issues: https://github.com/meinanzilinzhengying/cloudflow/issues
- Email: cloudflow@meinanzilinzhengying.com

## 致谢

感谢所有为 CloudFlow 做出贡献的开发者！🎉

---

**Happy Coding!** 🚀
