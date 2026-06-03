# CloudFlow 安全配置指南

## 🔐 密钥管理

### ⚠️ 严重安全警告

**请勿在配置文件中硬编码密钥！**

在之前的版本中，我们发现以下硬编码密钥问题并已修复：

#### 已修复的问题

1. **cloud-flow-center/configs/config.yaml**
   - ❌ 旧: `api_key: ef1a6aca1e43ff5d1c4e85845281153f`
   - ✅ 新: `api_key: "${CLOUDFLOW_CENTER_API_KEY}"`
   
2. **JWT Secret**
   - ❌ 旧: `secret_key: test-secret-1234567890123456` (仅32字节)
   - ✅ 新: `secret_key: "${CLOUDFLOW_JWT_SECRET_KEY}"` (至少64字节)

---

## 🔑 密钥生成工具

项目提供了密钥生成工具：

```bash
# 生成所有密钥
bash scripts/generate-secrets.sh all

# 仅生成 API Key
bash scripts/generate-secrets.sh api-key

# 仅生成 JWT Secret
bash scripts/generate-secrets.sh jwt-secret

# 生成 Kubernetes Secret YAML
bash scripts/generate-secrets.sh k8s

# 生成 Docker Secret 命令
bash scripts/generate-secrets.sh docker
```

---

##  生产环境配置方法

### 方法 1: 环境变量（推荐）

```bash
# 1. 生成密钥
bash scripts/generate-secrets.sh all

# 2. 设置环境变量
export CLOUDFLOW_CENTER_API_KEY="$(openssl rand -hex 32)"
export CLOUDFLOW_JWT_SECRET_KEY="$(openssl rand -base64 64)"

# 3. 启动服务
./cloud-flow-center
```

**持久化配置**（添加到 `/etc/environment` 或 `~/.bashrc`）:

```bash
echo 'export CLOUDFLOW_CENTER_API_KEY="your-key-here"' >> /etc/environment
echo 'export CLOUDFLOW_JWT_SECRET_KEY="your-secret-here"' >> /etc/environment
```

### 方法 2: Kubernetes Secrets

```bash
# 1. 生成 K8s Secret YAML
bash scripts/generate-secrets.sh k8s > cloudflow-secrets.yaml

# 2. 应用到集群
kubectl apply -f cloudflow-secrets.yaml

# 3. 在 Deployment 中引用
# 参见 deploy/kubernetes/deployment.yaml
```

**示例 Deployment 配置**:

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: cloud-flow-center
spec:
  template:
    spec:
      containers:
      - name: center
        env:
        - name: CLOUDFLOW_CENTER_API_KEY
          valueFrom:
            secretKeyRef:
              name: cloudflow-secrets
              key: api-key
        - name: CLOUDFLOW_JWT_SECRET_KEY
          valueFrom:
            secretKeyRef:
              name: cloudflow-secrets
              key: jwt-secret
```

### 方法 3: Docker Secrets

```bash
# 1. 创建 Docker Secret
bash scripts/generate-secrets.sh docker

# 2. 执行输出的命令
docker secret create cloudflow-api-key <<< 'your-api-key'

# 3. 在 docker-compose.yml 中使用
# 参见 docker-compose.prod.yml
```

**示例 docker-compose 配置**:

```yaml
services:
  center:
    image: cloudflow/center:latest
    secrets:
      - cloudflow-api-key
    environment:
      - CLOUDFLOW_CENTER_API_KEY_FILE=/run/secrets/cloudflow-api-key
    volumes:
      - /run/secrets:/run/secrets:ro

secrets:
  cloudflow-api-key:
    external: true
```

### 方法 4: HashiCorp Vault

```bash
# 1. 存储密钥到 Vault
vault kv put secret/cloudflow \
  api-key=$(openssl rand -hex 32) \
  jwt-secret=$(openssl rand -base64 64)

# 2. 应用启动时读取
export CLOUDFLOW_CENTER_API_KEY=$(vault kv get -field=api-key secret/cloudflow)
export CLOUDFLOW_JWT_SECRET_KEY=$(vault kv get -field=jwt-secret secret/cloudflow)

# 3. 启动服务
./cloud-flow-center
```

**Vault Agent 自动注入**:

```yaml
# 在 Kubernetes Pod 注解中配置
annotations:
  vault.hashicorp.com/agent-inject: "true"
  vault.hashicorp.com/agent-inject-secret-config: "secret/data/cloudflow"
  vault.hashicorp.com/agent-inject-template-config: |
    {{- with secret "secret/data/cloudflow" -}}
    export CLOUDFLOW_CENTER_API_KEY="{{ .Data.data.api-key }}"
    export CLOUDFLOW_JWT_SECRET_KEY="{{ .Data.data.jwt-secret }}"
    {{- end -}}
```

---

##  密钥要求

| 密钥类型 | 最小长度 | 生成命令 | 用途 | 轮换周期 |
|---------|---------|----------|------|---------|
| API Key | 32 字节 (64 hex字符) | `openssl rand -hex 32` | 服务间认证 | 90天 |
| JWT Secret | 64 字节 (base64) | `openssl rand -base64 64` | JWT 令牌签名 | 90天 |
| Webhook Secret | 32 字节 (64 hex字符) | `openssl rand -hex 32` | Webhook HMAC 签名 | 90天 |

---

## 🔄 密钥轮换

### 手动轮换

```bash
# 1. 生成新密钥
bash scripts/generate-secrets.sh all

# 输出示例:
#  生成 API Key (32字节 hex):
#    a1b2c3d4e5f6...
# 
# 🔐 生成 JWT Secret (64字节 base64):
#    xYzAbC123...

# 2. 更新环境变量
export CLOUDFLOW_CENTER_API_KEY="新密钥"
export CLOUDFLOW_JWT_SECRET_KEY="新密钥"

