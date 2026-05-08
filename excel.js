/**
 * Excel生成类
 */

const ExcelJS = require('exceljs');
const fs = require('fs');
const path = require('path');
const config = require('./config');

class ExcelGenerator {
  constructor() {
    this.outputDir = config.excel.outputDir;
    // 确保输出目录存在
    if (!fs.existsSync(this.outputDir)) {
      fs.mkdirSync(this.outputDir, { recursive: true });
    }
  }

  /**
   * 生成Excel文件
   * @param {Array} results - 筛选后的币种结果
   * @returns {string} 文件路径
   */
  async generate(results) {
    const workbook = new ExcelJS.Workbook();
    workbook.creator = 'Binance Scanner';
    workbook.created = new Date();

    // 创建工作表
    const worksheet = workbook.addWorksheet('合约扫描结果');

    // 设置列标题
    worksheet.columns = [
      { header: '币种', key: 'symbol', width: 15 },
      { header: '当前价格', key: 'currentPrice', width: 15 },
      { header: '100日最低点', key: 'lowestLow', width: 15 },
      { header: '10日最高点', key: 'highestHigh', width: 15 },
      { header: '回调幅度(%)', key: 'pullbackPercent', width: 15 },
      { header: '从底涨幅(%)', key: 'riseFromBottom', width: 15 },
      { header: '上线日期', key: 'launchDate', width: 15 },
      { header: '24h涨幅(%)', key: 'priceChange24h', width: 15 },
      { header: '24h成交量', key: 'volume24h', width: 15 }
    ];

    // 添加数据行
    results.forEach(item => {
      worksheet.addRow({
        symbol: item.symbol,
        currentPrice: item.currentPrice.toFixed(4),
        lowestLow: item.lowestLow.toFixed(4),
        highestHigh: item.highestHigh.toFixed(4),
        pullbackPercent: item.pullbackPercent.toFixed(2),
        riseFromBottom: item.riseFromBottom.toFixed(2),
        launchDate: item.launchDate,
        priceChange24h: item.priceChangePercent ? item.priceChangePercent.toFixed(2) : 'N/A',
        volume24h: item.volume ? item.volume.toFixed(2) : 'N/A'
      });
    });

    // 生成文件名 (包含时间戳)
    const timestamp = this.formatDate(new Date());
    const filename = `binance_scan_${timestamp}.xlsx`;
    const filepath = path.join(this.outputDir, filename);

    // 保存文件
    await workbook.xlsx.writeFile(filepath);
    console.log(`Excel文件已生成: ${filepath}`);

    return filepath;
  }

  /**
   * 格式化日期为文件名格式
   */
  formatDate(date) {
    const year = date.getFullYear();
    const month = String(date.getMonth() + 1).padStart(2, '0');
    const day = String(date.getDate()).padStart(2, '0');
    const hours = String(date.getHours()).padStart(2, '0');
    const minutes = String(date.getMinutes()).padStart(2, '0');
    return `${year}${month}${day}_${hours}${minutes}`;
  }
}

module.exports = ExcelGenerator;