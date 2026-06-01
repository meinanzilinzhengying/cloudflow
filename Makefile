.PHONY: help build test lint docker up down prod-up prod-down prod-check prod-health

# 帮助菜单
help: ## 显示帮助信息
	@echo "CloudFlow 开发工具"
	@echo "======================"
	@echo ""
	@echo "开发环境命令:"
	@grep -E '^[a-z-]+:.*?## .*$$' $(MAKEFILE_LIST) | grep -v "^prod-" | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2}'
	@echo ""
	@echo "生产环境命令:"
	@grep -E '^prod-[a-z-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[35m%-20s\033[0m %s\n", $$1, $$2}'

# 构建所有 Go 服务
build: ## 构建所有 Go 服务
	@echo "Building all services..."
	@for dir in cloud-flow-* services/*; do \
		if [ -f "$$dir/go.mod" ]; then \
			echo "Building $$dir..."; \
			cd $$dir && go build -v ./... && cd - > /dev/null; \
		fi \
	done

# 运行所有测试
test: ## 运行所有 Go 测试
	@echo "Running all tests..."
	@for dir in cloud-flow-* services/*; do \
		if [ -f "$$dir/go.mod" ]; then \
			echo "Testing $$dir..."; \
			cd $$dir && go test -v ./... && cd - > /dev/null; \
		fi \
	done

# 运行 lint
lint: ## 运行 golangci-lint
	@echo "Running lint..."
	@which golangci-lint > /dev/null || (echo "请先安装 golangci-lint" && exit 1)
	@for dir in cloud-flow-* services/*; do \
		if [ -f "$$dir/go.mod" ]; then \
			echo "Linting $$dir..."; \
			cd $$dir && golangci-lint run && cd - > /dev/null; \
		fi \
	done

# Docker 构建
docker-build: ## 构建所有 Docker 镜像
	@echo "Building Docker images..."
	docker buildx bake -f docker-bake.hcl

# Docker Compose 开发环境
up: ## 启动开发环境 (docker-compose)
	@echo "Starting development environment..."
	docker-compose up -d

down: ## 停止开发环境
	@echo "Stopping development environment..."
	docker-compose down

logs: ## 查看服务日志
	@echo "Showing logs..."
	docker-compose logs -f

# 清理构建产物
clean: ## 清理构建产物
	@echo "Cleaning up..."
	@for dir in cloud-flow-* services/*; do \
		if [ -f "$$dir/go.mod" ]; then \
			rm -f $$dir/cloud-flow-* $$dir/*.test $$dir/coverage.out; \
		fi \
	done

# 检查 Go 工作区
work-sync: ## 同步 Go 工作区
	@echo "Syncing Go workspace..."
	go work sync
	@for dir in cloud-flow-* services/* pkg proto; do \
		if [ -f "$$dir/go.mod" ]; then \
			cd $$dir && go mod tidy && cd - > /dev/null; \
		fi \
	done

# 生产环境命令
prod-up: ## 启动生产环境
	@echo "Starting production environment..."
	@[ -f .env ] || (echo "错误：.env 文件不存在，请先复制 .env.example" && exit 1)
	docker-compose -f docker-compose.prod.yml up -d

prod-down: ## 停止生产环境
	@echo "Stopping production environment..."
	docker-compose -f docker-compose.prod.yml down

prod-check: ## 运行生产环境部署检查清单
	@echo "Running production checklist..."
	./scripts/prod-checklist.sh

prod-health: ## 运行生产环境健康检查
	@echo "Running health check..."
	./scripts/health-check.sh

prod-logs: ## 查看生产环境服务日志
	@echo "Showing production logs..."
	docker-compose -f docker-compose.prod.yml logs -f

