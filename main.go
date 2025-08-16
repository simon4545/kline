package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/joho/godotenv"
	"golang.org/x/sync/errgroup"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// ================= 数据更新逻辑 =================
func updateKlines(db *gorm.DB, symbol string) error {
	// 创建一个带有symbol的Kline实例，用于获取表名
	kline := Kline{Symbol: symbol}

	// 自动迁移表结构
	if err := db.Table(kline.TableName()).AutoMigrate(&Kline{}); err != nil {
		return err
	}

	var last Kline
	res := db.Table(kline.TableName()).Order("open_time DESC").Limit(1).Find(&last)

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
		k.Symbol = symbol // 确保kline记录包含symbol信息
		var existing Kline
		if err := db.Table(kline.TableName()).Where("open_time = ?", k.OpenTime).First(&existing).Error; err == nil {
			// 更新（收盘时间可能未完成）
			db.Table(kline.TableName()).Model(&existing).Updates(k)
		} else {
			// 新增
			db.Table(kline.TableName()).Create(&k)
		}
	}
	return nil
}

var botToken, chatID string

func init() {
	// 读取 .env
	if err := godotenv.Load(); err != nil {
		log.Fatal("Error loading .env file")
	}
	botToken = os.Getenv("TELEGRAM_BOT_TOKEN")
	chatID = os.Getenv("TELEGRAM_CHAT_ID")
}

// ================= 主程序 =================
func main() {
	// 检查命令行参数
	if len(os.Args) > 1 && os.Args[1] == "migrate" {
		// 执行数据迁移
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

		if err := migrateToUnifiedTable(db); err != nil {
			log.Fatal("数据迁移失败:", err)
		}

		log.Println("数据迁移成功完成")
		return
	}

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
	// 从 symbols.json 读取 symbols
	symbols, err := loadSymbolsFromFile("symbols.json")
	if err != nil {
		return
	}

	// 遍历所有代币
	for _, symbol := range symbols {
		// 创建一个带有symbol的Kline实例，用于获取表名
		kline := Kline{Symbol: symbol}

		// 确保表存在
		if err := db.Table(kline.TableName()).AutoMigrate(&Kline{}); err != nil {
			log.Printf("自动迁移表 %s 失败: %v", kline.TableName(), err)
			continue
		}
	}
	// 启动 HTTP 服务
	go func() {
		http.HandleFunc("/klines", handleKlineQuery(db))
		http.HandleFunc("/symbols", handleSymbols())
		log.Println("HTTP server started on :3000")
		if err := http.ListenAndServe(":3000", nil); err != nil {
			log.Fatal(err)
		}
	}()

	go func() {
		// 定时任务：每分钟更新一次
		ticker := time.NewTicker(1 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			// 从 symbols.json 读取 symbols
			symbols, err := loadSymbolsFromFile("symbols.json")
			if err != nil {
				log.Fatalf("读取 symbols.json 失败: %v", err)
			}
			if err := processSymbols(symbols, db); err != nil {
				log.Println("部分任务失败:", err)
			}
		}
	}()

	// 定时任务：每5分钟检查一次MACD水上金叉
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()

		for range ticker.C {
			if err := CheckAllSymbolsMACDBullishCross(db); err != nil {
				log.Printf("检查MACD水上金叉失败: %v", err)
			}
		}
	}()

	select {}
}

func processSymbols(symbols []string, db *gorm.DB) error {
	var g errgroup.Group
	sem := make(chan struct{}, 3) // 限制并行 4 个

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

func clean(db *gorm.DB) {
	// 定时清理任务：每24小时执行一次
	go func() {
		ticker := time.NewTicker(24 * time.Hour)
		defer ticker.Stop()

		for {
			<-ticker.C
			// 从 symbols.json 读取 symbols
			symbols, err := loadSymbolsFromFile("symbols.json")
			if err != nil {
				log.Printf("读取 symbols.json 失败: %v", err)
				continue
			}

			cutoff := time.Now().AddDate(0, -1, 0).UnixMilli()
			totalDeleted := int64(0)

			// 为每个symbol清理旧数据
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

// migrateToUnifiedTable 将所有symbol的kline数据迁移到统一的表中
func migrateToUnifiedTable(db *gorm.DB) error {
	log.Println("开始数据迁移...")
	
	// 创建统一表
	if err := db.AutoMigrate(&UnifiedKline{}); err != nil {
		return fmt.Errorf("创建统一表失败: %v", err)
	}
	
	// 从 symbols.json 读取 symbols
	symbols, err := loadSymbolsFromFile("symbols.json")
	if err != nil {
		return fmt.Errorf("读取 symbols.json 失败: %v", err)
	}
	
	// 统计总迁移记录数
	totalMigrated := 0
	
	// 遍历所有symbol
	for _, symbol := range symbols {
		log.Printf("正在迁移 %s 的数据...", symbol)
		
		// 创建一个带有symbol的Kline实例，用于获取表名
		kline := Kline{Symbol: symbol}
		tableName := kline.TableName()
		
		// 检查表是否存在
		if !db.Migrator().HasTable(tableName) {
			log.Printf("表 %s 不存在，跳过", tableName)
			continue
		}
		
		// 从旧表中读取所有数据
		var klines []Kline
		if err := db.Table(tableName).Find(&klines).Error; err != nil {
			log.Printf("读取表 %s 数据失败: %v", tableName, err)
			continue
		}
		
		log.Printf("从表 %s 读取到 %d 条记录", tableName, len(klines))
		
		// 如果没有数据，跳过
		if len(klines) == 0 {
			continue
		}
		
		// 转换为UnifiedKline并插入到新表
		unifiedKlines := make([]UnifiedKline, len(klines))
		for i, k := range klines {
			unifiedKlines[i] = UnifiedKline{
				Symbol:    k.Symbol,
				OpenTime:  k.OpenTime,
				Open:      k.Open,
				High:      k.High,
				Low:       k.Low,
				Close:     k.Close,
				Volume:    k.Volume,
				CloseTime: k.CloseTime,
			}
		}
		
		// 批量插入到新表
		if err := db.Table("klines_unified").CreateInBatches(unifiedKlines, 1000).Error; err != nil {
			log.Printf("插入数据到统一表失败: %v", err)
			continue
		}
		
		totalMigrated += len(klines)
		log.Printf("成功迁移 %s 的 %d 条记录", symbol, len(klines))
	}
	
	log.Printf("数据迁移完成，总共迁移 %d 条记录", totalMigrated)
	return nil
}
