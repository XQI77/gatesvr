#!/bin/bash
# gatesvr Docker服务部署脚本
set -e

# 颜色输出
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

print_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

print_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

print_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

print_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

print_info "=== gatesvr服务集群部署脚本 ==="

# 解析命令行参数
ACTION=${1:-"deploy"}
ENVIRONMENT=${2:-"local"}

case $ACTION in
    "deploy"|"start")
        DEPLOY_ACTION="up"
        ;;
    "stop")
        DEPLOY_ACTION="down"
        ;;
    "restart")
        DEPLOY_ACTION="restart"
        ;;
    "logs")
        DEPLOY_ACTION="logs"
        ;;
    "status")
        DEPLOY_ACTION="ps"
        ;;
    *)
        print_error "未知的操作: $ACTION"
        echo "使用方法: $0 [deploy|start|stop|restart|logs|status] [local|dev|prod]"
        exit 1
        ;;
esac

# 检查Docker和docker-compose
if ! docker info > /dev/null 2>&1; then
    print_error "Docker未运行，请启动Docker"
    exit 1
fi

if ! command -v docker-compose &> /dev/null; then
    print_error "docker-compose未安装"
    exit 1
fi

# 检查必要的文件
REQUIRED_FILES=(
    "docker-compose.yml"
    "docker/config/docker-config.yaml"
    "certs/server.crt"
    "certs/server.key"
)

for file in "${REQUIRED_FILES[@]}"; do
    if [[ ! -f "$file" ]]; then
        print_warning "缺少文件: $file"
        if [[ "$file" == "certs/server.crt" ]] || [[ "$file" == "certs/server.key" ]]; then
            print_info "正在生成自签名证书..."
            mkdir -p certs
            openssl req -x509 -newkey rsa:4096 -keyout certs/server.key -out certs/server.crt -days 365 -nodes -subj "/CN=localhost"
            print_success "自签名证书生成完成"
        else
            print_error "必需文件不存在，请检查"
            exit 1
        fi
    fi
done

# 设置环境变量
export VERSION=${VERSION:-"latest"}
export BUILD_DATE=${BUILD_DATE:-$(date -u +'%Y-%m-%dT%H:%M:%SZ')}
export GIT_COMMIT=${GIT_COMMIT:-$(git rev-parse --short HEAD 2>/dev/null || echo "unknown")}
export ENVIRONMENT=$ENVIRONMENT

print_info "部署环境: $ENVIRONMENT"
print_info "镜像版本: $VERSION"

# 执行部署操作
case $DEPLOY_ACTION in
    "up")
        print_info "启动gatesvr服务集群..."
        
        # 停止现有服务（如果存在）
        print_info "停止现有服务..."
        docker-compose down --remove-orphans 2>/dev/null || true
        
        # 清理网络和数据卷（可选）
        if [[ "${CLEAN_DEPLOY}" == "true" ]]; then
            print_info "清理环境..."
            docker system prune -f
            docker volume prune -f
        fi
        
        # 启动服务
        print_info "启动服务..."
        if docker-compose up -d; then
            print_success "服务集群启动成功"
        else
            print_error "服务集群启动失败"
            exit 1
        fi
        
        # 等待服务启动
        print_info "等待服务启动..."
        sleep 15
        
        # 检查服务状态
        print_info "检查服务状态..."
        docker-compose ps
        
        # 健康检查
        print_info "执行健康检查..."
        check_services_health
        
        # 显示访问信息
        show_access_info
        ;;
        
    "down")
        print_info "停止gatesvr服务集群..."
        docker-compose down --remove-orphans
        if [[ "${REMOVE_VOLUMES}" == "true" ]]; then
            docker-compose down -v
            print_info "已删除数据卷"
        fi
        print_success "服务集群已停止"
        ;;
        
    "restart")
        print_info "重启gatesvr服务集群..."
        docker-compose restart
        sleep 10
        docker-compose ps
        check_services_health
        print_success "服务集群重启完成"
        ;;
        
    "logs")
        SERVICE=${3:-""}
        if [[ -n "$SERVICE" ]]; then
            print_info "显示服务 $SERVICE 的日志..."
            docker-compose logs -f "$SERVICE"
        else
            print_info "显示所有服务的日志..."
            docker-compose logs -f
        fi
        ;;
        
    "ps")
        print_info "服务状态:"
        docker-compose ps
        print_info "网络信息:"
        docker network ls | grep gatesvr || true
        print_info "数据卷信息:"
        docker volume ls | grep gatesvr || true
        ;;
esac

# 健康检查函数
check_services_health() {
    local services=("hellosvr:8081" "businesssvr:8082" "zonesvr:8083" "gatesvr:8090")
    local max_attempts=30
    local attempt=1
    
    for service_endpoint in "${services[@]}"; do
        IFS=':' read -r service port <<< "$service_endpoint"
        print_info "检查 $service 健康状态..."
        
        while [[ $attempt -le $max_attempts ]]; do
            if curl -f -s "http://localhost:$port/health" >/dev/null 2>&1 || 
               curl -f -s "http://localhost:$port/api/status" >/dev/null 2>&1; then
                print_success "✓ $service 运行正常"
                break
            fi
            
            if [[ $attempt -eq $max_attempts ]]; then
                print_error "✗ $service 启动超时或异常"
                print_info "查看 $service 日志:"
                docker-compose logs --tail=20 "$service"
            else
                sleep 2
                attempt=$((attempt + 1))
            fi
        done
        attempt=1
    done
}

# 显示访问信息
show_access_info() {
    print_success "=== 服务访问信息 ==="
    echo "网关服务 (QUIC):    localhost:8080"
    echo "网关API:            http://localhost:8090"
    echo "网关gRPC:           localhost:8092"
    echo "监控指标:           http://localhost:9090"
    echo ""
    echo "上游服务:"
    echo "  Hello服务:        http://localhost:8081"
    echo "  Business服务:     http://localhost:8082"
    echo "  Zone服务:         http://localhost:8083"
    echo ""
    echo "常用命令:"
    echo "  查看状态:         docker-compose ps"
    echo "  查看日志:         docker-compose logs -f [service]"
    echo "  停止服务:         docker-compose down"
    echo "  重启服务:         docker-compose restart [service]"
}