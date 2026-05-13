// 存储所有创建的图表实例
const charts = {};

// 存储每个图表的指标系列引用
const chartSeries = {};

// 指标显示状态
let emaVisible = true;
let maVisible = true;
let bbVisible = true;

// 收藏相关
const FAVORITES_KEY = 'kline_favorites';
let favorites = [];

// 当前筛选模式：'all' 或 'favorites'
let filterMode = 'all';

// 获取收藏列表
function getFavorites() {
    try {
        const stored = localStorage.getItem(FAVORITES_KEY);
        favorites = stored ? JSON.parse(stored) : [];
    } catch (e) {
        favorites = [];
    }
    return favorites;
}

// 保存收藏列表
function saveFavorites() {
    localStorage.setItem(FAVORITES_KEY, JSON.stringify(favorites));
}

// 切换收藏状态
function toggleFavorite(symbol) {
    const index = favorites.indexOf(symbol);
    if (index > -1) {
        favorites.splice(index, 1);
    } else {
        favorites.push(symbol);
    }
    saveFavorites();
    updateFavoriteButtons();
    updateFavoritesCount();
}

// 更新所有收藏按钮状态
function updateFavoriteButtons() {
    document.querySelectorAll('.favorite-btn').forEach(btn => {
        const symbol = btn.getAttribute('data-symbol');
        if (favorites.includes(symbol)) {
            btn.classList.add('active');
            btn.textContent = '★';
        } else {
            btn.classList.remove('active');
            btn.textContent = '☆';
        }
    });
}

// 更新收藏计数
function updateFavoritesCount() {
    const countEl = document.getElementById('favorites-count');
    if (countEl) {
        countEl.textContent = `已收藏 ${favorites.length} 个`;
    }
}

// 获取过滤后的合约列表
function getFilteredContracts(contracts) {
    if (filterMode === 'favorites') {
        return contracts.filter(c => favorites.includes(c));
    }
    return contracts;
}

// 页面加载完成后初始化
document.addEventListener('DOMContentLoaded', function() {
    // 初始化收藏
    getFavorites();
    
    // 如果有收藏，自动勾选"只看收藏"开关
    const filterFavorites = document.getElementById('filter-favorites');
    if (favorites.length > 0) {
        filterFavorites.checked = true;
        filterMode = 'favorites';
    }
    updateFavoritesCount();
    // 获取所有时间级别按钮
    const timeframeButtons = document.querySelectorAll('.timeframe-btn');
    
    // 为每个按钮添加点击事件监听器
    timeframeButtons.forEach(button => {
        button.addEventListener('click', function() {
            // 移除所有按钮的 active 类
            timeframeButtons.forEach(btn => btn.classList.remove('active'));
            
            // 为当前点击的按钮添加 active 类
            this.classList.add('active');
            
            // 获取选中的时间级别
            const interval = this.getAttribute('data-interval');
            
            // 加载对应时间级别的数据
            loadCharts(interval);
        });
    });
    
    // EMA指标切换开关
    const toggleEma = document.getElementById('toggle-ema');
    toggleEma.addEventListener('change', function() {
        emaVisible = this.checked;
        Object.keys(chartSeries).forEach(symbol => {
            if (chartSeries[symbol] && chartSeries[symbol].ema) {
                chartSeries[symbol].ema.applyOptions({ visible: emaVisible });
            }
        });
    });

    // MA指标切换开关
    const toggleMa = document.getElementById('toggle-ma');
    toggleMa.addEventListener('change', function() {
        maVisible = this.checked;
        Object.keys(chartSeries).forEach(symbol => {
            if (chartSeries[symbol] && chartSeries[symbol].ma) {
                chartSeries[symbol].ma.applyOptions({ visible: maVisible });
            }
        });
    });

    // 布林带切换开关
    const toggleBb = document.getElementById('toggle-bb');
    toggleBb.addEventListener('change', function() {
        bbVisible = this.checked;
        Object.keys(chartSeries).forEach(symbol => {
            if (chartSeries[symbol] && chartSeries[symbol].bb) {
                chartSeries[symbol].bb.middle.applyOptions({ visible: bbVisible });
                chartSeries[symbol].bb.upper.applyOptions({ visible: bbVisible });
                chartSeries[symbol].bb.lower.applyOptions({ visible: bbVisible });
            }
        });
    });

    // 收藏筛选开关
    filterFavorites.addEventListener('change', function() {
        filterMode = this.checked ? 'favorites' : 'all';
        if (currentInterval) {
            loadCharts(currentInterval, true);
        }
    });

    // K线图表显示开关
    const toggleChart = document.getElementById('toggle-chart');
    toggleChart.addEventListener('change', function() {
        document.querySelectorAll('.chart-container').forEach(container => {
            if (toggleChart.checked) {
                container.style.display = '';
            } else {
                container.style.display = 'none';
            }
        });
    });
});

