package main

// EMA 计算指数移动平均线
func EMA(data []float64, period int) []float64 {
	if len(data) < period {
		return make([]float64, len(data))
	}

	multiplier := 2.0 / float64(period+1)
	ema := make([]float64, len(data))

	// 初始化第一个EMA值为简单移动平均
	sum := 0.0
	for i := 0; i < period; i++ {
		sum += data[i]
	}
	ema[period-1] = sum / float64(period)

	// 计算后续EMA值
	for i := period; i < len(data); i++ {
		ema[i] = (data[i]-ema[i-1])*multiplier + ema[i-1]
	}

	return ema
}

// MACD 计算MACD指标
func MACD(data []float64) (macdLine []float64, signalLine []float64, histogram []float64) {
	// 计算12日EMA和26日EMA
	ema12 := EMA(data, 12)
	ema26 := EMA(data, 26)

	// 计算MACD线 (12日EMA - 26日EMA)
	macdLine = make([]float64, len(data))
	for i := 0; i < len(data); i++ {
		macdLine[i] = ema12[i] - ema26[i]
	}

	// 计算信号线 (MACD线的9日EMA)
	signalLine = EMA(macdLine, 9)

	// 计算柱状图 (MACD线 - 信号线)
	histogram = make([]float64, len(data))
	for i := 0; i < len(data); i++ {
		histogram[i] = macdLine[i] - signalLine[i]
	}

	return macdLine, signalLine, histogram
}

// IsBullishCross 检查是否出现MACD水上金叉
// 水上金叉：MACD线从下方向上穿过信号线，且两条线都在零轴之上
func IsBullishCross(macdLine []float64, signalLine []float64) bool {
	if len(macdLine) < 2 || len(signalLine) < 2 {
		return false
	}

	// 检查当前和前一个周期的数据
	currentMacd := macdLine[len(macdLine)-1]
	previousMacd := macdLine[len(macdLine)-2]
	currentSignal := signalLine[len(signalLine)-1]
	previousSignal := signalLine[len(signalLine)-2]

	// 条件1：两条线都在零轴之上
	if currentMacd <= 0 || currentSignal <= 0 {
		return false
	}

	// 条件2：MACD线从下方向上穿过信号线
	// 前一个周期：MACD线 < 信号线
	// 当前周期：MACD线 > 信号线
	if previousMacd < previousSignal && currentMacd > currentSignal {
		return true
	}

	return false
}
