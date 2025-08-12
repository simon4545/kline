package main

import (
	"compress/gzip"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"
)

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
