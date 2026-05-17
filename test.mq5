#property strict
#property version   "1.40"
#property description "XAU Strategy EA with Magic-Isolated Native UI"

#include <Trade/Trade.mqh> // 核心交易库

// -------- 输入参数 --------
input string InpSymbol = "XAUUSD";
input ENUM_TIMEFRAMES InpTFTrend = PERIOD_H1;
input ENUM_TIMEFRAMES InpTFSignal = PERIOD_M15;

// Trend filter
input int    InpEmaPeriod = 50;
input int    InpSlopeLookbackBars = 5;      // EMA50回溯数量
input double InpSlopeThreshold = 0.001;     // 斜率阈值

// StdDev phase
input int    InpStdPeriod = 20;
input int    InpStdConfirmWindowBars = 8;    // 形态确认数量

// Setup / invalidation
input int    InpSwingLookback = 5;
input double InpSLBufferPct = 0.002; // 止损容差阈值

// Risk / execution
input double InpFixedLots = 1.0;
input int    InpSlippagePoints = 50;
input uint   InpMagic = 260517;             // 核心魔术字隔离标识

// TP split
input double InpTP1_R = 1.0;
input double InpTP2_R = 2.0;
input double InpTP1_ClosePct = 0.40;
input double InpTP2_ClosePct = 0.30;

// trailing
input int    InpTrailEmaPeriod = 20;

// session filter
input bool   InpNoTradeWeekendUTC = true;

// -------- 全局交易对象 --------
CTrade trade;

// -------- UI 监视配置 --------
#define UI_PREFIX "XAU_UI_"
#define UI_ROWS 8

// -------- 内部状态变量 --------
int hEmaH1 = INVALID_HANDLE;
int hStdM15 = INVALID_HANDLE;
int hEmaTrailM15 = INVALID_HANDLE;

struct SetupState
{
   bool active;
   int  setupBarShift;      
   datetime setupBarTime;
   double setupHigh;
   double setupLow;
   bool bullish;
};

SetupState g_setup;
datetime g_lastM15BarTime = 0;

bool g_tp1Done = false;
bool g_tp2Done = false;
double g_entryPrice = 0.0;
double g_initialSL = 0.0;

// 存储实时检测状态用于 UI 显示
string g_ui_trend_status = "无趋势 (震荡)";
string g_ui_pattern_status = "未匹配";
string g_ui_std_status = "未满足拐头";
string g_ui_break_status = "等待信号";

// ---------- 原生 UI 模块 (100%无外部引入依赖) ----------
void CreateDashboard()
{
   int x = 10;
   int y = 20;
   int width = 280;
   int height = UI_ROWS * 22 + 10;
   string bg_name = UI_PREFIX + "BG";
   
   // 创建半透明暗色底色背板
   if(ObjectCreate(0, bg_name, OBJ_RECTANGLE_LABEL, 0, 0, 0))
   {
      ObjectSetInteger(0, bg_name, OBJPROP_XDISTANCE, x);
      ObjectSetInteger(0, bg_name, OBJPROP_YDISTANCE, y);
      ObjectSetInteger(0, bg_name, OBJPROP_XSIZE, width);
      ObjectSetInteger(0, bg_name, OBJPROP_YSIZE, height);
      ObjectSetInteger(0, bg_name, OBJPROP_BGCOLOR, C'25,25,25'); 
      ObjectSetInteger(0, bg_name, OBJPROP_BORDER_TYPE, BORDER_FLAT);
      ObjectSetInteger(0, bg_name, OBJPROP_COLOR, C'60,60,60');
      ObjectSetInteger(0, bg_name, OBJPROP_SELECTABLE, false);
      ObjectSetInteger(0, bg_name, OBJPROP_HIDDEN, true);
   }
   
   // 初始化多行文本标签
   for(int i=0; i<UI_ROWS; i++)
   {
      string lbl_name = UI_PREFIX + "LBL_" + (string)i;
      if(ObjectCreate(0, lbl_name, OBJ_LABEL, 0, 0, 0))
      {
         ObjectSetInteger(0, lbl_name, OBJPROP_XDISTANCE, x + 15);
         ObjectSetInteger(0, lbl_name, OBJPROP_YDISTANCE, y + 10 + (i * 22));
         ObjectSetString(0, lbl_name, OBJPROP_FONT, "Microsoft YaHei");
         ObjectSetInteger(0, lbl_name, OBJPROP_FONTSIZE, 10);
         ObjectSetInteger(0, lbl_name, OBJPROP_COLOR, C'200,200,200');
         ObjectSetInteger(0, lbl_name, OBJPROP_SELECTABLE, false);
         ObjectSetInteger(0, lbl_name, OBJPROP_HIDDEN, true);
      }
   }
   ChartRedraw();
}

