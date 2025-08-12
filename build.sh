#!/bin/bash
set -e
# git commit -m "as"
# 1. 编译 Go 程序（去掉调试符）
echo "编译 Go 程序..."
GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o kline ./

# 2. 使用 pm2 reload
echo "重新加载 PM2 进程..."
pm2 reload kline || pm2 start ./kline --name kline

echo "完成。"