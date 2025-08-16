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

var symbols []string

// ================= 数据更新逻辑 =================
func updateKlines(db *gorm.DB, symbol string) error {
	// 创建一个带有symbol的Kline实例，用于获取表名
	kline := Kline{Symbol: symbol}

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

	// 显式设置WAL模式
	if err := db.Exec("PRAGMA journal_mode=WAL;").Error; err != nil {
		log.Printf("设置WAL模式失败: %v", err)
	} else {
		log.Println("成功设置WAL模式")
	}
	sqlDB, err := db.DB()
	if err != nil {
		log.Fatal(err)
	}
	sqlDB.SetMaxIdleConns(10)
	sqlDB.SetMaxOpenConns(100)
	// 从 symbols.json 读取 symbols
	symbols, err = loadSymbolsFromFile("symbols.json")
	if err != nil {
		return
	}
	// for _, symbol := range symbols {
	// 	db.Migrator().DropTable(fmt.Sprintf("kline_%s", symbol))
	// }
	// 遍历所有代币
	for _, symbol := range symbols {
		// 创建一个带有symbol的Kline实例，用于获取表名
		kline := Kline{Symbol: symbol}

		// 确保表存在
		if err := db.Table(kline.TableName()).AutoMigrate(&Kline{}); err != nil {
			log.Printf("自动迁移表 %s 失败: %v", kline.TableName(), err)
			continue
		}

		// 动态创建联合索引
		if err := createIndexForKlineTable(db, kline.TableName()); err != nil {
			log.Printf("为表 %s 创建索引失败: %v", kline.TableName(), err)
			// 不中断流程，继续处理下一个symbol
		}
	}
	// 检查命令行参数
	// if err := migrateFromUnifiedTable(db); err != nil {
	// 	log.Fatal("数据迁移失败:", err)
	// }

	// log.Println("数据迁移成功完成")
	// return
	// // 启动 HTTP 服务
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

// migrateFromUnifiedTable 将统一表中的kline数据拆分到各个以symbol为名字的表中
func migrateFromUnifiedTable(db *gorm.DB) error {
	log.Println("开始数据迁移...")

	// 检查统一表是否存在
	if !db.Migrator().HasTable("kline") {
		return fmt.Errorf("统一表 kline 不存在")
	}

	// 使用游标方式逐行读取统一表中的数据
	rows, err := db.Raw("SELECT symbol, open_time, open, high, low, close, volume, close_time FROM kline ORDER BY symbol, open_time").Rows()
	if err != nil {
		return fmt.Errorf("查询统一表数据失败: %v", err)
	}
	defer rows.Close()

	// 统计总迁移记录数
	totalMigrated := 0

	// 用于批量插入的数据缓存
	var batchKlines []Kline
	currentSymbol := ""

	// 处理每一行数据
	for rows.Next() {
		var symbol string
		var openTime int64
		var open, high, low, close, volume float64
		var closeTime int64

		if err := rows.Scan(&symbol, &openTime, &open, &high, &low, &close, &volume, &closeTime); err != nil {
			log.Printf("扫描行数据失败: %v", err)
			continue
		}

		// 创建Kline实例
		kline := Kline{
			Symbol:    symbol,
			OpenTime:  openTime,
			Open:      open,
			High:      high,
			Low:       low,
			Close:     close,
			Volume:    volume,
			CloseTime: closeTime,
		}

		// 检查是否需要切换symbol并执行批量插入
		if currentSymbol != "" && currentSymbol != symbol {
			// 执行批量插入
			tableName := Kline{Symbol: currentSymbol}.TableName()
			if err := db.Table(tableName).CreateInBatches(batchKlines, 100).Error; err != nil {
				log.Printf("批量插入数据到表 %s 失败: %v", tableName, err)
			} else {
				totalMigrated += len(batchKlines)
				log.Printf("成功迁移 %d 条记录到表 %s", len(batchKlines), tableName)
			}
			// 清空batchKlines继续处理下一个symbol
			batchKlines = batchKlines[:0]
		}

		// 更新当前symbol
		currentSymbol = symbol
		// 添加到批量插入缓存
		batchKlines = append(batchKlines, kline)
	}

	// 处理最后一个symbol的剩余数据
	if len(batchKlines) > 0 {
		tableName := Kline{Symbol: currentSymbol}.TableName()
		if err := db.Table(tableName).CreateInBatches(batchKlines, 100).Error; err != nil {
			log.Printf("批量插入数据到表 %s 失败: %v", tableName, err)
		} else {
			totalMigrated += len(batchKlines)
			log.Printf("成功迁移 %d 条记录到表 %s", len(batchKlines), tableName)
		}
	}

	log.Printf("数据迁移完成，总共迁移 %d 条记录", totalMigrated)
	return nil
}
