// Package message 提供消息编解码和处理功能
package message

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"time"

	pb "gatesvr/proto"

	"google.golang.org/protobuf/proto"
)

const (
	// 消息头大小（4字节表示消息长度）
	MessageHeaderSize = 4
	// 最大消息大小（1MB）
	MaxMessageSize = 1024 * 1024
)

// ReadLatencyCallback 读取时延回调函数类型
type ReadLatencyCallback func(time.Duration)

// MessageCodec 消息编解码器
type MessageCodec struct {
	// 读取时延回调函数
	readLatencyCallback ReadLatencyCallback
}

// NewMessageCodec 创建新的消息编解码器
func NewMessageCodec() *MessageCodec {
	return &MessageCodec{}
}

// SetReadLatencyCallback 设置读取时延回调函数
func (c *MessageCodec) SetReadLatencyCallback(callback ReadLatencyCallback) {
	c.readLatencyCallback = callback
}

// EncodeClientRequest 编码客户端请求消息
func (c *MessageCodec) EncodeClientRequest(req *pb.ClientRequest) ([]byte, error) {
	// 序列化protobuf消息
	data, err := proto.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("序列化ClientRequest失败: %w", err)
	}

	// 检查消息大小
	if len(data) > MaxMessageSize {
		return nil, fmt.Errorf("消息太大: %d 字节, 最大允许: %d 字节", len(data), MaxMessageSize)
	}

	// 直接返回protobuf数据，长度前缀由WriteMessage处理
	return data, nil
}

// DecodeClientRequest 解码客户端请求消息
func (c *MessageCodec) DecodeClientRequest(data []byte) (*pb.ClientRequest, error) {
	req := &pb.ClientRequest{}
	if err := proto.Unmarshal(data, req); err != nil {
		return nil, fmt.Errorf("反序列化ClientRequest失败: %w", err)
	}
	return req, nil
}

// EncodeServerPush 编码服务器推送消息
func (c *MessageCodec) EncodeServerPush(push *pb.ServerPush) ([]byte, error) {
	// 序列化protobuf消息
	data, err := proto.Marshal(push)
	if err != nil {
		return nil, fmt.Errorf("序列化ServerPush失败: %w", err)
	}

	// 检查消息大小
	if len(data) > MaxMessageSize {
		return nil, fmt.Errorf("消息太大: %d 字节, 最大允许: %d 字节", len(data), MaxMessageSize)
	}

	// 直接返回protobuf数据，长度前缀由WriteMessage处理
	return data, nil
}

// DecodeServerPush 解码服务器推送消息
func (c *MessageCodec) DecodeServerPush(data []byte) (*pb.ServerPush, error) {
	push := &pb.ServerPush{}
	if err := proto.Unmarshal(data, push); err != nil {
		return nil, fmt.Errorf("反序列化ServerPush失败: %w", err)
	}
	return push, nil
}

// EncodeClientAck 编码客户端ACK消息
func (c *MessageCodec) EncodeClientAck(ack *pb.ClientAck) ([]byte, error) {
	// 序列化protobuf消息
	data, err := proto.Marshal(ack)
	if err != nil {
		return nil, fmt.Errorf("序列化ClientAck失败: %w", err)
	}

	// 直接返回protobuf数据，长度前缀由WriteMessage处理
	return data, nil
}

// DecodeClientAck 解码客户端ACK消息
func (c *MessageCodec) DecodeClientAck(data []byte) (*pb.ClientAck, error) {
	ack := &pb.ClientAck{}
	if err := proto.Unmarshal(data, ack); err != nil {
		return nil, fmt.Errorf("反序列化ClientAck失败: %w", err)
	}
	return ack, nil
}

// ReadMessage 从io.Reader中读取一个完整的消息
func (c *MessageCodec) ReadMessage(reader io.Reader) ([]byte, error) {
	readStartTime := time.Now()
	defer func() {
		// 记录读取时延
		if c.readLatencyCallback != nil {
			c.readLatencyCallback(time.Since(readStartTime))
		}
	}()

	// 读取消息头（长度）
	header := make([]byte, MessageHeaderSize)
	if _, err := io.ReadFull(reader, header); err != nil {
		return nil, fmt.Errorf("读取消息头失败: %w", err)
	}

	// 解析消息长度
	messageLen := binary.BigEndian.Uint32(header)

	// 检查消息长度
	if messageLen > MaxMessageSize {
		return nil, fmt.Errorf("消息长度过大: %d 字节, 最大允许: %d 字节", messageLen, MaxMessageSize)
	}

	// 读取消息体
	messageData := make([]byte, messageLen)
	if _, err := io.ReadFull(reader, messageData); err != nil {
		return nil, fmt.Errorf("读取消息体失败: %w", err)
	}

	return messageData, nil
}

// WriteMessage 向io.Writer写入一个消息
func (c *MessageCodec) WriteMessage(writer io.Writer, data []byte) error {
	// 检查消息大小
	if len(data) > MaxMessageSize {
		return fmt.Errorf("消息太大: %d 字节, 最大允许: %d 字节", len(data), MaxMessageSize)
	}

	// 创建缓冲区
	buf := bytes.NewBuffer(make([]byte, 0, MessageHeaderSize+len(data)))

	// 写入消息长度
	if err := binary.Write(buf, binary.BigEndian, uint32(len(data))); err != nil {
		return fmt.Errorf("写入消息长度失败: %w", err)
	}

	// 写入消息数据
	if _, err := buf.Write(data); err != nil {
		return fmt.Errorf("写入消息数据失败: %w", err)
	}

	// 发送完整消息
	if _, err := writer.Write(buf.Bytes()); err != nil {
		return fmt.Errorf("发送消息失败: %w", err)
	}

	return nil
}

