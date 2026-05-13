// 获取币安永续合约列表
async function fetchBinanceContracts() {
    try {
        const res = await fetch("/symbols");
        const data = await res.json();
        return data.slice(0, 200);
    } catch (error) {
        console.error('获取合约列表失败:', error);
        return [];
    }
}
// 获取币安永续合约列表
async function fetchMyContracts() {
    try {
        const res = await fetch("/symbols");
        const data = await res.json();
        return data.slice(0, 200);
    } catch (error) {
        console.error('获取合约列表失败:', error);
        return [];
    }
}
async function fetchMyAlpha() {
    try {
        const res = await fetch("https://chart.1pan.me/symbols");
        const data = await res.json();
        return data.slice(0, 200);
    } catch (error) {
        console.error('获取合约列表失败:', error);
        return [];
    }
}
// 获取K线数据
async function fetchMyKlines(symbol, interval, limit) {
    const apiUrl = `/klines?symbol=${symbol}&interval=${interval}&limit=${limit}`;

    try {
        const response = await fetch(apiUrl);
        return await response.json();
    } catch (error) {
        console.error(`获取${symbol}数据失败:`, error);
        return null;
    }
}

// 批量获取K线数据
async function fetchBatchKlines(symbols, interval, limit) {
    const apiUrl = `/klines/batch`;

    try {
        const response = await fetch(apiUrl, {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json'
            },
            body: JSON.stringify({ symbols: symbols, interval: interval, limit: limit })
        });
        return await response.json();
    } catch (error) {
        console.error(`批量获取K线数据失败:`, error);
        return null;
    }
}
async function fetchAlphaKlines(symbol, interval, limit) {
    const apiUrl = `https://chart.1pan.me/klines?symbol=${symbol}&interval=${interval}&limit=${limit}`;

    try {
        const response = await fetch(apiUrl);
        return await response.json();
    } catch (error) {
        console.error(`获取${symbol}数据失败:`, error);
        return null;
    }
}

// 获取K线数据
async function fetchAlphaKlines1(symbol, interval, limit) {
    const apiUrl = `https://chart.binance.com/fapi/v1/klines?symbol=${symbol}&interval=${interval}&limit=${limit}`;

    try {
        const response = await fetch(apiUrl);
        return await response.json();
    } catch (error) {
        console.error(`获取${symbol}数据失败:`, error);
        return null;
    }
}
// 获取K线数据
async function fetchBinanceKlines(symbol, interval, limit) {
    const apiUrl = `https://fapi.binance.com/fapi/v1/klines?symbol=${symbol}&interval=${interval}&limit=${limit}`;

    try {
        const response = await fetch(apiUrl);
        return await response.json();
    } catch (error) {
        console.error(`获取${symbol}数据失败:`, error);
        return null;
    }
}
async function fetchSpotKlines(symbol, interval, limit) {
    const apiUrl = `https://api.binance.com/api/v3/klines?symbol=${symbol}&interval=${interval}&limit=${limit}`;

    try {
        const response = await fetch(apiUrl);
        return await response.json();
    } catch (error) {
        console.error(`获取${symbol}数据失败:`, error);
        return null;
    }
}
async function fetchFurtureTop(limit = 40) {
    const apiUrl = `https://fapi.binance.com/fapi/v1/ticker/24hr`;
    const ONE_DAY = 25 * 60 * 60 * 1000;
    const cutoff = Date.now() - ONE_DAY;
    try {
        const response = await fetch(apiUrl);
        let result = await response.json();
        const top40 = result.filter(item => item.openTime >= cutoff && item.symbol.endsWith("USDT")).sort((a, b) => Number(b.priceChangePercent) - Number(a.priceChangePercent)).slice(0, limit);
        return top40.map((item, idx) => item.symbol)
    } catch (error) {
        console.error(`获取${symbol}数据失败:`, error);
        return null;
    }
}
async function fetchSpotTop(limit = 40, windowSize = '1d') {
    const ONE_DAY = 25 * 60 * 60 * 1000;
    const cutoff = Date.now() - ONE_DAY;

    try {
        const topUrl = `https://api.binance.com/api/v3/ticker/24hr`
        let response = await fetch(topUrl);
        let result = await response.json();
        result = result.filter(item => item.openTime >= cutoff && item.symbol.endsWith("USDT")).sort((a, b) => Number(b.priceChangePercent) - Number(a.priceChangePercent)).slice(0, 90);
        let top40 = result.map((item) => item.symbol)
        let symbolsstr = JSON.stringify(top40);
        const apiUrl = `https://api.binance.com/api/v3/ticker?symbols=${symbolsstr}&windowSize=${windowSize}`;
        response = await fetch(apiUrl);
        result = await response.json();
        top40 = result.sort((a, b) => Number(b.priceChangePercent) - Number(a.priceChangePercent)).slice(0, limit);
        return top40
    } catch (error) {
        console.error(`获取数据失败:`, error);
        return null;
    }
}
// 检查颈线是否被跌破
function isNecklineBroken(prices, patternData) {
    const necklinePrice = patternData.troughPrice;
    let endIndex = patternData.endIndex;
    // 检查形态结束点后的价格序列
    for (let i = patternData.endIndex + 1; i < endIndex + 15; i++) {
        if (prices[i] < necklinePrice) {
            // 计算突破幅度
            const breakoutPercent = ((necklinePrice - prices[i]) / necklinePrice * 100);
            return {
                broken: true,
                breakoutIndex: i,
                breakoutPercent: breakoutPercent
            };
        }
    }

    return { broken: false };
}

