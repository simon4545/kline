/**
 * 扫描器主类
 */

const cron = require('node-cron');
const config = require('./config');
const BinanceAPI = require('./binance-api');
const TechnicalAnalyzer = require('./analyzer');
const EmailSender = require('./email');
const ExcelGenerator = require('./excel');

class Scanner {
  constructor() {
    this.excludeSymbols = config.filter.excludeSymbols;
    this.maxAgeDays = config.filter.maxAgeDays;
    this.binance = new BinanceAPI();
    this.analyzer = new TechnicalAnalyzer();
    this.emailSender = new EmailSender();
    this.excelGenerator = new ExcelGenerator();
  }

  /**
   * 检查币种是否应该被排除
   */
  isExcluded(symbol) {
    return this.excludeSymbols.includes(symbol);
  }

  /**
   * 检查币种上线时间是否超过365天
   */
  isTooOld(launchDate) {
    if (!launchDate) return true;
    const now = new Date();
    const diffDays = (now - launchDate) / (1000 * 60 * 60 * 24);
    return diffDays > this.maxAgeDays;
  }

  /**
   * 执行扫描
   */
  async run() {
    console.log('=== 币安合约扫描开始 ===');
    console.log(`扫描时间: ${new Date().toLocaleString('zh-CN', { timeZone: 'Asia/Shanghai' })}`);

    try {
      // 0. 获取现货交易对列表用于过滤
      console.log('\n[0/5] 获取现货交易对...');
      const spotSymbols = await this.binance.getSpotSymbols();
      console.log(`找到 ${spotSymbols.size} 个现货交易对`);

      // 1. 获取所有USDT合约列表
      console.log('\n[1/5] 获取合约列表...');
      const symbols = await this.binance.getExchangeInfo();
      console.log(`找到 ${symbols.length} 个USDT合约`);

      // 2. 过滤掉大币种和现货已有的币种
      const filteredSymbols = symbols.filter(s => !this.isExcluded(s) && !spotSymbols.has(s));
      console.log(`排除大币种和现货后剩余 ${filteredSymbols.length} 个`);

      // 3. 获取每个币种的K线数据并分析
      console.log('\n[2/6] 开始分析币种...');
      const results = [];
      let processed = 0;

      for (const symbol of filteredSymbols) {
        await this.binance.sleep(100)
        processed++;
        console.log(`分析中 [${processed}/${filteredSymbols.length}]: ${symbol}`);

        try {
          // 获取K线数据 (获取365天数据以满足各种筛选条件)
          const klines = await this.binance.getKlines(symbol, '1d', 365);

          if (klines.length < 10) {
            console.log(`  -> 数据不足，跳过`);
            continue;
          }
          if (klines.length >= 300) {
            console.log(`  -> 数据太久，跳过`);
            continue;
          }
          // 获取24小时ticker数据
          const ticker = await this.binance.get24hrTicker(symbol);

          // 分析币种
          const analysis = this.analyzer.analyze(klines);

          // 检查筛选条件
          if (analysis.isAboveEMA20 && analysis.pullbackInfo.meetsPullbackCriteria) {
            const result = {
              symbol,
              currentPrice: analysis.pullbackInfo.currentPrice,
              lowestLow: analysis.pullbackInfo.lowestLow,
              highestHigh: analysis.pullbackInfo.highestHigh,
              pullbackPercent: analysis.pullbackInfo.pullbackFromTop,
              riseFromBottom: analysis.pullbackInfo.riseFromBottom,
              launchDate: ticker ? new Date(ticker.closeTime).toISOString().split('T')[0] : 'N/A',
              priceChangePercent: ticker ? ticker.priceChangePercent : null,
              volume: ticker ? ticker.volume : null
            };
            results.push(result);
            console.log(`  -> ✓ 符合条件! 回调幅度: ${analysis.pullbackInfo.pullbackFromTop.toFixed(2)}%`);
          } else {
            console.log(`  -> 不符合条件`);
          }
        } catch (error) {
          console.log(`  -> 错误: ${error.message}`);
        }
      }

      console.log(`\n[3/6] 找到 ${results.length} 个符合条件的币种`);

      // 4. 生成Excel文件
      console.log('\n[4/6] 生成Excel报告...');
      const excelPath = await this.excelGenerator.generate(results);
      console.log(`Excel文件: ${excelPath}`);

      // 5. 发送邮件
      // console.log('\n[5/6] 发送邮件...');
      // const emailSent = await this.emailSender.send(excelPath, results);

      // if (emailSent) {
      //   console.log('邮件已发送到: ' + config.email.to);
      // } else {
      //   console.log('邮件发送失败，请检查邮件配置');
      // }

      console.log('\n=== 扫描完成 ===');
      return results;
    } catch (error) {
      console.error('扫描过程中发生错误:', error);
      throw error;
    }
  }

  /**
   * 启动定时任务
   */
  start() {
    console.log('='.repeat(50));
    console.log('币安合约扫描器已启动');
    console.log(`定时任务: 每半天执行一次 (${config.schedule.cron})`);
    console.log(`邮件发送至: ${config.email.to}`);
    console.log('='.repeat(50));

    // 立即执行一次
    this.run().catch(console.error);

    // 设置定时任务
    cron.schedule(config.schedule.cron, () => {
      this.run().catch(console.error);
    });
  }
}

module.exports = Scanner;