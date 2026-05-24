#property strict
#property description "Keyboard hotkey trading EA"

#include <Trade/Trade.mqh>
#include <Trade/PositionInfo.mqh>

input double InpLots             = 0.10;
input long   InpMagic            = 20260521;
input int    InpDeviationPoints  = 30;
input string InpTradeSymbol      = "";
input string InpOrderComment     = "keyboard-ea";
input double InpCloseDeltaUSD    = 0.5;
input bool   InpOnlyCurrentChart = true;
input int    InpPairDiffTickWindow = 30;
input bool   InpUseTimerForPanel   = false;
input int    InpPanelTimerSeconds  = 1;

CTrade         g_trade;
CPositionInfo  g_pos;

string g_panelName = "KB_EA_PANEL";

double g_tickPrices[];
double g_pairwiseAbsDiffSum = 0.0;

string ResolveTradeSymbol()
{
   string symbol = InpTradeSymbol;
   StringTrimLeft(symbol);
   StringTrimRight(symbol);
   if(symbol == "")
      symbol = _Symbol;
   return symbol;
}

bool EnsureSymbolReady(string symbol)
{
   if(symbol == "")
      return false;

   if(!SymbolSelect(symbol, true))
   {
      Print("[EA] SymbolSelect failed: ", symbol);
      return false;
   }

   MqlTick tick;
   if(!SymbolInfoTick(symbol, tick))
   {
      Print("[EA] SymbolInfoTick failed: ", symbol);
      return false;
   }

   return true;
}

double NormalizeLots(string symbol, double lots)
{
   double minLot  = SymbolInfoDouble(symbol, SYMBOL_VOLUME_MIN);
   double maxLot  = SymbolInfoDouble(symbol, SYMBOL_VOLUME_MAX);
   double stepLot = SymbolInfoDouble(symbol, SYMBOL_VOLUME_STEP);

   if(stepLot <= 0.0)
      stepLot = 0.01;

   lots = MathMax(minLot, MathMin(maxLot, lots));
   lots = MathFloor(lots / stepLot) * stepLot;

   int digits = 2;
   if(stepLot == 1.0) digits = 0;
   else if(stepLot == 0.1) digits = 1;
   else if(stepLot == 0.01) digits = 2;
   else if(stepLot == 0.001) digits = 3;

   return NormalizeDouble(lots, digits);
}

bool PlaceMarketOrder(bool isBuy)
{
   string symbol = ResolveTradeSymbol();
   if(!EnsureSymbolReady(symbol))
      return false;

   double lots = NormalizeLots(symbol, InpLots);
   if(lots <= 0.0)
   {
      Print("[EA] invalid lots after normalize: ", DoubleToString(lots, 2));
      return false;
   }

   g_trade.SetAsyncMode(false);
   g_trade.SetExpertMagicNumber(InpMagic);
   g_trade.SetDeviationInPoints(InpDeviationPoints);

   bool ok = false;
   if(isBuy)
      ok = g_trade.Buy(lots, symbol, 0.0, 0.0, 0.0, InpOrderComment);
   else
      ok = g_trade.Sell(lots, symbol, 0.0, 0.0, 0.0, InpOrderComment);

   if(!ok)
   {
      Print("[EA] place order failed. side=", (isBuy ? "BUY" : "SELL"),
            " ret=", g_trade.ResultRetcode(),
            " msg=", g_trade.ResultRetcodeDescription());
      return false;
   }

   Print("[EA] order sent. side=", (isBuy ? "BUY" : "SELL"),
         " symbol=", symbol,
         " lots=", DoubleToString(lots, 2),
         " order=", (string)g_trade.ResultOrder(),
         " deal=", (string)g_trade.ResultDeal());
   return true;
}

bool ShouldManageCurrentPosition(string symbol)
{
   if(InpOnlyCurrentChart && PositionGetString(POSITION_SYMBOL) != symbol)
      return false;

   long magic = (long)PositionGetInteger(POSITION_MAGIC);
   if(magic != InpMagic)
      return false;

   return true;
}

