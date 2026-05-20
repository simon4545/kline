// 配置
const INTERVALS = [
    { key: '5m', label: '5分钟', wsInterval: '5m' },
    { key: '15m', label: '15分钟', wsInterval: '15m' },
    { key: '1h', label: '1小时', wsInterval: '1h' },
    { key: '4h', label: '4小时', wsInterval: '4h' },
    { key: '1d', label: '日线', wsInterval: '1d' }
];
const SYMBOL = 'XAUUSDT';
const LIMIT = 500;

// 全局存储
const chartData = {};
const charts = {};
let wsConnections = {};

// TD Sequential(九转)参数
const TD_LOOKBACK = 4;
const TD_MAX_COUNT = 9;
const TD_SHOW_FROM = 6;

// EMA计算
function calculateEMA(data, period) {
    if (data.length < period) return data.map(() => null);
    const emaResult = EMA.calculate({ period: period, values: data });
    // 将结果对齐到原始数据长度，前面填充null
    const ema = new Array(data.length).fill(null);
    for (let i = period - 1; i < data.length; i++) {
        ema[i] = emaResult[i - (period - 1)];
    }
    return ema;
}

// 计算布林带
function calculateBollingerBands(closes, period = 20, stdDev = 2) {
    const bbResult = BollingerBands.calculate({ period: period, stdDev: stdDev, values: closes });
    const result = { middle: new Array(closes.length).fill(null), upper: new Array(closes.length).fill(null), lower: new Array(closes.length).fill(null) };
    for (let i = period - 1; i < closes.length; i++) {
        const bbIndex = i - (period - 1);
        if (bbIndex >= 0 && bbIndex < bbResult.length) {
            result.middle[i] = bbResult[bbIndex].middle;
            result.upper[i] = bbResult[bbIndex].upper;
            result.lower[i] = bbResult[bbIndex].lower;
        }
    }
    return result;
}

// 计算CCI
function calculateCCI(high, low, close, period = 20) {
    const cciResult = CCI.calculate({ period: period, high: high, low: low, close: close });
    const result = new Array(close.length).fill(null);
    for (let i = period - 1; i < close.length; i++) {
        const cciIndex = i - (period - 1);
        if (cciIndex >= 0 && cciIndex < cciResult.length) {
            result[i] = cciResult[cciIndex];
        }
    }
    return result;
}

// 计算RSI
function calculateRSI(closes, period = 14) {
    const rsiResult = RSI.calculate({ period: period, values: closes });
    const result = new Array(closes.length).fill(null);
    for (let i = period; i < closes.length; i++) {
        const rsiIndex = i - period;
        if (rsiIndex >= 0 && rsiIndex < rsiResult.length) {
            result[i] = rsiResult[rsiIndex];
        }
    }
    return result;
}

// 计算TD九转 Setup（收盘价与4根前比较）
function calculateTDSequential(closes, lookback = TD_LOOKBACK) {
    const sequences = [];

    let activeType = null; // 'buy' | 'sell' | null
    let activeStart = -1;
    let activeBars = [];
    let activeCount = 0;

    function flushActive(finalized) {
        if (!activeType || activeBars.length === 0) return;
        sequences.push({
            type: activeType,
            start: activeStart,
            end: activeBars[activeBars.length - 1].index,
            bars: activeBars.slice(),
            completed9: finalized && activeCount >= TD_MAX_COUNT
        });
        activeType = null;
        activeStart = -1;
        activeBars = [];
        activeCount = 0;
    }

    for (let i = 0; i < closes.length; i++) {
        if (i < lookback) {
            flushActive(false);
            continue;
        }

        const c = closes[i];
        const c4 = closes[i - lookback];

        let barType = null;
        if (c < c4) barType = 'buy';
        else if (c > c4) barType = 'sell';

        if (!barType) {
            flushActive(false);
            continue;
        }

        if (activeType === barType) {
            activeCount += 1;
        } else {
            flushActive(false);
            activeType = barType;
            activeStart = i;
            activeCount = 1;
            activeBars = [];
        }

        const displayCount = activeCount <= TD_MAX_COUNT ? activeCount : ((activeCount - 1) % TD_MAX_COUNT) + 1;
        activeBars.push({ index: i, count: displayCount });

        // 满9后结束本轮，后续重新开始下一轮
        if (activeCount === TD_MAX_COUNT) {
            flushActive(true);
        }
    }

    // 未满9的末尾序列视为未成立
    flushActive(false);

    return sequences;
}

