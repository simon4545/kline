package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/joho/godotenv"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/klauspost/compress/gzhttp"
	"github.com/pretty66/websocketproxy"
)

// 全局缓存实例
var cache = NewLedisCache()
var symbols []string
var botToken, chatID string
var port *int

func init() {
	fmt.Println("启动程序")
	// 读取 .env
	if err := godotenv.Load(); err != nil {
		log.Fatal("Error loading .env file")
	}
	// 定义命令行参数 -port，默认值 3000
	port = flag.Int("port", 3000, "服务监听端口号")
	flag.Parse()
	botToken = os.Getenv("TELEGRAM_BOT_TOKEN")
	chatID = os.Getenv("TELEGRAM_CHAT_ID")
}

// ================= 主程序 =================
func main() {
	fmt.Println("启动程序")
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
	if err := db.Exec("PRAGMA journal_mode=DELETE;").Error; err != nil {
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

	// 遍历所有代币，确保表存在
	for _, symbol := range symbols {
		kline := Kline{Symbol: symbol}
		if err := db.Table(kline.TableName()).AutoMigrate(&Kline{}); err != nil {
			log.Printf("自动迁移表 %s 失败: %v", kline.TableName(), err)
			continue
		}
		if err := createIndexForKlineTable(db, kline.TableName()); err != nil {
			log.Printf("为表 %s 创建索引失败: %v", kline.TableName(), err)
		}
	}

	// 启动 HTTP 服务
	go startHttpServer(db)

	// 启动定时任务
	go startScheduledTasks(db)

	clean(db)
	select {}
}

// startHttpServer 启动 HTTP 服务器
func startHttpServer(db *gorm.DB) {
	wp, err := websocketproxy.NewProxy("wss://fstream.binance.com:443/stream", func(r *http.Request) error {
		return nil
	})
	if err != nil {
		log.Fatal()
	}

	http.Handle("/", gzhttp.GzipHandler(http.FileServer(http.Dir("./public"))))
	http.HandleFunc("/klines", gzhttp.GzipHandler(handleKlineQuery(db)))
	http.HandleFunc("/klines/batch", gzhttp.GzipHandler(handleKlinesBatch(db)))
	http.HandleFunc("/symbols", handleSymbols())
	http.HandleFunc("/hot", handleHotSymbols())
	http.HandleFunc("/marketcap", handleMarketCap())
	http.HandleFunc("/fapi/marketcap", handleFutureMarketCap())
	http.HandleFunc("/stream", wp.Proxy)

	addr := fmt.Sprintf(":%d", *port)
	log.Printf("HTTP server started on %s", addr)
	if err := http.ListenAndServe(addr, nil); err != nil {
		log.Fatal(err)
	}
}

// startScheduledTasks 启动定时任务
func startScheduledTasks(db *gorm.DB) {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		if err := processSymbols(symbols, db); err != nil {
			log.Println("部分任务失败:", err)
		}
	}
}
