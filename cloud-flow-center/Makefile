.PHONY: all build tidy vet test clean migrate-up migrate-down migrate-create

APP_NAME := cloud-flow-center
CMD_DIR := ./cmd/main.go
GOFLAGS := -ldflags="-s -w"

# 数据库迁移配置
MIGRATE_TOOL := migrate
MIGRATE_DIR := ./deployments/migrations
MIGRATE_DSN := "mysql://root:@tcp(127.0.0.1:4000)/cloud_flow?parseTime=true&charset=utf8mb4&collation=utf8mb4_bin"

all: vet build

build:
	CGO_ENABLED=0 go build $(GOFLAGS) -o bin/$(APP_NAME) $(CMD_DIR)

tidy:
	go mod tidy

vet:
	go vet ./...

test:
	go test -v -race ./...

clean:
	rm -rf bin/

# ---------------------------------------------------------------------------
# 数据库迁移 (使用 golang-migrate/migrate)
# 安装: go install -tags 'mysql' github.com/golang-migrate/migrate/v4/cmd/migrate@latest
# ---------------------------------------------------------------------------

# 执行所有待执行的迁移 (up)
migrate-up:
	$(MIGRATE_TOOL) -path $(MIGRATE_DIR) -database $(MIGRATE_DSN) up

# 回滚最近一次迁移 (down 1)
migrate-down:
	$(MIGRATE_TOOL) -path $(MIGRATE_DIR) -database $(MIGRATE_DSN) down 1

# 创建新的迁移文件对 (用法: make migrate-create NAME=init_xxx)
migrate-create:
	@if [ -z "$(NAME)" ]; then \
		echo "Usage: make migrate-create NAME=<migration_name>"; \
		exit 1; \
	fi; \
	NEXT=$$(ls $(MIGRATE_DIR)/*.up.sql 2>/dev/null | wc -l | awk '{printf "%03d", $$1+1}'); \
	UP_FILE=$(MIGRATE_DIR)/$${NEXT}_$(NAME).up.sql; \
	DOWN_FILE=$(MIGRATE_DIR)/$${NEXT}_$(NAME).down.sql; \
	touch "$$UP_FILE" "$$DOWN_FILE"; \
	echo "Created: $$UP_FILE"; \
	echo "Created: $$DOWN_FILE"
