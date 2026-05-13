package main

import (
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"
)

// setCORS 设置通用的 CORS 头
func setCORS(w http.ResponseWriter, methods string) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", methods)
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
}

// fetchMarketCapData 调用币安 API 获取 market cap 数据
func fetchMarketCapData() ([]byte, error) {
	resp, err := http.Get("https://www.binance.com/bapi/apex/v1/friendly/apex/marketing/complianceSymbolList")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return io.ReadAll(resp.Body)
}

// fetchFutureMarketCapData 调用币安期货 API 获取 market cap 数据
func fetchFutureMarketCapData() ([]byte, error) {
	resp, err := http.Get("https://www.binance.com/bapi/defi/v1/public/wallet-direct/buw/wallet/cex/alpha/all/token/list")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return io.ReadAll(resp.Body)
}

// handleMarketCap 返回 spot market cap 数据
func handleMarketCap() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		setCORS(w, "GET, OPTIONS")
		if r.Method == http.MethodOptions {
			return // 处理预检请求
		}

		data, err := fetchMarketCapData()
		if err != nil {
			http.Error(w, fmt.Sprintf("fetch error: %v", err), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.Write(data)
	}
}

// handleFutureMarketCap 返回 futures market cap 数据
func handleFutureMarketCap() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		setCORS(w, "GET, OPTIONS")
		if r.Method == http.MethodOptions {
			return // 处理预检请求
		}

		data, err := fetchFutureMarketCapData()
		if err != nil {
			http.Error(w, fmt.Sprintf("fetch error: %v", err), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.Write(data)
	}
}

func handleSymbols() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		setCORS(w, "GET, OPTIONS")
		if r.Method == http.MethodOptions {
			return // 处理预检请求
		}

		// 不支持 gzip，直接返回
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(symbols)
	}
}
func handleHotSymbols() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		js, err := json.Marshal(HotList())
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		setCORS(w, "GET, OPTIONS")
		if r.Method == http.MethodOptions {
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write(js)
	}
}

// ================= 批量K线查询接口 =================
// KlinesRequest 批量K线请求结构
type KlinesRequest struct {
	Symbols  []string `json:"symbols"`
	Interval string   `json:"interval"`
	Limit    int      `json:"limit"`
}

// KlinesResponse 批量K线响应结构
type KlinesResponse struct {
	Symbol string          `json:"symbol"`
	Klines [][]interface{} `json:"klines"`
	Error  string          `json:"error,omitempty"`
}

func handleKlinesBatch(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		setCORS(w, "POST, OPTIONS")
		if r.Method == http.MethodOptions {
			return
		}

		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req KlinesRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, fmt.Sprintf("json decode error: %v", err), http.StatusBadRequest)
			return
		}

		if len(req.Symbols) == 0 || req.Interval == "" {
			http.Error(w, "missing symbols or interval", http.StatusBadRequest)
			return
		}

		if req.Limit == 0 {
			req.Limit = 500
		}

		// 批量查询每个symbol的K线数据
		results := make([]KlinesResponse, 0, len(req.Symbols))

		for _, symbol := range req.Symbols {
			data, err := queryAggregatedKlines(db, symbol, req.Interval, req.Limit)
			if err != nil {
				results = append(results, KlinesResponse{
					Symbol: symbol,
					Klines: nil,
					Error:  err.Error(),
				})
				continue
			}
			results = append(results, KlinesResponse{
				Symbol: symbol,
				Klines: data,
				Error:  "",
			})
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(results)
	}
}

// ================= HTTP 接口 =================
func handleKlineQuery(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		setCORS(w, "GET, OPTIONS")
		if r.Method == http.MethodOptions {
			return // 处理预检请求
		}
		symbol := r.URL.Query().Get("symbol")
		interval := r.URL.Query().Get("interval")
		limit := r.URL.Query().Get("limit")
		if symbol == "" || interval == "" || limit == "" {
			http.Error(w, "missing symbol or interval", http.StatusBadRequest)
			return
		}
		limitCount, err := strconv.Atoi(limit)
		if err != nil {
			limitCount = 100
		}
		time1 := time.Now()

		data, err := queryAggregatedKlines(db, symbol, interval, limitCount)
		if err != nil {
			http.Error(w, fmt.Sprintf("query error: %v", err), http.StatusInternalServerError)
			return
		}
		fmt.Println("统计", time.Since(time1).Milliseconds())

		// 判断是否支持 gzip
		if strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
			w.Header().Set("Content-Encoding", "gzip")
			gz := gzip.NewWriter(w)
			defer gz.Close()

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(gz).Encode(data)
			return
		}

		// 不支持 gzip，直接返回
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(data)
	}
}
func loadSymbolsFromFile(filename string) ([]string, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	var symbols []string
	if err := json.Unmarshal(data, &symbols); err != nil {
		return nil, err
	}
	return symbols, nil
}
