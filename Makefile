# 高性能网关服务器 Makefile

# 项目信息
PROJECT_NAME := gatesvr
VERSION := v1.0.0
BUILD_TIME := $(shell date +%Y-%m-%d_%H:%M:%S)
GIT_COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")

# 编译配置
GO := go
GOFLAGS := -ldflags "-X main.version=$(VERSION) -X main.buildTime=$(BUILD_TIME) -X main.gitCommit=$(GIT_COMMIT)"
BINARY_DIR := bin
CERT_DIR := certs

# 二进制文件
GATESVR_BIN := $(BINARY_DIR)/gatesvr
UPSTREAM_BIN := $(BINARY_DIR)/upstream
CLIENT_BIN := $(BINARY_DIR)/client

# 操作系统检测
UNAME_S := $(shell uname -s 2>/dev/null || echo "Windows")
ifeq ($(UNAME_S),Windows_NT)
	GATESVR_BIN := $(GATESVR_BIN).exe
	UPSTREAM_BIN := $(UPSTREAM_BIN).exe
	CLIENT_BIN := $(CLIENT_BIN).exe
	CERT_SCRIPT := scripts/generate_certs.bat
else
	CERT_SCRIPT := scripts/generate_certs.sh
endif

.PHONY: all build clean test proto certs certs-script deps help run-upstream run-gatesvr run-client demo tools perf-test monitor load-test continuous-test

# 默认目标
all: deps proto certs build

# 显示帮助信息
help:
	@echo "高性能网关服务器构建工具"
	@echo ""
	@echo "可用命令:"
	@echo "  all          - 完整构建（依赖、protobuf、证书、编译）"
	@echo "  build        - 构建所有二进制文件"
	@echo "  clean        - 清理构建文件"
	@echo "  test         - 运行测试"
	@echo "  proto        - 生成protobuf代码"
	@echo "  certs        - 生成TLS证书"
	@echo "  deps         - 安装依赖"
	@echo "  run-upstream - 运行上游服务"
	@echo "  run-gatesvr  - 运行网关服务器"
	@echo "  run-client   - 运行客户端"
	@echo "  demo         - 运行演示"
	@echo "  help         - 显示此帮助信息"

# 安装依赖
deps:
	@echo "安装Go依赖..."
	$(GO) mod download
	$(GO) mod tidy
	@echo "安装protobuf工具..."
	$(GO) install google.golang.org/protobuf/cmd/protoc-gen-go@latest
	$(GO) install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest

