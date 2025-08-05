package gateway

import (
	"context"
	"io"
	"log"
	"sync"
	"time"

	"google.golang.org/protobuf/proto"

	"gatesvr/internal/session"
	pb "gatesvr/proto"

	"github.com/quic-go/quic-go"
)

// 异步消息读取事件驱动实现
type MessageEvent struct {
	Data      []byte
	Err       error
	ReadTime  time.Time // 读取完成时间
	StartTime time.Time // 开始读取时间
}

// acceptConnections 接受QUIC连接
func (s *Server) acceptConnections(ctx context.Context) {
	defer s.wg.Done()

	for {
		select {
		case <-ctx.Done():
			return
		case <-s.stopCh:
			return
		default:
			// 接受新连接
			conn, err := s.quicListener.Accept(ctx)
			if err != nil {
				if !s.isRunning() {
					return
				}
				log.Printf("接受QUIC连接失败: %v", err)
				s.performanceTracker.RecordError()
				continue
			}

			// 记录新连接
			s.performanceTracker.RecordConnection()

			// 为每个连接启动处理goroutine
			s.wg.Add(1)
			go s.handleConnection(ctx, conn)
		}
	}
}

// handleConnection 处理单个QUIC连接 - 异步IO + 事件驱动实现
func (s *Server) handleConnection(ctx context.Context, conn *quic.Conn) {
	defer s.wg.Done()
	defer func() {
		conn.CloseWithError(0, "connection closed")
		s.performanceTracker.RecordDisconnection()
	}()

	log.Printf("新的QUIC连接: %s", conn.RemoteAddr())

	// 接受双向流
	stream, err := conn.AcceptStream(ctx)
	if err != nil {
		log.Printf("接受流失败: %v", err)
		s.performanceTracker.RecordError()
		return
	}
	defer stream.Close()

	// 首先读取第一个消息以获取身份标识信息
	log.Printf("等待第一个消息以获取身份标识 - 连接: %s", conn.RemoteAddr())

	firstMsgData, err := s.messageCodec.ReadMessage(stream)
	if err != nil {
		log.Printf("读取第一个消息失败 - 连接: %s, 错误: %v", conn.RemoteAddr(), err)
		s.performanceTracker.RecordError()
		return
	}

	// 解析第一个消息以提取身份信息
	firstClientReq, err := s.messageCodec.DecodeClientRequest(firstMsgData)
	if err != nil {
		log.Printf("解析第一个消息失败 - 连接: %s, 错误: %v", conn.RemoteAddr(), err)
		s.performanceTracker.RecordError()
		return
	}

	// 从第一个消息中提取身份标识信息
	var openID, accessToken, clientID string
	if firstClientReq.Type == pb.RequestType_REQUEST_START {
		// 解析START请求以获取身份信息
		startReq := &pb.StartRequest{}
		if err := proto.Unmarshal(firstClientReq.Payload, startReq); err == nil {
			openID = startReq.Openid
			accessToken = startReq.AuthToken
			clientID = startReq.ClientId
		}
	} else if firstClientReq.Type == pb.RequestType_REQUEST_BUSINESS {
		// 如果第一个消息是业务请求，尝试从中提取身份信息
		businessReq := &pb.BusinessRequest{}
		if err := proto.Unmarshal(firstClientReq.Payload, businessReq); err == nil {
			// 根据业务请求的参数提取身份信息
			if params := businessReq.Params; params != nil {
				if uid, exists := params["Openid"]; exists {
					openID = uid
				}
				if token, exists := params["accessToken"]; exists {
					accessToken = token
				}
				if cid, exists := params["clientId"]; exists {
					clientID = cid
				}
			}
		}
	}

	// 如果无法从消息中提取到身份信息，使用连接地址作为后备方案
	remoteAddr := conn.RemoteAddr().String()
	if clientID == "" {
		clientID = generateClientIDFromAddr(remoteAddr)
	}
	userIP := conn.RemoteAddr().String()

	log.Printf("从第一个消息中提取身份信息 - OpenID: %s, ClientID: %s", openID, clientID)

	// 现在使用提取到的身份信息创建或重连会话
	session, isReconnect := s.sessionManager.CreateOrReconnectSession(conn, stream, clientID, openID, accessToken, userIP)
	log.Printf("根据第一个消息提取出session - sessionid: %s", session.ID)
	if isReconnect {
		log.Printf("检测到重连 - 会话: %s, 客户端: %s, 用户: %s", session.ID, clientID, openID)
	}

	// 根据断开原因决定清理策略
	defer func() {
		// 检查断开原因
		reason := "unknown"
		delay := true // 默认延迟清理，支持重连

		// 在实际项目中，可以根据错误类型判断是否为网络问题
		if session.Gid() == 0 {
			// 未登录的会话，立即清理
			delay = false
			reason = "not logged in"
		}

		s.sessionManager.RemoveSessionWithDelay(session.ID, delay, reason)
	}()

	log.Printf("创建会话: %s", session.ID)

	// 更新连接数指标
	s.metrics.SetActiveConnections(s.sessionManager.GetSessionCount())

	// 先处理第一个消息
	firstMsgEvent := MessageEvent{
		Data:      firstMsgData,
		Err:       nil,
		ReadTime:  time.Now(),
		StartTime: time.Now(),
	}
	s.handleMessageEvent(ctx, session, firstMsgEvent)

	// 创建消息通道，适当的缓冲区避免阻塞
	msgChan := make(chan MessageEvent, 32)
	readCtx, readCancel := context.WithCancel(ctx)
	defer readCancel()

	// 启动异步读取goroutine
	var readWg sync.WaitGroup
	readWg.Add(1)
	go func() {
		defer readWg.Done()
		defer close(msgChan) // 确保channel被关闭

		log.Printf("启动异步读取协程 - 会话: %s", session.ID)

		for {
			select {
			case <-readCtx.Done():
				log.Printf("读取协程收到取消信号 - 会话: %s", session.ID)
				return
			default:
				readStartTime := time.Now()
				data, err := s.messageCodec.ReadMessage(stream)
				readEndTime := time.Now()

				// 发送消息事件
				select {
				case msgChan <- MessageEvent{
					Data:      data,
					Err:       err,
					ReadTime:  readEndTime,
					StartTime: readStartTime,
				}:
				case <-readCtx.Done():
					return
				}

				// 如果读取出错，退出读取循环
				if err != nil {
					log.Printf("读取消息出错，退出读取协程 - 会话: %s, 错误: %v", session.ID, err)
					return
				}
			}
		}
	}()

	// 主事件处理循环
	log.Printf("开始事件驱动消息处理 - 会话: %s", session.ID)

	for {
		select {
		case <-ctx.Done():
			log.Printf("收到上下文取消信号 - 会话: %s", session.ID)
			readCancel()
			readWg.Wait()
			return

		case <-s.stopCh:
			log.Printf("收到服务器停止信号 - 会话: %s", session.ID)
			readCancel()
			readWg.Wait()
			return

		case event, ok := <-msgChan:
			if !ok {
				// Channel已关闭，说明读取goroutine已退出
				log.Printf("消息通道已关闭 - 会话: %s", session.ID)
				readWg.Wait()
				return
			}

			// 处理消息事件
			if event.Err != nil {
				if event.Err == io.EOF {
					log.Printf("客户端正常断开连接 - 会话: %s", session.ID)
				} else {
					log.Printf("读取消息失败 - 会话: %s, 错误: %v", session.ID, event.Err)
					s.metrics.IncError("read_error")
					s.performanceTracker.RecordError()
				}
				readCancel()
				readWg.Wait()
				return
			}

			s.handleMessageEvent(ctx, session, event)
		}
	}
}

