package main

import (
	"fmt"
	"log"
	"slices"

	"github.com/markcheno/go-talib"
	"github.com/samber/lo"
	"gorm.io/gorm"
)

type KeyPair struct {
	Key   string
	Value float64
}

// CheckAllSymbolsMACDBullishCross 检查所有代币的MACD水上金叉
func CheckAllSymbolsRSI(db *gorm.DB) error {
	// 存储出现水上金叉的代币
	bullishCrossSymbols := []KeyPair{}

	// 遍历所有代币
	for _, symbol := range symbols {
		klines := getAggKline(db, symbol, "4h", 30)

		// 检查是否有足够的数据
		if len(klines) < 10 { // 至少需要26个数据点来计算MACD
			continue
		}
		// 提取收盘价
		closingPrices := make([]float64, len(klines))
		for i, kline := range klines {
			closingPrices[i] = kline.Close
		}
		slices.Reverse(closingPrices)
		// 计算MACD
		emas := talib.Rsi(closingPrices, 6)
		lastRsi, _ := lo.Last(emas)
		if lastRsi >= 75 || lastRsi <= 25 {
			// 检查缓存中是否已经有这个代币的水上金叉记录
			cacheKey := "rsi_" + symbol
			if _, exists := cache.Get(cacheKey); !exists {
				// 如果缓存中没有记录，则添加到结果中，并设置4小时的缓存
				bullishCrossSymbols = append(bullishCrossSymbols, KeyPair{symbol, lastRsi})
				cache.SetEx(cacheKey, true, 4) // 设置4小时有效期
			}
		}

	}
	// 如果有代币出现水上金叉，发送到Telegram
	if len(bullishCrossSymbols) > 0 {
		message := "以下代币出现RSI异动：\n"
		for _, k := range bullishCrossSymbols {
			message += fmt.Sprintf("- %s (%.2f)\n", k.Key, k.Value)
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