// 检测双顶形态
function detectDoubleTops(prices, params) {
    const results = [];

    // 1. 寻找局部高点
    const peaks = findPeaksWithZigZag(prices);
    if (peaks.length < 2) return results;

    // 2. 寻找可能的双顶配对
    for (let i = 0; i < peaks.length - 1; i++) {
        const leftPeak = peaks[i];
        const rightPeak = peaks[i + 1];

        // 计算高度差和时间差
        const priceDiff = Math.abs(leftPeak.price - rightPeak.price);
        const priceAvg = (leftPeak.price + rightPeak.price) / 2;
        const normalizedDiff = priceDiff / priceAvg;

        const timeDiff = Math.abs(leftPeak.index - rightPeak.index);
        const avgPatternWidth = 100; // 假设平均模式宽度

        // 检查是否在容差范围内
        if (normalizedDiff <= params.heightTolerance && timeDiff <= avgPatternWidth * params.timeTolerance) {
            // if (normalizedDiff <= params.heightTolerance && Math.abs(timeDiff - avgPatternWidth) <= avgPatternWidth * params.timeTolerance) {

            // 寻找两个高点之间的低点 (颈线)
            let troughIndex = -1;
            let troughPrice = Number.MAX_VALUE;

            for (let j = leftPeak.index + 1; j < rightPeak.index; j++) {
                if (prices[j] < troughPrice) {
                    troughPrice = prices[j];
                    troughIndex = j;
                }
            }

            // 计算回撤深度
            const dropLeft = 100 - (troughPrice / leftPeak.price * 100);
            const dropRight = 100 - (troughPrice / rightPeak.price * 100);

            // 计算形态相似度
            const patternSeries = extractPattern(prices, leftPeak.index - 15, rightPeak.index + 10);
            const similarity = calculateSimilarity(patternSeries);

            // 检查颈线突破
            const breakout = isNecklineBroken(prices, { troughIndex, troughPrice, endIndex: rightPeak.index + 15 });

            // 确定形态的起止位置
            const patternStart = Math.max(0, leftPeak.index - 15);
            const patternEnd = Math.min(prices.length - 1, rightPeak.index + 10);

            if (similarity >= params.similarityThreshold && breakout.broken) {
                if (rightPeak.index < 175) continue;
                console.log(params.symbol, leftPeak.index, rightPeak.index)

                results.push({
                    symbol: params.symbol,
                    similarity: similarity * 100,
                    prices: prices,
                    patternData: {
                        leftPeakIndex: leftPeak.index, rightPeakIndex: rightPeak.index,
                        troughIndex: troughIndex, startIndex: patternStart, endIndex: patternEnd,
                        patternEndTime: formatDateTime(roundTime(new Date(), timeInterval, 200 - rightPeak.index + 1)),
                        leftPeakPrice: leftPeak.price, rightPeakPrice: rightPeak.price, troughPrice: troughPrice,
                        dropLeft: dropLeft, dropRight: dropRight
                    },
                    necklineBroken: breakout.broken,
                    breakoutIndex: breakout.breakoutIndex,
                    breakoutPercent: breakout.breakoutPercent
                });
            }
        }
    }

    return results;
}

