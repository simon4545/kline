#property strict

#include <Trade/Trade.mqh>

// EA: Local Socket Trade Receiver (MT5)
// 通过 ws2_32.dll 监听本地 127.0.0.1:18555，接收文本命令并下单
// 命令格式（单行，\n 结尾）：
// BUY,EURUSD,0.10,0,0,ea-from-socket
// SELL,XAUUSD,0.20,0,0,test
// 字段: side,symbol,lots,sl_points,tp_points,comment

input string InpBindIP          = "0.0.0.0";
input int    InpBindPort        = 18555;
input long   InpMagic           = 185551;
input int    InpDeviationPts    = 30;
input int    InpTimerMs         = 200;
input double InpMaxLots         = 5.0;
input bool   InpAllowCloseCmd   = true;
input bool   InpEnableGoldTrail = true;
input string InpGoldSymbolMatch = "XAUUSD";
input double InpGoldMoveUSD     = 10.0;

#define INVALID_SOCKET (-1)
#define SOCKET_ERROR   (-1)
#define WSAEWOULDBLOCK 10035
#define AF_INET        2
#define SOCK_STREAM    1
#define IPPROTO_TCP    6
#define SOL_SOCKET     0xFFFF
#define SO_REUSEADDR   0x0004
#define FIONBIO        0x8004667E

uchar  g_wsadata[512];
int    g_listenSock = INVALID_SOCKET;
int    g_clientSock = INVALID_SOCKET;
bool   g_started    = false;
uchar  g_addr[16];

CTrade g_trade;

string BytesToHex(uchar &buf[], int count)
{
   string out = "";
   for(int i = 0; i < count; i++)
   {
      if(i > 0) out += " ";
      out += StringFormat("%02X", (int)buf[i]);
   }
   return out;
}

string ExtractAsciiLine(uchar &buf[], int count)
{
   string out = "";
   for(int i = 0; i < count; i++)
   {
      int b = (int)buf[i];
      if(b == 0) continue;
      if(b == 13) continue;
      if(b == 10) break;
      out += CharToString((ushort)b);
   }
   return out;
}

#import "ws2_32.dll"
int    WSAStartup(ushort wVersionRequested, uchar &lpWSAData[]);
int    WSACleanup();
int    socket(int af, int type, int protocol);
int    bind(int s, uchar &name[], int namelen);
int    listen(int s, int backlog);
int    accept(int s, uchar &addr[], int &addrlen);
int    closesocket(int s);
int    ioctlsocket(int s, int cmd, uint &argp);
int    setsockopt(int s, int level, int optname, int &optval, int optlen);
int    recv(int s, uchar &buf[], int len, int flags);
int    send(int s, uchar &buf[], int len, int flags);
uint   inet_addr(string cp);
ushort htons(ushort hostshort);
int    WSAGetLastError();
#import

string ToUpperEx(string s)
{
   string t=s;
   StringToUpper(t);
   return t;
}

string SafeStr(string s)
{
   return StringSubstr(s, 0, StringLen(s));
}

string Trim(string s)
{
   string t=s;
   StringTrimLeft(t);
   StringTrimRight(t);
   return t;
}

void SetSockAddr(uchar &addr[], string ip, int port)
{
   ArrayInitialize(addr, 0);
   addr[0] = (uchar)AF_INET;
   addr[1] = 0;

   ushort p = htons((ushort)port);
   addr[2] = (uchar)(p & 0xFF);
   addr[3] = (uchar)((p >> 8) & 0xFF);

   uint nip = inet_addr(ip);
   addr[4] = (uchar)(nip & 0xFF);
   addr[5] = (uchar)((nip >> 8) & 0xFF);
   addr[6] = (uchar)((nip >> 16) & 0xFF);
   addr[7] = (uchar)((nip >> 24) & 0xFF);
}

