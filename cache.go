package main

import (
	"errors"
	"fmt"
	"log"
	"os"
	"sync"

	"github.com/ledisdb/ledisdb/config"
	"github.com/ledisdb/ledisdb/ledis"
)

// LedisCache 使用ledisdb实现的缓存
type LedisCache struct {
	mu    sync.RWMutex
	ledis *ledis.Ledis
	db    *ledis.DB
}

// NewLedisCache 创建新的缓存实例
func NewLedisCache() *LedisCache {
	cache, err := NewLedisCacheWithErr()
	if err != nil {
		log.Printf("Failed to initialize LedisCache: %v", err)
		return &LedisCache{}
	}
	return cache
}

// NewLedisCacheWithErr 创建新的缓存实例并返回错误
func NewLedisCacheWithErr() (*LedisCache, error) {
	dataDir := "./cache_data"
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return nil, fmt.Errorf("create cache data directory: %w", err)
	}

	cfg := config.NewConfigDefault()
	cfg.DataDir = dataDir

	l, err := ledis.Open(cfg)
	if err != nil {
		return nil, fmt.Errorf("open ledis: %w", err)
	}

	db, err := l.Select(0)
	if err != nil {
		l.Close()
		return nil, fmt.Errorf("select database: %w", err)
	}

	return &LedisCache{ledis: l, db: db}, nil
}

// SetEx 设置键值对，过期时间以小时为单位
func (c *LedisCache) SetEx(key string, value interface{}, hours int64) {
	if err := c.SetExWithErr(key, value, hours); err != nil {
		log.Printf("Failed to set key %s: %v", key, err)
	}
}

// SetExWithErr 设置键值对，过期时间以小时为单位并返回错误
func (c *LedisCache) SetExWithErr(key string, value interface{}, hours int64) error {
	if c == nil {
		return errors.New("cache is nil")
	}
	if key == "" {
		return errors.New("key is empty")
	}
	if hours <= 0 {
		return fmt.Errorf("invalid expiration hours: %d", hours)
	}

	c.mu.RLock()
	db := c.db
	c.mu.RUnlock()
	if db == nil {
		return errors.New("cache db is nil")
	}

	valueBytes := normalizeValue(value)

	if err := db.Set([]byte(key), valueBytes); err != nil {
		return fmt.Errorf("set value: %w", err)
	}

	duration := hours * 3600
	if _, err := db.Expire([]byte(key), duration); err != nil {
		return fmt.Errorf("set expiration: %w", err)
	}
	return nil
}

// Get 获取键对应的值
func (c *LedisCache) Get(key string) (interface{}, bool) {
	v, ok, err := c.GetString(key)
	if err != nil {
		log.Printf("Failed to get key %s: %v", key, err)
		return nil, false
	}
	if !ok {
		return nil, false
	}
	return v, true
}

// GetString 获取键对应的字符串值
func (c *LedisCache) GetString(key string) (string, bool, error) {
	if c == nil {
		return "", false, errors.New("cache is nil")
	}
	if key == "" {
		return "", false, errors.New("key is empty")
	}

	c.mu.RLock()
	db := c.db
	c.mu.RUnlock()
	if db == nil {
		return "", false, errors.New("cache db is nil")
	}

	value, err := db.Get([]byte(key))
	if err != nil {
		return "", false, fmt.Errorf("get value: %w", err)
	}
	if value == nil {
		return "", false, nil
	}

	ttl, err := db.TTL([]byte(key))
	if err != nil {
		return "", false, fmt.Errorf("get ttl: %w", err)
	}
	if ttl <= 0 {
		return "", false, nil
	}

	return string(value), true, nil
}

// Close 关闭缓存连接
func (c *LedisCache) Close() error {
	if c == nil {
		return nil
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.ledis != nil {
		c.ledis.Close()
		c.ledis = nil
	}
	c.db = nil
	return nil
}

func normalizeValue(value interface{}) []byte {
	switch v := value.(type) {
	case string:
		return []byte(v)
	case []byte:
		return v
	default:
		return []byte(fmt.Sprintf("%v", v))
	}
}
