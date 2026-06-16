# CloudFlow 分步部署教程

## 目录
1. [环境准备](#环境准备)
2. [单机部署](#单机部署)
3. [集群部署](#集群部署)
4. [国产数据库配置](#国产数据库配置)
5. [eBPF Agent部署](#ebpf-agent部署)
6. [功能验证](#功能验证)
7. [常见问题排查](#常见问题排查)

---

## 环境准备

### 系统要求

| 组件 | 最低要求 | 推荐配置 |
|------|---------|---------|
| 操作系统 | CentOS 7.9 / Ubuntu 20.04 | CentOS 8 / Ubuntu 22.04 / 麒麟V10 |
| 内核版本 | >= 5.4 | >= 5.10 |
| CPU | 4核 | 8核+ |
| 内存 | 8GB | 16GB+ |
| 磁盘 | 100GB | 500GB SSD |

### 依赖安装

#### Go 1.22
```bash
# 下载安装
wget https://go.dev/dl/go1.22.0.linux-amd64.tar.gz
tar -C /usr/local -xzf go1.22.0.linux-amd64.tar.gz

# 配置环境变量
echo 'export PATH=$PATH:/usr/local/go/bin' >> /etc/profile
source /etc/profile

# 验证
go version
```

#### Docker & Docker Compose
```bash
# 安装Docker
curl -fsSL https://get.docker.com | bash
systemctl enable --now docker

# 安装Docker Compose
curl -L "https://github.com/docker/compose/releases/latest/download/docker-compose-$(uname -s)-$(uname -m)" \
  -o /usr/local/bin/docker-compose
chmod +x /usr/local/bin/docker-compose

# 验证
docker --version
docker-compose --version
```

#### Clang & LLVM（eBPF编译）
```bash
# CentOS/RHEL
yum install -y clang llvm libbpf-devel

# Ubuntu/Debian
apt install -y clang llvm libbpf-dev

# 麒麟/UOS
apt install -y clang llvm libbpf-dev

# 验证
clang --version
```

---

## 单机部署

### 步骤1：克隆代码
```bash
git clone https://github.com/meinanzilinzhengying/cloudflow.git
cd cloudflow
```

### 步骤2：配置环境变量
```bash
# 创建环境变量文件
cat > .env << 'EOF'
# JWT密钥（必须≥32位）
CLOUD_FLOW_JWT_SECRET=your-32-character-secret-key-here

# MySQL配置
MYSQL_ROOT_PASSWORD=your-mysql-password
MYSQL_DATABASE=cloudflow

# ClickHouse配置
CLICKHOUSE_PASSWORD=your-clickhouse-password

# Redis配置
REDIS_PASSWORD=your-redis-password

# CORS白名单
CORS_ALLOWED_ORIGINS=http://localhost:3000,http://localhost:8080
