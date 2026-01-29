const TelegramBot = require('node-telegram-bot-api');
const axios = require('axios');

// --- 配置参数 ---
const TELEGRAM_TOKEN = '6470491933:AAElEX4z2OnPHrp3b6-SzHMng_ISUW0xNPo';
const CHAT_ID = '@chaisanye'; 
const SYMBOL = 'XAUUSDT';          // 币安合约代码
const DROP_THRESHOLD = 25;        // 跌幅阈值（美元）
const KLINE_LIMIT = 10;           // 监控最近 10 根 K 线

const bot = new TelegramBot(TELEGRAM_TOKEN, { polling: false });

/**
 * 获取币安【期货/合约】K 线数据
 */
async function checkFuturePrice() {
    try {
        // 注意：合约接口地址是 fapi.binance.com
        const url = 'https://fapi.binance.com/fapi/v1/klines';
        
        const response = await axios.get(url, {
            params: {
                symbol: SYMBOL,
                interval: '1m',
                limit: KLINE_LIMIT
            }
        });

        const klines = response.data;
        if (!klines || klines.length === 0) return;

        // klines[x]: [开盘时间, 开, 高, 低, 收, 成交量, 收盘时间, 成交额, 成交笔数...]
        const highs = klines.map(k => parseFloat(k[2]));
        // const lows = klines.map(k => parseFloat(k[3]));

        const maxHigh = Math.max(...highs);
        // const minLow = Math.min(...lows);
        const minLow=parseFloat(klines[klines.length-1][4]);
        const amplitude = maxHigh - minLow;

        console.log(`[${new Date().toLocaleTimeString()}] 合约最高: ${maxHigh}, 最低: ${minLow}, 波幅: ${amplitude.toFixed(2)}`);

        // 逻辑：10根K线内极差超过阈值
        if (amplitude >= DROP_THRESHOLD) {
            const message = `⚠️ **币安合约预警: ${SYMBOL}**\n\n` +
                            `10分钟内价格波动超过 $${DROP_THRESHOLD}！\n` +
                            `● 峰值最高: ${maxHigh}\n` +
                            `● 谷值最低: ${minLow}\n` +
                            `● 当前波动: **$${amplitude.toFixed(2)}**\n` +
                            `● 类型: 1M级别 K线监控`;
            
            sendTelegramMessage(message);
        }

    } catch (error) {
        console.error('获取合约数据失败:', error.response?.data || error.message);
    }
}

function sendTelegramMessage(text) {
    bot.sendMessage(CHAT_ID, text, { parse_mode: 'Markdown' })
        .then(() => console.log('✅ 警报已推送到频道'))
        .catch((err) => console.error('❌ TG推送失败:', err.message));
}

// 每一分钟轮询一次
setInterval(checkFuturePrice, 60000);
checkFuturePrice(); // 启动时运行一次