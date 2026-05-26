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

var cache = NewLedisCache()
var symbols []string
var botToken, chatID string
var port *int

func init() {
	fmt.Println("启动程序")
	if err := godotenv.Load(); err != nil {
		log.Fatal("Error loading .env file")
	}
	port = flag.Int("port", 3000, "服务监听端口号")
	flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ContinueOnError)
	flag.CommandLine.SetOutput(os.Stderr)
	_ = flag.CommandLine.Parse(os.Args[1:])
	botToken = os.Getenv("TELEGRAM_BOT_TOKEN")
	chatID = os.Getenv("TELEGRAM_CHAT_ID")
}

func main() {
	fmt.Println("启动程序")
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		log.Fatal(err)
	}
	db, err := gorm.Open(sqlite.Open("klines.db"), &gorm.Config{
		NowFunc: func() time.Time { return time.Now().In(loc) },
		Logger:  logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		log.Fatal(err)
	}

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

	symbols, err = loadSymbolsFromFile("symbols.json")
	if err != nil {
		return
	}

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

	go startHttpServer(db)
	go startScheduledTasks(db)
	go startXauMonitor()
	go clean(db)

	select {}
}

func startHttpServer(db *gorm.DB) {
	wp, err := websocketproxy.NewProxy("wss://fstream.binance.com:443/market", func(r *http.Request) error { return nil })
	if err != nil {
		log.Fatal(err)
	}

	http.Handle("/", gzhttp.GzipHandler(http.FileServer(http.Dir("./public"))))
	http.HandleFunc("/klines", gzhttp.GzipHandler(handleKlineQuery(db)))
	http.HandleFunc("/klines/batch", gzhttp.GzipHandler(handleKlinesBatch(db)))
	http.HandleFunc("/symbols", handleSymbols())
	http.HandleFunc("/hot", handleHotSymbols())
	http.HandleFunc("/marketcap", handleProxyFetch("https://www.binance.com/bapi/apex/v1/friendly/apex/marketing/complianceSymbolList"))
	http.HandleFunc("/fapi/marketcap", handleProxyFetch("https://www.binance.com/bapi/defi/v1/public/wallet-direct/buw/wallet/cex/alpha/all/token/list"))
	http.HandleFunc("/stream", wp.Proxy)
	http.HandleFunc("/trade/signal", gzhttp.GzipHandler(handleTradeSignal()))

	addr := fmt.Sprintf(":%d", *port)
	log.Printf("HTTP server started on %s", addr)
	if err := http.ListenAndServe(addr, nil); err != nil {
		log.Fatal(err)
	}
}

func startScheduledTasks(db *gorm.DB) {
	if err := processSymbols(symbols, db); err != nil {
		log.Println("部分任务失败:", err)
	}
	ticker := time.NewTicker(8 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		if err := processSymbols(symbols, db); err != nil {
			log.Println("部分任务失败:", err)
		}
	}
}
