#!/bin/bash
# gatesvr Docker镜像构建脚本
set -e

# 颜色输出
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# 打印带颜色的消息
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

print_info "=== gatesvr容器化镜像构建脚本 ==="

# 检查Docker是否运行
if ! docker info > /dev/null 2>&1; then
    print_error "Docker未运行，请启动Docker"
    exit 1
fi

# 检查docker-compose是否可用
if ! command -v docker-compose &> /dev/null; then
    print_error "docker-compose未安装，请安装docker-compose"
    exit 1
fi

# 设置构建参数
BUILD_DATE=$(date -u +'%Y-%m-%dT%H:%M:%SZ')
GIT_COMMIT=$(git rev-parse --short HEAD 2>/dev/null || echo "unknown")
VERSION=${VERSION:-"latest"}
REGISTRY=${REGISTRY:-""}

print_info "构建信息:"
echo "  版本: $VERSION"
echo "  构建时间: $BUILD_DATE"
echo "  Git提交: $GIT_COMMIT"
echo "  镜像仓库: ${REGISTRY:-"本地构建"}"

# 导出环境变量供docker-compose使用
export VERSION
export BUILD_DATE
export GIT_COMMIT

# 清理旧的构建缓存（可选）
if [[ "${CLEAN_BUILD}" == "true" ]]; then
    print_info "清理Docker构建缓存..."
    docker system prune -f --volumes
fi

# 构建所有镜像
print_info "开始构建所有服务镜像..."

# 使用docker-compose构建所有镜像
if docker-compose build --parallel; then
    print_success "所有镜像构建完成"
else
    print_error "镜像构建失败"
    exit 1
fi

# 显示构建结果
print_info "构建完成的镜像列表:"
docker images | grep -E "(gatesvr|hellosvr|businesssvr|zonesvr)" | head -20

# 标记镜像（如果指定了仓库）
if [[ -n "$REGISTRY" ]]; then
    print_info "标记镜像到仓库: $REGISTRY"
    
    for service in gatesvr hellosvr businesssvr zonesvr; do
        print_info "标记 ${service}:${VERSION}"
        docker tag ${service}:${VERSION} ${REGISTRY}/${service}:${VERSION}
        docker tag ${service}:${VERSION} ${REGISTRY}/${service}:latest
    done
    
    print_success "镜像标记完成"
fi

# 推送镜像（可选）
if [[ "${PUSH_IMAGES}" == "true" && -n "$REGISTRY" ]]; then
    print_info "推送镜像到仓库..."
    
    for service in gatesvr hellosvr businesssvr zonesvr; do
        print_info "推送 ${REGISTRY}/${service}:${VERSION}"
        if docker push ${REGISTRY}/${service}:${VERSION}; then
            docker push ${REGISTRY}/${service}:latest
            print_success "${service} 推送成功"
        else
            print_error "${service} 推送失败"
        fi
    done
fi

# 镜像大小统计
print_info "镜像大小统计:"
for service in gatesvr hellosvr businesssvr zonesvr; do
    size=$(docker images ${service}:${VERSION} --format "table {{.Size}}" | tail -n +2)
    echo "  ${service}: ${size}"
done

print_success "=== 构建脚本执行完成 ==="
print_info "使用以下命令启动服务："
echo "  docker-compose up -d"
print_info "查看服务状态："
echo "  docker-compose ps"