void UpdateDashboard()
{
   string labels[UI_ROWS];
   color colors[UI_ROWS];
   
   labels[0] = "====== XAU 策略监视面板 ======"; colors[0] = C'255,165,0';
   
   // 1. 大趋势过滤
   labels[1] = "1. H1大趋势过滤: " + g_ui_trend_status;
   if(StringFind(g_ui_trend_status, "多头") >= 0) colors[1] = clrLime;
   else if(StringFind(g_ui_trend_status, "空头") >= 0) colors[1] = clrRed;
   else colors[1] = C'180,180,180';
   
   // 2. K线形态触发
   labels[2] = "2. M15形态触发 : " + g_ui_pattern_status;
   if(g_setup.active) 
   {
      labels[2] += (g_setup.bullish ? " (看涨Setup)" : " (看跌Setup)");
      colors[2] = clrAqua;
   }
   else colors[2] = C'180,180,180';
   
   // 3. 标准差转折
   labels[3] = "3. StdDev 拐头 : " + g_ui_std_status;
   if(g_ui_std_status == "已转折向下(合格)") colors[3] = clrLime;
   else colors[3] = C'180,180,180';
   
   // 4. 突破确认
   labels[4] = "4. 价格突破确认: " + g_ui_break_status;
   if(g_ui_break_status == "突破成功(开仓)") colors[4] = clrLime;
   else if(StringFind(g_ui_break_status, "失效") >= 0) colors[4] = clrCrimson;
   else colors[4] = C'180,180,180';
   
   labels[5] = "------------------------------"; colors[5] = C'80,80,80';
   
   // 5. 订单与信号周期信息
   if(g_setup.active)
   {
      int barsSince = iBarShift(_Symbol, InpTFSignal, g_setup.setupBarTime, false);
      labels[6] = StringFormat("Setup有效窗口: %d / %d 根K线", barsSince, InpStdConfirmWindowBars);
      labels[7] = StringFormat("待破高低值: H:%.2f | L:%.2f", g_setup.setupHigh, g_setup.setupLow);
      colors[6] = clrYellow; colors[7] = clrYellow;
   }
   else
   {
      labels[6] = "当前无激活的交易信号 (等待条件中)";
      labels[7] = PositionExists(_Symbol) ? ">> 策略已有持仓 (不重复开仓) <<" : "";
      colors[6] = C'130,130,130'; colors[7] = clrLime;
   }
   
   // 映射刷新至原生图表层
   for(int i=0; i<UI_ROWS; i++)
   {
      string lbl_name = UI_PREFIX + "LBL_" + (string)i;
      ObjectSetString(0, lbl_name, OBJPROP_TEXT, labels[i]);
      ObjectSetInteger(0, lbl_name, OBJPROP_COLOR, colors[i]);
   }
   ChartRedraw();
}

void DestroyDashboard()
{
   ObjectsDeleteAll(0, UI_PREFIX);
   ChartRedraw();
}

