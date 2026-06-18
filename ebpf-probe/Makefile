# CloudFlow eBPF Probe v3
# Makefile for build, package, and deploy

.PHONY: all build clean bpf install docker rpm deb

VERSION := 3.0.0
BINARY := cloudflow-ebpf-probe
CMD := ./cmd/probe
GOFLAGS := -ldflags "-X github.com/meinanzilinzhengying/ebpf-probe.Version=$(VERSION) -s -w"

all: bpf build

# Step 1: Compile BPF objects and copy to collector
bpf:
	@echo "=== Compiling BPF programs ==="
	clang -O2 -g -target bpf -D__TARGET_ARCH_x86 -c bpf/network_flow.bpf.c -o bpf/network_flow.bpf.o -I bpf
	clang -O2 -g -target bpf -D__TARGET_ARCH_x86 -c bpf/process_exec.bpf.c -o bpf/process_exec.bpf.o -I bpf
	clang -O2 -g -target bpf -D__TARGET_ARCH_x86 -c bpf/file_open.bpf.c -o bpf/file_open.bpf.o -I bpf
	clang -O2 -g -target bpf -D__TARGET_ARCH_x86 -c bpf/tcp_connect.bpf.c -o bpf/tcp_connect.bpf.o -I bpf
	clang -O2 -g -target bpf -D__TARGET_ARCH_x86 -c bpf/syscall.bpf.c -o bpf/syscall.bpf.o -I bpf
	clang -O2 -g -target bpf -D__TARGET_ARCH_x86 -c bpf/http_trace.bpf.c -o bpf/http_trace.bpf.o -I bpf
	clang -O2 -g -target bpf -D__TARGET_ARCH_x86 -c bpf/dns_trace.bpf.c -o bpf/dns_trace.bpf.o -I bpf
	clang -O2 -g -target bpf -D__TARGET_ARCH_x86 -c bpf/db_trace.bpf.c -o bpf/db_trace.bpf.o -I bpf
	clang -O2 -g -target bpf -D__TARGET_ARCH_x86 -c bpf/sched_trace.bpf.c -o bpf/sched_trace.bpf.o -I bpf
	clang -O2 -g -target bpf -D__TARGET_ARCH_x86 -c bpf/mem_trace.bpf.c -o bpf/mem_trace.bpf.o -I bpf
	clang -O2 -g -target bpf -D__TARGET_ARCH_x86 -c bpf/block_trace.bpf.c -o bpf/block_trace.bpf.o -I bpf
	clang -O2 -g -target bpf -D__TARGET_ARCH_x86 -c bpf/security_trace.bpf.c -o bpf/security_trace.bpf.o -I bpf
	cp bpf/*.o internal/collector/

# Step 2: Build Go binary
build: bpf
	go build $(GOFLAGS) -o $(BINARY) $(CMD)

# Build for Linux amd64
build-amd64: bpf
	GOOS=linux GOARCH=amd64 go build $(GOFLAGS) -o $(BINARY)-amd64 $(CMD)

# Build for Linux arm64
build-arm64: bpf
	GOOS=linux GOARCH=arm64 go build $(GOFLAGS) -o $(BINARY)-arm64 $(CMD)

# Build static binary
build-static: bpf
	CGO_ENABLED=0 GOOS=linux go build $(GOFLAGS) -a -installsuffix cgo -o $(BINARY)-static $(CMD)

# Docker image
docker:
	docker build -t cloudflow-ebpf-probe:$(VERSION) -f deploy/docker/Dockerfile .

# Package
package: build-static
	mkdir -p dist/$(BINARY)-$(VERSION)
	cp $(BINARY)-static dist/$(BINARY)-$(VERSION)/$(BINARY)
	cp deploy/install/install.sh dist/$(BINARY)-$(VERSION)/
	cp deploy/systemd/cloudflow-ebpf-probe.service dist/$(BINARY)-$(VERSION)/
	cp config/config.yaml dist/$(BINARY)-$(VERSION)/
	cp README.md dist/$(BINARY)-$(VERSION)/
	tar czf dist/$(BINARY)-$(VERSION)-linux-amd64.tar.gz -C dist $(BINARY)-$(VERSION)

# Install locally
install: build
	install -Dm755 $(BINARY) /usr/local/bin/$(BINARY)
	install -Dm644 deploy/systemd/cloudflow-ebpf-probe.service /etc/systemd/system/

# Clean
clean:
	rm -f $(BINARY) $(BINARY)-* bpf/*.bpf.o
	rm -rf dist/

# Test
test:
	go test ./...
