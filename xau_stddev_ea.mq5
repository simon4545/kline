#property strict
#property version   "1.10"
#property description "XAU Strategy EA"

#include <Trade/Trade.mqh>

CTrade trade;

input string InpSymbol = "XAUUSD";
input ENUM_TIMEFRAMES InpTFTrend = PERIOD_H1;
input ENUM_TIMEFRAMES InpTFSignal = PERIOD_M15;

// Trend filter
input int    InpEmaPeriod = 50;
input int    InpSlopeLookbackBars = 5;      // EMA50回溯数量
input double InpSlopeThreshold = 0.001;     // 斜率阈值
// input int    InpAtrPeriodH1 = 14;
// input double InpAtrMinPct = 0.0012;         // 0.12% (ATR/Close)

// StdDev phase
input int    InpStdPeriod = 20;
input int    InpStdConfirmWindowBars = 4;    // 形态确认数量

// Setup / invalidation
input int    InpSwingLookback = 5;
input double InpSLBufferPct = 0.002; // 止损容差阈值

// Risk / execution
input double InpFixedLots = 1.0;
input int    InpSlippagePoints = 50;
input uint   InpMagic = 260517;

// TP split
input double InpTP1_R = 1.0;
input double InpTP2_R = 2.0;
input double InpTP1_ClosePct = 0.40;
input double InpTP2_ClosePct = 0.30;

// trailing
input int    InpTrailEmaPeriod = 20;

// session filter
input bool   InpNoTradeWeekendUTC = true;

// -------- internal state --------
int hEmaH1 = INVALID_HANDLE;
int hAtrH1 = INVALID_HANDLE;
int hStdM15 = INVALID_HANDLE;
int hEmaTrailM15 = INVALID_HANDLE;

struct SetupState
{
   bool active;
   int  setupBarShift;      // signal bar shift at time of detection (usually 1)
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

// ---------- helpers ----------
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
   // MQL5: day_of_week => 0 Sunday ... 6 Saturday
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

bool TrendBull(const string symbol)
{
   int lb = MathMax(1, InpSlopeLookbackBars);

   MqlRates h1[1];
   if(!GetRates(symbol, InpTFTrend, 1, 1, h1)) return false;

   double ema[2];
   if(!GetIndicatorBuffer(hEmaH1, 1, lb+1, ema)) return false;

   // 条件A：EMA50斜率
   double slope = (ema[0] - ema[lb]) / ema[lb];
   bool condA = slope > InpSlopeThreshold;

   // 条件B：价格位置
   bool condB = h1[0].close > ema[0];

   // 条件C：ATR(14)/Close 去纠缠
   // double atr[1];
   // if(!GetIndicatorBuffer(hAtrH1, 1, 1, atr)) return false;
   // double atrPct = atr[0] / h1[0].close;
   // bool condC = atrPct > InpAtrMinPct;

   return (condA && condB);
}

bool TrendBear(const string symbol)
{
   int lb = MathMax(1, InpSlopeLookbackBars);

   MqlRates h1[1];
   if(!GetRates(symbol, InpTFTrend, 1, 1, h1)) return false;

   double ema[2];
   if(!GetIndicatorBuffer(hEmaH1, 1, lb+1, ema)) return false;

   // 条件A：EMA50斜率
   double slope = (ema[0] - ema[lb]) / ema[lb];
   bool condA = slope < -InpSlopeThreshold;

   // 条件B：价格位置
   bool condB = h1[0].close < ema[0];

   // 条件C：ATR(14)/Close 去纠缠
   // double atr[1];
   // if(!GetIndicatorBuffer(hAtrH1, 1, 1, atr)) return false;
   // double atrPct = atr[0] / h1[0].close;
   // bool condC = atrPct > InpAtrMinPct;

   return (condA && condB );
}

bool StdExpandingThenContracting(const string symbol, int shiftNow)
{
   // need SD[shift+4]..SD[shift]
   double sd[5];
   if(!GetIndicatorBuffer(hStdM15, shiftNow, 5, sd)) return false;
   // series: sd[0]=current at shiftNow, sd[1]=older1 ...
   // want: old->new: sd4 < sd3 < sd2 < sd1 and sd0 < sd1
   return (sd[4] < sd[3] && sd[3] < sd[2] && sd[2] < sd[1] && sd[0] < sd[1]);
}

bool StdTurnDownAtShift(int shift)
{
   double sd[3];
   if(!GetIndicatorBuffer(hStdM15, shift, 3, sd)) return false;
   // old->new in local window: sd2 < sd1 and sd0 < sd1
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
   return true;
}

bool PositionExists(const string symbol)
{
   return PositionSelect(symbol);
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
}

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

