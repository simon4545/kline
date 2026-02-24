const axios = require('axios');
const nodemailer = require('nodemailer');

// --- 配置参数 ---
const EMAIL_CONFIG = {
    user: 'simon4547@qq.com',
    pass: 'mvypepqmwwsiceae',
    to:   'simon4545@qq.com,535415790@qq.com,278699832@qq.com'      
};

const SYMBOL = 'XAUUSDT';          
const DROP_THRESHOLD = 35;
const COOLDOWN_TIME = 10 * 60 * 1000; 

// --- 状态追踪 ---
let lastVolTime = 0;
let confirmedTD = {
    "5m": { lastKTime: 0 },
    "15m": { lastKTime: 0 }
};

// --- 创建邮件传输对象 ---
const transporter = nodemailer.createTransport({
    service: 'qq',
    auth: {
        user: EMAIL_CONFIG.user,
        pass: EMAIL_CONFIG.pass
    }
});

/**
 * 发送邮件函数
 */
async function sendEmailNotification(subject, text) {
    const mailOptions = {
        from: `"给黄金之王们" <${EMAIL_CONFIG.user}>`,
        to: EMAIL_CONFIG.to,
        subject: subject,
        // 将 Markdown 风格稍微转为简单的 HTML 换行
        html: text.replace(/\n/g, '<br>').replace(/\*\*(.*?)\*\*/g, '<strong>$1</strong>')
    };

    try {
        await transporter.sendMail(mailOptions);
        console.log(`[${new Date().toLocaleTimeString()}] ✅ 邮件推送成功: ${subject}`);
    } catch (e) {
        console.error('❌ 邮件推送失败:', e.message);
    }
}

/**
 * 判定逻辑：从刚收盘的 K 线开始倒序回溯 9 根
 */
function checkClosedTD9(klines) {
    if (klines.length < 13) return null;
    const targetIdx = klines.length - 2; 

    // 买入九转
    let isBuy9 = true;
    for (let i = 0; i < 9; i++) {
        const currentCheckIdx = targetIdx - i;
        const close = parseFloat(klines[currentCheckIdx][4]);
        const close4Before = parseFloat(klines[currentCheckIdx - 4][4]);
        if (!(close < close4Before)) { isBuy9 = false; break; }
    }
    if (isBuy9) return { type: '买入 (TD9)', side: 'BUY' };

    // 卖出九转
    let isSell9 = true;
    for (let i = 0; i < 9; i++) {
        const currentCheckIdx = targetIdx - i;
        const close = parseFloat(klines[currentCheckIdx][4]);
        const close4Before = parseFloat(klines[currentCheckIdx - 4][4]);
        if (!(close > close4Before)) { isSell9 = false; break; }
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
    console.log(`[${new Date().toLocaleTimeString()}] 正在扫描行情...`);
    
    // 1. 监控 1m 波动
    const k1m = await fetchKlines('1m', 10);
    if (k1m) {
        const highs = k1m.map(k => parseFloat(k[2]));
        const maxHigh = Math.max(...highs);
        const minLow = parseFloat(k1m[k1m.length - 1][3]);
        const amp = maxHigh - minLow;
        const now = Date.now();
        if (amp >= DROP_THRESHOLD && (now - lastVolTime > COOLDOWN_TIME)) {
            sendEmailNotification(`⚠️ 波动预警: ${SYMBOL}`, `跌幅已达: $${amp.toFixed(2)}`);
            lastVolTime = now;
        }
    }

    // 2. 监控 5m 和 15m 的收盘九转
    const intervals = ['5m', '15m'];
    for (const interval of intervals) {
        const klines = await fetchKlines(interval, 30);
        if (!klines) continue;
        
        const closedKTime = klines[klines.length - 2][0];
        
        if (confirmedTD[interval].lastKTime !== closedKTime) {
            const result = checkClosedTD9(klines);
            if (result) {
                const emoji = result.side === 'BUY' ? '🟢' : '🔴';
                const price = klines[klines.length - 2][4];
                const subject = `${emoji} 九转信号: ${SYMBOL} (${interval})`;
                const msg = `**神奇九转·收盘确认: ${SYMBOL}**\n\n` +
                            `● 周期: ${interval}\n` +
                            `● 信号: ${result.type}\n` +
                            `● 收盘价: ${price}\n` +
                            `● 状态: 已收盘确认`;
                
                sendEmailNotification(subject, msg);
            }
            confirmedTD[interval].lastKTime = closedKTime;
        }
    }
}
// sendEmailNotification("test","simon4545")
// 每 15 秒检查一次
setInterval(monitorTask, 15000);
monitorTask();