package main

import (
	"context"
	"log"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	pb "gatesvr/proto"
)

func main() {
	// 连接到网关的gRPC服务
	conn, err := grpc.Dial("localhost:8082", grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("连接网关gRPC服务失败: %v", err)
	}
	defer conn.Close()

	client := pb.NewGatewayServiceClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	log.Println("=== 网关gRPC单播推送功能测试 ===")

	// 测试1: 单播推送到GID
	log.Println("\n1. 测试单播推送到GID...")
	resp1, err := client.PushToClient(ctx, &pb.UnicastPushRequest{
		TargetType: "gid",
		TargetId:   "12345",
		MsgType:    "system",
		Title:      "系统通知",
		Content:    "这是一条测试推送消息",
		Data:       []byte("test data"),
	})
	if err != nil {
		log.Printf("单播推送失败: %v", err)
	} else {
		log.Printf("推送结果: success=%v, message=%s", resp1.Success, resp1.Message)
	}

	// 测试2: 单播推送到OpenID
	log.Println("\n2. 测试单播推送到OpenID...")
	resp2, err := client.PushToClient(ctx, &pb.UnicastPushRequest{
		TargetType: "openid",
		TargetId:   "user123",
		MsgType:    "personal",
		Title:      "个人消息",
		Content:    "您有新的消息",
		Data:       []byte("personal data"),
	})
	if err != nil {
		log.Printf("单播推送失败: %v", err)
	} else {
		log.Printf("推送结果: success=%v, message=%s", resp2.Success, resp2.Message)
	}

	// 测试3: 批量单播推送
	log.Println("\n3. 测试批量单播推送...")
	targets := []*pb.UnicastTarget{
		{TargetType: "gid", TargetId: "12345"},
		{TargetType: "gid", TargetId: "67890"},
		{TargetType: "openid", TargetId: "user456"},
	}

	resp3, err := client.BatchPushToClients(ctx, &pb.BatchUnicastPushRequest{
		Targets: targets,
		MsgType: "broadcast",
		Title:   "批量通知",
		Content: "这是一条批量推送消息",
		Data:    []byte("batch data"),
	})
	if err != nil {
		log.Printf("批量推送失败: %v", err)
	} else {
		log.Printf("批量推送结果: 成功 %d/%d", resp3.SuccessCount, resp3.TotalCount)
		for _, result := range resp3.Results {
			if result.Success {
				log.Printf("  ✓ %s:%s 推送成功", result.TargetType, result.TargetId)
			} else {
				log.Printf("  ✗ %s:%s 推送失败: %s", result.TargetType, result.TargetId, result.ErrorMessage)
			}
		}
	}

	// 测试4: 推送到不存在的目标（应该失败）
	log.Println("\n4. 测试推送到不存在的目标...")
	resp4, err := client.PushToClient(ctx, &pb.UnicastPushRequest{
		TargetType: "gid",
		TargetId:   "99999",
		MsgType:    "test",
		Title:      "测试推送",
		Content:    "目标不存在的推送测试",
	})
	if err != nil {
		log.Printf("推送失败（预期行为）: %v", err)
	} else {
		log.Printf("推送结果: success=%v, message=%s", resp4.Success, resp4.Message)
		if !resp4.Success {
			log.Printf("推送失败（预期行为）: %s", resp4.Message)
		}
	}

	log.Println("\n=== 测试完成 ===")
}
