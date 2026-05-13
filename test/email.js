/**
 * 邮件发送类
 */

const nodemailer = require('nodemailer');
const config = require('./config');

class EmailSender {
  constructor() {
    this.transporter = nodemailer.createTransport({
      service: 'gmail',
      auth: {
        user: config.email.auth.user,
        pass: config.email.auth.pass
      }
    });
  }

  /**
   * 发送邮件
   * @param {string} filepath - Excel文件路径
   * @param {Array} results - 扫描结果
   */
  async send(filepath, results) {
    const mailOptions = {
      from: config.email.from,
      to: config.email.to,
      subject: `${config.email.subject} - ${this.formatDate(new Date())}`,
      text: this.generateEmailBody(results),
      attachments: [
        {
          filename: filepath.split('/').pop(),
          path: filepath
        }
      ]
    };

    try {
      await this.transporter.sendMail(mailOptions);
      console.log('邮件已发送成功');
      return true;
    } catch (error) {
      console.error('邮件发送失败:', error.message);
      return false;
    }
  }

  /**
   * 生成邮件正文
   */
  generateEmailBody(results) {
    const date = new Date().toLocaleString('zh-CN', { timeZone: 'Asia/Shanghai' });
    let body = `币安合约扫描报告\n`;
    body += `扫描时间: ${date}\n`;
    body += `符合条件币种数量: ${results.length}\n\n`;

    if (results.length > 0) {
      body += `筛选条件:\n`;
      body += `1. 上线时间不超过365天\n`;
      body += `2. 日线级别价格突破EMA20\n`;
      body += `3. 最近100天最低点为底，最近10天最高点为顶，回调超过10%\n`;
      body += `4. 排除BTC、ETH、SOL等大币种\n\n`;

      body += `符合条件币种列表:\n`;
      body += `-`.repeat(50) + `\n`;

      results.forEach((item, index) => {
        body += `${index + 1}. ${item.symbol}\n`;
        body += `   当前价格: ${item.currentPrice.toFixed(4)}\n`;
        body += `   100日最低点: ${item.lowestLow.toFixed(4)}\n`;
        body += `   10日最高点: ${item.highestHigh.toFixed(4)}\n`;
        body += `   回调幅度: ${item.pullbackPercent.toFixed(2)}%\n`;
        body += `   从底涨幅: ${item.riseFromBottom.toFixed(2)}%\n`;
        body += `\n`;
      });
    } else {
      body += `\n本次扫描没有找到符合条件的币种。\n`;
    }

    body += `\n请查看附件中的Excel文件获取详细信息。\n`;
    body += `\n---\n本邮件由币安合约扫描器自动发送`;

    return body;
  }

  /**
   * 格式化日期
   */
  formatDate(date) {
    const year = date.getFullYear();
    const month = String(date.getMonth() + 1).padStart(2, '0');
    const day = String(date.getDate()).padStart(2, '0');
    return `${year}-${month}-${day}`;
  }
}

module.exports = EmailSender;