// CreateHeartbeatRequest 创建心跳请求
func (c *MessageCodec) CreateHeartbeatRequest(msgID uint32, seqID uint64, openID string) *pb.ClientRequest {
	heartbeat := &pb.HeartbeatRequest{
		ClientTimestamp: getCurrentTimestamp(),
	}

	payload, _ := proto.Marshal(heartbeat)

	return &pb.ClientRequest{
		MsgId:   msgID,
		SeqId:   seqID,
		Type:    pb.RequestType_REQUEST_HEARTBEAT,
		Payload: payload,
		Openid:  openID,
	}
}

// CreateHeartbeatResponse 创建心跳响应
func (c *MessageCodec) CreateHeartbeatResponse(msgID uint32, seqID uint64, clientTimestamp int64) *pb.ServerPush {
	heartbeat := &pb.HeartbeatResponse{
		ClientTimestamp: clientTimestamp,
		ServerTimestamp: getCurrentTimestamp(),
	}

	payload, _ := proto.Marshal(heartbeat)

	return &pb.ServerPush{
		MsgId:   msgID,
		SeqId:   seqID,
		Type:    pb.PushType_PUSH_HEARTBEAT_RESP,
		Payload: payload,
	}
}

// CreateBusinessRequest 创建业务请求
func (c *MessageCodec) CreateBusinessRequest(msgID uint32, seqID uint64, action string, params map[string]string, data []byte, openID string) *pb.ClientRequest {
	business := &pb.BusinessRequest{
		Action: action,
		Params: params,
		Data:   data,
	}

	payload, _ := proto.Marshal(business)

	return &pb.ClientRequest{
		MsgId:   msgID,
		SeqId:   seqID,
		Type:    pb.RequestType_REQUEST_BUSINESS,
		Payload: payload,
		Openid:  openID,
	}
}

// CreateBusinessResponse 创建业务响应
func (c *MessageCodec) CreateBusinessResponse(msgID uint32, seqID uint64, code int32, message string, data []byte) *pb.ServerPush {
	business := &pb.BusinessResponse{
		Code:    code,
		Message: message,
		Data:    data,
	}

	payload, _ := proto.Marshal(business)

	return &pb.ServerPush{
		MsgId:   msgID,
		SeqId:   seqID,
		Type:    pb.PushType_PUSH_BUSINESS_DATA,
		Payload: payload,
	}
}

// CreateAckMessage 创建ACK消息
func (c *MessageCodec) CreateAckMessage(seqID uint64, openID string) *pb.ClientRequest {
	ack := &pb.ClientAck{
		AckedSeqId: seqID,
	}

	payload, _ := proto.Marshal(ack)

	return &pb.ClientRequest{
		MsgId:   0, // ACK消息不需要msgID
		SeqId:   0, // ACK消息不需要seqID
		Type:    pb.RequestType_REQUEST_ACK,
		Payload: payload,
		Openid:  openID,
	}
}

// CreateErrorMessage 创建错误消息
func (c *MessageCodec) CreateErrorMessage(msgID uint32, seqID uint64, code int32, message, detail string) *pb.ServerPush {
	errorMsg := &pb.ErrorMessage{
		ErrorCode:    code,
		ErrorMessage: message,
		Detail:       detail,
	}

	payload, _ := proto.Marshal(errorMsg)

	return &pb.ServerPush{
		MsgId:   msgID,
		SeqId:   seqID,
		Type:    pb.PushType_PUSH_ERROR,
		Payload: payload,
	}
}

// getCurrentTimestamp 获取当前时间戳（毫秒）
func getCurrentTimestamp() int64 {
	return time.Now().UnixMilli()
}

// CreateStartResponse 创建连接建立响应
func (c *MessageCodec) CreateStartResponse(msgID uint32, seqID uint64, success bool, errorMsg *pb.ErrorMessage, heartbeatInterval int32, connectionID string) *pb.ServerPush {
	startResp := &pb.StartResponse{
		Success:           success,
		Error:             errorMsg,
		HeartbeatInterval: heartbeatInterval,
		ConnectionId:      connectionID,
	}

	payload, _ := proto.Marshal(startResp)

	return &pb.ServerPush{
		MsgId:   msgID,
		SeqId:   seqID,
		Type:    pb.PushType_PUSH_START_RESP,
		Payload: payload,
	}
}

// CreateStartRequest 创建连接建立请求
func (c *MessageCodec) CreateStartRequest(msgID uint32, seqID uint64, openID, authToken string, lastAckedSeqID uint64) *pb.ClientRequest {
	startReq := &pb.StartRequest{
		Openid:         openID,
		AuthToken:      authToken,
		LastAckedSeqId: lastAckedSeqID,
	}

	payload, _ := proto.Marshal(startReq)

	return &pb.ClientRequest{
		MsgId:   msgID,
		SeqId:   seqID,
		Type:    pb.RequestType_REQUEST_START,
		Payload: payload,
		Openid:  openID,
	}
}

// CreateStopRequest 创建连接断开请求
func (c *MessageCodec) CreateStopRequest(msgID uint32, seqID uint64, reason pb.StopRequest_Reason, openID string) *pb.ClientRequest {
	stopReq := &pb.StopRequest{
		Reason: reason,
	}

	payload, _ := proto.Marshal(stopReq)

	return &pb.ClientRequest{
		MsgId:   msgID,
		SeqId:   seqID,
		Type:    pb.RequestType_REQUEST_STOP,
		Payload: payload,
		Openid:  openID,
	}
}
