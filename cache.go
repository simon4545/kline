package main

import (
	"fmt"
	"log"
	"os"

	"github.com/ledisdb/ledisdb/config"
	"github.com/ledisdb/ledisdb/ledis"
)

// LedisCache 使用ledisdb实现的缓存
type LedisCache struct {
	ledis *ledis.Ledis
	db    *ledis.DB
}

// NewLedisCache 创建新的缓存实例
func NewLedisCache() *LedisCache {
	// 创建数据目录
	dataDir := "./cache_data"
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		log.Fatal("Failed to create cache data directory:", err)
	}

	// 配置ledisdb
	cfg := config.NewConfigDefault()
	cfg.DataDir = dataDir

	// 创建ledis实例
	l, err := ledis.Open(cfg)
	if err != nil {
		log.Fatal("Failed to open ledis:", err)
	}

	// 选择数据库0
	db, err := l.Select(0)
	if err != nil {
		log.Fatal("Failed to select database:", err)
	}

	cache := &LedisCache{
		ledis: l,
		db:    db,
	}

	return cache
}

// SetEx 设置键值对，过期时间以小时为单位
func (c *LedisCache) SetEx(key string, value interface{}, hours int64) {
	// 将值转换为字节
	var valueBytes []byte
	switch v := value.(type) {
	case string:
		valueBytes = []byte(v)
	case []byte:
		valueBytes = v
	default:
		// 对于其他类型，转换为字符串
		valueBytes = []byte(fmt.Sprintf("%v", v))
	}

	// 设置键值对
	if err := c.db.Set([]byte(key), valueBytes); err != nil {
		log.Printf("Failed to set key %s: %v", key, err)
		return
	}

	// 设置过期时间（以秒为单位）
	duration := hours * 3600 // 转换为秒
	if _, err := c.db.Expire([]byte(key), duration); err != nil {
		log.Printf("Failed to set expiration for key %s: %v", key, err)
	}
}

// Get 获取键对应的值
func (c *LedisCache) Get(key string) (interface{}, bool) {
	value, err := c.db.Get([]byte(key))
	if err != nil {
		log.Printf("Failed to get key %s: %v", key, err)
		return nil, false
	}

	// 检查键是否存在
	if value == nil {
		return nil, false
	}

	// 检查是否过期
	ttl, err := c.db.TTL([]byte(key))
	if err != nil {
		log.Printf("Failed to get TTL for key %s: %v", key, err)
		return nil, false
	}

	// 如果TTL为-1，表示键已过期或不存在
	// 如果TTL为-2，表示键不存在
	if ttl == -1 || ttl == -2 {
		return nil, false
	}

	return string(value), true
}

// Close 关闭缓存连接
func (c *LedisCache) Close() error {
	if c.ledis != nil {
		c.ledis.Close()
	}
	return nil
}
