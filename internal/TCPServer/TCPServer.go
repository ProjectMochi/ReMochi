package TCPServer

import (
	"ReMochi/internal/UUID2IP"
	"ReMochi/internal/protocol"
	"ReMochi/internal/redis"
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"
)

const listenPort = "17890"

// 消息转发相关常量
const (
	MessageForwardTimeout = 5 * time.Minute // 消息转发超时时间
	MessageRetryInterval  = 1 * time.Minute // 消息重试间隔
	MaxMessageRetryCount  = 3               // 最大重试次数
)

// 客户端连接管理器
type ConnectionManager struct {
	mu          sync.RWMutex
	connections map[string]net.Conn // key: UUID, value: 客户端连接
}

// 消息状态
type MessageStatus struct {
	Message     *protocol.Message
	SenderUUID  string
	Retries     int
	LastAttempt time.Time
	Status      string // "pending", "sent", "failed", "delivered"
}

// 消息状态管理器
type MessageManager struct {
	mu       sync.RWMutex
	messages map[string]*MessageStatus // key: 消息ID, value: 消息状态
}

// 全局管理器实例
var (
	connManager = &ConnectionManager{
		connections: make(map[string]net.Conn),
	}
	msgManager = &MessageManager{
		messages: make(map[string]*MessageStatus),
	}
)

// 连接管理器方法
func (cm *ConnectionManager) AddConnection(uuid string, conn net.Conn) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	cm.connections[uuid] = conn
}

func (cm *ConnectionManager) GetConnection(uuid string) (net.Conn, bool) {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	conn, exists := cm.connections[uuid]
	return conn, exists
}

func (cm *ConnectionManager) RemoveConnection(uuid string) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	delete(cm.connections, uuid)
}

// 消息管理器方法
func (mm *MessageManager) AddMessage(msgID string, msg *protocol.Message, senderUUID string) {
	mm.mu.Lock()
	defer mm.mu.Unlock()
	mm.messages[msgID] = &MessageStatus{
		Message:     msg,
		SenderUUID:  senderUUID,
		Retries:     0,
		LastAttempt: time.Now(),
		Status:      "pending",
	}
}

func (mm *MessageManager) UpdateMessageStatus(msgID string, status string) {
	mm.mu.Lock()
	defer mm.mu.Unlock()
	if msgStatus, exists := mm.messages[msgID]; exists {
		msgStatus.Status = status
		if status == "failed" {
			msgStatus.Retries++
			msgStatus.LastAttempt = time.Now()
		}
	}
}

func (mm *MessageManager) GetMessage(msgID string) (*MessageStatus, bool) {
	mm.mu.RLock()
	defer mm.mu.RUnlock()
	msgStatus, exists := mm.messages[msgID]
	return msgStatus, exists
}

func (mm *MessageManager) RemoveMessage(msgID string) {
	mm.mu.Lock()
	defer mm.mu.Unlock()
	delete(mm.messages, msgID)
}

func (mm *MessageManager) GetPendingMessages() []*MessageStatus {
	mm.mu.RLock()
	defer mm.mu.RUnlock()
	var pending []*MessageStatus
	for _, msgStatus := range mm.messages {
		if msgStatus.Status == "pending" ||
			(msgStatus.Status == "failed" &&
				msgStatus.Retries < MaxMessageRetryCount &&
				time.Since(msgStatus.LastAttempt) >= MessageRetryInterval) {
			pending = append(pending, msgStatus)
		}
	}
	return pending
}

// GetAllMessages 获取所有消息
func (mm *MessageManager) GetAllMessages() []*MessageStatus {
	mm.mu.RLock()
	defer mm.mu.RUnlock()
	var allMessages []*MessageStatus
	for _, msgStatus := range mm.messages {
		allMessages = append(allMessages, msgStatus)
	}
	return allMessages
}

func StartTCPServer() {
	listener, err := net.Listen("tcp", ":"+listenPort)
	if err != nil {
		fmt.Printf("启动TCP服务失败: %v\n", err)
		return
	}
	defer func(listener net.Listener) {
		err := listener.Close()
		if err != nil {

		}
	}(listener)

	// 启动消息重试处理器
	go messageRetryProcessor()

	fmt.Printf("TCP服务已启动，监听端口: %s\n", listenPort)

	for {
		conn, err := listener.Accept()
		if err != nil {
			fmt.Printf("接收客户端连接失败: %v\n", err)
			continue
		}
		// 并发处理连接
		go handleClient(conn)
	}
}

