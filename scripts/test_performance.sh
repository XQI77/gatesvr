#!/bin/bash

# 性能测试脚本
echo "========================================="
echo "      网关服务器性能测试工具"
echo "========================================="

# 配置
GATESVR_HTTP="localhost:8080"
GATESVR_QUIC="localhost:8443"
UPSTREAM="localhost:9000"

# 颜色输出
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# 检查服务状态
check_service() {
    local url=$1
    local name=$2
    
    if curl -s "$url" > /dev/null 2>&1; then
        echo -e "${GREEN}✓${NC} $name 服务运行正常"
        return 0
    else
        echo -e "${RED}✗${NC} $name 服务连接失败"
        return 1
    fi
}

# 获取性能指标
get_performance_stats() {
    echo -e "\n${BLUE}正在获取性能指标...${NC}"
    
    # 获取网关性能数据
    PERF_DATA=$(curl -s "http://$GATESVR_HTTP/performance" 2>/dev/null)
    if [ $? -eq 0 ]; then
        echo "$PERF_DATA" | jq '.' 2>/dev/null || echo "$PERF_DATA"
    else
        echo -e "${RED}无法获取性能数据${NC}"
    fi
}

# 获取会话统计
get_session_stats() {
    echo -e "\n${BLUE}正在获取会话统计...${NC}"
    
    STATS_DATA=$(curl -s "http://$GATESVR_HTTP/stats" 2>/dev/null)
    if [ $? -eq 0 ]; then
        echo "$STATS_DATA" | jq '.' 2>/dev/null || echo "$STATS_DATA"
    else
        echo -e "${RED}无法获取会话统计${NC}"
    fi
}

# 测试上游广播功能
test_broadcast() {
    echo -e "\n${BLUE}测试上游广播功能...${NC}"
    
    echo "上游服务会自动发送定时广播消息"
    echo "启动客户端以接收广播消息:"
    echo "  ./bin/client -server localhost:8443"
    echo ""
    echo "观察客户端接收到的广播消息日志"
}

# 监控模式
monitor_mode() {
    echo -e "\n${YELLOW}进入监控模式 (按Ctrl+C退出)${NC}"
    echo -e "${BLUE}时间        活跃连接  总请求   QPS     成功率   平均延迟${NC}"
    echo "---------------------------------------------------------------"
    
    while true; do
        PERF_DATA=$(curl -s "http://$GATESVR_HTTP/performance" 2>/dev/null)
        if [ $? -eq 0 ]; then
            # 解析JSON数据 (如果有jq的话)
            if command -v jq >/dev/null 2>&1; then
                TIME=$(date '+%H:%M:%S')
                ACTIVE_CONN=$(echo "$PERF_DATA" | jq -r '.active_connections // 0')
                TOTAL_REQ=$(echo "$PERF_DATA" | jq -r '.total_requests // 0')
                QPS=$(echo "$PERF_DATA" | jq -r '.qps // 0')
                SUCCESS_RATE=$(echo "$PERF_DATA" | jq -r '.success_rate // 0')
                AVG_LATENCY=$(echo "$PERF_DATA" | jq -r '.avg_latency_ms // 0')
                
                printf "%-11s %8s %8s %7.1f %8.1f%% %8.1fms\n" \
                    "$TIME" "$ACTIVE_CONN" "$TOTAL_REQ" "$QPS" "$SUCCESS_RATE" "$AVG_LATENCY"
            else
                echo "$(date '+%H:%M:%S') - 需要安装jq来解析JSON数据"
            fi
        else
            echo "$(date '+%H:%M:%S') - 无法获取数据"
        fi
        
        sleep 3
    done
}

# 启动多个测试客户端
start_test_clients() {
    local client_count=${1:-3}
    local request_interval=${2:-500ms}
    
    echo -e "\n${BLUE}启动 $client_count 个测试客户端 (请求间隔: $request_interval)${NC}"
    
    for i in $(seq 1 $client_count); do
        echo "启动客户端 $i..."
        ./bin/client.exe -server "$GATESVR_QUIC" -performance \
            -request-interval "$request_interval" \
            -clients 1 > "client_$i.log" 2>&1 &
        
        CLIENT_PIDS="$CLIENT_PIDS $!"
        sleep 0.5
    done
    
    echo "客户端已启动，PID: $CLIENT_PIDS"
    echo "日志文件: client_*.log"
}

# 停止测试客户端
stop_test_clients() {
    echo -e "\n${YELLOW}停止测试客户端...${NC}"
    if [ -n "$CLIENT_PIDS" ]; then
        for pid in $CLIENT_PIDS; do
            kill $pid 2>/dev/null
        done
        CLIENT_PIDS=""
        echo "测试客户端已停止"
    fi
}

# 主菜单
show_menu() {
    echo -e "\n${YELLOW}选择操作:${NC}"
    echo "1. 检查服务状态"
    echo "2. 获取性能指标"
    echo "3. 获取会话统计"
    echo "4. 测试广播功能"
    echo "5. 启动测试客户端"
    echo "6. 停止测试客户端"
    echo "7. 监控模式"
    echo "8. 完整性能测试"
    echo "0. 退出"
    echo -n "请输入选择: "
}

# 完整性能测试
full_performance_test() {
    echo -e "\n${BLUE}开始完整性能测试...${NC}"
    
    # 检查服务
    echo -e "\n1. 检查服务状态:"
    check_service "http://$GATESVR_HTTP/health" "网关HTTP"
    check_service "http://$GATESVR_HTTP/stats" "网关统计"
    
    # 启动测试客户端
    echo -e "\n2. 启动测试客户端:"
    start_test_clients 5 "200ms"
    
    # 等待一段时间让客户端建立连接并发送请求
    echo -e "\n3. 等待30秒收集数据..."
    sleep 30
    
    # 获取性能数据
    echo -e "\n4. 性能数据:"
    get_performance_stats
    
    # 测试广播
    echo -e "\n5. 测试广播:"
    test_broadcast
    
    # 停止客户端
    stop_test_clients
    
    echo -e "\n${GREEN}完整性能测试完成${NC}"
}

# 信号处理
trap 'stop_test_clients; exit 0' INT TERM

# 主程序
if [ "$1" = "--monitor" ]; then
    monitor_mode
elif [ "$1" = "--auto" ]; then
    full_performance_test
elif [ "$1" = "--help" ]; then
    echo "用法: $0 [选项]"
    echo "选项:"
    echo "  --monitor    进入监控模式"
    echo "  --auto       运行自动性能测试"
    echo "  --help       显示帮助信息"
    echo "  (无参数)     显示交互菜单"
else
    # 交互模式
    while true; do
        show_menu
        read choice
        
        case $choice in
            1) 
                echo -e "\n检查服务状态:"
                check_service "http://$GATESVR_HTTP/health" "网关HTTP"
                check_service "http://$GATESVR_HTTP/stats" "网关统计"
                ;;
            2) get_performance_stats ;;
            3) get_session_stats ;;
            4) test_broadcast ;;
            5) 
                echo -n "客户端数量 [3]: "
                read client_count
                client_count=${client_count:-3}
                echo -n "请求间隔 [500ms]: "
                read interval
                interval=${interval:-500ms}
                start_test_clients $client_count $interval
                ;;
            6) stop_test_clients ;;
            7) monitor_mode ;;
            8) full_performance_test ;;
            0) stop_test_clients; echo "退出"; exit 0 ;;
            *) echo -e "${RED}无效选择${NC}" ;;
        esac
    done
fi 