bool SendText(int sock, string text)
{
   uchar out[];
   // 使用 ANSI 单字节编码，避免 UTF-8/BOM 兼容性问题
   StringToCharArray(text, out, 0, WHOLE_ARRAY, CP_ACP);
   int n = ArraySize(out);
   if(n <= 1) return true;

   int sent = send(sock, out, n - 1, 0); // 不发送结尾\0
   return (sent > 0);
}

bool StartServer()
{
   int rc = WSAStartup(0x0202, g_wsadata);
   if(rc != 0)
   {
      Print("WSAStartup failed: rc=", rc, " wsa=", WSAGetLastError());
      return false;
   }

   g_listenSock = socket(AF_INET, SOCK_STREAM, IPPROTO_TCP);
   if(g_listenSock == INVALID_SOCKET)
   {
      Print("socket failed wsa=", WSAGetLastError());
      WSACleanup();
      return false;
   }

   int reuse = 1;
   setsockopt(g_listenSock, SOL_SOCKET, SO_REUSEADDR, reuse, 4);

   SetSockAddr(g_addr, InpBindIP, InpBindPort);
   rc = bind(g_listenSock, g_addr, 16);
   if(rc == SOCKET_ERROR)
   {
      int wsa = WSAGetLastError();
      Print("bind failed: ", InpBindIP, ":", InpBindPort, " wsa=", wsa,
            " (10048=地址已在使用,10013=权限拒绝,10049=地址无效)");
      closesocket(g_listenSock);
      g_listenSock = INVALID_SOCKET;
      WSACleanup();
      return false;
   }

   rc = listen(g_listenSock, 5);
   if(rc == SOCKET_ERROR)
   {
      Print("listen failed wsa=", WSAGetLastError());
      closesocket(g_listenSock);
      g_listenSock = INVALID_SOCKET;
      WSACleanup();
      return false;
   }

   uint nonblock = 1;
   ioctlsocket(g_listenSock, FIONBIO, nonblock);

   g_started = true;
   Print("EA socket server started at ", InpBindIP, ":", InpBindPort,
         " (提示: 0.0.0.0 表示监听本机所有网卡)");
   return true;
}

void StopServer()
{
   if(g_clientSock != INVALID_SOCKET)
   {
      closesocket(g_clientSock);
      g_clientSock = INVALID_SOCKET;
   }

   if(g_listenSock != INVALID_SOCKET)
   {
      closesocket(g_listenSock);
      g_listenSock = INVALID_SOCKET;
   }

   WSACleanup();
   g_started = false;
}

bool EnsureSymbolReady(string symbol)
{
   symbol = Trim(symbol);
   if(symbol == "") return false;

   if(!SymbolSelect(symbol, true))
   {
      Print("SymbolSelect failed: ", symbol);
      return false;
   }

   MqlTick tk;
   if(!SymbolInfoTick(symbol, tk))
   {
      Print("SymbolInfoTick failed: ", symbol);
      return false;
   }
   return true;
}

double NormalizeVolumeBySymbol(string symbol,double volume)
{
   double minv = SymbolInfoDouble(symbol, SYMBOL_VOLUME_MIN);
   double maxv = SymbolInfoDouble(symbol, SYMBOL_VOLUME_MAX);
   double step = SymbolInfoDouble(symbol, SYMBOL_VOLUME_STEP);
   if(step <= 0.0) step = minv;
   if(step <= 0.0) step = 0.01;

   volume = MathMax(minv, MathMin(maxv, volume));
   volume = MathFloor(volume / step) * step;
   int vd = 2;
   if(step == 1.0) vd = 0;
   else if(step == 0.1) vd = 1;
   else if(step == 0.01) vd = 2;
   else if(step == 0.001) vd = 3;
   return NormalizeDouble(volume, vd);
}

bool IsGoldManagedSymbol(string symbol)
{
   string up = ToUpperEx(symbol);
   string key = ToUpperEx(InpGoldSymbolMatch);
   return StringFind(up, key) >= 0;
}