double EstimateBalanceLiquidationPrice(string symbol)
{
   double balance = AccountInfoDouble(ACCOUNT_BALANCE);
   double totalSwap = 0.0;
   double totalCommission = 0.0;
   double totalDirectionalLots = 0.0;
   double weightedOpenPrice = 0.0;
   long positionType = -1;
 
   for(int i = PositionsTotal() - 1; i >= 0; --i)
   {
      ulong ticket = PositionGetTicket(i);
      if(ticket == 0)
         continue;
      if(!PositionSelectByTicket(ticket))
         continue;
      if(!ShouldManageCurrentPosition(symbol))
         continue;
 
      long currentType = PositionGetInteger(POSITION_TYPE);
      double volume = PositionGetDouble(POSITION_VOLUME);
      double openPrice = PositionGetDouble(POSITION_PRICE_OPEN);
 
      if(positionType == -1)
         positionType = currentType;
      else if(positionType != currentType)
         return 0.0;
 
      totalDirectionalLots += volume;
      weightedOpenPrice += openPrice * volume;
      totalSwap += PositionGetDouble(POSITION_SWAP);
      totalCommission += PositionGetDouble(POSITION_COMMISSION);
   }
 
   if(totalDirectionalLots <= 0.0 || positionType == -1)
      return 0.0;
 
   double avgOpenPrice = weightedOpenPrice / totalDirectionalLots;
   double tickSize = SymbolInfoDouble(symbol, SYMBOL_TRADE_TICK_SIZE);
   double tickValue = SymbolInfoDouble(symbol, SYMBOL_TRADE_TICK_VALUE);
 
   if(tickSize <= 0.0 || tickValue <= 0.0)
      return 0.0;
 
   double valuePerPriceUnit = (tickValue / tickSize) * totalDirectionalLots;
   if(valuePerPriceUnit <= 0.0)
      return 0.0;
 
   double maxLossAbs = balance + totalSwap + totalCommission;
   if(maxLossAbs <= 0.0)
      return avgOpenPrice;
 
   double priceMove = maxLossAbs / valuePerPriceUnit;
   if(positionType == POSITION_TYPE_BUY)
      return avgOpenPrice - priceMove;
 
   if(positionType == POSITION_TYPE_SELL)
      return avgOpenPrice + priceMove;
 
   return 0.0;
}

void TrimTickWindow()
{
   int maxWindow = MathMax(2, InpPairDiffTickWindow);
   int count = ArraySize(g_tickPrices);
   while(count > maxWindow)
   {
      for(int i = 1; i < count; ++i)
         g_tickPrices[i - 1] = g_tickPrices[i];
      ArrayResize(g_tickPrices, count - 1);
      count--;
   }
}

void RecalculatePairwiseAbsDiffSum()
{
   g_pairwiseAbsDiffSum = 0.0;
   int count = ArraySize(g_tickPrices);
   if(count < 2)
      return;

   for(int i = 0; i < count; ++i)
   {
      for(int j = i + 1; j < count; ++j)
         g_pairwiseAbsDiffSum += MathAbs(g_tickPrices[i] - g_tickPrices[j]);
   }
}

void RecordTickPrice(double price)
{
   int nextIndex = ArraySize(g_tickPrices);
   ArrayResize(g_tickPrices, nextIndex + 1);
   g_tickPrices[nextIndex] = price;

   TrimTickWindow();
   RecalculatePairwiseAbsDiffSum();
}
 