// 限制同时加载的图表数量
const MAX_CONCURRENT_CHARTS = 2;

// 当前选中的时间级别
let currentInterval = null;

// 加载图表的主函数
async function loadCharts(interval, force = false) {
    // 如果重复点击同一个时间级别，不重新加载（除非force为true）
    if (currentInterval === interval && !force) {
        return;
    }
    
    currentInterval = interval;
    
    const statusElement = document.getElementById('status');
    const loadingElement = document.getElementById('loading');
    const chartsContainer = document.getElementById('charts-container');
    
    // 清理之前的图表
    Object.keys(charts).forEach(symbol => {
        if (charts[symbol] && typeof charts[symbol].remove === 'function') {
            charts[symbol].remove();
        }
    });

    // 清空图表对象
    Object.keys(charts).forEach(key => delete charts[key]);
    Object.keys(chartSeries).forEach(key => delete chartSeries[key]);
    
    // 显示加载状态
    loadingElement.style.display = 'block';
    statusElement.textContent = `正在获取合约列表...`;
    chartsContainer.innerHTML = '';
    
    try {
        // 获取合约列表
        const allContracts = await fetchMyContracts();
        
        // 根据筛选模式过滤
        const contracts = getFilteredContracts(allContracts);
        
        const filterText = filterMode === 'favorites' ? '(已收藏)' : '(全部)';
        statusElement.textContent = `找到 ${contracts.length} 个合约代币 ${filterText}，正在加载数据...`;
        
        // 如果没有合约，显示提示信息
        if (!contracts || contracts.length === 0) {
            if (filterMode === 'favorites') {
                chartsContainer.innerHTML = '<div style="text-align: center; width: 100%; padding: 20px;">暂无收藏的代币<br><br>点击代币名称旁的☆即可收藏</div>';
            } else {
                chartsContainer.innerHTML = '<div style="text-align: center; width: 100%; padding: 20px;">未找到合约数据</div>';
            }
            statusElement.textContent = '未找到合约数据';
            loadingElement.style.display = 'none';
            return;
        }
        
        // 清空图表容器
        chartsContainer.innerHTML = '';
        
        // 限制同时处理的图表数量
        let loadedCharts = 0;
        const totalCharts = contracts.length;
        
        // 创建一个处理图表的函数
        const processChart = async (symbol) => {
            try {
                // 获取K线数据
                const klines = await fetchBinanceKlines(symbol, interval, 500);
                
                if (klines && klines.length > 0) {
                    // 创建图表容器
                    const chartWrapper = document.createElement('div');
                    chartWrapper.className = 'chart-wrapper';
                    const isFavorite = favorites.includes(symbol);
                    chartWrapper.innerHTML = `
                        <div class="chart-header">
                            ${symbol}
                            <button class="favorite-btn ${isFavorite ? 'active' : ''}" data-symbol="${symbol}" onclick="toggleFavorite('${symbol}')">${isFavorite ? '★' : '☆'}</button>
                        </div>
                        <div class="chart-container" id="chart-${symbol}"></div>
                    `;
                    chartsContainer.appendChild(chartWrapper);
                    
                    // 渲染图表
                    renderChart(symbol, klines, interval);
                    
                    loadedCharts++;
                    statusElement.textContent = `正在加载图表 ${loadedCharts}/${totalCharts}...`;
                }
            } catch (error) {
                console.error(`加载 ${symbol} 的数据失败:`, error);
            }
        };
        
        // 分批处理图表以避免浏览器过载
        const processChartsInBatches = async (symbols, batchSize) => {
            for (let i = 0; i < symbols.length; i += batchSize) {
                const batch = symbols.slice(i, i + batchSize);
                await Promise.all(batch.map(processChart));
            }
        };
        
        // 处理所有合约
        await processChartsInBatches(contracts, MAX_CONCURRENT_CHARTS);
        
        statusElement.textContent = `加载完成，共显示 ${chartsContainer.children.length} 个图表`;
    } catch (error) {
        console.error('加载图表失败:', error);
        statusElement.textContent = `加载失败: ${error.message}`;
    } finally {
        // 隐藏加载状态
        loadingElement.style.display = 'none';
    }
}

// 页面卸载时清理图表
window.addEventListener('beforeunload', function() {
    Object.keys(charts).forEach(symbol => {
        if (charts[symbol] && typeof charts[symbol].remove === 'function') {
            charts[symbol].remove();
        }
    });
});

// 处理窗口大小调整
window.addEventListener('resize', function() {
    Object.keys(charts).forEach(symbol => {
        const chart = charts[symbol];
        const chartContainer = document.getElementById(`chart-${symbol}`);
        if (chart && chartContainer) {
            chart.resize(chartContainer.clientWidth, 400);
        }
    });
});

