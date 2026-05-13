/**
 * 技术分析类
 */

const config = require('./config');

class TechnicalAnalyzer {
  constructor() {
    this.emaPeriod = config.filter.emaPeriod;
    this.bottomDays = config.filter.bottomDays;
    this.topDays = config.filter.topDays;
    this.pullbackPercent = config.filter.pullbackPercent;
  }

  /**
   * 计算EMA
   */
  calculateEMA(prices, period) {
    if (prices.length < period) return null;

    const k = 2 / (period + 1);
    let ema = prices.slice(0, period).reduce((sum, price) => sum + price, 0) / period;

    for (let i = period; i < prices.length; i++) {
      ema = prices[i] * k + ema * (1 - k);
    }

    return ema;
  }

  /**
   * 获取最近N天的最低点
   */
  getLowestLow(klines, days) {
    const recentKlines = klines.slice(-days);
    return Math.min(...recentKlines.map(k => k.low));
  }

  /**
   * 获取最近N天的最高点
   */
  getHighestHigh(klines, days) {
    const recentKlines = klines.slice(-days);
    return Math.max(...recentKlines.map(k => k.high));
  }

  /**
   * 检查是否满足EMA20突破条件 (当前价格在EMA20之上)
   */
  isAboveEMA20(klines) {
    if (klines.length < this.emaPeriod + 1) return false;

    const closes = klines.map(k => k.close);
    const ema = this.calculateEMA(closes, this.emaPeriod);

    if (ema === null) return false;

    const currentPrice = closes[closes.length - 1];
    const previousPrice = closes[closes.length - 2];

    // 当前价格在EMA20之上，且前一根K线价格在EMA20之下 (价格上穿)
    // return currentPrice > ema && previousPrice <= ema;
    return currentPrice > ema;
  }

  /**
   * 检查是否满足回调幅度条件
   * 最近100天的最低点为底，最近10天的最高点为顶，回调超过10%
   */
  checkPullback(klines) {
    if (klines.length < this.bottomDays) return false;

    const lowestLow = this.getLowestLow(klines, this.bottomDays);
    const highestHigh = this.getHighestHigh(klines, this.topDays);
    const currentPrice = klines[klines.length - 1].close;

    // 顶到当前的回调幅度
    const pullbackFromTop = ((highestHigh - currentPrice) / highestHigh) * 100;

    // 底到顶的涨幅
    const riseFromBottom = ((highestHigh - lowestLow) / lowestLow) * 100;

    return {
      lowestLow,
      highestHigh,
      currentPrice,
      pullbackFromTop,
      riseFromBottom,
      meetsPullbackCriteria: pullbackFromTop >= this.pullbackPercent
    };
  }

  /**
   * 分析单个币种
   */
  analyze(klines) {
    const result = {
      isAboveEMA20: this.isAboveEMA20(klines),
      pullbackInfo: this.checkPullback(klines)
    };

    return result;
  }
}

module.exports = TechnicalAnalyzer;