// 消息重试处理器
func messageRetryProcessor() {
	for {
		// 每30秒检查一次需要重试的消息
		time.Sleep(30 * time.Second)

		pendingMessages := msgManager.GetPendingMessages()
		if len(pendingMessages) == 0 {
			continue
		}

		for _, msgStatus := range pendingMessages {
			// 检查是否超过最大重试次数
			if msgStatus.Retries >= MaxMessageRetryCount {
				fmt.Printf("消息[%s]超过最大重试次数，标记为永久失败\n", msgStatus.Message.Payload.ID)
				msgManager.UpdateMessageStatus(msgStatus.Message.Payload.ID, "permanent_failed")

				// 通知发送方消息发送失败
				if senderConn, exists := connManager.GetConnection(msgStatus.SenderUUID); exists {
					errorNote := &protocol.Note{
						Op: "Notes",
						Payload: protocol.NotePayload{
							Type:      "error",
							Code:      "0013",
							Timestamp: time.Now().UnixMilli(),
						},
					}
					sendResponse(senderConn, errorNote)
				}
				continue
			}

			// 检查消息是否超时
			if time.Since(msgStatus.LastAttempt) >= MessageForwardTimeout {
				fmt.Printf("消息[%s]转发超时，标记为失败\n", msgStatus.Message.Payload.ID)
				msgManager.UpdateMessageStatus(msgStatus.Message.Payload.ID, "failed")
				continue
			}

			// 尝试重新发送消息
			fmt.Printf("尝试重新发送消息[%s]，重试次数: %d\n", msgStatus.Message.Payload.ID, msgStatus.Retries+1)
			forwardMessage(msgStatus.Message, msgStatus.SenderUUID)
		}
	}
}

// 处理客户端连接的核心函数
func handleClient(conn net.Conn) {
	// 强制首次认证检查
	isAuthenticated := false
	uuid := ""

	defer func() {
		fmt.Printf("客户端[%s]连接关闭\n", conn.RemoteAddr())
		// 如果已认证，从连接管理器中移除
		if isAuthenticated {
			connManager.RemoveConnection(uuid)
		}
		conn.Close()
	}()

	clientAddr := conn.RemoteAddr().String()
	fmt.Printf("客户端[%s]已连接\n", clientAddr)
	reader := bufio.NewReader(conn)

	for {
		data, err := reader.ReadString('\n')
		if err != nil {
			// 正常断开或错误都退出循环
			return
		}

		reqData := strings.TrimSpace(data)
		if reqData == "" {
			continue
		}

		// 调试日志
		fmt.Printf("收到客户端[%s]请求: %s\n", clientAddr, reqData)

		// 1. 解析 op 类型
		var baseReq struct {
			Op string `json:"op"`
		}
		if err := json.Unmarshal([]byte(reqData), &baseReq); err != nil {
			sendErrorNote(conn, "0001", "请求格式错误，非合法JSON")
			continue
		}

		// 检查是否已认证
		if !isAuthenticated {
			// 如果不是认证请求，则拒绝处理其他请求
			if baseReq.Op != "auth" {
				sendErrorNote(conn, "0010", "未授权：必须先进行身份验证")
				continue
			}

			// 处理认证请求
			authResult, authUUID := handleAuthRequest(conn, reqData)
			if authResult {
				isAuthenticated = true
				uuid = authUUID
				fmt.Printf("客户端[%s]认证成功，UUID: %s\n", clientAddr, uuid)
				// 将连接添加到连接管理器
				connManager.AddConnection(uuid, conn)
				// 跳过当前认证请求的分发处理，等待下一个请求
				continue
			} else {
				// 认证失败，继续等待认证或关闭连接
				fmt.Printf("客户端[%s]认证失败\n", clientAddr)
				continue
			}
		} else {
			// 已认证的情况下，检查UUID是否仍然有效
			storedIP, exists := UUID2IP.GetTargetIPFromRedis(uuid)
			if !exists || storedIP != clientAddr {
				sendErrorNote(conn, "0011", "会话失效：UUID与IP不匹配")
				return
			}
		}

		// 2. 分发处理（仅在认证后允许）
		switch baseReq.Op {
		case "Beat", "Ping", "Pong":
			handleBeatRequest(conn, reqData)
		case "Message":
			handleMessageRequest(conn, reqData, uuid)
		case "MessageAck":
			// 处理消息确认
			var ackReq struct {
				Op      string `json:"op"`
				Payload struct {
					MessageID string `json:"message_id"`
				} `json:"payload"`
			}
			if err := json.Unmarshal([]byte(reqData), &ackReq); err == nil {
				fmt.Printf("收到消息[%s]的确认\n", ackReq.Payload.MessageID)
				// 更新消息状态为已送达
				msgManager.UpdateMessageStatus(ackReq.Payload.MessageID, "delivered")
				// 从消息管理器中移除已送达的消息
				msgManager.RemoveMessage(ackReq.Payload.MessageID)
			}
		default:
			sendErrorNote(conn, "0002", fmt.Sprintf("不支持的op类型: %s", baseReq.Op))
		}
	}
}