// ---------- 核心隔离逻辑：精准筛选当前EA的持仓 ----------
bool PositionExists(const string symbol)
{
   int totalPositions = PositionsTotal();
   for(int i = 0; i < totalPositions; i++)
   {
      ulong ticket = PositionGetTicket(i);
      if(ticket > 0)
      {
         // 检查品种与魔术字是否双重匹配
         if(PositionGetString(POSITION_SYMBOL) == symbol && PositionGetInteger(POSITION_MAGIC) == InpMagic)
         {
            return true; 
         }
      }
   }
   return false; 
}

bool PositionSelectByMagic(const string symbol, const long magic)
{
   int total = PositionsTotal();
   for(int i = 0; i < total; i++)
   {
      ulong ticket = PositionGetTicket(i);
      if(ticket > 0)
      {
         if(PositionGetString(POSITION_SYMBOL) == symbol && PositionGetInteger(POSITION_MAGIC) == magic)
         {
            return PositionSelectByTicket(ticket); // 精准锚定选定单
         }
      }
   }
   return false;
}

// ---------- 基础算法辅助函数 ----------
bool IsNewBar(const string symbol, ENUM_TIMEFRAMES tf, datetime &lastBarTime)
{
   datetime t[1];
   if(CopyTime(symbol, tf, 0, 1, t) < 1) return false;
   if(t[0] != lastBarTime)
   {
      lastBarTime = t[0];
      return true;
   }
   return false;
}

bool IsWeekendUTC()
{
   datetime nowUtc = TimeGMT();
   MqlDateTime st;
   TimeToStruct(nowUtc, st);
   return (st.day_of_week == 0 || st.day_of_week == 6);
}

double RoundPrice(const string symbol, double price)
{
   int digits = (int)SymbolInfoInteger(symbol, SYMBOL_DIGITS);
   return NormalizeDouble(price, digits);
}

bool GetRates(const string symbol, ENUM_TIMEFRAMES tf, int startShift, int count, MqlRates &rates[])
{
   ArraySetAsSeries(rates, true);
   int copied = CopyRates(symbol, tf, startShift, count, rates);
   return copied == count;
}

bool GetIndicatorBuffer(int handle, int startShift, int count, double &buf[])
{
   ArraySetAsSeries(buf, true);
   int copied = CopyBuffer(handle, 0, startShift, count, buf);
   return copied == count;
}

// ---------- K线形态识别驱动模块 ----------
bool IsBullishEngulfing(const MqlRates &prev, const MqlRates &cur)
{
   bool prevBear = prev.close < prev.open;
   bool curBull = cur.close > cur.open;
   if(!prevBear || !curBull) return false;

   double prevBodyLow = MathMin(prev.open, prev.close);
   double prevBodyHigh = MathMax(prev.open, prev.close);
   double curBodyLow = MathMin(cur.open, cur.close);
   double curBodyHigh = MathMax(cur.open, cur.close);

   return (curBodyLow <= prevBodyLow && curBodyHigh >= prevBodyHigh);
}

bool IsBearishEngulfing(const MqlRates &prev, const MqlRates &cur)
{
   bool prevBull = prev.close > prev.open;
   bool curBear = cur.close < cur.open;
   if(!prevBull || !curBear) return false;

   double prevBodyLow = MathMin(prev.open, prev.close);
   double prevBodyHigh = MathMax(prev.open, prev.close);
   double curBodyLow = MathMin(cur.open, cur.close);
   double curBodyHigh = MathMax(cur.open, cur.close);

   return (curBodyLow <= prevBodyLow && curBodyHigh >= prevBodyHigh);
}

bool IsDoji(const MqlRates &bar)
{
   double range = bar.high - bar.low;
   if(range <= 0) return false;
   double body = MathAbs(bar.close - bar.open);
   return body <= range * 0.15;
}

bool IsBullishPinbar(const MqlRates &bar)
{
   double body = MathAbs(bar.close - bar.open);
   double upper = bar.high - MathMax(bar.open, bar.close);
   double lower = MathMin(bar.open, bar.close) - bar.low;
   if(body <= 0) return false;
   return (lower >= 2.0 * body && upper <= body);
}

