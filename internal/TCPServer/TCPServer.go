package TCPServer

import (
	"ReMochi/internal/UUID2IP"
	"ReMochi/internal/protocol"
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"strings"
	"time"
)

const listenPort = "17890"

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

// 处理客户端连接的核心函数
func handleClient(conn net.Conn) {
	defer func() {
		fmt.Printf("客户端[%s]连接关闭\n", conn.RemoteAddr())
		conn.Close()
	}()

	clientAddr := conn.RemoteAddr().String()
	fmt.Printf("客户端[%s]已连接\n", clientAddr)
	reader := bufio.NewReader(conn)

	// 强制首次认证检查
	isAuthenticated := false
	uuid := ""

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
			} else {
				// 认证失败，继续等待认证或关闭连接
				fmt.Printf("客户端[%s]认证失败\n", clientAddr)
				continue
			}
		} else {
			// 已认证的情况下，检查UUID是否仍然有效
			storedIP, exists := UUID2IP.GetGlobalMap().GetIPByUUID(uuid)
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

	// 从Message的To字段获取目标UUID
	targetUUID := msgReq.Payload.To
	// 根据UUID查找目标IP
	targetIP, exists := UUID2IP.GetGlobalMap().GetIPByUUID(targetUUID)
	if !exists {
		fmt.Printf("转发失败：未找到UUID[%s]对应的IP\n", targetUUID)
		// 可选：给发送方返回转发失败的错误
		errorNote := &protocol.Note{
			Op: "Notes",
			Payload: protocol.NotePayload{
				Type:      "error",
				Code:      "0009",
				Timestamp: time.Now().UnixMilli(),
			},
		}
		sendResponse(conn, errorNote)
		return
	}

	// Now 模拟转发
	fmt.Printf("准备转发消息到 UUID[%s] → IP[%s]，消息内容：%s\n",
		targetUUID, targetIP, msgReq.Payload.Content)
	// TODO: finish

	// 构建 ACK 响应
	ackNote := &protocol.Note{
		Op: "Notes",
		Payload: protocol.NotePayload{
			Type:      "ack",
			Code:      "0000",
			Timestamp: msgReq.Payload.Timestamp, // 回显消息的时间戳，便于客户端追踪
		},
	}

	sendResponse(conn, ackNote)
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

	clientIP := UUID2IP.GetIPFromConn(conn) // 获取客户端IP（带端口）
	// 存入全局映射表：Key=鉴权ID(UUID)，Value=客户端IP
	UUID2IP.GetGlobalMap().Set(authReq.Payload.ID, clientIP)
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
