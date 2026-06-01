.PHONY: all build run tidy vet test clean docker

APP_NAME := edge-node
CMD_DIR := ./cmd/main.go
GOFLAGS := -ldflags="-s -w"

all: vet build

## build: 编译二进制
build:
	CGO_ENABLED=0 go build $(GOFLAGS) -o bin/$(APP_NAME) $(CMD_DIR)

## run: 本地运行
run: build
	./bin/$(APP_NAME)

## tidy: 整理依赖
tidy:
	go mod tidy

## vet: 静态检查
vet:
	go vet ./...

## test: 运行单元测试
test:
	go test -v -race -count=1 ./...

## test-cover: 带覆盖率的测试
test-cover:
	go test -v -race -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "覆盖率报告: coverage.html"

## docker: 构建 Docker 镜像
docker:
	docker build -t $(APP_NAME):latest -f deployments/Dockerfile .

## clean: 清理构建产物
clean:
	rm -rf bin/ coverage.out coverage.html

## help: 显示帮助
help:
	@echo "可用命令:"
	@echo "  make build       编译二进制到 bin/"
	@echo "  make run         编译并运行"
	@echo "  make tidy        整理 go.mod"
	@echo "  make vet         静态检查"
	@echo "  make test        运行单元测试"
	@echo "  make test-cover  带覆盖率测试"
	@echo "  make docker      构建 Docker 镜像"
	@echo "  make clean       清理构建产物"