bool IsBearishPinbar(const MqlRates &bar)
{
   double body = MathAbs(bar.close - bar.open);
   double upper = bar.high - MathMax(bar.open, bar.close);
   double lower = MathMin(bar.open, bar.close) - bar.low;
   if(body <= 0) return false;
   return (upper >= 2.0 * body && lower <= body);
}

bool IsBigBullConfirm(const MqlRates &bar,  MqlRates &hist[], int histCount)
{
   if(bar.close <= bar.open) return false;
   double body = MathAbs(bar.close - bar.open);
   double sum = 0.0;
   for(int i=0; i<histCount; i++)
      sum += MathAbs(hist[i].close - hist[i].open);
   double avg = (histCount > 0 ? sum / histCount : 0.0);
   return (avg > 0 && body > avg);
}

bool IsBigBearConfirm(const MqlRates &bar,  MqlRates &hist[], int histCount)
{
   if(bar.close >= bar.open) return false;
   double body = MathAbs(bar.close - bar.open);
   double sum = 0.0;
   for(int i=0; i<histCount; i++)
      sum += MathAbs(hist[i].close - hist[i].open);
   double avg = (histCount > 0 ? sum / histCount : 0.0);
   return (avg > 0 && body > avg);
}

// ---------- 核心策略过滤器逻辑 ----------
bool TrendBull(const string symbol)
{
   int lb = MathMax(1, InpSlopeLookbackBars);
   MqlRates h1[1];
   if(!GetRates(symbol, InpTFTrend, 1, 1, h1)) return false;

   double ema[2];
   if(!GetIndicatorBuffer(hEmaH1, 1, lb+1, ema)) return false;

   double slope = (ema[0] - ema[lb]) / ema[lb];
   bool condA = slope > InpSlopeThreshold;
   bool condB = h1[0].close > ema[0];

   if(condA && condB) { g_ui_trend_status = "多头 (斜率向上 & 价格在上)"; return true; }
   return false;
}

bool TrendBear(const string symbol)
{
   int lb = MathMax(1, InpSlopeLookbackBars);
   MqlRates h1[1];
   if(!GetRates(symbol, InpTFTrend, 1, 1, h1)) return false;

   double ema[2];
   if(!GetIndicatorBuffer(hEmaH1, 1, lb+1, ema)) return false;

   double slope = (ema[0] - ema[lb]) / ema[lb];
   bool condA = slope < -InpSlopeThreshold;
   bool condB = h1[0].close < ema[0];

   if(condA && condB) { g_ui_trend_status = "空头 (斜率向下 & 价格在下)"; return true; }
   return false;
}

bool StdTurnDownAtShift(int shift)
{
   double sd[3];
   if(!GetIndicatorBuffer(hStdM15, shift, 3, sd)) return false;
   return (sd[2] < sd[1] && sd[0] < sd[1]);
}

bool BuildBullSetup(const string symbol, SetupState &s)
{
   MqlRates bars[12];
   if(!GetRates(symbol, InpTFSignal, 1, 12, bars)) return false;

   MqlRates cur = bars[0];
   MqlRates prev = bars[1];

   bool engulf = IsBullishEngulfing(prev, cur);
   bool reversalConfirm = false;

   if((IsDoji(prev) || IsBullishPinbar(prev)) && IsBigBullConfirm(cur, bars, 10))
      reversalConfirm = true;

   if(!(engulf || reversalConfirm)) return false;

   s.active = true;
   s.setupBarShift = 1;
   s.setupBarTime = cur.time;
   s.setupHigh = cur.high;
   s.setupLow = cur.low;
   s.bullish = true;
   
   g_ui_pattern_status = engulf ? "看涨吞没" : "反转K线组合+大阳确立";
   return true;
}

