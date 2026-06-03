.PHONY: help build test lint clean docker-build docker-push install uninstall

# 变量定义
BINARY_NAME=cloud-flow-agent
VERSION=$(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
BUILD_TIME=$(shell date -u '+%Y-%m-%dT%H:%M:%SZ')
GIT_COMMIT=$(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")

# Go 参数
GO=go
GOFLAGS=-ldflags="-s -w -X main.Version=$(VERSION) -X main.BuildTime=$(BUILD_TIME) -X main.GitCommit=$(GIT_COMMIT)"
GOCMD=CGO_ENABLED=1 $(GO)

# Docker 参数
DOCKER_REGISTRY=docker.io
DOCKER_IMAGE=meinanzilinzhengying/cloudflow-agent
DOCKER_TAG=$(VERSION)

# 默认目标
help: ## 显示帮助信息
	@echo "CloudFlow Makefile"
	@echo ""
	@echo "Usage:"
	@echo "  make <target>"
	@echo ""
	@echo "Targets:"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2}'

# 构建
build: ## 构建二进制文件
	@echo "Building $(BINARY_NAME)..."
	cd cloud-flow-agent && $(GOCMD) build $(GOFLAGS) -o ../bin/$(BINARY_NAME) ./cmd/main.go
	@echo "Build complete: bin/$(BINARY_NAME)"
	@ls -lh bin/$(BINARY_NAME)

build-all: ## 构建所有组件
	@echo "Building all components..."
	make build-agent
	make build-edge
	make build-center

build-agent: ## 构建 Agent
	@echo "Building Agent..."
	cd cloud-flow-agent && $(GOCMD) build $(GOFLAGS) -o ../bin/cloud-flow-agent ./cmd/main.go

build-edge: ## 构建 Edge
	@echo "Building Edge..."
	cd cloud-flow-edge && $(GOCMD) build $(GOFLAGS) -o ../bin/cloud-flow-edge ./cmd/main.go

build-center: ## 构建 Center
	@echo "Building Center..."
	cd cloud-flow-center && $(GOCMD) build $(GOFLAGS) -o ../bin/cloud-flow-center ./cmd/main.go

# 测试
test: ## 运行所有测试
	@echo "Running tests..."
	cd cloud-flow-agent && $(GO) test -v -race -coverprofile=coverage.out ./...
	@echo ""
	@echo "Test coverage:"
	cd cloud-flow-agent && $(GO) tool cover -func=coverage.out | grep total

test-verbose: ## 运行详细测试
	@echo "Running verbose tests..."
	cd cloud-flow-agent && $(GO) test -v -race ./...

test-coverage: ## 生成覆盖率报告
	@echo "Generating coverage report..."
	cd cloud-flow-agent && $(GO) test -coverprofile=coverage.out ./...
	cd cloud-flow-agent && $(GO) tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: cloud-flow-agent/coverage.html"
	@open cloud-flow-agent/coverage.html 2>/dev/null || true

test-alert: ## 运行 alert 模块测试
	@echo "Testing alert module..."
	cd cloud-flow-agent && $(GO) test -v ./internal/alert/...

test-circuitbreaker: ## 运行 circuitbreaker 模块测试
	@echo "Testing circuitbreaker module..."
	cd cloud-flow-agent && $(GO) test -v ./internal/circuitbreaker/...

test-reliable: ## 运行 reliable 模块测试
	@echo "Testing reliable module..."
	cd cloud-flow-agent && $(GO) test -v ./internal/reliable/...

# 代码质量
lint: ## 运行代码检查
	@echo "Running linter..."
	@if command -v golangci-lint >/dev/null 2>&1; then \
		cd cloud-flow-agent && golangci-lint run ./...; \
	else \
		echo "golangci-lint not installed. Install with: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest"; \
	fi

fmt: ## 格式化代码
	@echo "Formatting code..."
	find . -name "*.go" -not -path "./vendor/*" -not -path "./.git/*" | xargs gofmt -s -w

vet: ## 运行 go vet
	@echo "Running go vet..."
	cd cloud-flow-agent && $(GO) vet ./...

security-scan: ## 运行安全扫描
	@echo "Running security scan..."
	@if command -v gosec >/dev/null 2>&1; then \
		cd cloud-flow-agent && gosec ./...; \
	else \
		echo "gosec not installed. Install with: go install github.com/securego/gosec/v2/cmd/gosec@latest"; \
	fi

# 清理
clean: ## 清理构建产物
	@echo "Cleaning..."
	rm -rf bin/
	rm -f cloud-flow-agent/coverage.out
	rm -f cloud-flow-agent/coverage.html
	find . -name "*.test" -delete
	@echo "Clean complete"

# Docker
docker-build: ## 构建 Docker 镜像
	@echo "Building Docker image..."
	docker build -t $(DOCKER_IMAGE):$(DOCKER_TAG) -f cloud-flow-agent/deployments/Dockerfile .
	@echo "Docker image built: $(DOCKER_IMAGE):$(DOCKER_TAG)"

docker-build-multiarch: ## 构建多架构 Docker 镜像
	@echo "Building multi-architecture Docker images..."
	docker buildx build --platform linux/amd64,linux/arm64 \
		-t $(DOCKER_IMAGE):$(DOCKER_TAG) \
		-f cloud-flow-agent/deployments/Dockerfile \
		--push .

docker-push: ## 推送 Docker 镜像
	@echo "Pushing Docker image..."
	docker push $(DOCKER_IMAGE):$(DOCKER_TAG)
	@echo "Docker image pushed: $(DOCKER_IMAGE):$(DOCKER_TAG)"

docker-run: ## 运行 Docker 容器（开发模式）
	@echo "Running Docker container..."
	docker run -it --rm \
		--privileged \
		--network host \
		-v /var/run:/var/run \
		-v /sys/kernel/debug:/sys/kernel/debug \
		$(DOCKER_IMAGE):$(DOCKER_TAG)

# 安装
install: ## 安装到系统
	@echo "Installing $(BINARY_NAME)..."
	sudo cp bin/$(BINARY_NAME) /usr/local/bin/
	sudo chmod +x /usr/local/bin/$(BINARY_NAME)
	@echo "Installed to /usr/local/bin/$(BINARY_NAME)"

uninstall: ## 从系统卸载
	@echo "Uninstalling $(BINARY_NAME)..."
	sudo rm -f /usr/local/bin/$(BINARY_NAME)
	@echo "Uninstalled"

# 开发环境
dev-up: ## 启动开发环境（Docker Compose）
	@echo "Starting development environment..."
	docker-compose up -d
	@echo "Development environment started"
	@echo "Access services:"
	@echo "  - Grafana: http://localhost:3000"
	@echo "  - Prometheus: http://localhost:9090"
	@echo "  - Jaeger: http://localhost:16686"

dev-down: ## 停止开发环境
	@echo "Stopping development environment..."
	docker-compose down
	@echo "Development environment stopped"

dev-logs: ## 查看开发环境日志
	@echo "Showing logs..."
	docker-compose logs -f

dev-reset: ## 重置开发环境
	@echo "Resetting development environment..."
	docker-compose down -v
	docker-compose up -d
	@echo "Development environment reset"

# 部署
deploy-dev: ## 部署到开发环境
	@echo "Deploying to development environment..."
	make build
	docker-compose up -d --build
	@echo "Deployed to development environment"

deploy-prod: ## 部署到生产环境（需要配置）
	@echo "Deploying to production environment..."
	@echo "WARNING: This will deploy to production!"
	@read -p "Are you sure? (y/N) " confirm && [ "$$confirm" = "y" ] || exit 1
	docker-compose -f docker-compose.prod.yml up -d --build
	@echo "Deployed to production environment"

# 文档
docs: ## 生成文档
	@echo "Generating documentation..."
	@if command -v godoc >/dev/null 2>&1; then \
		godoc -http=:6060 & \
		echo "Documentation server started at http://localhost:6060"; \
	else \
		echo "godoc not available. Use: go install golang.org/x/tools/cmd/godoc@latest"; \
	fi

# 版本管理
version: ## 显示版本信息
	@echo "Version: $(VERSION)"
	@echo "Build Time: $(BUILD_TIME)"
	@echo "Git Commit: $(GIT_COMMIT)"

tag: ## 创建新版本标签
	@read -p "Enter version (e.g., v0.2.0): " version; \
	git tag -a $$version -m "Release $$version"; \
	git push origin $$version; \
	echo "Tag $$version created and pushed"

# CI/CD
ci-test: ## CI 测试流程
	@echo "Running CI tests..."
	make fmt
	make vet
	make lint
	make test
	make security-scan
	@echo "CI tests passed"

ci-build: ## CI 构建流程
	@echo "Running CI build..."
	make build-all
	make docker-build
	@echo "CI build complete"

# 工具
check-deps: ## 检查依赖
	@echo "Checking dependencies..."
	@echo "Go version: $$(go version)"
	@echo "Docker version: $$(docker --version 2>/dev/null || echo 'not installed')"
	@echo "Docker Compose: $$(docker-compose --version 2>/dev/null || echo 'not installed')"
	@echo "golangci-lint: $$(golangci-lint --version 2>/dev/null || echo 'not installed')"
	@echo "gosec: $$(gosec --version 2>/dev/null || echo 'not installed')"

install-deps: ## 安装开发依赖
	@echo "Installing development dependencies..."
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	go install github.com/securego/gosec/v2/cmd/gosec@latest
	go install golang.org/x/tools/cmd/godoc@latest
	@echo "Dependencies installed"

# 快捷方式
run: build ## 构建并运行
	@echo "Running $(BINARY_NAME)..."
	./bin/$(BINARY_NAME)

debug: build ## 构建并以调试模式运行
	@echo "Running in debug mode..."
	dlv debug cloud-flow-agent/cmd/main.go

all: clean build test lint ## 完整流程：清理、构建、测试、lint
	@echo "All tasks completed successfully"
