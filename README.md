# 币安K线数据收集与MACD水上金叉检测

这个项目从币安API获取K线数据，存储在本地SQLite数据库中，并定期检测代币在5分钟级别是否出现MACD水上金叉，将结果发送到Telegram频道。

## 功能特性

- 从币安API获取K线数据
- 存储数据到SQLite数据库
- 提供HTTP API查询K线数据
- 定期检测MACD水上金叉
- 将检测结果发送到Telegram频道
- 使用内存缓存避免重复发送相同代币的水上金叉通知（4小时有效期）

## 配置

### 环境变量

要启用Telegram通知功能，需要设置以下环境变量：

- `TELEGRAM_BOT_TOKEN`: Telegram Bot的token
- `TELEGRAM_CHAT_ID`: 要发送消息的频道或用户ID

### symbols.json

`symbols.json`文件包含了要监控的代币符号列表。

## 使用方法

1. 编译项目：
   ```
   go build -o kline .
   ```

2. 设置环境变量（可选）：
   ```
   export TELEGRAM_BOT_TOKEN="your_bot_token"
   export TELEGRAM_CHAT_ID="your_chat_id"
   ```

3. 运行程序：
   ```
   ./kline
   ```

4. 程序将自动开始收集K线数据，并每5分钟检查一次MACD水上金叉。

## API接口

- `/symbols`: 获取监控的代币符号列表
- `/klines?symbol=SYMBOL&interval=INTERVAL&limit=LIMIT`: 获取指定代币和时间间隔的K线数据

## 定时任务

- 每分钟更新一次K线数据
- 每5分钟检查一次MACD水上金叉
- 每24小时清理一次旧数据（保留最近一个月的数据）

## MACD水上金叉定义

水上金叉指的是：
1. MACD线和信号线都在零轴之上
2. MACD线从下方向上穿过信号线

## 代码结构

- `main.go`: 主程序入口，包含数据更新逻辑和定时任务
- `binanceapi.go`: 币安API接口和数据模型
- `serve.go`: HTTP服务接口
- `judge.go`: MACD计算和判断逻辑
- `symbols.json`: 监控的代币符号列表

## 依赖

- Go 1.24+
- SQLite3
- GORM
- 其他依赖项请查看`go.mod`文件