bool BuildBearSetup(const string symbol, SetupState &s)
{
   MqlRates bars[12];
   if(!GetRates(symbol, InpTFSignal, 1, 12, bars)) return false;

   MqlRates cur = bars[0];
   MqlRates prev = bars[1];

   bool engulf = IsBearishEngulfing(prev, cur);
   bool reversalConfirm = false;

   if((IsDoji(prev) || IsBearishPinbar(prev)) && IsBigBearConfirm(cur, bars, 10))
      reversalConfirm = true;

   if(!(engulf || reversalConfirm)) return false;

   s.active = true;
   s.setupBarShift = 1;
   s.setupBarTime = cur.time;
   s.setupHigh = cur.high;
   s.setupLow = cur.low;
   s.bullish = false;
   
   g_ui_pattern_status = engulf ? "看跌吞没" : "反转K线组合+大阴确立";
   return true;
}

bool ComputeSL(const string symbol, bool bullish, double entry, double &slOut)
{
   MqlRates bars[10];
   if(!GetRates(symbol, InpTFSignal, 1, MathMax(6, InpSwingLookback+1), bars)) return false;

   double swing = bullish ? DBL_MAX : -DBL_MAX;
   for(int i=0; i<InpSwingLookback; i++)
   {
      if(bullish) swing = MathMin(swing, bars[i].low);
      else        swing = MathMax(swing, bars[i].high);
   }

   if(bullish)
      slOut = swing * (1.0 - InpSLBufferPct);
   else
      slOut = swing * (1.0 + InpSLBufferPct);

   slOut = RoundPrice(symbol, slOut);
   return true;
}

void ResetTradeState()
{
   g_setup.active = false;
   g_tp1Done = false;
   g_tp2Done = false;
   g_entryPrice = 0.0;
   g_initialSL = 0.0;
   g_ui_pattern_status = "未匹配";
   g_ui_std_status = "未满足";
   g_ui_break_status = "等待信号";
}

// ---------- 交易与风控流管理模块 ----------
bool OpenPositionBySetup(const string symbol, const SetupState &s)
{
   if(PositionExists(symbol)) return false;

   double ask = SymbolInfoDouble(symbol, SYMBOL_ASK);
   double bid = SymbolInfoDouble(symbol, SYMBOL_BID);
   if(ask <= 0 || bid <= 0) return false;

   double entry = s.bullish ? ask : bid;
   double sl = 0.0;
   if(!ComputeSL(symbol, s.bullish, entry, sl)) return false;

   trade.SetExpertMagicNumber((long)InpMagic);
   trade.SetDeviationInPoints(InpSlippagePoints);

   bool ok = false;
   if(s.bullish)
      ok = trade.Buy(InpFixedLots, symbol, 0.0, sl, 0.0, "XAU_STD_SETUP_BUY");
   else
      ok = trade.Sell(InpFixedLots, symbol, 0.0, sl, 0.0, "XAU_STD_SETUP_SELL");

   // 使用特定魔术字锚定，保护风控独立性
   if(ok && PositionSelectByMagic(symbol, InpMagic))
   {
      g_entryPrice = PositionGetDouble(POSITION_PRICE_OPEN);
      g_initialSL = PositionGetDouble(POSITION_SL);
      g_tp1Done = false;
      g_tp2Done = false;
   }
   return ok;
}

