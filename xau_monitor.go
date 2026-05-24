package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"
)

const (
	xauSymbol            = "XAUUSDT"
	dropThreshold        = 35.0
	shortWindowKlineSize = 15
	longWindowKlineSize  = 240
	monitorPollInterval  = 15 * time.Second
	alertCooldown        = 30 * time.Minute
)

type xauState struct {
	lastKTime      int64
	lastRiseTime   int64
	lastTD9Time    int64
	lastTD9CloseAt int64
}

type xauMonitorState struct {
	intervals map[string]*xauState
}

type td9Result struct {
	Type string
	Side string
}

func startXauMonitor() {
	state := &xauMonitorState{intervals: map[string]*xauState{"5m": {}, "15m": {}, "short": {}, "long": {}}}
	ticker := time.NewTicker(monitorPollInterval)
	defer ticker.Stop()

	monitor := func() {
		utcNow := time.Now().UTC()
		if utcNow.Weekday() == time.Saturday || utcNow.Weekday() == time.Sunday {
			return
		}
		if k1m, err := fetchXauKlines("1m", longWindowKlineSize); err == nil && len(k1m) > 0 {
			k1short := takeLastKlines(k1m, shortWindowKlineSize)
			dropdown(k1short, dropThreshold, true, state)
			riseAlert(k1short, dropThreshold, true, state)
			dropdown(k1m, dropThreshold*2, false, state)
			riseAlert(k1m, dropThreshold*2, false, state)
		}

		for _, interval := range []string{"5m", "15m"} {
			rawKlines, err := fetchXauKlines(interval, 30)
			if err != nil || len(rawKlines) == 0 {
				continue
			}
			closedKlines := getClosedKlines(rawKlines)
			if len(closedKlines) == 0 {
				continue
			}
			if result := checkClosedTD9(closedKlines); result != nil {
				stateSlot := state.intervals[interval]
				if stateSlot == nil {
					stateSlot = &xauState{}
					state.intervals[interval] = stateSlot
				}
				now := time.Now().UnixMilli()
				lastClosed := closedKlines[len(closedKlines)-1]
				closeAt, _ := toInt64(lastClosed[6])
				if closeAt != 0 && closeAt == stateSlot.lastTD9CloseAt {
					continue
				}
				if now-stateSlot.lastTD9Time <= alertCooldown.Milliseconds() {
					continue
				}
				emoji := "🔴"
				if result.Side == "BUY" {
					emoji = "🟢"
				}
				price := lastClosed[4]
				msg := fmt.Sprintf("%s *九转信号: %s (%s)*\n\n*神奇九转·收盘确认*\n• 周期: %s\n• 信号: %s\n• 收盘价: %v\n• 状态: 已收盘确认", emoji, xauSymbol, interval, interval, result.Type, price)
				if err := TelegramSendMessage(msg); err == nil {
					stateSlot.lastTD9Time = now
					stateSlot.lastTD9CloseAt = closeAt
				}
			}
		}
	}

	monitor()
	for range ticker.C {
		monitor()
	}
}

func fetchXauKlines(interval string, limit int) ([][]interface{}, error) {
	url := fmt.Sprintf("https://fapi.binance.com/fapi/v1/klines?symbol=%s&interval=%s&limit=%d", xauSymbol, interval, limit)
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var raw [][]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, err
	}
	return raw, nil
}

func getClosedKlines(klines [][]interface{}) [][]interface{} {
	now := time.Now().UnixMilli()
	out := make([][]interface{}, 0, len(klines))
	for _, k := range klines {
		if closeTime, ok := toInt64(k[6]); ok && closeTime < now {
			out = append(out, k)
		}
	}
	return out
}

