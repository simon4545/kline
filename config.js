/**
 * 币安合约扫描器 - 配置文件
 */

const config = {
  // 币安API配置
  binance: {
    // 合约行情API基础URL
    baseUrl: 'https://fapi.binance.com',
    // 现货行情API基础URL
    spotBaseUrl: 'https://api.binance.com',
    // 每个请求间隔500毫秒
    delay: 100
  },

  // 筛选条件
  filter: {
    // 上线不超过365天
    maxAgeDays: 365,
    // 日线EMA20周期
    emaPeriod: 20,
    // 最近100天最低点为底
    bottomDays: 100,
    // 最近10天最高点为顶
    topDays: 10,
    // 回调超过10%
    pullbackPercent: 10,
    // 排除的大币种
    excludeSymbols: ['BCHUSDT', 'TRXUSDT', 'BTCUSDT', 'ETHUSDT', 'SOLUSDT', 'BNBUSDT', 'XRPUSDT', 'ADAUSDT', 'DOGEUSDT', 'DOTUSDT', 'MATICUSDT', 'LTCUSDT']
  },

  // 邮件配置
  email: {
    // 发送者邮箱
    from: 'your-email@gmail.com',
    // 邮箱授权码
    auth: {
      user: 'your-email@gmail.com',
      pass: 'your-email-auth-code'
    },
    // 接收者邮箱
    to: 'simon4545@qq.com',
    // 邮件主题
    subject: '币安合约扫描报告'
  },

  // 定时任务配置
  schedule: {
    // 每半天执行一次 (0 0,12 * * * 表示凌晨0点和中午12点)
    cron: '0 0,12 * * *'
  },

  // Excel文件配置
  excel: {
    // 文件输出目录
    outputDir: './reports'
  }
};

module.exports = config;