# 3. 重启服务（无停机轮换）
# Kubernetes:
kubectl rollout restart deployment/cloud-flow-center

# Docker:
docker-compose restart center

# Systemd:
systemctl restart cloud-flow-center

# 4. 验证服务正常运行
curl http://localhost:8080/health
```

### 自动化轮换（Kubernetes）

使用 **Reloader** 或 **Secrets Store CSI Driver** 实现自动轮换：

```yaml
# 使用 Reloader
annotations:
  reloader.stakater.com/auto: "true"

# 使用 Secrets Store CSI Driver
volumes:
  - name: secrets-store-inline
    csi:
      driver: secrets-store.csi.k8s.io
      readOnly: true
      volumeAttributes:
        secretProviderClass: "cloudflow-vault"
```

---

## ️ 配置文件安全

### 权限设置

```bash
# 配置文件权限应设置为 600 (仅所有者可读写)
chmod 600 .env
chmod 600 configs/config.yaml
chmod 600 scripts/*.sh

# 检查权限
ls -l .env configs/config.yaml
```

### Git 安全

```bash
# 确保敏感文件已添加到 .gitignore
cat .gitignore | grep -E "\.env|config\.yaml|secrets"

# 预期输出:
# .env
# .env.local
# configs/*config.yaml
# **/secrets/
```

### 文件模板

项目提供了配置文件模板：

- `.env.example` - 环境变量模板
- `configs/config.yaml` - 配置模板（使用环境变量占位符）

**使用方法**:

```bash
# 1. 复制模板
cp .env.example .env

# 2. 编辑 .env 文件，填入实际值
vim .env

# 3. 确保 .env 不会提交到 Git
git check-ignore .env  # 应该输出 .env
```

---

## 🔍 安全审计清单

### 配置审计

- [x] 移除所有硬编码密钥
- [x] 实现环境变量加载机制
- [x] 创建密钥生成工具
- [x] 添加密钥轮换文档
- [x] 配置 .gitignore 排除敏感文件
- [ ] 集成密钥管理服务 (Vault)
- [ ] 自动化密钥轮换
- [ ] 定期安全扫描

### 运行时审计

```bash
# 1. 检查环境变量是否设置
env | grep CLOUDFLOW

# 2. 检查配置文件是否包含明文密钥
grep -r "api_key: [a-f0-9]" configs/ || echo "✅ 未发现硬编码密钥"

# 3. 检查文件权限
find . -name "*.yaml" -o -name ".env" | xargs ls -l

# 4. 检查 Git 历史是否泄露密钥
git log --all -p | grep -E "api_key:|secret_key:" || echo "✅ Git 历史中未发现密钥"
```

---

## 🚨 应急处理

### 密钥泄露处理流程

如果怀疑密钥已泄露：

```bash
# 1. 立即生成新密钥
bash scripts/generate-secrets.sh all

# 2. 更新所有服务的密钥
export CLOUDFLOW_CENTER_API_KEY="新密钥"
export CLOUDFLOW_JWT_SECRET_KEY="新密钥"

# 3. 重启所有服务
kubectl rollout restart deployment --all
# 或
docker-compose down && docker-compose up -d

# 4. 撤销旧密钥
# 如果是 JWT，需要清空 Redis 中的黑名单
redis-cli FLUSHDB

# 5. 审计日志
kubectl logs deployment/cloud-flow-center --since=1h | grep -i "unauthorized"

# 6. 通知团队
# 发送通知到安全团队
```

### 回滚步骤

如果新密钥导致问题：

```bash
# 1. 恢复旧密钥（如果尚未泄露）
export CLOUDFLOW_CENTER_API_KEY="旧密钥"

# 2. 回滚服务
kubectl rollout undo deployment/cloud-flow-center

# 3. 验证服务恢复
curl http://localhost:8080/health
```

---

## 📚 相关文档

- [SECURITY.md](./SECURITY.md) - 安全策略
- [.env.example](./.env.example) - 环境变量模板
- [scripts/generate-secrets.sh](./scripts/generate-secrets.sh) - 密钥生成工具
- [docker-compose.prod.yml](./docker-compose.prod.yml) - 生产环境配置

---

## ️ 常见问题

### Q: 如何验证密钥是否正确加载？

```bash
# 检查环境变量
echo $CLOUDFLOW_CENTER_API_KEY | wc -c  # 应该输出 65 (64字符 + 换行符)
echo $CLOUDFLOW_JWT_SECRET_KEY | wc -c  # 应该输出 >= 89 (88字符 + 换行符)

# 检查服务日志
kubectl logs deployment/cloud-flow-center | grep -i "api.key\|jwt"
```

### Q: 可以在开发环境使用硬编码密钥吗？

**不建议**。即使在开发环境，也应使用环境变量：

```bash
# .env.local (不提交到 Git)
CLOUDFLOW_CENTER_API_KEY=dev-key-not-for-production
CLOUDFLOW_JWT_SECRET_KEY=dev-secret-not-for-production
```

### Q: 密钥长度不够会怎样？

系统会拒绝启动并记录错误：

```
FATAL: JWT secret key is too short (minimum 64 bytes required)
FATAL: API key is too short (minimum 32 bytes required)
```

### Q: 如何测试密钥轮换？

```bash
# 1. 生成新密钥
bash scripts/generate-secrets.sh all

# 2. 在测试环境应用
export CLOUDFLOW_CENTER_API_KEY="测试密钥"

# 3. 运行集成测试
go test ./... -v -run TestAuth

# 4. 验证所有服务正常
curl http://localhost:8080/health
```

---

**最后更新**: 2024-01-XX  
**维护者**: CloudFlow Security Team