func checkClosedTD9(closedKlines [][]interface{}) *td9Result {
	if len(closedKlines) < 13 {
		return nil
	}
	targetIdx := len(closedKlines) - 1
	isBuy9 := true
	for i := 0; i < 9; i++ {
		currentCheckIdx := targetIdx - i
		close, _ := toFloat64(closedKlines[currentCheckIdx][4])
		close4Before, _ := toFloat64(closedKlines[currentCheckIdx-4][4])
		if !(close < close4Before) {
			isBuy9 = false
			break
		}
	}
	if isBuy9 {
		return &td9Result{Type: "买入 (TD9)", Side: "BUY"}
	}
	isSell9 := true
	for i := 0; i < 9; i++ {
		currentCheckIdx := targetIdx - i
		close, _ := toFloat64(closedKlines[currentCheckIdx][4])
		close4Before, _ := toFloat64(closedKlines[currentCheckIdx-4][4])
		if !(close > close4Before) {
			isSell9 = false
			break
		}
	}
	if isSell9 {
		return &td9Result{Type: "卖出 (TD9)", Side: "SELL"}
	}
	return nil
}

func dropdown(list [][]interface{}, threshold float64, isShort bool, state *xauMonitorState) {
	if len(list) == 0 {
		return
	}

	maxHigh := -1.0
	for _, k := range list {
		if v, ok := toFloat64(k[2]); ok && v > maxHigh {
			maxHigh = v
		}
	}

	currentLow, _ := toFloat64(list[len(list)-1][3])
	amp := maxHigh - currentLow
	now := time.Now().UnixMilli()
	key, label := "long", "波动预警2"
	if isShort {
		key, label = "short", "波动预警1"
	}
	stateSlot := state.intervals[key]
	if stateSlot == nil {
		stateSlot = &xauState{}
		state.intervals[key] = stateSlot
	}
	if amp >= threshold && now-stateSlot.lastKTime > alertCooldown.Milliseconds() {
		msg := fmt.Sprintf("⚠️ *%s: %s*\n\n跌幅已达: *$%.2f*\n\n最高价:%v\n当前K线最低价:%v", label, xauSymbol, amp, maxHigh, currentLow)
		if err := TelegramSendMessage(msg); err == nil {
			stateSlot.lastKTime = now
		}
	}
}

func riseAlert(list [][]interface{}, threshold float64, isShort bool, state *xauMonitorState) {
	if len(list) == 0 {
		return
	}

	minLow := 0.0
	first := true
	for _, k := range list {
		if v, ok := toFloat64(k[3]); ok {
			if first || v < minLow {
				minLow = v
				first = false
			}
		}
	}

	currentHigh, _ := toFloat64(list[len(list)-1][2])
	amp := currentHigh - minLow
	now := time.Now().UnixMilli()
	key, label := "long", "波动预警2(上涨)"
	if isShort {
		key, label = "short", "波动预警1(上涨)"
	}
	stateSlot := state.intervals[key]
	if stateSlot == nil {
		stateSlot = &xauState{}
		state.intervals[key] = stateSlot
	}
	if amp >= threshold && now-stateSlot.lastRiseTime > alertCooldown.Milliseconds() {
		msg := fmt.Sprintf("🚀 *%s: %s*\n\n涨幅已达: *$%.2f*\n\n窗口最低价:%v\n当前K线最高价:%v", label, xauSymbol, amp, minLow, currentHigh)
		if err := TelegramSendMessage(msg); err == nil {
			stateSlot.lastRiseTime = now
		}
	}
}

func takeLastKlines(list [][]interface{}, n int) [][]interface{} {
	if n <= 0 || len(list) <= n {
		return list
	}
	return list[len(list)-n:]
}

func toFloat64(v interface{}) (float64, bool) {
	switch t := v.(type) {
	case float64:
		return t, true
	case string:
		f, err := strconv.ParseFloat(t, 64)
		if err == nil {
			return f, true
		}
	case json.Number:
		f, err := t.Float64()
		if err == nil {
			return f, true
		}
	}
	return 0, false
}

func toInt64(v interface{}) (int64, bool) {
	switch t := v.(type) {
	case float64:
		return int64(t), true
	case int64:
		return t, true
	case json.Number:
		n, err := t.Int64()
		if err == nil {
			return n, true
		}
	case string:
		n, err := strconv.ParseInt(t, 10, 64)
		if err == nil {
			return n, true
		}
	}
	return 0, false
}
