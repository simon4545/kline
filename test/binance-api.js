/**
 * 币安API类
 */

const config = require('./config');

class BinanceAPI {
  constructor() {
    this.baseUrl = config.binance.baseUrl;
    this.delay = config.binance.delay;
  }

  /**
   * 发送HTTP/HTTPS请求
   */
  async request(method, url, options = {}) {
    const { params, ...rest } = options;

    // 构建完整URL
    const urlObj = new URL(url);
    if (params) {
      Object.keys(params).forEach(key => urlObj.searchParams.append(key, params[key]));
    }

    return new Promise((resolve, reject) => {
      const protocol = urlObj.protocol === 'https:' ? require('https') : require('http');

      const requestOptions = {
        method: method.toUpperCase(),
        hostname: urlObj.hostname,
        port: urlObj.port || (urlObj.protocol === 'https:' ? 443 : 80),
        path: urlObj.pathname + urlObj.search,
        headers: rest.headers || {},
        agent: false,
        timeout: 30000
      };

      const req = protocol.request(requestOptions, (res) => {
        let responseData = '';
        res.on('data', (chunk) => { responseData += chunk; });
        res.on('end', () => {
          try {
            resolve({
              data: JSON.parse(responseData),
              status: res.statusCode,
              headers: res.headers
            });
          } catch (e) {
            resolve({
              data: responseData,
              status: res.statusCode,
              headers: res.headers
            });
          }
        });
      });

      req.on('error', reject);
      req.on('timeout', () => reject(new Error('Request timeout')));
      req.end();
    });
  }

  /**
   * 延迟函数
   */
  async sleep(ms) {
    return new Promise(resolve => setTimeout(resolve, ms));
  }

  /**
   * 获取所有USDT合约列表
   */
  async getExchangeInfo() {
    try {
      const response = await this.request('GET', `${this.baseUrl}/fapi/v1/exchangeInfo`);
      // 过滤出USDT合约
      const symbols = response.data.symbols.filter(
        symbol => symbol.contractType === 'PERPETUAL' &&
                  symbol.quoteAsset === 'USDT' &&
                  symbol.status === 'TRADING'
      );
      return symbols.map(s => s.symbol);
    } catch (error) {
      console.error('获取合约列表失败:', error.message);
      return [];
    }
  }

  /**
   * 获取K线数据
   */
  async getKlines(symbol, interval = '1d', limit = 365) {
    try {
      await this.sleep(this.delay);
      const response = await this.request('GET', `${this.baseUrl}/fapi/v1/klines`, {
        params: { symbol, interval, limit }
      });
      return response.data.map(k => ({
        openTime: k[0],
        open: parseFloat(k[1]),
        high: parseFloat(k[2]),
        low: parseFloat(k[3]),
        close: parseFloat(k[4]),
        volume: parseFloat(k[5]),
        closeTime: k[6]
      }));
    } catch (error) {
      console.error(`获取K线数据失败 ${symbol}:`, error.message);
      return [];
    }
  }

  /**
   * 获取合约上线日期
   */
  async getSymbolLaunchDate(symbol) {
    try {
      await this.sleep(this.delay);
      const response = await this.request('GET', `${this.baseUrl}/fapi/v1/exchangeInfo`);
      const symbolInfo = response.data.symbols.find(s => s.symbol === symbol);
      if (symbolInfo && symbolInfo.onboardDate) {
        return new Date(symbolInfo.onboardDate);
      }
      return null;
    } catch (error) {
      console.error(`获取上线日期失败 ${symbol}:`, error.message);
      return null;
    }
  }

  /**
   * 获取24小时ticker数据
   */
  async get24hrTicker(symbol) {
    try {
      await this.sleep(this.delay);
      const response = await this.request('GET', `${this.baseUrl}/fapi/v1/ticker/24hr`, {
        params: { symbol }
      });
      return {
        symbol: response.data.symbol,
        priceChange: parseFloat(response.data.priceChange),
        priceChangePercent: parseFloat(response.data.priceChangePercent),
        lastPrice: parseFloat(response.data.lastPrice),
        volume: parseFloat(response.data.volume),
        closeTime: response.data.closeTime
      };
    } catch (error) {
      console.error(`获取24小时ticker失败 ${symbol}:`, error.message);
      return null;
    }
  }

  /**
   * 获取所有现货USDT交易对
   */
  async getSpotSymbols() {
    try {
      const response = await this.request('GET', `${config.binance.spotBaseUrl}/api/v3/exchangeInfo`);
      // 过滤出USDT交易对且状态为TRADING的币种
      const symbols = response.data.symbols.filter(
        symbol => symbol.quoteAsset === 'USDT' && symbol.status === 'TRADING'
      );
      return new Set(symbols.map(s => s.symbol));
    } catch (error) {
      console.error('获取现货交易对失败:', error.message);
      return new Set();
    }
  }
}

module.exports = BinanceAPI;