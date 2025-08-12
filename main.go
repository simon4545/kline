package main

import (
	"log"
	"net/http"
	"time"

	"golang.org/x/sync/errgroup"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// ================= 数据更新逻辑 =================
func updateKlines(db *gorm.DB, symbol string) error {
	var last Kline
	res := db.Where("symbol = ?", symbol).Order("open_time DESC").Limit(1).Find(&last)

	var startTime int64
	var limitCount int = 99
	if res.RowsAffected == 0 {
		limitCount = 499
		// 没有数据，从当前时间回溯 99*5m
		startTime = time.Now().Add(-time.Duration(2880*5) * time.Minute).UnixMilli()
	} else {
		// 有数据，从最新一条的时间开始拉取
		if time.Since(time.UnixMilli(last.OpenTime)) >= 5*time.Minute {
			startTime = last.OpenTime
		} else {
			return nil // 最新数据足够
		}
	}

	klines, err := fetchBinanceKlines(symbol, "5m", startTime, 0, limitCount)
	if err != nil {
		return err
	}

	for _, k := range klines {
		var existing Kline
		if err := db.Where("symbol = ? AND open_time = ?", k.Symbol, k.OpenTime).First(&existing).Error; err == nil {
			// 更新（收盘时间可能未完成）
			db.Model(&existing).Updates(k)
		} else {
			// 新增
			db.Create(&k)
		}
	}
	return nil
}

// ================= 主程序 =================
func main() {
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		log.Fatal(err)
	}

	db, err := gorm.Open(sqlite.Open("klines.db"), &gorm.Config{
		NowFunc: func() time.Time {
			return time.Now().In(loc)
		},
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		log.Fatal(err)
	}

	// 确认当前时间
	var now time.Time
	db.Raw("SELECT CURRENT_TIMESTAMP").Scan(&now)
	log.Println("当前时间（东八区）:", now)

	db.AutoMigrate(&Kline{})

	// 从 symbols.json 读取 symbols
	symbols, err := loadSymbolsFromFile("symbols.json")
	if err != nil {
		log.Fatalf("读取 symbols.json 失败: %v", err)
	}

	// 启动 HTTP 服务
	go func() {
		http.HandleFunc("/klines", handleKlineQuery(db))
		log.Println("HTTP server started on :3000")
		if err := http.ListenAndServe(":3000", nil); err != nil {
			log.Fatal(err)
		}
	}()
	// 定时清理任务：每24小时执行一次
	go func() {
		ticker := time.NewTicker(24 * time.Hour)
		defer ticker.Stop()

		for {
			<-ticker.C
			cutoff := time.Now().AddDate(0, -1, 0).UnixMilli()
			res := db.Where("open_time < ?", cutoff).Delete(&Kline{})
			if res.Error != nil {
				log.Println("清理旧数据失败:", res.Error)
			} else {
				log.Printf("清理旧数据完成: %d 条 (open_time < %d)\n", res.RowsAffected, cutoff)
			}
		}
	}()
	// 定时任务：每分钟更新一次
	ticker := time.NewTicker(1 * time.Minute)
	go func() {
		for range ticker.C {
			if err := processSymbols(symbols, db); err != nil {
				log.Println("部分任务失败:", err)
			}
		}
	}()

	select {}
}

func processSymbols(symbols []string, db *gorm.DB) error {
	var g errgroup.Group
	sem := make(chan struct{}, 2) // 限制并行 4 个

	for _, sym := range symbols {
		sym := sym // 避免闭包变量问题

		g.Go(func() error {
			sem <- struct{}{} // 占用一个并发槽
			defer func() { <-sem }()

			if err := updateKlines(db, sym); err != nil {
				log.Println("update error:", sym, err)
				return err
			}
			log.Println("updated", sym)
			time.Sleep(time.Millisecond * 200)
			return nil
		})
	}

	return g.Wait()
}