// 处理 Beat/Ping/Pong 请求
// 原样回显客户端的时间戳，让客户端计算延迟
func handleBeatRequest(conn net.Conn, reqData string) {
	beatReq, err := protocol.BeatFromJSON([]byte(reqData))
	if err != nil {
		sendErrorNote(conn, "0003", "Beat请求解析失败")
		return
	}

	// 构建响应
	// Timestamp 直接使用 beatReq.Payload.Timestamp
	beatResp := &protocol.Beat{
		Op: "Pong", // 通常 Ping 回复 Pong，Beat 回复 Beat 或 Pong
		Payload: protocol.BeatPayload{
			Version:   beatReq.Payload.Version,   // 回显版本
			Timestamp: beatReq.Payload.Timestamp, // 原样回显客户端时间戳
			Status:    "success",
			Delay:     0, // 服务器不计算单向延迟，留给客户端算 RTT
		},
	}

	// 如果是 Beat 请求，有些协议约定回 "Beat"，有些回 "Pong"
	if beatReq.Op == "Beat" {
		beatResp.Op = "Beat"
	}

	sendResponse(conn, beatResp)
}

// 处理 Message 请求
func handleMessageRequest(conn net.Conn, reqData string, senderUUID string) {
	msgReq, err := protocol.MessageFromJSON([]byte(reqData))
	if err != nil {
		sendErrorNote(conn, "0005", "Message请求解析失败")
		return
	}

	// 验证消息发送者是否与认证UUID一致
	if msgReq.Payload.Form != senderUUID {
		sendErrorNote(conn, "0012", "消息验证失败：发送者UUID与认证UUID不匹配")
		return
	}

	// 生成消息ID（如果没有）
	if msgReq.Payload.ID == "" {
		msgReq.Payload.ID = fmt.Sprintf("msg_%d", time.Now().UnixNano())
	}

	// 将消息添加到消息管理器进行跟踪
	msgManager.AddMessage(msgReq.Payload.ID, msgReq, senderUUID)

	// 构建 ACK 响应给发送方
	ackNote := &protocol.Note{
		Op: "Notes",
		Payload: protocol.NotePayload{
			Type:      "ack",
			Code:      "0000",
			Timestamp: msgReq.Payload.Timestamp, // 回显消息的时间戳，便于客户端追踪
		},
	}
	sendResponse(conn, ackNote)

	// 转发消息
	forwardMessage(msgReq, senderUUID)
}

