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

CTrade         g_trade;
CPositionInfo  g_pos;

string g_panelName = "KB_EA_PANEL";

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

   string pnlText =
      "持仓数: " + IntegerToString(totalPositions) + "\n"
      + "总手数: " + DoubleToString(totalLots, 2) + "\n"
      + "浮动盈亏: " + DoubleToString(totalProfit, 2);

   if(ObjectFind(0, g_panelName) < 0)
   {
      ObjectCreate(0, g_panelName, OBJ_LABEL, 0, 0, 0);
      ObjectSetInteger(0, g_panelName, OBJPROP_CORNER, CORNER_LEFT_UPPER);
      ObjectSetInteger(0, g_panelName, OBJPROP_XDISTANCE, 12);
      ObjectSetInteger(0, g_panelName, OBJPROP_YDISTANCE, 24);
      ObjectSetInteger(0, g_panelName, OBJPROP_COLOR, clrWhite);
      ObjectSetInteger(0, g_panelName, OBJPROP_FONTSIZE, 11);
      ObjectSetString(0, g_panelName, OBJPROP_FONT, "Consolas");
   }

   color pnlColor = clrWhite;
   if(totalProfit > 0.0)
      pnlColor = clrLime;
   else if(totalProfit < 0.0)
      pnlColor = clrTomato;

   ObjectSetInteger(0, g_panelName, OBJPROP_COLOR, pnlColor);
   ObjectSetString(0, g_panelName, OBJPROP_TEXT, pnlText);
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
   Print("[EA] keyboard trading EA started. symbol=", ResolveTradeSymbol());
   Print("[EA] focus chart then use arrow keys: up=buy, down=sell, left=close all, right=close by delta");
   UpdatePanel();
   return(INIT_SUCCEEDED);
}

void OnDeinit(const int reason)
{
   ObjectDelete(0, g_panelName);
}

void OnTick()
{
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
