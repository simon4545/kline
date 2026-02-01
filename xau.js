const TelegramBot = require('node-telegram-bot-api');
const axios = require('axios');

// --- 配置参数 ---
const TELEGRAM_TOKEN = '6470491933:AAElEX4z2OnPHrp3b6-SzHMng_ISUW0xNPo';
const CHAT_ID = '@chaisanye'; 
const SYMBOL = 'XAUUSDT';          
const DROP_THRESHOLD = 35;        
const KLINE_LIMIT = 10;           
const COOLDOWN_TIME = 10 * 60 * 1000; // 冷却时间：10分钟（毫秒）

// --- 缓存变量 ---
let lastNotificationTime = 0; // 记录上一次发送警报的时间戳

const bot = new TelegramBot(TELEGRAM_TOKEN, { polling: false });

/**
 * 获取币安【期货/合约】K 线数据
 */
async function checkFuturePrice() {
    try {
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

        // 提取逻辑
        const highs = klines.map(k => parseFloat(k[2]));
        const maxHigh = Math.max(...highs);
        const minLow = parseFloat(klines[klines.length - 1][3]); // 最新一根K线的最低价
        const amplitude = maxHigh - minLow;

        const currentTime = Date.now();
        console.log(`[${new Date().toLocaleTimeString()}] 最高: ${maxHigh}, 最低: ${minLow}, 波幅: ${amplitude.toFixed(2)}`);

        // 逻辑：1. 波动超过阈值 && 2. 距离上次发送已超过10分钟
        if (amplitude >= DROP_THRESHOLD) {
            if (currentTime - lastNotificationTime > COOLDOWN_TIME) {
                const message = `⚠️ **币安合约预警: ${SYMBOL}**\n\n` +
                                `检测到剧烈波动！\n` +
                                `● 10min内最高: ${maxHigh}\n` +
                                `● 最新最低价: ${minLow}\n` +
                                `● 当前跌幅: **$${amplitude.toFixed(2)}**\n` +
                                `● 状态: 触发警报（10分钟内不再重复提醒）`;
                
                sendTelegramMessage(message);
                
                // 更新最后发送时间
                lastNotificationTime = currentTime;
            } else {
                const remaining = Math.ceil((COOLDOWN_TIME - (currentTime - lastNotificationTime)) / 1000 / 60);
                console.log(`[跳过] 已达到阈值但处于冷却期，还剩约 ${remaining} 分钟`);
            }
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

// 建议：由于你监控的是10根1分钟K线，每 5-10 秒轮询一次是合理的
setInterval(checkFuturePrice, 5000);
checkFuturePrice();