function buildTDMarkers(data) {
    if (!data || data.length <= TD_LOOKBACK) return [];

    const closes = data.map(d => d.close);
    const sequences = calculateTDSequential(closes);
    const markers = [];

    for (const seq of sequences) {
        // 只有“最终达到9”的序列才显示；否则整段不显示（含此前6~8）
        if (!seq.completed9) continue;

        for (const b of seq.bars) {
            // 到第6转后，才把本轮1~9全部显示出来
            if (b.count < TD_SHOW_FROM) continue;

            const startIndex = Math.max(0, b.index - (b.count - 1));
            for (let j = startIndex; j <= b.index; j++) {
                const count = j - startIndex + 1;
                if (count > TD_MAX_COUNT) break;
                const bar = data[j];

                if (seq.type === 'buy') {
                    markers.push({
                        time: bar.time,
                        position: 'belowBar',
                        color: count === TD_MAX_COUNT ? '#00e676' : '#26a69a',
                        shape: 'circle',
                        text: `${count}`
                    });
                } else {
                    markers.push({
                        time: bar.time,
                        position: 'aboveBar',
                        color: count === TD_MAX_COUNT ? '#ff5252' : '#ef5350',
                        shape: 'circle',
                        text: `${count}`
                    });
                }
            }
        }
    }

    // 同一时间可能被重复推入，按 time+position 去重，保留最后一条
    const dedup = new Map();
    for (const m of markers) {
        dedup.set(`${m.time}-${m.position}`, m);
    }
    return Array.from(dedup.values()).sort((a, b) => a.time - b.time);
}

function updateTDMarkers(intervalKey) {
    const data = chartData[intervalKey];
    const s = charts[intervalKey];
    if (!data || !s || !s.tdSeries) return;

    s.tdSeries.setData(data.map(d => ({ time: d.time, value: d.close })));
    s.tdSeries.setMarkers(buildTDMarkers(data));
}

// 创建图表容器
function createChartPanel(intervalConfig) {
    const grid = document.getElementById('charts-grid');
    const panel = document.createElement('div');
    panel.className = 'chart-panel';
    panel.id = `panel-${intervalConfig.key}`;
    panel.innerHTML = `
        <div class="chart-panel-header">
            <div class="chart-panel-title">${intervalConfig.label}</div>
        </div>
        <div class="main-chart-container" id="main-chart-${intervalConfig.key}"></div>
        <div class="indicators-row">
            <div class="indicator-container" id="cci-chart-${intervalConfig.key}">
                <div class="indicator-label">CCI</div>
            </div>
            <div class="indicator-container" id="rsi-chart-${intervalConfig.key}">
                <div class="indicator-label">RSI</div>
            </div>
        </div>
    `;
    grid.appendChild(panel);
    return panel;
}

