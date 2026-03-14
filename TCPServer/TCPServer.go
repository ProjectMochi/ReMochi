package TCPServer

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"strings"
	"time"

	// 假设你的 protocol 包在这个路径，请根据实际项目结构调整
	// "ReMochi/protocol"
	// 由于我无法直接修改你的本地 import，这里假设 protocol 包已正确引入
	// 如果编译报错，请确保 protocol 包包含 Beat, Message, Auth, Note 结构体及对应的 FromJSON/ToJSON 方法
	"ReMochi/protocol"
)

const listenPort = "17890" // 修改为示例中的 8888

func StartTCPServer() {
	listener, err := net.Listen("tcp", ":"+listenPort)
	if err != nil {
		fmt.Printf("启动TCP服务失败: %v\n", err)
		return
	}
	defer listener.Close()

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

		// 2. 分发处理
		switch baseReq.Op {
		case "Beat", "Ping", "Pong":
			handleBeatRequest(conn, reqData)
		case "Message":
			handleMessageRequest(conn, reqData)
		case "auth":
			handleAuthRequest(conn, reqData)
		default:
			sendErrorNote(conn, "0002", fmt.Sprintf("不支持的op类型: %s", baseReq.Op))
		}
	}
}

// 处理 Beat/Ping/Pong 请求
// 核心逻辑：原样回显客户端的时间戳，让客户端计算延迟
func handleBeatRequest(conn net.Conn, reqData string) {
	beatReq, err := protocol.BeatFromJSON([]byte(reqData))
	if err != nil {
		sendErrorNote(conn, "0003", "Beat请求解析失败")
		return
	}

	// 构建响应
	// 关键点：Timestamp 直接使用 beatReq.Payload.Timestamp，不要换成 time.Now()
	beatResp := &protocol.Beat{
		Op: "Pong", // 通常 Ping 回复 Pong，Beat 回复 Beat 或 Pong，视协议约定而定
		Payload: protocol.BeatPayload{
			Version:   beatReq.Payload.Version,   // 回显版本
			Timestamp: beatReq.Payload.Timestamp, // <--- 关键：原样回显客户端时间戳
			Status:    "success",
			Delay:     0, // 服务器不计算单向延迟，留给客户端算 RTT
		},
	}

	// 如果是 Beat 请求，有些协议约定回 "Beat"，有些回 "Pong"，这里根据需求可调整
	if beatReq.Op == "Beat" {
		beatResp.Op = "Beat"
	}

	sendResponse(conn, beatResp)
}

// 处理 Message 请求
func handleMessageRequest(conn net.Conn, reqData string) {
	msgReq, err := protocol.MessageFromJSON([]byte(reqData))
	if err != nil {
		sendErrorNote(conn, "0005", "Message请求解析失败")
		return
	}

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

// 处理 Auth 请求
func handleAuthRequest(conn net.Conn, reqData string) {
	authReq, err := protocol.AuthFromJSON([]byte(reqData))
	if err != nil {
		sendErrorNote(conn, "0007", "Auth请求解析失败")
		return
	}

	// 简单鉴权逻辑
	if authReq.Payload.ID == "" || authReq.Payload.Token == "" {
		sendErrorNote(conn, "0008", "鉴权失败：ID或Token为空")
		return
	}

	// 鉴权成功
	successNote := &protocol.Note{
		Op: "Notes",
		Payload: protocol.NotePayload{
			Type:      "ack",
			Code:      "0000",
			Timestamp: time.Now().UnixMilli(), // 回显请求时间戳
		},
	}

	sendResponse(conn, successNote)
	fmt.Printf("客户端[%s]鉴权成功\n", conn.RemoteAddr())
}

// 通用发送响应函数 (支持泛型或接口，这里简化为直接写入 JSON)
// 注意：这里假设 beatResp, note 等结构体都有 ToJSON 方法
func sendResponse(conn net.Conn, data interface{}) {
	var jsonData []byte
	var err error

	// 尝试调用 ToJSON 方法 (需要 type assertion 或者接口约束，这里写死逻辑适配你的 protocol 包)
	// 由于 Go 是静态类型，这里需要根据传入类型分别处理，或者 protocol 包实现了统一接口
	// 假设你的 protocol 包结构体都有 ToJSON() ([]byte, error) 方法

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
			// 如果需要，可以把 msg 放入某个字段，但根据你的 Note 结构似乎只有 type/code/timestamp
			// 如果 protocol.NotePayload 有 Message 字段，可以加上
		},
	}

	// 注意：你的 Note 结构体定义里似乎没有直接存 msg 的字段？
	// 如果有，建议加上。如果没有，通常 Code 就代表了错误含义。
	// 这里为了演示，假设 Code 足够，或者你需要修改 protocol 包增加 Message 字段。

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
