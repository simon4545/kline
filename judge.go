package main

import (
	"fmt"
	"log"
	"slices"
	"time"

	"github.com/markcheno/go-talib"
	"github.com/samber/lo"
	"gorm.io/gorm"
)

// 全局缓存实例
var cache = NewLedisCache()

// CheckAllSymbolsMACDBullishCross 检查所有代币的MACD水上金叉
func CheckAllSymbolsMACDBullishCross(db *gorm.DB) error {
	// 存储出现水上金叉的代币
	var bullishCrossSymbols []string

	// 遍历所有代币
	for _, symbol := range symbols {
		klines := getAggKline(db, symbol, "15m", 300)

		// 检查是否有足够的数据
		if len(klines) < 26 { // 至少需要26个数据点来计算MACD
			continue
		}
		klines = lo.Filter(klines, func(item Kline, index int) bool { return item.CloseTime < time.Now().UnixMilli() })
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
		lastema, ok := lo.Last(emas)
		if !ok {
			continue
		}
		lastprice, ok := lo.Last(closingPrices)
		if !ok {
			continue
		}
		//整体曲线向上
		if result1 >= 4 && lastprice > lastema {
			macdLine, signalLine, macdHint := talib.Macd(closingPrices, 12, 26, 9)
			count := lo.CountBy(lo.Subset(macdHint, -6, 6), func(i float64) bool {
				return i < 0
			})
			fmt.Println(symbol, count)

			// 检查是否出现水上金叉 并且之前的macd水下不超过5根
			if IsBullishCross(macdLine, signalLine) && count < 5 {
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
		message := "以下代币出现MACD水上金叉：\n"
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