void UpdatePanel()
{
   string symbol = ResolveTradeSymbol();
 
   int totalPositions = 0;
   double totalLots = 0.0;
   double totalProfit = 0.0;
 
   for(int i = PositionsTotal() - 1; i >= 0; --i)
   {
      ulong ticket = PositionGetTicket(i);
      if(ticket == 0)
         continue;
      if(!PositionSelectByTicket(ticket))
         continue;
      if(!ShouldManageCurrentPosition(symbol))
         continue;
 
      totalPositions++;
      totalLots += PositionGetDouble(POSITION_VOLUME);
      totalProfit += PositionGetDouble(POSITION_PROFIT);
   }
 
   int pendingOrders = 0;
   for(int j = OrdersTotal() - 1; j >= 0; --j)
   {
      ulong ordTicket = OrderGetTicket(j);
      if(ordTicket == 0)
         continue;
      if(!OrderSelect(ordTicket))
         continue;
 
      string os = OrderGetString(ORDER_SYMBOL);
      long om = (long)OrderGetInteger(ORDER_MAGIC);
 
      if(InpOnlyCurrentChart && os != symbol)
         continue;
      if(om != InpMagic)
         continue;
 
      pendingOrders++;
   }
 
   int priceDigits = (int)SymbolInfoInteger(symbol, SYMBOL_DIGITS);
   double liquidationPrice = EstimateBalanceLiquidationPrice(symbol);
   string liquidationText = "强平价: --";
   if(liquidationPrice > 0.0)
      liquidationText = "强平价: " + DoubleToString(liquidationPrice, priceDigits);
   if(totalPositions > 0 && liquidationPrice <= 0.0)
      liquidationText = "强平价: 对冲/不可算";
 
   int tickWindowCount = ArraySize(g_tickPrices);
   string pnlText1 =
      "持仓数: " + IntegerToString(totalPositions) + "\n"
      + "总手数: " + DoubleToString(totalLots, 2) + "\n"
      + "浮动盈亏: " + DoubleToString(totalProfit, 2) + "\n"
      + liquidationText;
   string pnlText2 =
      "Tick窗口: " + IntegerToString(tickWindowCount) + "\n"
      + "价差绝对值和: " + DoubleToString(g_pairwiseAbsDiffSum, priceDigits);
 
   string panelMain = g_panelName + "_MAIN";
   string panelStat = g_panelName + "_STAT";

   if(ObjectFind(0, panelMain) < 0)
   {
      ObjectCreate(0, panelMain, OBJ_LABEL, 0, 0, 0);
      ObjectSetInteger(0, panelMain, OBJPROP_CORNER, CORNER_LEFT_UPPER);
      ObjectSetInteger(0, panelMain, OBJPROP_XDISTANCE, 12);
      ObjectSetInteger(0, panelMain, OBJPROP_YDISTANCE, 24);
      ObjectSetInteger(0, panelMain, OBJPROP_COLOR, clrWhite);
      ObjectSetInteger(0, panelMain, OBJPROP_FONTSIZE, 11);
      ObjectSetInteger(0, panelMain, OBJPROP_BACK, false);
      ObjectSetInteger(0, panelMain, OBJPROP_SELECTABLE, false);
      ObjectSetInteger(0, panelMain, OBJPROP_HIDDEN, true);
      ObjectSetString(0, panelMain, OBJPROP_FONT, "Consolas");
   }

   if(ObjectFind(0, panelStat) < 0)
   {
      ObjectCreate(0, panelStat, OBJ_LABEL, 0, 0, 0);
      ObjectSetInteger(0, panelStat, OBJPROP_CORNER, CORNER_LEFT_UPPER);
      ObjectSetInteger(0, panelStat, OBJPROP_XDISTANCE, 12);
      ObjectSetInteger(0, panelStat, OBJPROP_YDISTANCE, 92);
      ObjectSetInteger(0, panelStat, OBJPROP_COLOR, clrWhite);
      ObjectSetInteger(0, panelStat, OBJPROP_FONTSIZE, 11);
      ObjectSetInteger(0, panelStat, OBJPROP_BACK, false);
      ObjectSetInteger(0, panelStat, OBJPROP_SELECTABLE, false);
      ObjectSetInteger(0, panelStat, OBJPROP_HIDDEN, true);
      ObjectSetString(0, panelStat, OBJPROP_FONT, "Consolas");
   }

   color pnlColor = clrWhite;
   if(totalProfit > 0.0)
      pnlColor = clrLime;
   else if(totalProfit < 0.0)
      pnlColor = clrTomato;

   ObjectSetInteger(0, panelMain, OBJPROP_COLOR, pnlColor);
   ObjectSetString(0, panelMain, OBJPROP_TEXT, pnlText1);
   ObjectSetInteger(0, panelStat, OBJPROP_COLOR, clrAqua);
   ObjectSetString(0, panelStat, OBJPROP_TEXT, pnlText2);
}

bool CloseAllPositionsAsync()
{
   string symbol = ResolveTradeSymbol();
   bool any = false;

   g_trade.SetAsyncMode(true);
   g_trade.SetExpertMagicNumber(InpMagic);
   g_trade.SetDeviationInPoints(InpDeviationPoints);

   for(int i = PositionsTotal() - 1; i >= 0; --i)
   {
      ulong ticket = PositionGetTicket(i);
      if(ticket == 0)
         continue;
      if(!PositionSelectByTicket(ticket))
         continue;
      if(!ShouldManageCurrentPosition(symbol))
         continue;

      if(g_trade.PositionClose(ticket, InpDeviationPoints))
      {
         any = true;
         Print("[EA] async close all sent. ticket=", (string)ticket);
      }
      else
      {
         Print("[EA] async close all failed. ticket=", (string)ticket,
               " ret=", g_trade.ResultRetcode(),
               " msg=", g_trade.ResultRetcodeDescription());
      }
   }

   if(!any)
      Print("[EA] no position matched for close all");

   return any;
}

