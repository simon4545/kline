# Cloudflare Worker 静态部署

`worker` 分支只部署 `public/` 下的静态页面、样式和脚本，不包含 Go 后端、SQLite、缓存、指标计算或定时任务。

## 项目结构

```
├── wrangler.toml      # Cloudflare Worker 配置
├── src/index.ts       # Worker 入口（静态资源兜底）
├── package.json       # 部署脚本
└── public/            # 静态资源目录
    ├── index.html     # 首页
    ├── *.html         # 各功能页面
    └── static/        # CSS/JS 资源
```

## 部署前置条件

- 已安装 Node.js 和 npm
- 已安装 Wrangler：`npm install -g wrangler`
- 已登录 Cloudflare：`wrangler login`

## 本地预览

在仓库根目录执行：

```bash
npm run dev
```

或直接：

```bash
wrangler dev
```

然后访问 Wrangler 输出的本地地址。静态资源目录由 `wrangler.toml` 中的 `assets.directory` 指定为 `./public`。

## 部署

```bash
npm run deploy
```

或直接：

```bash
wrangler deploy
```

首次部署会创建名为 `kline-static` 的 Worker；如果该名称已被占用，修改 `wrangler.toml` 的 `name` 后重新部署。

## 说明

- 首页是 `public/index.html`，根路径 `/` 会重定向到 `/index.html`。
- 页面之间使用相对路径跳转，可以直接由 Static Assets 提供。
- 直接访问币安 API 的页面仍然依赖浏览器、币安接口可用性和对应的 CORS 策略。
- `marketcap.html`、`rsi.html`、`rsi1d.html`、`huiche.html`、`trade_signal.html` 等页面仍引用原 Go 服务的 `/api/*`、`/trade/signal` 等接口。由于本分支明确不部署 HTTP API，这些页面的相关功能不会在纯静态部署中工作。
- `stream.html` 当前使用外部服务地址，因此是否可用取决于该外部服务，不由本 Worker 提供。
- `auto.html`、`cong.html`、`trade.html` 等页面包含交易 API 调用逻辑。不要在不可信环境中填写 API Secret；Cloudflare Static Assets 不会替你保护浏览器端密钥。