// 处理 Auth 请求 - 返回认证结果和UUID
func handleAuthRequest(conn net.Conn, reqData string) (bool, string) {
	authReq, err := protocol.AuthFromJSON([]byte(reqData))
	if err != nil {
		sendErrorNote(conn, "0007", "Auth请求解析失败")
		return false, ""
	}

	// 简单鉴权逻辑
	if authReq.Payload.ID == "" || authReq.Payload.Token == "" {
		sendErrorNote(conn, "0008", "鉴权失败：ID或Token为空")
		return false, ""
	}

	clientIP := UUID2IP.GetIPFromConn(conn)
	// 使用请求级上下文（超时 5 秒）
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	// 调用 Redis 存储（替换原内存操作）
	redis.SetUUIDIP(ctx, authReq.Payload.ID, clientIP, 24*time.Hour)

	fmt.Printf("已绑定 UUID[%s] ↔ IP[%s]\n", authReq.Payload.ID, clientIP)

	// 鉴权成功响应
	successNote := &protocol.Note{
		Op: "Notes",
		Payload: protocol.NotePayload{
			Type:      "ack",
			Code:      "0000",
			Timestamp: time.Now().UnixMilli(),
		},
	}

	sendResponse(conn, successNote)
	fmt.Printf("客户端[%s]鉴权成功\n", conn.RemoteAddr())

	// 客户端重新上线后，立即检查并投递离线消息
	go func(uuid string) {
		// 等待连接被添加到连接管理器
		time.Sleep(100 * time.Millisecond)

		// 获取所有消息
		allMessages := msgManager.GetAllMessages()
		if len(allMessages) == 0 {
			return
		}

		// 检查是否有待发给该客户端的失败消息
		for _, msgStatus := range allMessages {
			if msgStatus.Message.Payload.To == uuid &&
				(msgStatus.Status == "failed" || msgStatus.Status == "pending") &&
				msgStatus.Retries < MaxMessageRetryCount {
				fmt.Printf("客户端[%s]重新上线，立即投递离线消息[%s]，状态: %s\n",
					uuid, msgStatus.Message.Payload.ID, msgStatus.Status)
				forwardMessage(msgStatus.Message, msgStatus.SenderUUID)
			}
		}
	}(authReq.Payload.ID)

	return true, authReq.Payload.ID
}

// 通用发送响应函数 (支持泛型或接口，这里简化为直接写入 JSON)
func sendResponse(conn net.Conn, data interface{}) {
	var jsonData []byte
	var err error

	switch v := data.(type) {
	case *protocol.Beat:
		jsonData, err = v.ToJSON()
	case *protocol.Note:
		jsonData, err = v.ToJSON()
	case *protocol.Message:
		jsonData, err = v.ToJSON()
	default:
		fmt.Println("未知响应类型")
		return
	}

	if err != nil {
		fmt.Printf("序列化响应失败: %v\n", err)
		return
	}

	// 追加换行符
	_, err = conn.Write(append(jsonData, '\n'))
	if err != nil {
		fmt.Printf("发送响应失败: %v\n", err)
	}
}

// 消息转发核心函数
func forwardMessage(msg *protocol.Message, senderUUID string) {
	// 从Message的To字段获取目标UUID
	targetUUID := msg.Payload.To

	// 从连接管理器获取目标客户端连接
	targetConn, exists := connManager.GetConnection(targetUUID)
	if !exists {
		// 如果连接管理器中没有找到，尝试从Redis获取IP
		_, redisExists := UUID2IP.GetTargetIPFromRedis(targetUUID)
		if !redisExists {
			fmt.Printf("转发失败：未找到UUID[%s]对应的客户端\n", targetUUID)
			msgManager.UpdateMessageStatus(msg.Payload.ID, "failed")
			return
		}

		// 连接不存在，标记为失败，等待重试
		fmt.Printf("转发失败：客户端[%s]当前未连接\n", targetUUID)
		msgManager.UpdateMessageStatus(msg.Payload.ID, "failed")
		return
	}

	// 发送消息给目标客户端
	sendResponse(targetConn, msg)

	// 更新消息状态为已发送
	msgManager.UpdateMessageStatus(msg.Payload.ID, "sent")

	// 启动超时检测协程
	go func() {
		time.Sleep(MessageForwardTimeout)

		// 检查消息状态
		if msgStatus, exists := msgManager.GetMessage(msg.Payload.ID); exists {
			if msgStatus.Status == "sent" {
				// 如果消息仍处于已发送状态，说明超时未收到确认
				fmt.Printf("消息[%s]转发超时，未收到确认\n", msg.Payload.ID)
				msgManager.UpdateMessageStatus(msg.Payload.ID, "failed")
			}
		}
	}()
}

// 发送错误 Note
func sendErrorNote(conn net.Conn, code, msg string) {
	errorNote := &protocol.Note{
		Op: "Notes",
		Payload: protocol.NotePayload{
			Type:      "error",
			Code:      code,
			Timestamp: time.Now().UnixMilli(), // 错误时间用服务器时间即可
		},
	}

	jsonData, err := errorNote.ToJSON()
	if err != nil {
		fmt.Printf("构建错误Note失败: %v\n", err)
		return
	}

	_, err = conn.Write(append(jsonData, '\n'))
	if err != nil {
		fmt.Printf("发送错误Note失败: %v\n", err)
	} else {
		fmt.Printf("发送错误响应 [%s]: %s\n", code, msg)
	}
}
