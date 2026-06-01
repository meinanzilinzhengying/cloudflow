// =============================================================================
// Docker Buildx Bake 配置 - 云内流量监测系统
// 用途: 多架构构建、缓存优化、批量镜像构建
// 使用方式:
//   docker buildx bake                    # 构建所有镜像
//   docker buildx bake frontend           # 仅构建前端
//   docker buildx bake agent              # 仅构建 Agent
//   docker buildx bake --push             # 构建并推送
// =============================================================================

// 变量定义
variable "REGISTRY" {
  default = "ghcr.io"
}

variable "NAMESPACE" {
  default = "cloud-flow"
}

variable "TAG" {
  default = "latest"
}

variable "PLATFORMS" {
  default = "linux/amd64,linux/arm64"
}

// 通用构建配置
group "default" {
  targets = ["frontend", "agent"]
}

// 前端镜像构建
target "frontend" {
  context = "./cloud-flow-frontend"
  dockerfile = "deployments/Dockerfile"
  tags = [
    "${REGISTRY}/${NAMESPACE}/frontend:${TAG}",
    "${REGISTRY}/${NAMESPACE}/frontend:latest",
  ]
  platforms = split(",", PLATFORMS)
  
  // BuildKit 缓存配置
  cache-from = [
    "type=gha,scope=frontend",
    "type=registry,ref=${REGISTRY}/${NAMESPACE}/frontend:cache",
  ]
  cache-to = [
    "type=gha,scope=frontend,mode=max",
    "type=registry,ref=${REGISTRY}/${NAMESPACE}/frontend:cache,mode=max",
  ]
  
  // 构建参数
  args = {
    VITE_API_BASE_URL = "/api"
  }
  
  // 标签
  labels = {
    "org.opencontainers.image.title" = "Cloud Flow Frontend"
    "org.opencontainers.image.description" = "云内流量监测系统前端"
    "org.opencontainers.image.source" = "https://github.com/cloud-flow/frontend"
  }
}

// Agent 镜像构建
target "agent" {
  context = "./cloud-flow-agent"
  dockerfile = "deployments/Dockerfile"
  tags = [
    "${REGISTRY}/${NAMESPACE}/agent:${TAG}",
    "${REGISTRY}/${NAMESPACE}/agent:latest",
  ]
  platforms = split(",", PLATFORMS)
  
  // BuildKit 缓存配置
  cache-from = [
    "type=gha,scope=agent",
    "type=registry,ref=${REGISTRY}/${NAMESPACE}/agent:cache",
  ]
  cache-to = [
    "type=gha,scope=agent,mode=max",
    "type=registry,ref=${REGISTRY}/${NAMESPACE}/agent:cache,mode=max",
  ]
  
  // 构建参数
  args = {
    CGO_ENABLED = "0"
  }
  
  // 标签
  labels = {
    "org.opencontainers.image.title" = "Cloud Flow Agent"
    "org.opencontainers.image.description" = "云内流量监测系统探针"
    "org.opencontainers.image.source" = "https://github.com/cloud-flow/agent"
  }
}

// 开发模式构建（单架构，快速迭代）
group "dev" {
  targets = ["frontend-dev", "agent-dev"]
}

target "frontend-dev" {
  inherits = ["frontend"]
  platforms = ["linux/amd64"]
  tags = ["${REGISTRY}/${NAMESPACE}/frontend:dev"]
  cache-to = []
}

target "agent-dev" {
  inherits = ["agent"]
  platforms = ["linux/amd64"]
  tags = ["${REGISTRY}/${NAMESPACE}/agent:dev"]
  cache-to = []
}

// 生产模式构建（多架构，优化体积）
group "prod" {
  targets = ["frontend-prod", "agent-prod"]
}

target "frontend-prod" {
  inherits = ["frontend"]
  args = {
    NODE_ENV = "production"
  }
}

target "agent-prod" {
  inherits = ["agent"]
  args = {
    CGO_ENABLED = "0"
    LDFLAGS = "-s -w -extldflags '-static'"
  }
}
