package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	"gorm.io/gorm"
)

// CacheItem 缓存项
type CacheItem struct {
	Value      interface{}
	Expiration int64
}

// SimpleCache 简单的内存缓存
type SimpleCache struct {
	items map[string]CacheItem
	mutex sync.RWMutex
}

// NewSimpleCache 创建新的缓存实例
func NewSimpleCache() *SimpleCache {
	cache := &SimpleCache{
		items: make(map[string]CacheItem),
	}
	// 启动清理过期缓存的goroutine
	go cache.cleanup()
	return cache
}

// SetEx 设置键值对，过期时间以小时为单位
func (c *SimpleCache) SetEx(key string, value interface{}, hours int64) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	
	expiration := time.Now().Add(time.Hour * time.Duration(hours)).Unix()
	c.items[key] = CacheItem{
		Value:      value,
		Expiration: expiration,
	}
}

// Get 获取键对应的值
func (c *SimpleCache) Get(key string) (interface{}, bool) {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	
	item, exists := c.items[key]
	if !exists {
		return nil, false
	}
	
	// 检查是否过期
	if time.Now().Unix() > item.Expiration {
		return nil, false
	}
	
	return item.Value, true
}

// cleanup 清理过期的缓存项
func (c *SimpleCache) cleanup() {
	ticker := time.NewTicker(time.Hour) // 每小时清理一次
	defer ticker.Stop()
	
	for {
		<-ticker.C
		c.mutex.Lock()
		
		now := time.Now().Unix()
		for key, item := range c.items {
			if now > item.Expiration {
				delete(c.items, key)
			}
		}
		
		c.mutex.Unlock()
	}
}

// 全局缓存实例
var cache = NewSimpleCache()

// EMA 计算指数移动平均线
func EMA(data []float64, period int) []float64 {
	if len(data) < period {
		return make([]float64, len(data))
	}

	multiplier := 2.0 / float64(period+1)
	ema := make([]float64, len(data))

	// 初始化第一个EMA值为简单移动平均
	sum := 0.0
	for i := 0; i < period; i++ {
		sum += data[i]
	}
	ema[period-1] = sum / float64(period)

	// 计算后续EMA值
	for i := period; i < len(data); i++ {
		ema[i] = (data[i]-ema[i-1])*multiplier + ema[i-1]
	}

	return ema
}

// MACD 计算MACD指标
func MACD(data []float64) (macdLine []float64, signalLine []float64, histogram []float64) {
	// 计算12日EMA和26日EMA
	ema12 := EMA(data, 12)
	ema26 := EMA(data, 26)

	// 计算MACD线 (12日EMA - 26日EMA)
	macdLine = make([]float64, len(data))
	for i := 0; i < len(data); i++ {
		macdLine[i] = ema12[i] - ema26[i]
	}

	// 计算信号线 (MACD线的9日EMA)
	signalLine = EMA(macdLine, 9)

	// 计算柱状图 (MACD线 - 信号线)
	histogram = make([]float64, len(data))
	for i := 0; i < len(data); i++ {
		histogram[i] = macdLine[i] - signalLine[i]
	}

	return macdLine, signalLine, histogram
}

// IsBullishCross 检查是否出现MACD水上金叉
// 水上金叉：MACD线从下方向上穿过信号线，且两条线都在零轴之上
func IsBullishCross(macdLine []float64, signalLine []float64) bool {
	if len(macdLine) < 2 || len(signalLine) < 2 {
		return false
	}

	// 检查当前和前一个周期的数据
	currentMacd := macdLine[len(macdLine)-1]
	previousMacd := macdLine[len(macdLine)-2]
	currentSignal := signalLine[len(signalLine)-1]
	previousSignal := signalLine[len(signalLine)-2]

	// 条件1：两条线都在零轴之上
	if currentMacd <= 0 || currentSignal <= 0 {
		return false
	}

	// 条件2：MACD线从下方向上穿过信号线
	// 前一个周期：MACD线 < 信号线
	// 当前周期：MACD线 > 信号线
	if previousMacd < previousSignal && currentMacd > currentSignal {
		return true
	}

	return false
}

func TelegramSendMessage(message string) error {
	botToken := os.Getenv("TELEGRAM_BOT_TOKEN")
	chatID := os.Getenv("TELEGRAM_CHAT_ID")

	if botToken == "" || chatID == "" {
		return nil
	}

	url := "https://api.telegram.org/bot" + botToken + "/sendMessage"

	data := map[string]string{
		"chat_id": chatID,
		"text":    message,
	}

	jsonData, err := json.Marshal(data)
	if err != nil {
		return err
	}

	// 发送POST请求
	resp, err := http.Post(url, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// 检查响应状态
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("发送Telegram消息失败，状态码: %d", resp.StatusCode)
	}

	return nil
}

// CheckAllSymbolsMACDBullishCross 检查所有代币的MACD水上金叉
func CheckAllSymbolsMACDBullishCross(db *gorm.DB) error {
	// 从 symbols.json 读取 symbols
	symbols, err := loadSymbolsFromFile("symbols.json")
	if err != nil {
		return fmt.Errorf("读取 symbols.json 失败: %v", err)
	}

	// 存储出现水上金叉的代币
	var bullishCrossSymbols []string

	// 遍历所有代币
	for _, symbol := range symbols {
		// 从数据库获取K线数据
		var klines []Kline
		result := db.Where("symbol = ?", symbol).Order("open_time ASC").Limit(100).Find(&klines)
		if result.Error != nil {
			log.Printf("获取 %s 的K线数据失败: %v", symbol, result.Error)
			continue
		}

		// 检查是否有足够的数据
		if len(klines) < 26 { // 至少需要26个数据点来计算MACD
			continue
		}

		// 提取收盘价
		closingPrices := make([]float64, len(klines))
		for i, kline := range klines {
			closingPrices[i] = kline.Close
		}

		// 计算MACD
		macdLine, signalLine, _ := MACD(closingPrices)

		// 检查是否出现水上金叉
		if IsBullishCross(macdLine, signalLine) {
			// 检查缓存中是否已经有这个代币的水上金叉记录
			cacheKey := "bullish_cross_" + symbol
			if _, exists := cache.Get(cacheKey); !exists {
				// 如果缓存中没有记录，则添加到结果中，并设置4小时的缓存
				bullishCrossSymbols = append(bullishCrossSymbols, symbol)
				cache.SetEx(cacheKey, true, 4) // 设置4小时有效期
			}
		}
	}

	// 如果有代币出现水上金叉，发送到Telegram
	if len(bullishCrossSymbols) > 0 {
		message := "以下代币在5分钟级别出现MACD水上金叉：\n"
		for _, symbol := range bullishCrossSymbols {
			message += "- " + symbol + "\n"
		}

		// 发送到Telegram
		if err := TelegramSendMessage(message); err != nil {
			log.Printf("发送Telegram消息失败: %v", err)
		} else {
			log.Printf("已发送Telegram消息，内容: %s", message)
		}
	}

	return nil
}