// 初始化图表
function initCharts(intervalConfig) {
    const mainContainer = document.getElementById(`main-chart-${intervalConfig.key}`);
    const cciContainer = document.getElementById(`cci-chart-${intervalConfig.key}`);
    const rsiContainer = document.getElementById(`rsi-chart-${intervalConfig.key}`);

    // 主图
    const chart = LightweightCharts.createChart(mainContainer, {
        width: mainContainer.clientWidth,
        height: 220,
        layout: { backgroundColor: '#1a1a1a', textColor: '#888' },
        grid: { vertLines: { color: '#2a2a2a' }, horzLines: { visible: false } },
        timeScale: { timeVisible: true, secondsVisible: false }
    });

    const candlestickSeries = chart.addCandlestickSeries({
        upColor: '#2ecc71', downColor: '#e74c3c',
        borderUpColor: '#2ecc71', borderDownColor: '#e74c3c',
        wickUpColor: '#2ecc71', wickDownColor: '#e74c3c'
    });
    const lineOpts = {
        // priceScaleId: 'indicator',
        priceLineVisible: false,
        lastValueVisible: false
    };
    // EMA线 (使用独立价格刻度，隐藏标签)
    const ema36Series = chart.addLineSeries({ ...lineOpts, color: '#e74c3c', lineWidth: 1 });
    const ema43Series = chart.addLineSeries({ ...lineOpts, color: '#e74c3c', lineWidth: 1 });
    const ema144Series = chart.addLineSeries({ ...lineOpts, color: '#3498db', lineWidth: 1 });
    const ema169Series = chart.addLineSeries({ ...lineOpts, color: '#3498db', lineWidth: 1 });

    // 布林带 (使用独立价格刻度，隐藏标签)
    const bbUpperSeries = chart.addLineSeries({ ...lineOpts, color: '#9b59b6', lineWidth: 1, lineStyle: 2 });
    const bbMiddleSeries = chart.addLineSeries({ ...lineOpts, color: '#f39c12', lineWidth: 1 });
    const bbLowerSeries = chart.addLineSeries({ ...lineOpts, color: '#9b59b6', lineWidth: 1, lineStyle: 2 });

    // 配置独立价格刻度，隐藏所有指标标签
    // chart.priceScale('indicator').applyOptions({
    //     scaleMargins: { top: 0.1, bottom: 0.1 },
    //     entireTextVisible: false,
    //     drawTicks: false,
    //     borderVisible: false
    // });

    charts[intervalConfig.key] = {
        main: chart,
        candlestick: candlestickSeries,
        ema36: ema36Series,
        ema43: ema43Series,
        ema144: ema144Series,
        ema169: ema169Series,
        bbUpper: bbUpperSeries,
        bbMiddle: bbMiddleSeries,
        bbLower: bbLowerSeries
    };

    // TD九转标记承载序列（透明，不显示线，仅用于markers）
    const tdSeries = chart.addLineSeries({
        color: 'transparent',
        lineWidth: 1,
        priceLineVisible: false,
        lastValueVisible: false
    });
    charts[intervalConfig.key].tdSeries = tdSeries;

    // CCI图
    const cciChart = LightweightCharts.createChart(cciContainer, {
        width: cciContainer.clientWidth, height: 100,
        layout: { backgroundColor: '#1a1a1a', textColor: '#888' },
        grid: { vertLines: { color: '#2a2a2a' }, horzLines: { color: '#2a2a2a' } },
        timeScale: { visible: false },
        crosshair: { mode: LightweightCharts.CrosshairMode.Normal }
    });
    const cciSeries = cciChart.addLineSeries({ color: '#00bcd4', lineWidth: 1 });
    charts[intervalConfig.key].cci = cciChart;
    charts[intervalConfig.key].cciSeries = cciSeries;

    // RSI图
    const rsiChart = LightweightCharts.createChart(rsiContainer, {
        width: rsiContainer.clientWidth, height: 100,
        layout: { backgroundColor: '#1a1a1a', textColor: '#888' },
        grid: { vertLines: { color: '#2a2a2a' }, horzLines: { color: '#2a2a2a' } },
        timeScale: { visible: false },
        crosshair: { mode: LightweightCharts.CrosshairMode.Normal }
    });
    const rsiSeries = rsiChart.addLineSeries({ color: '#ff9800', lineWidth: 1 });
    charts[intervalConfig.key].rsi = rsiChart;
    charts[intervalConfig.key].rsiSeries = rsiSeries;

    // 响应式
    // window.addEventListener('resize', () => {
    //     chart.applyOptions({ width: mainContainer.clientWidth });
    //     cciChart.applyOptions({ width: cciContainer.clientWidth });
    //     rsiChart.applyOptions({ width: rsiContainer.clientWidth });
    // });
}

// 更新指标数据
function updateIndicators(intervalKey) {
    const data = chartData[intervalKey];
    if (!data || data.length < 169) return;

    const closes = data.map(d => d.close);
    const highs = data.map(d => d.high);
    const lows = data.map(d => d.low);
    const times = data.map(d => d.time);

    const s = charts[intervalKey];

    // EMA
    const ema36 = calculateEMA(closes, 36);
    const ema43 = calculateEMA(closes, 43);
    const ema144 = calculateEMA(closes, 144);
    const ema169 = calculateEMA(closes, 169);

    s.ema36.setData(times.map((t, i) => ({ time: t, value: ema36[i] })).filter(d => d.value !== null));
    s.ema43.setData(times.map((t, i) => ({ time: t, value: ema43[i] })).filter(d => d.value !== null));
    s.ema144.setData(times.map((t, i) => ({ time: t, value: ema144[i] })).filter(d => d.value !== null));
    s.ema169.setData(times.map((t, i) => ({ time: t, value: ema169[i] })).filter(d => d.value !== null));

    // 布林带
    const bb = calculateBollingerBands(closes, 21, 2);
    s.bbUpper.setData(times.map((t, i) => ({ time: t, value: bb.upper[i] })).filter(d => d.value !== null));
    s.bbMiddle.setData(times.map((t, i) => ({ time: t, value: bb.middle[i] })).filter(d => d.value !== null));
    s.bbLower.setData(times.map((t, i) => ({ time: t, value: bb.lower[i] })).filter(d => d.value !== null));

    // CCI
    const cci = calculateCCI(highs, lows, closes, 20);
    s.cciSeries.setData(times.map((t, i) => ({ time: t, value: cci[i] })).filter(d => d.value !== null));

    // RSI
    const rsi = calculateRSI(closes, 14);
    s.rsiSeries.setData(times.map((t, i) => ({ time: t, value: rsi[i] })).filter(d => d.value !== null));

    // TD九转
    updateTDMarkers(intervalKey);
}