void ManagePosition(const string symbol)
{
   // 使用魔术字锁定过滤，全面保障手动跟单安全
   if(!PositionSelectByMagic(symbol, InpMagic))
   {
      g_tp1Done = false;
      g_tp2Done = false;
      return;
   }

   long type = PositionGetInteger(POSITION_TYPE);
   double vol = PositionGetDouble(POSITION_VOLUME);
   double sl = PositionGetDouble(POSITION_SL);
   double priceOpen = PositionGetDouble(POSITION_PRICE_OPEN);

   double bid = SymbolInfoDouble(symbol, SYMBOL_BID);
   double ask = SymbolInfoDouble(symbol, SYMBOL_ASK);

   bool bullish = (type == POSITION_TYPE_BUY);
   double current = bullish ? bid : ask;

   if(g_entryPrice <= 0.0) g_entryPrice = priceOpen;
   if(g_initialSL <= 0.0) g_initialSL = sl;

   double riskR = MathAbs(g_entryPrice - g_initialSL);
   if(riskR <= 0) return;

   double profitDist = bullish ? (current - g_entryPrice) : (g_entryPrice - current);

   // 分批减仓 TP1
   if(!g_tp1Done && profitDist >= InpTP1_R * riskR)
   {
      double closeLots = NormalizeDouble(vol * InpTP1_ClosePct, 2);
      if(closeLots > 0.0)
      {
         ulong ticket = (ulong)PositionGetInteger(POSITION_TICKET);
         trade.PositionClosePartial(ticket, closeLots);
      }
      double be = RoundPrice(symbol, g_entryPrice);
      ulong ticketForBE = (ulong)PositionGetInteger(POSITION_TICKET);
      trade.PositionModify(ticketForBE, be, 0.0);
      g_tp1Done = true;
   }

   // 分批减仓 TP2
   if(!g_tp2Done && profitDist >= InpTP2_R * riskR)
   {
      if(PositionSelectByMagic(symbol, InpMagic))
      {
         double v2 = PositionGetDouble(POSITION_VOLUME);
         double closeLots2 = NormalizeDouble(v2 * InpTP2_ClosePct, 2);
         if(closeLots2 > 0.0)
         {
            ulong ticket2 = (ulong)PositionGetInteger(POSITION_TICKET);
            trade.PositionClosePartial(ticket2, closeLots2);
         }
      }
      double lockPrice = bullish ? (g_entryPrice + riskR) : (g_entryPrice - riskR);
      lockPrice = RoundPrice(symbol, lockPrice);
      if(PositionSelectByMagic(symbol, InpMagic))
      {
         ulong ticketForLock = (ulong)PositionGetInteger(POSITION_TICKET);
         trade.PositionModify(ticketForLock, lockPrice, 0.0);
      }
      g_tp2Done = true;
   }
}

void ManageTrailingByEmaClose(const string symbol)
{
   if(!PositionSelectByMagic(symbol, InpMagic)) return;

   long type = PositionGetInteger(POSITION_TYPE);
   MqlRates m15[2];
   if(!GetRates(symbol, InpTFSignal, 1, 2, m15)) return;

   double ema[2];
   if(!GetIndicatorBuffer(hEmaTrailM15, 1, 2, ema)) return;

   bool closeCond = false;
   if(type == POSITION_TYPE_BUY)   closeCond = (m15[0].close < ema[0]);
   else                            closeCond = (m15[0].close > ema[0]);

   if(closeCond)
   {
      ulong ticket = (ulong)PositionGetInteger(POSITION_TICKET);
      trade.PositionClose(ticket);
   }
}