bool CloseOffsetPositionsAsync()
{
   string symbol = ResolveTradeSymbol();
   if(!EnsureSymbolReady(symbol))
      return false;

   MqlTick tick;
   if(!SymbolInfoTick(symbol, tick))
      return false;

   bool any = false;
   int priceDigits = (int)SymbolInfoInteger(symbol, SYMBOL_DIGITS);

   g_trade.SetAsyncMode(true);
   g_trade.SetExpertMagicNumber(InpMagic);
   g_trade.SetDeviationInPoints(InpDeviationPoints);

   for(int i = PositionsTotal() - 1; i >= 0; --i)
   {
      ulong ticket = PositionGetTicket(i);
      if(ticket == 0)
         continue;
      if(!PositionSelectByTicket(ticket))
         continue;
      if(!ShouldManageCurrentPosition(symbol))
         continue;

      long posType = PositionGetInteger(POSITION_TYPE);
      double openPrice = PositionGetDouble(POSITION_PRICE_OPEN);
      string posSymbol = PositionGetString(POSITION_SYMBOL);
      if(posSymbol != symbol)
         continue;

      bool shouldClose = false;
      if(posType == POSITION_TYPE_BUY)
         shouldClose = (openPrice <= tick.bid - InpCloseDeltaUSD);
      else if(posType == POSITION_TYPE_SELL)
         shouldClose = (openPrice >= tick.ask + InpCloseDeltaUSD);

      if(!shouldClose)
         continue;

      if(g_trade.PositionClose(ticket, InpDeviationPoints))
      {
         any = true;
         Print("[EA] async close by delta sent. ticket=", (string)ticket,
               " open=", DoubleToString(openPrice, priceDigits));
      }
      else
      {
         Print("[EA] async close by delta failed. ticket=", (string)ticket,
               " ret=", g_trade.ResultRetcode(),
               " msg=", g_trade.ResultRetcodeDescription());
      }
   }

   if(!any)
      Print("[EA] no position matched for close by delta");

   return any;
}

void HandleHotkey(int key)
{
   Print("[EA] key event received: ", key);

   if(key == 38)
   {
      PlaceMarketOrder(true);
      return;
   }

   if(key == 40)
   {
      PlaceMarketOrder(false);
      return;
   }

   if(key == 37)
   {
      CloseAllPositionsAsync();
      return;
   }

   if(key == 39)
   {
      CloseOffsetPositionsAsync();
      return;
   }
}

int OnInit()
{
   ArrayResize(g_tickPrices, 0);
   g_pairwiseAbsDiffSum = 0.0;

   if(InpUseTimerForPanel)
      EventSetTimer(MathMax(1, InpPanelTimerSeconds));

   Print("[EA] keyboard trading EA started. symbol=", ResolveTradeSymbol());
   Print("[EA] focus chart then use arrow keys: up=buy, down=sell, left=close all, right=close by delta");
   UpdatePanel();
   return(INIT_SUCCEEDED);
}

void OnDeinit(const int reason)
{
   EventKillTimer();
   ObjectDelete(0, g_panelName + "_MAIN");
   ObjectDelete(0, g_panelName + "_STAT");
}

void OnTick()
{
   string symbol = ResolveTradeSymbol();
   MqlTick tick;
   if(SymbolInfoTick(symbol, tick))
   {
      double price = tick.last;
      if(price <= 0.0)
         price = (tick.bid + tick.ask) * 0.5;
      RecordTickPrice(price);
   }

   if(!InpUseTimerForPanel)
      UpdatePanel();
}

void OnTimer()
{
   if(InpUseTimerForPanel)
      UpdatePanel();
}

void OnChartEvent(const int id,
                  const long &lparam,
                  const double &dparam,
                  const string &sparam)
{
   if(id == CHARTEVENT_KEYDOWN)
   {
      HandleHotkey((int)lparam);
      UpdatePanel();
   }
}