// 获取历史K线
async function fetchHistoricalKlines(symbol, interval, limit) {
    const url = `https://fapi.binance.com/fapi/v1/klines?symbol=${symbol}&interval=${interval}&limit=${limit}`;
    try {
        const response = await fetch(url);
        const data = await response.json();
        return data.map(k => ({
            time: Math.floor(k[0] / 1000),
            open: parseFloat(k[1]),
            high: parseFloat(k[2]),
            low: parseFloat(k[3]),
            close: parseFloat(k[4]),
            volume: parseFloat(k[5]),
        }));
    } catch (error) {
        console.error('获取历史K线失败:', error);
        return [];
    }
}

// 连接WebSocket
function connectWebSocket(intervalConfig) {
    const wsUrl = `wss://fstream.binance.com/market/ws/${SYMBOL.toLowerCase()}@kline_${intervalConfig.wsInterval}`;
    console.log('连接WebSocket:', wsUrl);

    const ws = new WebSocket(wsUrl);

    ws.onopen = () => {
        console.log('WebSocket已连接:', intervalConfig.key);
        updateConnectionStatus();
    };

    ws.onmessage = (event) => {
        const msg = JSON.parse(event.data);
        const kline = msg.k;
        const newKline = {
            time: Math.floor(kline.t / 1000),
            open: parseFloat(kline.o),
            high: parseFloat(kline.h),
            low: parseFloat(kline.l),
            close: parseFloat(kline.c),
        };

        // 更新页面标题显示实时价格
        document.title = `$${kline.c} 多周期K线图表`;

        const data = chartData[intervalConfig.key];
        if (!data) return;

        const lastIndex = data.findIndex(k => k.time === newKline.time);
        if (lastIndex >= 0) {
            data[lastIndex] = newKline;
        } else {
            data.push(newKline);
        }

        charts[intervalConfig.key].candlestick.update(newKline);
        updateIndicators(intervalConfig.key);
    };

    ws.onerror = (error) => {
        console.error('WebSocket错误:', intervalConfig.key, error);
    };

    ws.onclose = () => {
        console.log('WebSocket断开:', intervalConfig.key);
        updateConnectionStatus();
        // 5秒后重连
        setTimeout(() => {
            console.log('重新连接:', intervalConfig.key);
            connectWebSocket(intervalConfig);
        }, 5000);
    };

    wsConnections[intervalConfig.key] = ws;
}

// 更新连接状态
function updateConnectionStatus() {
    const total = INTERVALS.length;
    const connected = Object.values(wsConnections).filter(ws => ws.readyState === WebSocket.OPEN).length;
    if (connected === total) {
        document.getElementById('status').textContent = '已连接 - 实时数据';
        document.getElementById('status').className = 'status connected';
    } else {
        document.getElementById('status').textContent = `连接中... (${connected}/${total})`;
        document.getElementById('status').className = 'status';
    }
}

// 加载数据
async function loadData(intervalConfig) {
    const data = await fetchHistoricalKlines(SYMBOL, intervalConfig.key, LIMIT);
    if (data.length > 0) {
        chartData[intervalConfig.key] = data;
        charts[intervalConfig.key].candlestick.setData(data);
        updateIndicators(intervalConfig.key);

        const visibleStart = data[Math.max(0, data.length - 100)].time;
        const visibleEnd = data[data.length - 1].time;
        charts[intervalConfig.key].main.timeScale().setVisibleRange({
            from: visibleStart,
            to: visibleEnd,
        });
    }
}

// 初始化所有图表
async function init() {
    document.getElementById('status').textContent = '正在加载历史数据...';

    // 创建所有图表
    for (const intervalConfig of INTERVALS) {
        createChartPanel(intervalConfig);
    }

    // 等待DOM更新
    await new Promise(resolve => setTimeout(resolve, 100));

    // 初始化图表
    for (const intervalConfig of INTERVALS) {
        initCharts(intervalConfig);
    }

    // 加载历史数据
    await Promise.all(INTERVALS.map(loadData));

    // 连接WebSocket
    for (const intervalConfig of INTERVALS) {
        connectWebSocket(intervalConfig);
    }
}

// 启动
document.addEventListener('DOMContentLoaded', init);

// 响应式 - 窗口大小变化时调整图表
window.addEventListener('resize', () => {
    INTERVALS.forEach(intervalConfig => {
        const s = charts[intervalConfig.key];
        if (!s) return;
        const mainContainer = document.getElementById(`main-chart-${intervalConfig.key}`);
        const cciContainer = document.getElementById(`cci-chart-${intervalConfig.key}`);
        const rsiContainer = document.getElementById(`rsi-chart-${intervalConfig.key}`);
        if (mainContainer) s.main.resize(mainContainer.clientWidth, 220);
        if (cciContainer) s.cci.resize(cciContainer.clientWidth, 100);
        if (rsiContainer) s.rsi.resize(rsiContainer.clientWidth, 100);
    });
});
