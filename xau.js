const TelegramBot = require('node-telegram-bot-api');
const axios = require('axios');

// --- 配置参数 ---
const TELEGRAM_TOKEN = '6470491933:AAElEX4z2OnPHrp3b6-SzHMng_ISUW0xNPo';
const CHAT_ID = '@chaisanye'; 
const SYMBOL = 'XAUUSDT';          
const DROP_THRESHOLD = 35;
const COOLDOWN_TIME = 10 * 60 * 1000; 

// --- 状态追踪 ---
let lastVolTime = 0;
// 记录每个周期“已确认收盘”的K线时间戳，防止重复推送
let confirmedTD = {
    "5m": { lastKTime: 0 },
    "15m": { lastKTime: 0 }
};

const bot = new TelegramBot(TELEGRAM_TOKEN, { polling: false });

/**
 * 判定逻辑：从刚收盘的 K 线开始倒序回溯 9 根
 */
function checkClosedTD9(klines) {
    // 判定目标是 [length-2] (刚收盘的那根)
    // 判定它是否是连续第 9 根，至少需要 9 + 4 = 13 根数据
    if (klines.length < 13) return null;

    const targetIdx = klines.length - 2; 

    // --- 检查买入九转 (Buy Setup) ---
    // 条件：连续 9 根 K 线，每根的收盘价都 < 4 根前的收盘价
    let isBuy9 = true;
    for (let i = 0; i < 9; i++) {
        const currentCheckIdx = targetIdx - i;
        const close = parseFloat(klines[currentCheckIdx][4]);
        const close4Before = parseFloat(klines[currentCheckIdx - 4][4]);
        
        if (!(close < close4Before)) {
            isBuy9 = false;
            break;
        }
    }
    if (isBuy9) return { type: '买入 (TD9)', side: 'BUY' };

    // --- 检查卖出九转 (Sell Setup) ---
    // 条件：连续 9 根 K 线，每根的收盘价都 > 4 根前的收盘价
    let isSell9 = true;
    for (let i = 0; i < 9; i++) {
        const currentCheckIdx = targetIdx - i;
        const close = parseFloat(klines[currentCheckIdx][4]);
        const close4Before = parseFloat(klines[currentCheckIdx - 4][4]);

        if (!(close > close4Before)) {
            isSell9 = false;
            break;
        }
    }
    if (isSell9) return { type: '卖出 (TD9)', side: 'SELL' };

    return null;
}

async function fetchKlines(interval, limit = 20) {
    try {
        const res = await axios.get('https://fapi.binance.com/fapi/v1/klines', {
            params: { symbol: SYMBOL, interval, limit }
        });
        return res.data;
    } catch (e) {
        console.error(`获取${interval}数据失败:`, e.message);
        return null;
    }
}

async function monitorTask() {
    console.log("开始任务")
    // 1. 监控 1m 剧烈波动（保留原逻辑，实时性较强）
    const k1m = await fetchKlines('1m', 10);
    
    if (k1m) {
        const highs = k1m.map(k => parseFloat(k[2]));
        const maxHigh = Math.max(...highs);
        const minLow = parseFloat(k1m[k1m.length - 1][3]);
        const amp = maxHigh - minLow;
        const now = Date.now();
        if (amp >= DROP_THRESHOLD && (now - lastVolTime > COOLDOWN_TIME)) {
            sendTelegramMessage(`⚠️ **波动预警: ${SYMBOL}**\n跌幅: $${amp.toFixed(2)}`);
            lastVolTime = now;
        }
    }

    // 2. 监控 5m 和 15m 的收盘九转
    const intervals = ['5m', '15m'];
    for (const interval of intervals) {
        const klines = await fetchKlines(interval, 30);
        if (!klines) continue;
        
        // 获取刚收盘的那根K线的“开盘时间”作为唯一标识
        const closedKTime = klines[klines.length - 2][0];
        
        // 如果这根K线还没处理过
        if (confirmedTD[interval].lastKTime !== closedKTime) {
            const result = checkClosedTD9(klines);
            
            if (result) {
                const emoji = result.side === 'BUY' ? '🟢' : '🔴';
                const price = klines[klines.length - 2][4]; // 收盘价
                const msg = `${emoji} **神奇九转·收盘确认: ${SYMBOL}**\n\n` +
                            `● 周期: ${interval}\n` +
                            `● 信号: **${result.type}**\n` +
                            `● 收盘价: ${price}\n` +
                            `● 状态: 已收盘确认，信号固定`;
                
                sendTelegramMessage(msg);
            }
            // 标记这根K线已检查完，无论是否有信号，本周期不再重复
            confirmedTD[interval].lastKTime = closedKTime;
        }
    }
}

function sendTelegramMessage(text) {
    bot.sendMessage(CHAT_ID, text, { parse_mode: 'Markdown' })
        .then(() => console.log(`[${new Date().toLocaleTimeString()}] ✅ 推送成功`))
        .catch(function(e){ 
            console.error('❌ 推送失败:', e.message)
        });
}
const msg = `🟢 **神奇九转·收盘确认: XAUUSD**\n\n` +
                            `● 周期: 15m\n` +
                            `● 信号: **BUY**\n` +
                            `● 收盘价: 5000\n` +
                            `● 状态: 已收盘确认，信号固定`;
sendTelegramMessage(msg)
// 每 15 秒检查一次（足够捕捉收盘瞬间）
//setInterval(monitorTask, 15000);
//monitorTask();