void ManageGoldRunner()
{
   if(!InpEnableGoldTrail) return;

   for(int i = PositionsTotal() - 1; i >= 0; --i)
   {
      ulong ticket = PositionGetTicket(i);
      if(ticket == 0) continue;
      if(!PositionSelectByTicket(ticket)) continue;

      string symbol = PositionGetString(POSITION_SYMBOL);
      long magic = (long)PositionGetInteger(POSITION_MAGIC);
      if(magic != InpMagic) continue;
      if(!IsGoldManagedSymbol(symbol)) continue;

      string comment = PositionGetString(POSITION_COMMENT);
      if(StringFind(comment, "|HALF_DONE") >= 0)
         continue;

      long posType = PositionGetInteger(POSITION_TYPE);
      double openPrice = PositionGetDouble(POSITION_PRICE_OPEN);
      double sl = PositionGetDouble(POSITION_SL);
      double tp = PositionGetDouble(POSITION_TP);
      double volume = PositionGetDouble(POSITION_VOLUME);
      double minv = SymbolInfoDouble(symbol, SYMBOL_VOLUME_MIN);
      double point = SymbolInfoDouble(symbol, SYMBOL_POINT);
      int digits = (int)SymbolInfoInteger(symbol, SYMBOL_DIGITS);

      MqlTick tk;
      if(!SymbolInfoTick(symbol, tk)) continue;

      double curPrice = (posType == POSITION_TYPE_BUY ? tk.bid : tk.ask);
      double move = (posType == POSITION_TYPE_BUY ? (curPrice - openPrice) : (openPrice - curPrice));
      if(move < InpGoldMoveUSD) continue;

      double half = NormalizeVolumeBySymbol(symbol, volume / 2.0);
      bool canHalf = (half >= minv && half < volume);

      g_trade.SetExpertMagicNumber(InpMagic);
      g_trade.SetDeviationInPoints(InpDeviationPts);

      bool close_ok = false;
      if(canHalf)
      {
         close_ok = g_trade.PositionClosePartial(ticket, half, InpDeviationPts);
         if(close_ok)
         {
            Print("[GOLD MANAGE] partial close symbol=", symbol, " ticket=", (string)ticket,
                  " close=", DoubleToString(half, 2), " remain~", DoubleToString(volume - half, 2));
         }
         else
         {
            Print("[GOLD MANAGE] PositionClosePartial failed symbol=", symbol, " ret=", g_trade.ResultRetcode(),
                  " ", g_trade.ResultRetcodeDescription());
            continue;
         }
      }
      else
      {
         close_ok = g_trade.PositionClose(ticket, InpDeviationPts);
         if(close_ok)
         {
            Print("[GOLD MANAGE] full close symbol=", symbol, " ticket=", (string)ticket,
                  " reason=volume too small for half");
         }
         else
         {
            Print("[GOLD MANAGE] full close failed symbol=", symbol, " ret=", g_trade.ResultRetcode(),
                  " ", g_trade.ResultRetcodeDescription());
         }
         continue;
      }

      if(!PositionSelectByTicket(ticket))
         continue;

      double remain_sl = PositionGetDouble(POSITION_SL);
      double remain_tp = PositionGetDouble(POSITION_TP);
      double remain_volume = PositionGetDouble(POSITION_VOLUME);
      string remain_comment = PositionGetString(POSITION_COMMENT);

      bool be_ok = g_trade.PositionModify(symbol, NormalizeDouble(openPrice, digits), remain_tp);
      if(be_ok)
      {
         Print("[GOLD MANAGE] move SL to BE symbol=", symbol, " ticket=", (string)ticket,
               " open=", DoubleToString(openPrice, digits), " remain=", DoubleToString(remain_volume, 2));
      }
      else
      {
         Print("[GOLD MANAGE] PositionModify failed symbol=", symbol, " ret=", g_trade.ResultRetcode(),
               " ", g_trade.ResultRetcodeDescription(), " oldsl=", DoubleToString(remain_sl, digits));
      }
   }
}

