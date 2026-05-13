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
		// 允许跨域
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

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
		// 允许跨域
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

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

func gzipMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
			w.Header().Set("Content-Encoding", "gzip")

			gz := gzip.NewWriter(w)
			defer gz.Close()

			wrapped := &gzipResponseWriter{Writer: gz, ResponseWriter: w}
			next(wrapped, r)
		} else {
			next(w, r)
		}
	}
}

type gzipResponseWriter struct {
	io.Writer
	http.ResponseWriter
}

func (w *gzipResponseWriter) Write(data []byte) (int, error) {
	return w.Writer.Write(data)
}

func (w *gzipResponseWriter) WriteHeader(statusCode int) {
	w.ResponseWriter.WriteHeader(statusCode)
}

func handleSymbols() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// 允许跨域
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

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
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

		if r.Method == http.MethodOptions {
			return // 处理预检请求
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write(js)
	}
}

// ================= HTTP 接口 =================
func handleKlineQuery(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// 允许跨域
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

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
