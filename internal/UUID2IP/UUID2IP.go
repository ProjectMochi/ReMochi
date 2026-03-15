package UUID2IP

import (
	"ReMochi/internal/redis"
	"context"
	"fmt"
	"net"
	"sync"
	"time"
)

// 全局上下文（可根据业务调整为请求级上下文）
var ctx = context.Background()

// Set 新增/更新 UUID-IP 映射（改用 Redis，过期时间 24h）
func Set(uuid string, ip string) {
	err := redis.SetUUIDIP(ctx, uuid, ip, 24*time.Hour)
	if err != nil {
		fmt.Printf("Redis存储UUID-IP失败: %v\n", err)
	}
}

// GetIPByUUID 根据UUID获取对应的IP
func GetIPByUUID(uuid string) (string, bool) {
	return redis.GetIPByUUID(ctx, uuid)
}

// Delete 根据UUID删除映射关系
func Delete(uuid string) {
	err := redis.DeleteUUIDIP(ctx, uuid)
	if err != nil {
		fmt.Printf("Redis删除UUID-IP失败: %v\n", err)
	}
}

// GetUUIDByIP 反向根据IP获取UUID
func GetUUIDByIP(ip string) (string, bool) {
	return redis.GetUUIDByIP(ctx, ip)
}

// UUIDIPMap 内存中的UUID-IP映射表，支持并发安全
type UUIDIPMap struct {
	mu sync.RWMutex // 读写锁，保证高并发下的安全
	// key: UUID(string) | value: 客户端IP(string)（格式如 "192.168.1.1:12345"）
	store map[string]string
}

// 全局单例（也可根据需求局部实例化）
var globalUUIDIPMap = NewUUIDIPMap()

// NewUUIDIPMap 创建新的UUID-IP映射实例
func NewUUIDIPMap() *UUIDIPMap {
	return &UUIDIPMap{
		store: make(map[string]string),
	}
}

// GetGlobalMap 获取全局UUID-IP映射实例
func GetGlobalMap() *UUIDIPMap {
	return globalUUIDIPMap
}

// Clear 清空所有映射（测试/重置场景用）
func (m *UUIDIPMap) Clear() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.store = make(map[string]string)
}

// GetIPFromConn 从net.Conn中提取客户端IP（封装通用方法）
// 返回: 客户端IP字符串（带端口，如 "192.168.1.1:54321"）
func GetIPFromConn(conn net.Conn) string {
	if conn == nil {
		return ""
	}
	return conn.RemoteAddr().String()
}

// GetPureIP 从带端口的IP字符串中提取纯IP（如 "192.168.1.1:54321" → "192.168.1.1"）
func GetPureIP(ipWithPort string) string {
	host, _, err := net.SplitHostPort(ipWithPort)
	if err != nil {
		return ipWithPort // 解析失败则返回原字符串
	}
	return host
}

// GetTargetIPFromRedis
func GetTargetIPFromRedis(targetUUID string) (string, bool) {
	// 1. 创建请求级上下文（推荐，避免全局上下文泄漏，设置5秒超时）
	_, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel() // 函数结束释放上下文

	// 2. 调用改造后的UUID2IP方法（底层已对接Redis）
	// 注意：我们之前改造的UUID2IP.GetIPByUUID已默认使用Redis，无需直接操作Redis客户端
	targetIP, exists := GetIPByUUID(targetUUID)

	// 3. （可选）打印日志，方便调试
	if !exists {
		println("Redis中未找到UUID[" + targetUUID + "]对应的IP")
	} else {
		println("从Redis获取到UUID[" + targetUUID + "] → IP[" + targetIP + "]")
	}

	return targetIP, exists
}
