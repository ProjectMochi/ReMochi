package redis

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// RedisClient Redis 客户端单例
var RedisClient *redis.Client

// InitRedis 初始化 Redis 连接
func InitRedis() error {
	RedisClient = redis.NewClient(&redis.Options{
		Addr:     "127.0.0.1:6379", // Redis 地址
		Password: "",               // 无密码则为空
		DB:       0,                // 使用第0个数据库
		PoolSize: 10,               // 连接池大小
	})

	// 测试连接
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_, err := RedisClient.Ping(ctx).Result()
	if err != nil {
		return fmt.Errorf("redis 连接失败: %v", err)
	}
	fmt.Println("redis 连接成功")
	return nil
}

// SetUUIDIP 存储 UUID-IP 映射（带过期时间，可选）
func SetUUIDIP(ctx context.Context, uuid, ip string, expire time.Duration) error {
	return RedisClient.Set(ctx, fmt.Sprintf("uuid2ip:%s", uuid), ip, expire).Err()
}

// GetIPByUUID 根据 UUID 获取 IP
func GetIPByUUID(ctx context.Context, uuid string) (string, bool) {
	ip, err := RedisClient.Get(ctx, fmt.Sprintf("uuid2ip:%s", uuid)).Result()
	if err == redis.Nil {
		return "", false // key 不存在
	} else if err != nil {
		fmt.Printf("redis 获取IP失败: %v\n", err)
		return "", false
	}
	return ip, true
}

// DeleteUUIDIP 删除 UUID-IP 映射
func DeleteUUIDIP(ctx context.Context, uuid string) error {
	return RedisClient.Del(ctx, fmt.Sprintf("uuid2ip:%s", uuid)).Err()
}

// GetUUIDByIP 反向根据 IP 获取 UUID（遍历，性能较低，非高频场景可用）
func GetUUIDByIP(ctx context.Context, ip string) (string, bool) {
	// 匹配所有 uuid2ip:* 键
	iter := RedisClient.Scan(ctx, 0, "uuid2ip:*", 0).Iterator()
	for iter.Next(ctx) {
		key := iter.Val()
		val, err := RedisClient.Get(ctx, key).Result()
		if err != nil {
			continue
		}
		if val == ip {
			// 提取 UUID（去掉前缀 uuid2ip:）
			uuid := key[len("uuid2ip:"):]
			return uuid, true
		}
	}
	if err := iter.Err(); err != nil {
		fmt.Printf("redis 遍历失败: %v\n", err)
	}
	return "", false
}
