/**
 * 币安合约扫描器 - 入口文件
 * 
 * 功能:
 * 1. 扫描币安交易所所有USDT合约
 * 2. 筛选上线不超过365天的币种
 * 3. 筛选日线级别价格突破EMA20的币种
 * 4. 筛选最近100天最低点为底，最近10天最高点为顶，回调超过10%的币种
 * 5. 排除BTC、ETH、SOL等大币种
 * 6. 生成Excel报告并发送邮件
 */

const Scanner = require('./scanner');

// 启动扫描器
const scanner = new Scanner();
scanner.start();
