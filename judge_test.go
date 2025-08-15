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
