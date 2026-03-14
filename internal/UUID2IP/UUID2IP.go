package UUID2IP

import (
	"net"
	"sync"
)

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

// Set 新增/更新 UUID-IP 映射关系
// uuid: 目标UUID | ip: 客户端IP（建议带端口，如 conn.RemoteAddr().String()）
func (m *UUIDIPMap) Set(uuid string, ip string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.store[uuid] = ip
}

// GetIPByUUID 根据UUID获取对应的IP
// 返回: IP字符串（如 "127.0.0.1:8080"）| bool（是否存在该UUID）
func (m *UUIDIPMap) GetIPByUUID(uuid string) (string, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	ip, ok := m.store[uuid]
	return ip, ok
}

// Delete 根据UUID删除映射关系
func (m *UUIDIPMap) Delete(uuid string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.store, uuid)
}

// GetUUIDByIP 反向根据IP获取UUID（可选，按需使用）
func (m *UUIDIPMap) GetUUIDByIP(ip string) (string, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for uuid, storedIP := range m.store {
		if storedIP == ip {
			return uuid, true
		}
	}
	return "", false
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