# 生成protobuf代码
proto:
	@echo "生成protobuf代码..."
	protoc --go_out=. --go_opt=paths=source_relative \
	       --go-grpc_out=. --go-grpc_opt=paths=source_relative \
	       proto/*.proto

# 生成TLS证书
certs:
	@echo "生成TLS证书..."
	@if [ ! -f $(CERT_DIR)/server.crt ]; then \
		echo "使用Go工具生成证书..."; \
		$(MAKE) tools; \
		if [ "$(UNAME_S)" = "Windows_NT" ]; then \
			./$(BINARY_DIR)/gencerts.exe; \
		else \
			./$(BINARY_DIR)/gencerts; \
		fi \
	else \
		echo "TLS证书已存在，跳过生成"; \
	fi

# 使用脚本生成证书（备选方案）
certs-script:
	@echo "使用脚本生成TLS证书..."
	@if [ "$(UNAME_S)" = "Windows_NT" ]; then \
		$(CERT_SCRIPT); \
	else \
		chmod +x $(CERT_SCRIPT) && $(CERT_SCRIPT); \
	fi

# 创建目录
$(BINARY_DIR):
	mkdir -p $(BINARY_DIR)

# 构建网关服务器
$(GATESVR_BIN): $(BINARY_DIR)
	@echo "构建网关服务器..."
	$(GO) build $(GOFLAGS) -o $(GATESVR_BIN) ./cmd/gatesvr

# 构建上游服务
$(UPSTREAM_BIN): $(BINARY_DIR)
	@echo "构建上游服务..."
	$(GO) build $(GOFLAGS) -o $(UPSTREAM_BIN) ./cmd/upstream

# 构建客户端
$(CLIENT_BIN): $(BINARY_DIR)
	@echo "构建客户端..."
	$(GO) build $(GOFLAGS) -o $(CLIENT_BIN) ./cmd/client

# 构建所有组件
build: $(GATESVR_BIN) $(UPSTREAM_BIN) $(CLIENT_BIN) tools
	@echo "构建完成!"
	@echo "二进制文件位置:"
	@echo "  网关服务器: $(GATESVR_BIN)"
	@echo "  上游服务:   $(UPSTREAM_BIN)"
	@echo "  客户端:     $(CLIENT_BIN)"
	@echo "  性能监控:   $(BINARY_DIR)/monitor$(shell echo $(GATESVR_BIN) | grep -o '\.exe$$' || echo '')"
	@echo "  证书生成:   $(BINARY_DIR)/gencerts$(shell echo $(GATESVR_BIN) | grep -o '\.exe$$' || echo '')"

# 构建工具
tools:
	@echo "构建工具..."
ifeq ($(UNAME_S),Windows_NT)
	$(GO) build -o $(BINARY_DIR)/monitor.exe ./cmd/monitor
	$(GO) build -o $(BINARY_DIR)/gencerts.exe ./cmd/gencerts
	chmod +x scripts/test_performance.sh
else
	$(GO) build -o $(BINARY_DIR)/monitor ./cmd/monitor
	$(GO) build -o $(BINARY_DIR)/gencerts ./cmd/gencerts
	chmod +x scripts/test_performance.sh
endif

# 运行测试
test:
	@echo "运行测试..."
	$(GO) test -v ./...

# 清理构建文件
clean:
	@echo "清理构建文件..."
	rm -rf $(BINARY_DIR)
	rm -rf $(CERT_DIR)
	$(GO) clean

# 运行上游服务
run-upstream: $(UPSTREAM_BIN)
	@echo "启动上游服务..."
	./$(UPSTREAM_BIN) -addr :9000

# 运行网关服务器
run-gatesvr: $(GATESVR_BIN) certs
	@echo "启动网关服务器..."
	./$(GATESVR_BIN) -quic :8443 -http :8080 -upstream localhost:9000

# 运行客户端（交互模式）
run-client: $(CLIENT_BIN)
	@echo "启动客户端（交互模式）..."
	./$(CLIENT_BIN) -server localhost:8443 -interactive

# 演示模式
demo: build certs
	@echo "=========================================="
	@echo "           演示模式"
	@echo "=========================================="
	@echo "1. 在新终端中运行: make run-upstream"
	@echo "2. 在新终端中运行: make run-gatesvr"
	@echo "3. 在新终端中运行: make run-client"
	@echo ""
	@echo "或者使用以下命令进行快速测试:"
	@echo "./$(CLIENT_BIN) -server localhost:8443 -test echo -data 'Hello World'"
	@echo ""
	@echo "HTTP API测试:"
	@echo "curl http://localhost:8080/health"
	@echo "curl http://localhost:8080/stats"
	@echo "curl http://localhost:9090/metrics"

# 快速测试
quick-test: build certs
	@echo "启动服务进行快速测试..."
	@echo "启动上游服务..."
	./$(UPSTREAM_BIN) &
	@sleep 2
	@echo "启动网关服务器..."
	./$(GATESVR_BIN) &
	@sleep 3
	@echo "运行客户端测试..."
	./$(CLIENT_BIN) -server localhost:8443 -test echo -data "Quick test message"
	./$(CLIENT_BIN) -server localhost:8443 -test time
	./$(CLIENT_BIN) -server localhost:8443 -test hello
	@echo "停止服务..."
	@pkill -f "$(UPSTREAM_BIN)" || true
	@pkill -f "$(GATESVR_BIN)" || true

# 安装到系统
install: build
	@echo "安装到系统..."
	cp $(GATESVR_BIN) /usr/local/bin/
	cp $(UPSTREAM_BIN) /usr/local/bin/
	cp $(CLIENT_BIN) /usr/local/bin/

# 打包发布
package: clean build
	@echo "打包发布..."
	mkdir -p release
	tar -czf release/$(PROJECT_NAME)-$(VERSION)-$(shell uname -m).tar.gz \
		$(BINARY_DIR)/ $(CERT_DIR)/ scripts/ README.md

# 开发模式（监听文件变化并重新构建）
dev:
	@echo "开发模式（需要安装 air 工具）..."
	@if ! command -v air >/dev/null 2>&1; then \
		echo "安装 air 工具..."; \
		$(GO) install github.com/cosmtrek/air@latest; \
	fi
	air

# 代码格式化
fmt:
	@echo "格式化代码..."
	$(GO) fmt ./...
	$(GO) vet ./...

# 代码检查
lint:
	@echo "代码检查..."
	@if ! command -v golangci-lint >/dev/null 2>&1; then \
		echo "安装 golangci-lint..."; \
		curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $(go env GOPATH)/bin v1.54.2; \
	fi
	golangci-lint run

# Docker 构建
docker:
	@echo "构建Docker镜像..."
	docker build -t $(PROJECT_NAME):$(VERSION) .
	docker build -t $(PROJECT_NAME):latest .

# 显示版本信息
version:
	@echo "项目: $(PROJECT_NAME)"
	@echo "版本: $(VERSION)"
	@echo "构建时间: $(BUILD_TIME)"
	@echo "Git提交: $(GIT_COMMIT)"

# 运行性能测试
perf-test: tools
	@echo "运行性能测试..."
	chmod +x scripts/test_performance.sh
	./scripts/test_performance.sh --auto

# 启动性能监控
monitor: tools
	@echo "启动性能监控..."
ifeq ($(UNAME_S),Windows_NT)
	./$(BINARY_DIR)/monitor.exe -server localhost:8080 -continuous
else
	./$(BINARY_DIR)/monitor -server localhost:8080 -continuous
endif

# 启动多客户端性能测试
load-test: $(CLIENT_BIN)
	@echo "启动负载测试 (5个客户端)..."
	./$(CLIENT_BIN) -server localhost:8443 -performance -clients 5 -request-interval 200ms

# 启动单客户端持续测试
continuous-test: $(CLIENT_BIN)
	@echo "启动持续测试 (单客户端)..."
	./$(CLIENT_BIN) -server localhost:8443 