   if(ok && PositionSelect(symbol))
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
   if(!PositionSelect(symbol))
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

   // TP1
   if(!g_tp1Done && profitDist >= InpTP1_R * riskR)
   {
      double closeLots = NormalizeDouble(vol * InpTP1_ClosePct, 2);
      if(closeLots > 0.0)
      {
         trade.PositionClosePartial(symbol, closeLots);
      }
      // move SL to breakeven
      double be = RoundPrice(symbol, g_entryPrice);
      trade.PositionModify(symbol, be, 0.0);
      g_tp1Done = true;
   }

   // TP2
   if(!g_tp2Done && profitDist >= InpTP2_R * riskR)
   {
      if(PositionSelect(symbol))
      {
         double v2 = PositionGetDouble(POSITION_VOLUME);
         double closeLots2 = NormalizeDouble(v2 * InpTP2_ClosePct, 2);
         if(closeLots2 > 0.0)
            trade.PositionClosePartial(symbol, closeLots2);
      }

      // lock +1R
      double lockPrice = bullish ? (g_entryPrice + riskR) : (g_entryPrice - riskR);
      lockPrice = RoundPrice(symbol, lockPrice);
      trade.PositionModify(symbol, lockPrice, 0.0);
      g_tp2Done = true;
   }

   // trailing by EMA20 close rule (checked on new bar only externally)
}

void ManageTrailingByEmaClose(const string symbol)
{
   if(!PositionSelect(symbol)) return;

   long type = PositionGetInteger(POSITION_TYPE);

   MqlRates m15[2];
   if(!GetRates(symbol, InpTFSignal, 1, 2, m15)) return;

   double ema[2];
   if(!GetIndicatorBuffer(hEmaTrailM15, 1, 2, ema)) return;

   bool closeCond = false;
   if(type == POSITION_TYPE_BUY)
      closeCond = (m15[0].close < ema[0]);
   else
      closeCond = (m15[0].close > ema[0]);

   if(closeCond)
      trade.PositionClose(symbol);
}

void ProcessSignal(const string symbol)
{
   if(InpNoTradeWeekendUTC && IsWeekendUTC())
      return;

   if(PositionExists(symbol))
      return;

   // if no active setup, try to create one from latest closed bar
   if(!g_setup.active)
   {
      if(TrendBull(symbol))
      {
         SetupState s;
         if(BuildBullSetup(symbol, s))
            g_setup = s;
      }
      else if(TrendBear(symbol))
      {
         SetupState s;
         if(BuildBearSetup(symbol, s))
            g_setup = s;
      }
      return;
   }

   // setup active: within window wait stddev turn-down and breakout confirmation
   MqlRates last1[1];
   if(!GetRates(symbol, InpTFSignal, 1, 1, last1)) return;

   int barsSince = iBarShift(symbol, InpTFSignal, g_setup.setupBarTime, false);
   if(barsSince < 0)
   {
      g_setup.active = false;
      return;
   }

   // invalidation by setup low/high break
   double bid = SymbolInfoDouble(symbol, SYMBOL_BID);
   double ask = SymbolInfoDouble(symbol, SYMBOL_ASK);
   if(g_setup.bullish && bid < g_setup.setupLow)
   {
      g_setup.active = false;
      return;
   }
   if(!g_setup.bullish && ask > g_setup.setupHigh)
   {
      g_setup.active = false;
      return;
   }

   if(barsSince > InpStdConfirmWindowBars)
   {
      g_setup.active = false;
      return;
   }

   bool stdOk = StdTurnDownAtShift(1);
   if(!stdOk) return;

   // breakout confirmation
   bool breakout = false;
   if(g_setup.bullish)
      breakout = (ask > g_setup.setupHigh);
   else
      breakout = (bid < g_setup.setupLow);

   if(!breakout) return;

   if(OpenPositionBySetup(symbol, g_setup))
   {
      g_setup.active = false;
   }
}

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
   return INIT_SUCCEEDED;
}

void OnDeinit(const int reason)
{
   if(hEmaH1 != INVALID_HANDLE) IndicatorRelease(hEmaH1);
   if(hStdM15 != INVALID_HANDLE) IndicatorRelease(hStdM15);
   if(hEmaTrailM15 != INVALID_HANDLE) IndicatorRelease(hEmaTrailM15);
}

void OnTick()
{
   string symbol = _Symbol;

   // Always manage open position
   ManagePosition(_Symbol);

   // Run signal and trailing on new M15 bar
   if(IsNewBar(symbol, InpTFSignal, g_lastM15BarTime))
   {
      ManageTrailingByEmaClose(symbol);
      ProcessSignal(symbol);
   }
}