bool CloseBySymbol(string symbol)
{
   symbol = Trim(symbol);
   bool any = false;

   for(int i=PositionsTotal()-1; i>=0; --i)
   {
      ulong ticket = PositionGetTicket(i);
      if(ticket == 0) continue;
      if(!PositionSelectByTicket(ticket)) continue;

      string psym = PositionGetString(POSITION_SYMBOL);
      long   pmgc = (long)PositionGetInteger(POSITION_MAGIC);
      if(psym != symbol) continue;
      if(pmgc != InpMagic) continue;

      if(g_trade.PositionClose(ticket, InpDeviationPts)) any = true;
      else Print("PositionClose failed ticket=", (string)ticket, " ret=", g_trade.ResultRetcode(), " ", g_trade.ResultRetcodeDescription());
   }

   return any;
}

bool PlaceMarketOrder(string side, string symbol, double lots, int sl_points, int tp_points, string comment, string &msg)
{
   side    = SafeStr(ToUpperEx(Trim(side)));
   symbol  = SafeStr(Trim(symbol));
   comment = SafeStr(Trim(comment));

   Print("[PLACE] side='", side, "' len=", StringLen(side));

   if(lots <= 0.0 || lots > InpMaxLots)
   {
      msg = "ERR lots out of range";
      return false;
   }

   if(!EnsureSymbolReady(symbol))
   {
      msg = "ERR symbol not ready";
      return false;
   }

   double point = SymbolInfoDouble(symbol, SYMBOL_POINT);
   int    digits = (int)SymbolInfoInteger(symbol, SYMBOL_DIGITS);

   MqlTick tk;
   if(!SymbolInfoTick(symbol, tk))
   {
      msg = "ERR no tick";
      return false;
   }

   double price = 0.0;
   bool isBuy = false;
   if(side == "BUY")
   {
      isBuy = true;
      price = tk.ask;
   }
   else if(side == "SELL")
   {
      isBuy = false;
      price = tk.bid;
   }
   else
   {
      msg = "ERR side must BUY/SELL";
      return false;
   }

   double sl = 0.0, tp = 0.0;
   if(sl_points > 0)
   {
      if(isBuy) sl = NormalizeDouble(price - sl_points * point, digits);
      else      sl = NormalizeDouble(price + sl_points * point, digits);
   }
   if(tp_points > 0)
   {
      if(isBuy) tp = NormalizeDouble(price + tp_points * point, digits);
      else      tp = NormalizeDouble(price - tp_points * point, digits);
   }

   g_trade.SetExpertMagicNumber(InpMagic);
   g_trade.SetDeviationInPoints(InpDeviationPts);

   bool ok;
   if(isBuy) ok = g_trade.Buy(lots, symbol, 0.0, sl, tp, comment);
   else      ok = g_trade.Sell(lots, symbol, 0.0, sl, tp, comment);

   if(!ok)
   {
      msg = "ERR trade ret=" + IntegerToString((int)g_trade.ResultRetcode()) + " " + g_trade.ResultRetcodeDescription();
      return false;
   }

   msg = "OK order=" + IntegerToString((int)g_trade.ResultOrder()) + " deal=" + IntegerToString((int)g_trade.ResultDeal());
   return true;
}

