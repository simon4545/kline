package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strconv"

	"gorm.io/gorm"
)

// ================= 数据模型 =================
type Kline struct {
	ID        uint `gorm:"primaryKey"`
	Symbol    string
	OpenTime  int64
	Open      float64
	High      float64
	Low       float64
	Close     float64
	Volume    float64
	CloseTime int64
}

// TableName 为Kline结构体动态生成表名
func (k Kline) TableName() string {
	if k.Symbol != "" {
		return "kline_" + k.Symbol
	}
	return "kline"
}

// UnifiedKline 统一的K线数据模型，用于存储所有symbol的数据
type UnifiedKline struct {
	ID        uint   `gorm:"primaryKey"`
	Symbol    string `gorm:"index:idx_symbol_open_time"`
	OpenTime  int64  `gorm:"index:idx_symbol_open_time"`
	Open      float64
	High      float64
	Low       float64
	Close     float64
	Volume    float64
	CloseTime int64
}

// TableName 为UnifiedKline结构体指定表名
func (k UnifiedKline) TableName() string {
	return "kline"
}

// ================= 币安 API 拉取 =================
func fetchBinanceKlines(symbol string, interval string, startTime, endTime int64, limit int) ([]Kline, error) {
	url := fmt.Sprintf(
		"https://fapi.binance.com/fapi/v1/klines?symbol=%s&interval=%s&limit=%d",
		symbol, interval, limit,
	)
	if startTime > 0 {
		url += fmt.Sprintf("&startTime=%d", startTime)
	}
	if endTime > 0 {
		url += fmt.Sprintf("&endTime=%d", endTime)
	}

	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var raw [][]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, err
	}

	klines := make([]Kline, 0, len(raw))
	for _, item := range raw {
		openTime := int64(item[0].(float64))
		open, _ := strconv.ParseFloat(item[1].(string), 64)
		high, _ := strconv.ParseFloat(item[2].(string), 64)
		low, _ := strconv.ParseFloat(item[3].(string), 64)
		closePrice, _ := strconv.ParseFloat(item[4].(string), 64)
		volume, _ := strconv.ParseFloat(item[5].(string), 64)
		closeTime := int64(item[6].(float64))

		klines = append(klines, Kline{
			Symbol:    symbol,
			OpenTime:  openTime,
			Open:      open,
			High:      high,
			Low:       low,
			Close:     closePrice,
			Volume:    volume,
			CloseTime: closeTime,
		})
	}
	return klines, nil
}

// ================= 动态窗口聚合查询 =================
func queryAggregatedKlines(db *gorm.DB, symbol string, interval string, limit int) ([][]interface{}, error) {
	var result = getAggKline(db, symbol, interval, limit)
	// 按币安 API 返回格式组装（二维数组）
	resp := make([][]interface{}, 0)
	for _, k := range result {
		resp = append(resp, []interface{}{
			k.OpenTime,                    // 开盘时间 (ms)
			fmt.Sprintf("%.8f", k.Open),   // 开盘价
			fmt.Sprintf("%.8f", k.High),   // 最高价
			fmt.Sprintf("%.8f", k.Low),    // 最低价
			fmt.Sprintf("%.8f", k.Close),  // 收盘价
			fmt.Sprintf("%.8f", k.Volume), // 成交量
			k.CloseTime,                   // 收盘时间 (ms)
			"0",                           // Quote asset volume
			0,                             // Number of trades
			"0",                           // Taker buy base asset volume
			"0",                           // Taker buy quote asset volume
			"0",                           // Ignore
		})
	}
	slices.Reverse(resp)
	return resp, nil
}
func getAggKline(db *gorm.DB, symbol string, interval string, limit int) (result []Kline) {
	kline := Kline{Symbol: symbol}
	if limit == 0 {
		limit = 200
	}
	tableName := kline.TableName()

	if interval == "4h" {
		query := fmt.Sprintf(`SELECT symbol, open_time, open, high, low, close, volume, close_time FROM %s ORDER BY open_time DESC LIMIT %d;`, tableName, limit)
		db.Raw(query).Scan(&result)
		return
	}

	var bucketInterval int64
	var offset int64

	const msDay int64 = 24 * 60 * 60 * 1000
	const msWeek int64 = 7 * msDay
	const ms3Days int64 = 3 * msDay // Unix 0 (周四) 到 周一 的差值

	if interval == "1d" {
		bucketInterval = msDay
		offset = 0 // UTC 0点对齐，不需要偏移
	} else if interval == "1w" {
		bucketInterval = msWeek
		// 1970-01-01 是周四，为了让聚合结果的 bucket_start 落在周一 00:00:00 UTC
		// 我们需要给当前时间戳加 3 天进行计算，计算完后再减去 3 天
		offset = ms3Days
	} else {
		return
	}

	// SQL 说明：
	// bucket_start: 确保时间戳对齐到 UTC 的自然天/周起点
	// close_time: 设为该周期的最后一毫秒
	query := fmt.Sprintf(`
    WITH base AS (
        SELECT 
            symbol, open, high, low, close, volume, open_time,
            CAST(((open_time + %d) / %d) AS INTEGER) * %d - %d AS bucket_start
        FROM %s
    ),
    agg AS (
        SELECT
            symbol,
            bucket_start,
            FIRST_VALUE(open) OVER (PARTITION BY bucket_start ORDER BY open_time ASC) AS open,
            MAX(high) OVER (PARTITION BY bucket_start) AS high,
            MIN(low) OVER (PARTITION BY bucket_start) AS low,
            FIRST_VALUE(close) OVER (PARTITION BY bucket_start ORDER BY open_time DESC) AS close,
            SUM(volume) OVER (PARTITION BY bucket_start) AS volume,
            (bucket_start + %d - 1) AS calculated_close_time,
            ROW_NUMBER() OVER (PARTITION BY bucket_start ORDER BY open_time ASC) AS rn
        FROM base
    )
    SELECT 
        symbol, 
        bucket_start AS open_time, 
        open, high, low, close, volume, 
        calculated_close_time AS close_time 
    FROM agg 
    WHERE rn = 1 
    ORDER BY open_time DESC 
    LIMIT %d;
    `, offset, bucketInterval, bucketInterval, offset, tableName, bucketInterval, limit)

	db.Raw(query).Scan(&result)
	return
}

// createIndexForKlineTable 为Kline表动态创建联合索引
func createIndexForKlineTable(db *gorm.DB, tableName string) error {
	// 生成动态索引名，包含表名以确保唯一性
	indexName := fmt.Sprintf("idx_%s_symbol_open_time", tableName)

	// 使用原生SQL创建联合索引
	sql := fmt.Sprintf("CREATE UNIQUE INDEX IF NOT EXISTS %s ON %s (symbol, open_time)", indexName, tableName)
	return db.Exec(sql).Error
}
