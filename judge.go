package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"slices"

	"github.com/markcheno/go-talib"
	"github.com/samber/lo"
	"gorm.io/gorm"
)

// 全局缓存实例
var cache = NewLedisCache()

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
		result := db.Where("symbol = ?", symbol).Order("open_time desc").Limit(500).Find(&klines)
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
		slices.Reverse(closingPrices)
		// 计算MACD
		emas := talib.Ema(closingPrices, 144)
		emas = lo.Subset(emas, -5, 5)
		result1 := lo.ReduceRight(emas, func(agg int, item float64, idx int) int {
			if idx > 0 && item > emas[idx-1] {
				return agg + 1
			}
			return agg
		}, 0)
		if result1 >= 4 {
			macdLine, signalLine, _ := talib.Macd(closingPrices, 12, 26, 9)

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