// handleMessageEvent 处理单个消息事件
func (s *Server) handleMessageEvent(ctx context.Context, session *session.Session, event MessageEvent) {
	log.Printf("处理消息 - 会话: %s, 读取时间: %v, 处理开始时间: %v", session.ID, event.ReadTime, event.StartTime)
	// 记录消息处理开始时间
	processStartTime := time.Now()

	// 记录请求和字节数
	s.performanceTracker.RecordRequest()
	s.performanceTracker.RecordBytes(int64(len(event.Data)))

	// 更新会话活动时间和指标
	session.UpdateActivity()
	s.metrics.AddThroughput("inbound", int64(len(event.Data)))
	s.metrics.IncQPS()

	// 解析消息
	parseStartTime := time.Now()
	clientReq, err := s.messageCodec.DecodeClientRequest(event.Data)
	parseEndTime := time.Now()

	if err != nil {
		log.Printf("解码消息失败 - 会话: %s, 错误: %v", session.ID, err)
		s.metrics.IncError("decode_error")
		s.performanceTracker.RecordError()
		return
	}

	// 记录解析时延
	parseLatency := parseEndTime.Sub(parseStartTime)
	s.performanceTracker.RecordParseLatency(parseLatency)

	// 处理不同类型的消息
	var success bool
	switch clientReq.Type {
	case pb.RequestType_REQUEST_START:
		success = s.handleStart(session, clientReq)
	case pb.RequestType_REQUEST_STOP:
		success = s.handleStop(session, clientReq)
	case pb.RequestType_REQUEST_HEARTBEAT:
		success = s.handleHeartbeat(session, clientReq)
	case pb.RequestType_REQUEST_BUSINESS:
		success = s.handleBusinessRequest(ctx, session, clientReq)
	case pb.RequestType_REQUEST_ACK:
		success = s.handleAck(session, clientReq)
	default:
		log.Printf("未知消息类型: %d - 会话: %s", clientReq.Type, session.ID)
		s.metrics.IncError("unknown_message_type")
		s.performanceTracker.RecordError()
		success = false
	}

	// 记录处理结果
	if success {
		s.performanceTracker.RecordResponse()
	} else {
		s.performanceTracker.RecordError()
	}

	// 记录总处理时延（不包含读取等待时间）
	totalProcessLatency := time.Since(processStartTime)
	s.performanceTracker.RecordTotalLatency(totalProcessLatency)
	s.performanceTracker.RecordProcessLatency(totalProcessLatency)

	// 记录主要延迟统计（用于monitor连续监控显示）
	// 在异步架构中，我们使用纯处理时延作为主要指标
	s.performanceTracker.RecordLatency(totalProcessLatency)

	// 详细日志记录
	log.Printf("消息处理完成 - 会话: %s, 类型: %d, 成功: %t, 处理时延: %.2fms",
		session.ID, clientReq.Type, success, totalProcessLatency.Seconds()*1000)
}
