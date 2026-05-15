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
	dataDir := "./cache_data"
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		log.Printf("Failed to create cache data directory: %v", err)
		return &LedisCache{}
	}

	cfg := config.NewConfigDefault()
	cfg.DataDir = dataDir

	l, err := ledis.Open(cfg)
	if err != nil {
		log.Printf("Failed to open ledis: %v", err)
		return &LedisCache{}
	}

	db, err := l.Select(0)
	if err != nil {
		log.Printf("Failed to select database: %v", err)
		l.Close()
		return &LedisCache{}
	}

	return &LedisCache{ledis: l, db: db}
}

// SetEx 设置键值对，过期时间以小时为单位
func (c *LedisCache) SetEx(key string, value interface{}, hours int64) {
	if c == nil || c.db == nil {
		return
	}

	var valueBytes []byte
	switch v := value.(type) {
	case string:
		valueBytes = []byte(v)
	case []byte:
		valueBytes = v
	default:
		valueBytes = []byte(fmt.Sprintf("%v", v))
	}

	if err := c.db.Set([]byte(key), valueBytes); err != nil {
		log.Printf("Failed to set key %s: %v", key, err)
		return
	}

	duration := hours * 3600
	if _, err := c.db.Expire([]byte(key), duration); err != nil {
		log.Printf("Failed to set expiration for key %s: %v", key, err)
	}
}

// Get 获取键对应的值
func (c *LedisCache) Get(key string) (interface{}, bool) {
	if c == nil || c.db == nil {
		return nil, false
	}

	value, err := c.db.Get([]byte(key))
	if err != nil {
		log.Printf("Failed to get key %s: %v", key, err)
		return nil, false
	}
	if value == nil {
		return nil, false
	}

	ttl, err := c.db.TTL([]byte(key))
	if err != nil {
		log.Printf("Failed to get TTL for key %s: %v", key, err)
		return nil, false
	}
	if ttl == -1 || ttl == -2 {
		return nil, false
	}

	return string(value), true
}

// Close 关闭缓存连接
func (c *LedisCache) Close() error {
	if c != nil && c.ledis != nil {
		c.ledis.Close()
	}
	return nil
}
