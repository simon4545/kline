require('dotenv').config(); // 加载环境变量
const { Bot, HttpError, GrammyError } = require('grammy');
const axios = require('axios');

// --- 从环境变量读取配置 ---
const TG_BOT_TOKEN = process.env.TELEGRAM_BOT_TOKEN;
const TG_CHAT_ID = "@chaisanye";
const SYMBOL = 'XAUUSDT';
const DROP_THRESHOLD = 35;
const COOLDOWN_TIME = 3000 * 1000;
const bot = new Bot(TG_BOT_TOKEN);
// --- 状态追踪 ---
let confirmedTD = {
    "5m": { lastKTime: 0 },
    "15m": { lastKTime: 0 },
    "short":{ lastKTime: 0, lastRiseTime: 0 },
    "long":{ lastKTime: 0, lastRiseTime: 0 }
};

/**
 * 核心推送函数：使用 grammY 的 API
 */
async function sendNotification(text) {
    try {
        await bot.api.sendMessage(TG_CHAT_ID, text, { parse_mode: "Markdown" });
        console.log(`[${new Date().toLocaleTimeString()}] ✅ 推送成功`);
    } catch (error) {
        if (error instanceof GrammyError) {
            console.error("❌ Telegram 接口错误:", error.description);
        } else if (error instanceof HttpError) {
            console.error("❌ 网络连接失败 (可能是防火墙问题):", error);
        } else {
            console.error("❌ 未知错误:", error);
        }
    }
}

/**
 * 筛选出已收盘的 K 线
 */
function getClosedKlines(klines) {
    const now = Date.now();
    return klines.filter(k => k[6] < now);
}

/**
 * TD9 判定逻辑
 */
function checkClosedTD9(closedKlines) {
    if (closedKlines.length < 13) return null;
    const targetIdx = closedKlines.length - 1;

    // 买入九转
    let isBuy9 = true;
    for (let i = 0; i < 9; i++) {
        const currentCheckIdx = targetIdx - i;
        const close = parseFloat(closedKlines[currentCheckIdx][4]);
        const close4Before = parseFloat(closedKlines[currentCheckIdx - 4][4]);
        if (!(close < close4Before)) { isBuy9 = false; break; }
    }
    if (isBuy9) return { type: '买入 (TD9)', side: 'BUY' };

    // 卖出九转
    let isSell9 = true;
    for (let i = 0; i < 9; i++) {
        const currentCheckIdx = targetIdx - i;
        const close = parseFloat(closedKlines[currentCheckIdx][4]);
        const close4Before = parseFloat(closedKlines[currentCheckIdx - 4][4]);
        if (!(close > close4Before)) { isSell9 = false; break; }
    }
    if (isSell9) return { type: '卖出 (TD9)', side: 'SELL' };

    return null;
}
function dropdown(list, threshold, isshort) {
    const highs = list.map(k => parseFloat(k[2]));
    const maxHigh = Math.max(...highs);
    const minLow = parseFloat(list[list.length - 1][3]);
    const amp = maxHigh - minLow;
    const now = Date.now();
    let lastVolTime=isshort?confirmedTD["short"].lastKTime:confirmedTD["long"].lastKTime;
    if (amp >= threshold && (now - lastVolTime > COOLDOWN_TIME)) {
        let alertMsg = "";
        if (isshort) {
            alertMsg = `⚠️ *波动预警1: ${SYMBOL}*\n\n跌幅已达: *$${amp.toFixed(2)}*\n\n价格:${minLow}`;
            confirmedTD["short"].lastKTime = now;
        } else {
            alertMsg = `⚠️ *波动预警2: ${SYMBOL}*\n\n跌幅已达: *$${amp.toFixed(2)}*\n\n价格:${minLow}`;
            confirmedTD["long"].lastKTime = now;
        }
        sendNotification(alertMsg);
    }
}
function riseAlert(list, threshold, isshort) {
    const lows = list.map(k => parseFloat(k[3]));
    const minLow = Math.min(...lows);
    const maxHigh = parseFloat(list[list.length - 1][2]);
    const amp = maxHigh - minLow;
    const now = Date.now();
    let lastRiseTime=isshort?confirmedTD["short"].lastRiseTime:confirmedTD["long"].lastRiseTime;
    if (amp >= threshold && (now - lastRiseTime > COOLDOWN_TIME)) {
        let alertMsg = "";
        if (isshort) {
            alertMsg = `🚀 *波动预警1(上涨): ${SYMBOL}*\n\n涨幅已达: *$${amp.toFixed(2)}*\n\n价格:${maxHigh}`;
            confirmedTD["short"].lastRiseTime = now;
        } else {
            alertMsg = `🚀 *波动预警2(上涨): ${SYMBOL}*\n\n涨幅已达: *$${amp.toFixed(2)}*\n\n价格:${maxHigh}`;
            confirmedTD["long"].lastRiseTime = now;
        }
        sendNotification(alertMsg);
    }
}
async function fetchKlines(interval, limit = 30) {
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
    console.log(`[${new Date().toLocaleTimeString()}] 正在扫描行情...`);

    // 1. 监控 1m 波动
    const k1m = await fetchKlines('1m', 240);
    if (k1m) {
        let k1short = k1m.slice(-15);
        dropdown(k1short, DROP_THRESHOLD,true);
        riseAlert(k1short, DROP_THRESHOLD,true);
        dropdown(k1m, DROP_THRESHOLD * 2,false);
        riseAlert(k1m, DROP_THRESHOLD * 2,false);
    }

    // 2. 监控 5m 和 15m 的收盘九转
    const intervals = ['5m', '15m'];
    for (const interval of intervals) {
        const rawKlines = await fetchKlines(interval, 30);
        if (!rawKlines) continue;

        const closedKlines = getClosedKlines(rawKlines);
        if (closedKlines.length === 0) continue;

        const now = Date.now();

        if ((now - confirmedTD[interval].lastKTime) > COOLDOWN_TIME) {
            const result = checkClosedTD9(closedKlines);
            if (result) {
                const emoji = result.side === 'BUY' ? '🟢' : '🔴';
                const price = closedKlines[closedKlines.length - 1][4];

                const msg = `${emoji} *九转信号: ${SYMBOL} (${interval})*\n\n` +
                    `*神奇九转·收盘确认*\n` +
                    `• 周期: ${interval}\n` +
                    `• 信号: ${result.type}\n` +
                    `• 收盘价: ${price}\n` +
                    `• 状态: 已收盘确认`;

                sendNotification(msg);
                confirmedTD[interval].lastKTime = now;
            }
        }
    }
}

// 每 15 秒检查一次
setInterval(monitorTask, 15000);
monitorTask();
// (async function(){
//     sendNotification("asdf")
// })()