void HandleCommand(string line)
{
   line = Trim(line);
   Print("[RX RAW] '", line, "' len=", StringLen(line));
   if(line == "") return;

   string parts[];
   int n = StringSplit(line, ',', parts);

   if(n >= 1)
   {
      string cmd = ToUpperEx(Trim(parts[0]));
      if(cmd == "PING")
      {
         SendText(g_clientSock, "PONG\n");
         return;
      }
   }

   if(n < 6)
   {
      Print("[RX ERR] split fields=", n, " line='", line, "'");
      for(int i = 0; i < n; i++)
      {
         string pv = SafeStr(parts[i]);
         Print("[RX PART] idx=", i, " val='", pv, "' len=", StringLen(pv));
      }
      SendText(g_clientSock, "ERR format: side,symbol,lots,sl_points,tp_points,comment\n");
      return;
   }

   string side    = SafeStr(parts[0]);
   string symbol  = SafeStr(parts[1]);
   double lots    = StringToDouble(parts[2]);
   int slp        = (int)StringToInteger(parts[3]);
   int tpp        = (int)StringToInteger(parts[4]);
   string comment = SafeStr(parts[5]);

   string up = SafeStr(ToUpperEx(Trim(side)));
   Print("[RX PARSED] side='", side, "' len=", StringLen(side), " up='", up, "' len2=", StringLen(up), " symbol='", symbol, "' lots=", DoubleToString(lots, 2));
   if(InpAllowCloseCmd && up == "CLOSE")
   {
      bool okc = CloseBySymbol(symbol);
      if(okc) SendText(g_clientSock, "OK close sent\n");
      else    SendText(g_clientSock, "ERR no position closed\n");
      return;
   }

   string msg;
   bool ok = PlaceMarketOrder(up, symbol, lots, slp, tpp, comment, msg);
   SendText(g_clientSock, msg + "\n");
}

void PumpSocket()
{
   if(!g_started) return;

   if(g_clientSock == INVALID_SOCKET)
   {
      uchar caddr[16];
      int clen = 16;
      int cs = accept(g_listenSock, caddr, clen);
      if(cs != INVALID_SOCKET)
      {
         g_clientSock = cs;
         uint nonblock = 1;
         ioctlsocket(g_clientSock, FIONBIO, nonblock);
         Print("Client connected");
      }
      else
      {
         int awsa = WSAGetLastError();
         if(awsa != WSAEWOULDBLOCK)
            Print("accept failed wsa=", awsa);
      }
   }

   if(g_clientSock == INVALID_SOCKET)
      return;

   uchar buf[1024];
   int got = recv(g_clientSock, buf, 1024, 0);

   if(got > 0)
   {
      Print("[RECV] got=", got, " hex=", BytesToHex(buf, got));

      // 先按字节清理 UTF-8 BOM，避免首字母被吞
      int start = 0;
      if(got >= 3 && buf[0] == 0xEF && buf[1] == 0xBB && buf[2] == 0xBF)
         start = 3;

      string s_utf8 = CharArrayToString(buf, start, got - start, CP_UTF8);
      string s_acp  = CharArrayToString(buf, start, got - start, CP_ACP);
      string s      = s_utf8;
      if(StringLen(s) == 0)
         s = s_acp;

      Print("[RECV UTF8] '", s_utf8, "' len=", StringLen(s_utf8));
      Print("[RECV ACP ] '", s_acp,  "' len=", StringLen(s_acp));
      Print("[RECV USE ] '", s,      "' len=", StringLen(s));

      string raw = ExtractAsciiLine(buf, got);
      Print("[BUFFER] '", raw, "' len=", StringLen(raw));

      if(StringLen(raw) > 0)
      {
         HandleCommand(raw);
      }
      else
      {
         Print("[FRAME] empty after ExtractAsciiLine");
      }
   }
   else if(got == 0)
   {
      closesocket(g_clientSock);
      g_clientSock = INVALID_SOCKET;
      Print("Client disconnected");
   }
   else
   {
      int wsa = WSAGetLastError();
      if(wsa != WSAEWOULDBLOCK)
         Print("recv failed wsa=", wsa);
   }
}

int OnInit()
{
   if(!StartServer())
      return(INIT_FAILED);

   EventSetMillisecondTimer(InpTimerMs);
   return(INIT_SUCCEEDED);
}

void OnDeinit(const int reason)
{
   EventKillTimer();
   StopServer();
}

void OnTick()
{
   PumpSocket();
   ManageGoldRunner();
}

void OnTimer()
{
   PumpSocket();
   ManageGoldRunner();
}