// 渲染单个图表的函数
function renderChart(symbol, klines, interval) {
    // 获取图表容器
    const chartContainer = document.getElementById(`chart-${symbol}`);
    if (!chartContainer) return;
    
    // 清空容器
    chartContainer.innerHTML = '';
    
    // 将K线数据转换为图表所需的格式
    const chartData = klines.map(kline => ({
        time: kline[0] / 1000, // 转换为秒
        open: parseFloat(kline[1]),
        high: parseFloat(kline[2]),
        low: parseFloat(kline[3]),
        close: parseFloat(kline[4])
    }));
    
    // 创建图表
    const chart = LightweightCharts.createChart(chartContainer, {
        width: chartContainer.clientWidth,
        height: 400,
        layout: {
            backgroundColor: '#ffffff',
            textColor: 'rgba(33, 56, 77, 1)',
        },
        grid: {
            vertLines: {
                color: 'rgba(197, 203, 206, 0.5)',
            },
            horzLines: {
                color: 'rgba(197, 203, 206, 0.5)',
            },
        },
        timeScale: {
            timeVisible: true,
            secondsVisible: false,
        }
    });
    
    // 添加K线系列
    const candlestickSeries = chart.addCandlestickSeries({
        upColor: '#26a69a',
        downColor: '#ef5350',
        borderDownColor: '#ef5350',
        borderUpColor: '#26a69a',
        wickDownColor: '#ef5350',
        wickUpColor: '#26a69a',
    });
    
    // 设置K线数据
    candlestickSeries.setData(chartData);
    
    // 计算并添加EMA指标
    const closes = chartData.map(d => d.close);
    const series = {};

    if (emaVisible) {
        series.ema = renderEma(chart, 25, closes, chartData);
    }
    if (maVisible) {
        series.ma = renderSma(chart, 60, closes, chartData);
    }

    // 绘制布林带（周期20，标准差2）
    if (bbVisible) {
        series.bb = renderBollingerBands(chart, 20, 2, closes, chartData);
    }

    // 存储指标系列引用
    chartSeries[symbol] = series;

    // 调整图表以适应数据
    const total = chartData.length;
    const visibleStart = chartData[total - 60].time;
    const visibleEnd = chartData[total - 1].time;

    chart.timeScale().setVisibleRange({
        from: visibleStart,
        to: visibleEnd,
    });
    // 存储图表实例
    charts[symbol] = chart;
}

function renderEma(chart, period, closes, chartData) {
    const ema169 = EMA.calculate({ period: period, values: closes });
    const ema169Data = chartData.slice(period - 1).map((d, i) => ({
        time: d.time,
        value: ema169[i]
    }));
    let color = period < 200 ? '#eaaa42' : '#a691ed';
    const ema169Series = chart.addLineSeries({
        color: color,
        lineWidth: 1,
    });
    ema169Series.setData(ema169Data);
    return ema169Series;
}

function renderSma(chart, period, closes, chartData) {
    const ema169 = SMA.calculate({ period: period, values: closes });
    const ema169Data = chartData.slice(period - 1).map((d, i) => ({
        time: d.time,
        value: ema169[i]
    }));
    let color = '#a691ed';
    const ema169Series = chart.addLineSeries({
        color: color,
        lineWidth: 1,
    });
    ema169Series.setData(ema169Data);
    return ema169Series;
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

// 渲染布林带
function renderBollingerBands(chart, period, stdDev, closes, chartData) {
    const bands = calculateBollingerBands(closes, period, stdDev);

    // 创建带有时间戳的数据点
    const times = chartData.map(d => d.time);
    
    // 过滤掉null值
    const middleData = times.map((t, i) => ({ time: t, value: bands.middle[i] })).filter(d => d.value !== null);
    const upperData = times.map((t, i) => ({ time: t, value: bands.upper[i] })).filter(d => d.value !== null);
    const lowerData = times.map((t, i) => ({ time: t, value: bands.lower[i] })).filter(d => d.value !== null);

    // 绘制中轨（MA）
    const middleSeries = chart.addLineSeries({
        color: '#3498db',
        lineWidth: 1,
        lineStyle: 2 // 虚线
    });
    middleSeries.setData(middleData);

    // 绘制上轨
    const upperSeries = chart.addLineSeries({
        color: '#e74c3c',
        lineWidth: 1,
        lineStyle: 1
    });
    upperSeries.setData(upperData);

    // 绘制下轨
    const lowerSeries = chart.addLineSeries({
        color: '#2ecc71',
        lineWidth: 1,
        lineStyle: 1
    });
    lowerSeries.setData(lowerData);

    return { middle: middleSeries, upper: upperSeries, lower: lowerSeries };
}