function roundTime(date, precision, offset = 0) {
    const result = date;
    // 按精度取整
    switch (precision) {
        case '1h':
            result.setMinutes(0, 0, 0);
            result.setHours(result.getHours() - offset);
            break;
        case '4h':
            const hour = result.getHours();
            const roundedHour = Math.floor(hour / 4) * 4;
            result.setHours(roundedHour, 0, 0, 0);
            result.setHours(result.getHours() - (offset * 4));
            break;
        case '1d':
            result.setHours(0, 0, 0, 0);
            result.setDate(result.getDate() - offset);
            break;
        default:
            throw new Error('Unsupported precision. Use "hour", "4hour" or "day"');
    }

    return result;
}

function hoursago(n) {
    const now = new Date();
    const roundedHour = new Date(now);
    roundedHour.setMinutes(0, 0, 0);
    const twentyFiveHoursAgo = new Date(roundedHour);
    twentyFiveHoursAgo.setHours(twentyFiveHoursAgo.getHours() - (n * 4));
    return twentyFiveHoursAgo;
}

// 之字反转算法实现
function findPeaksWithZigZag(klines, reversalThreshold = 2) {
    if (klines.length < 2) return [];

    const highPoints = [];
    let lastPotentialHigh = null;

    // 遍历K线（为回溯考虑，从第6根K线开始分析）
    for (let i = 5; i < klines.length; i++) {
        let isHigh = true;

        // 检查前5根K线中的价格是否低于当前K线
        for (let j = i - 5; j < i; j++) {
            if (klines[j] > klines[i]) {
                isHigh = false;
                break;
            }
        }

        if (isHigh) {
            // 检查此K线之后是否还有更高的价格（暂时标记为潜在高点）
            lastPotentialHigh = i;
        }

        // 当价格出现超过阈值幅度的下跌时，确认之前标记的潜在高点
        if (lastPotentialHigh !== null && i > lastPotentialHigh) {
            const reversalPercent =
                (klines[lastPotentialHigh] - klines[i]) / klines[lastPotentialHigh] * 100;

            if (reversalPercent >= reversalThreshold) {
                highPoints.push({ index: lastPotentialHigh, price: klines[lastPotentialHigh] });
                lastPotentialHigh = null; // 重置
            }
        }
    }

    // 处理最后一个潜在高点
    if (lastPotentialHigh !== null) {
        highPoints.push({ index: lastPotentialHigh, price: klines[lastPotentialHigh] });
    }

    return highPoints;
}

// 提取形态序列 (标准化)
function extractPattern(prices, startIndex, endIndex) {
    if (endIndex <= startIndex || endIndex >= prices.length) return [];

    const segment = prices.slice(startIndex, endIndex + 1);
    const minPrice = Math.min(...segment);
    const maxPrice = Math.max(...segment);

    // 标准化到0-1范围
    return segment.map(price => (price - minPrice) / (maxPrice - minPrice));
}

// 计算双顶相似度
function calculateSimilarity(series) {
    // 标准双顶形态模板 (M形)
    const doubleTopTemplate = [0.2, 0.3, 0.4, 0.5, 0.8, 0.95, 1.0, 0.95, 0.8, 0.5, 0.6, 0.85, 0.98, 1.0, 0.96, 0.8, 0.65, 0.5];//, 0.35, 0.2

    // 如果序列太短，返回0
    if (series.length < 10) return 0;

    // 计算DTW距离
    const dtwMatrix = [];

    // 初始化矩阵
    for (let i = 0; i <= series.length; i++) {
        dtwMatrix[i] = [];
        for (let j = 0; j <= doubleTopTemplate.length; j++) {
            if (i === 0 && j === 0) {
                dtwMatrix[i][j] = 0;
            } else if (i === 0 || j === 0) {
                dtwMatrix[i][j] = Infinity;
            } else {
                const cost = Math.abs(series[i - 1] - doubleTopTemplate[j - 1]);
                dtwMatrix[i][j] = cost + Math.min(dtwMatrix[i - 1][j], dtwMatrix[i][j - 1], dtwMatrix[i - 1][j - 1]);
            }
        }
    }

    // 获取DTW距离
    const distance = dtwMatrix[series.length][doubleTopTemplate.length];

    // 转换为相似度 (0-1)
    return 1 / (1 + distance / doubleTopTemplate.length);
}

// 格式化时间
function formatTime(date) {
    return date.toTimeString().substring(0, 8);
}

// 格式化日期
function formatDate(date) {
    const year = date.getFullYear();
    const month = (date.getMonth() + 1).toString().padStart(2, '0');
    const day = date.getDate().toString().padStart(2, '0');
    return `${year}年${month}月${day}日`;
}

// 完整日期时间格式
function formatDateTime(date) {
    return `${formatDate(date)} ${formatTime(date)}`;
}