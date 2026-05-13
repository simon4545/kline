package main

import (
	"errors"
	"fmt"
	"log"
	"time"

	"gorm.io/gorm"
)

// updateKlines 更新指定 symbol 的 K 线数据
func updateKlines(db *gorm.DB, symbol string) error {
	const interval = "4h"
	periodMs := int64(4 * 3600 * 1000) // 4小时对应的毫秒数
	klineModel := Kline{Symbol: symbol}
	tableName := klineModel.TableName()

	// 1. 确定起始时间
	var last Kline
	res := db.Table(tableName).Order("open_time DESC").Limit(1).Find(&last)
	var startTime int64
	if res.RowsAffected == 0 {
		// 没有历史数据：从一年前开始拉取
		startTime = time.Now().AddDate(-1, 0, 0).UnixMilli()
	} else {
		// 已有数据：从最后一条的下一个周期开始（避免重复拉取）
		startTime = last.OpenTime + periodMs
	}

	nowMs := time.Now().UnixMilli()
	maxLoops := 10 // 一年最多2190条，循环3次足够，设10次作为安全上限

	for i := 0; i < maxLoops; i++ {
		if startTime > nowMs {
			break
		}

		// 2. 拉取最多1000条K线
		klines, err := fetchBinanceKlines(symbol, interval, startTime, 0, 1000)
		if err != nil {
			return fmt.Errorf("fetch klines for %s: %w", symbol, err)
		}
		if len(klines) == 0 {
			break
		}

		// 3. 写入/更新数据库
		for _, k := range klines {
			k.Symbol = symbol
			var existing Kline
			err := db.Table(tableName).Where("open_time = ?", k.OpenTime).First(&existing).Error
			if err == nil {
				// 已存在且未收盘 → 更新（通常只发生在最新一根未完整K线上）
				if existing.CloseTime > time.Now().UnixMilli() {
					db.Table(tableName).Model(&existing).Updates(k)
				}
			} else if errors.Is(err, gorm.ErrRecordNotFound) {
				// 不存在 → 插入
				db.Table(tableName).Create(&k)
			}
		}

		// 4. 更新下一次拉取的起始时间
		lastKline := klines[len(klines)-1]
		startTime = lastKline.OpenTime + periodMs

		// 5. 如果本次返回数量不足1000，说明已经是最新数据，结束循环
		if len(klines) < 1000 {
			break
		}

		// 避免币安API限频
		time.Sleep(200 * time.Millisecond)
	}

	return nil
}

// processSymbols 处理所有 symbols 的 K 线更新
func processSymbols(symbols []string, db *gorm.DB) error {
	for _, sym := range symbols {
		if err := updateKlines(db, sym); err != nil {
			log.Println("update error:", sym, err)
			return err
		}
		log.Println("updated", sym)
		time.Sleep(time.Millisecond * 400)
	}
	return nil
}

// clean 定时清理任务：删除旧数据和不需要的表
func clean(db *gorm.DB) {
	go func() {
		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()

		for {
			<-ticker.C
			symbols, err := loadSymbolsFromFile("symbols.json")
			if err != nil {
				log.Printf("读取 symbols.json 失败: %v", err)
				continue
			}

			symbolSet := make(map[string]struct{})
			for _, s := range symbols {
				symbolSet["kline_"+s] = struct{}{}
			}
			var tables []string
			err = db.Raw("SELECT name FROM sqlite_master WHERE type='table'").Scan(&tables).Error
			if err != nil {
				log.Printf("获取数据库表名失败: %v", err)
				continue
			}

			for _, table := range tables {
				if _, ok := symbolSet[table]; !ok && table != "sqlite_sequence" {
					log.Printf("删除表: %s", table)
					if err := db.Exec(fmt.Sprintf("DROP TABLE IF EXISTS `%s`", table)).Error; err != nil {
						log.Printf("删除表 %s 失败: %v", table, err)
					}
				}
			}

			cutoff := time.Now().AddDate(0, -6, 0).UnixMilli()
			totalDeleted := int64(0)

			for _, symbol := range symbols {
				kline := Kline{Symbol: symbol}
				res := db.Table(kline.TableName()).Where("open_time < ?", cutoff).Delete(&Kline{})
				if res.Error != nil {
					log.Printf("清理 %s 旧数据失败: %v", symbol, res.Error)
				} else {
					totalDeleted += res.RowsAffected
				}
			}

			log.Printf("清理旧数据完成: %d 条 (open_time < %d)\n", totalDeleted, cutoff)
		}
	}()
}