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

const tradeQueueKey = "trade:queue"

// ================= 中间件与辅助函数 =================

// corsMiddleware 包装 handler，统一处理 CORS 头和 OPTIONS 预检请求
func corsMiddleware(methods string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", methods)
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == http.MethodOptions {
			return
		}
		next(w, r)
	}
}

// fetchJSON 通用远程 JSON 获取函数
func fetchJSON(url string) ([]byte, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return io.ReadAll(resp.Body)
}

// writeJSON 根据客户端是否支持 gzip，选择压缩或直接写入 JSON 响应
func writeJSON(w http.ResponseWriter, r *http.Request, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	if strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
		w.Header().Set("Content-Encoding", "gzip")
		gz := gzip.NewWriter(w)
		defer gz.Close()
		json.NewEncoder(gz).Encode(data)
		return
	}
	json.NewEncoder(w).Encode(data)
}

func writePlainText(w http.ResponseWriter, status int, text string) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(text))
}

// handleProxyFetch 通用代理转发 handler 工厂
func handleProxyFetch(url string) http.HandlerFunc {
	return corsMiddleware("GET, OPTIONS", func(w http.ResponseWriter, r *http.Request) {
		data, err := fetchJSON(url)
		if err != nil {
			http.Error(w, fmt.Sprintf("fetch error: %v", err), http.StatusInternalServerError)
			return
		}
		w.Write(data)
	})
}

// ================= 业务 handler =================

func handleSymbols() http.HandlerFunc {
	return corsMiddleware("GET, OPTIONS", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(symbols)
	})
}

func handleHotSymbols() http.HandlerFunc {
	return corsMiddleware("GET, OPTIONS", func(w http.ResponseWriter, r *http.Request) {
		js, err := json.Marshal(HotList())
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write(js)
	})
}

// ================= 批量K线查询接口 =================

type KlinesRequest struct {
	Symbols  []string `json:"symbols"`
	Interval string   `json:"interval"`
	Limit    int      `json:"limit"`
}

type KlinesResponse struct {
	Symbol string          `json:"symbol"`
	Klines [][]interface{} `json:"klines"`
	Error  string          `json:"error,omitempty"`
}

func handleKlinesBatch(db *gorm.DB) http.HandlerFunc {
	return corsMiddleware("POST, OPTIONS", func(w http.ResponseWriter, r *http.Request) {
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

		results := make([]KlinesResponse, 0, len(req.Symbols))
		for _, symbol := range req.Symbols {
			data, err := queryAggregatedKlines(db, symbol, req.Interval, req.Limit)
			resp := KlinesResponse{Symbol: symbol, Klines: data}
			if err != nil {
				resp.Error = err.Error()
			}
			results = append(results, resp)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(results)
	})
}

// ================= 单条K线查询接口 =================

func handleKlineQuery(db *gorm.DB) http.HandlerFunc {
	return corsMiddleware("GET, OPTIONS", func(w http.ResponseWriter, r *http.Request) {
		symbol := r.URL.Query().Get("symbol")
		interval := r.URL.Query().Get("interval")
		limit := r.URL.Query().Get("limit")
		if symbol == "" || interval == "" {
			http.Error(w, "missing symbol or interval", http.StatusBadRequest)
			return
		}

		limitCount, _ := strconv.Atoi(limit)
		if limitCount == 0 {
			limitCount = 100
		}

		t0 := time.Now()
		data, err := queryAggregatedKlines(db, symbol, interval, limitCount)
		if err != nil {
			http.Error(w, fmt.Sprintf("query error: %v", err), http.StatusInternalServerError)
			return
		}
		fmt.Println("统计", time.Since(t0).Milliseconds())
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(data)
	})
}

func handleTradeSignal() http.HandlerFunc {
	return corsMiddleware("POST, GET, OPTIONS", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			var req struct {
				Side string `json:"side"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writePlainText(w, http.StatusBadRequest, "INVALID")
				return
			}

			side := strings.ToUpper(strings.TrimSpace(req.Side))
			if side != "BUY" && side != "SELL" {
				writePlainText(w, http.StatusBadRequest, "INVALID")
				return
			}

			if _, err := cache.db.RPush([]byte(tradeQueueKey), []byte(side)); err != nil {
				writePlainText(w, http.StatusInternalServerError, "ERROR")
				return
			}
			if _, err := cache.db.Expire([]byte(tradeQueueKey), 100); err != nil {
				writePlainText(w, http.StatusInternalServerError, "ERROR")
				return
			}

			writePlainText(w, http.StatusOK, side)
			return
		}

		if r.Method == http.MethodGet {
			value, err := cache.db.LPop([]byte(tradeQueueKey))
			if err != nil {
				writePlainText(w, http.StatusInternalServerError, "ERROR")
				return
			}
			if value == nil {
				writePlainText(w, http.StatusOK, "")
				return
			}

			side := strings.ToUpper(strings.TrimSpace(string(value)))
			if side != "BUY" && side != "SELL" {
				writePlainText(w, http.StatusOK, "")
				return
			}
			writePlainText(w, http.StatusOK, side)
			return
		}

		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	})
}

// ================= 工具函数 =================

func loadSymbolsFromFile(filename string) ([]string, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	var symbols []string
	return symbols, json.Unmarshal(data, &symbols)
}
