package main

import (
	"ReMochi/internal/TCPServer"
	"ReMochi/internal/redis"
	"fmt"
)

// 17890

func main() {
	// 1. 初始化 Redis 连接
	if err := redis.InitRedis(); err != nil {
		fmt.Printf("Redis初始化失败，服务启动异常: %v\n", err)
		return
	}

	// 2. 启动 TCP 服务
	TCPServer.StartTCPServer()
}
