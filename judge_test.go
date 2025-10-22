package main

import (
	"fmt"
	"log"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// setupTestDB 设置测试数据库
func setupTestDB() (*gorm.DB, error) {
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
	return db, nil
}

// TestCheckAllSymbolsMACDBullishCross 测试CheckAllSymbolsMACDBullishCross函数
func TestCheckAllSymbolsMACDBullishCross(t *testing.T) {
	// 设置测试数据库
	db, err := setupTestDB()
	err = CheckAllSymbolsMACDBullishCross(db)
	fmt.Println(botToken, chatID, err)

}

func TestDrop(t *testing.T) {
	db, err := setupTestDB()
		if err != nil {
		log.Printf("读取 symbols.json 失败: %v", err)
		return
	}
	// 从 symbols.json 读取 symbols
	symbols, err := loadSymbolsFromFile("symbols.json")
	if err != nil {
		log.Printf("读取 symbols.json 失败: %v", err)
		return
	}

	// 建立一个 map，方便快速判断
	symbolSet := make(map[string]struct{})
	for _, s := range symbols {
		symbolSet["kline_"+s] = struct{}{}
	}
	// 2️⃣ 获取数据库中所有表名
	var tables []string
	err = db.Raw("SELECT name FROM sqlite_master WHERE type='table'").Scan(&tables).Error
	if err != nil {
		// 如果是 MySQL 可改为：db.Raw("SHOW TABLES").Scan(&tables)
		log.Printf("获取数据库表名失败: %v", err)
		return
	}

	// 3️⃣ 删除不在 symbols.json 列表中的表
	for _, table := range tables {
		if _, ok := symbolSet[table]; !ok && table != "sqlite_sequence" {
			log.Printf("删除表: %s", table)
			if err := db.Exec(fmt.Sprintf("DROP TABLE IF EXISTS `%s`", table)).Error; err != nil {
				log.Printf("删除表 %s 失败: %v", table, err)
			}
		}
	}
}