// ---------- 核心信号决策引擎 ----------
void ProcessSignal(const string symbol)
{
   g_ui_trend_status = "无趋势 (震荡)";
   
   if(InpNoTradeWeekendUTC && IsWeekendUTC())
   {
      g_ui_trend_status = "周末休市(不交易)";
      return;
   }

   // 如果账户已有本EA开出来的持仓，直接熔断放弃开仓计算
   if(PositionExists(symbol)) return;

   // 1. 未触发Setup时，寻找形态
   if(!g_setup.active)
   {
      bool isBull = TrendBull(symbol);
      bool isBear = TrendBear(symbol);
      
      if(isBull)
      {
         SetupState s;
         if(BuildBullSetup(symbol, s)) g_setup = s;
      }
      else if(isBear)
      {
         SetupState s;
         if(BuildBearSetup(symbol, s)) g_setup = s;
      }
      
      if(!g_setup.active) g_ui_pattern_status = "等待K线形态触发...";
      return;
   }

   // 2. 有激活的Setup，处理时间窗口、高低点跌破失效判定
   MqlRates last1[1];
   if(!GetRates(symbol, InpTFSignal, 1, 1, last1)) return;

   int barsSince = iBarShift(symbol, InpTFSignal, g_setup.setupBarTime, false);
   if(barsSince < 0)
   {
      g_setup.active = false;
      g_ui_break_status = "数据异常失效";
      return;
   }

   // 检查反向扫单造成的Setup失效
   double bid = SymbolInfoDouble(symbol, SYMBOL_BID);
   double ask = SymbolInfoDouble(symbol, SYMBOL_ASK);
   if(g_setup.bullish && bid < g_setup.setupLow)
   {
      g_setup.active = false;
      g_ui_break_status = "跌破Setup低点(信号失效)";
      return;
   }
   if(!g_setup.bullish && ask > g_setup.setupHigh)
   {
      g_setup.active = false;
      g_ui_break_status = "涨破Setup高点(信号失效)";
      return;
   }

   // 检查超出确认窗口期
   if(barsSince > InpStdConfirmWindowBars)
   {
      g_setup.active = false;
      g_ui_break_status = "超出时间窗口(已失效)";
      return;
   }

   // 3. 检查StdDev动能拐头转折
   bool stdOk = StdTurnDownAtShift(1);
   if(!stdOk)
   {
      g_ui_std_status = "波动率未转折向下";
      g_ui_break_status = "等待StdDev拐头...";
      return;
   }
   g_ui_std_status = "已转折向下(合格)";

   // 4. 突破开仓确认
   bool breakout = false;
   if(g_setup.bullish)
   {
      breakout = (ask > g_setup.setupHigh);
      g_ui_break_status = breakout ? "突破成功(开仓)" : StringFormat("等待突破高点(%.2f)", g_setup.setupHigh);
   }
   else
   {
      breakout = (bid < g_setup.setupLow);
      g_ui_break_status = breakout ? "突破成功(开仓)" : StringFormat("等待跌破低点(%.2f)", g_setup.setupLow);
   }

   if(!breakout) return;

   if(OpenPositionBySetup(symbol, g_setup))
   {
      g_setup.active = false;
   }
}

// ---------- MQL5 驱动主事件 ----------
int OnInit()
{
   string symbol = _Symbol;
   if(!SymbolSelect(symbol, true))
   {
      Print("Failed to select symbol: ", symbol);
      return INIT_FAILED;
   }

   hEmaH1 = iMA(symbol, InpTFTrend, InpEmaPeriod, 0, MODE_EMA, PRICE_CLOSE);
   hStdM15 = iStdDev(symbol, InpTFSignal, InpStdPeriod, 0, MODE_SMA, PRICE_CLOSE);
   hEmaTrailM15 = iMA(symbol, InpTFSignal, InpTrailEmaPeriod, 0, MODE_EMA, PRICE_CLOSE);

   if(hEmaH1 == INVALID_HANDLE || hStdM15 == INVALID_HANDLE || hEmaTrailM15 == INVALID_HANDLE)
   {
      Print("Indicator handle creation failed");
      return INIT_FAILED;
   }

   ResetTradeState();
   
   // 创建系统原生UI面板
   CreateDashboard();
   
   return INIT_SUCCEEDED;
}

void OnDeinit(const int reason)
{
   if(hEmaH1 != INVALID_HANDLE) IndicatorRelease(hEmaH1);
   if(hStdM15 != INVALID_HANDLE) IndicatorRelease(hStdM15);
   if(hEmaTrailM15 != INVALID_HANDLE) IndicatorRelease(hEmaTrailM15);
   
   // 卸载时彻底移除UI资源
   DestroyDashboard();
}

void OnTick()
{
   string symbol = _Symbol;

   // 仓位追踪管理
   ManagePosition(_Symbol);

   // 新K线生成执行条件
   if(IsNewBar(symbol, InpTFSignal, g_lastM15BarTime))
   {
      ManageTrailingByEmaClose(symbol);
      // 放到外层以达到高频Tick级监听突破和高低点失效
      ProcessSignal(symbol);
   }
   
   // Tick级重绘刷新UI状态字
   UpdateDashboard(); 
}