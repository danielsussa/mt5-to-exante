//+------------------------------------------------------------------+
//|                                                         tick.mq5 |
//|                                  Copyright 2023, MetaQuotes Ltd. |
//|                                             https://www.mql5.com |
//+------------------------------------------------------------------+
#property copyright "Copyright 2023, MetaQuotes Ltd."
#property link      "https://www.mql5.com"
#property version   "1.00"
#include <jason.mqh>


//--- input parameters
input string         sdkUrl="http://127.0.0.1:1323";
input string         baseURL="https://api-demo.exante.eu";
input string         applicationID="218cd7b4-2da4-4f4c-9f3c-9f47b0cdfc41";
input string         clientID="31c78477-c140-4014-be90-e6b24a52f199";
input string         sharedKey="Xw4B87A8NF0F02H9LZhGtrl5zL0Q6g5W";

//+------------------------------------------------------------------+
//| Expert initialization function                                   |
//+------------------------------------------------------------------+
int OnInit() {
   string cookie=NULL;
   string headers = "Accept: application/json";
   char   post[],result[];
   string url= StringFormat("%s/health", sdkUrl);
   int res=WebRequest("GET",url,cookie,NULL,2000,post,0,result,headers);
   if(res==-1){
      MessageBox("cannot perform healthcheck on url: " + url,"Error",MB_ICONERROR);
      return(-1);
   }
   if(res!=200){
      MessageBox("no response from: " + url,"Error",MB_ICONERROR);
      return(-1);
   }

   return(INIT_SUCCEEDED);
}

//+------------------------------------------------------------------+
//| Expert deinitialization function                                 |
//+------------------------------------------------------------------+
void OnDeinit(const int reason)
  {
//---

  }

void OnTrade() {
//---
  // Print("OnTrade");

}

void OnTradeTransaction(const MqlTradeTransaction &trans,const MqlTradeRequest &request,const MqlTradeResult &result){
   ulong            lastOrderID   =trans.order;
   double            priceSl   =trans.price_sl;
   double            priceTp   =trans.price_tp;
   double            volume   =trans.volume;
   double            price   =trans.price;
   ENUM_ORDER_TYPE  lastOrderType =trans.order_type;
   ENUM_ORDER_STATE lastOrderState=trans.order_state;
   string trans_symbol=trans.symbol;



   if (lastOrderState != ORDER_STATE_PLACED && lastOrderState != ORDER_STATE_CANCELED) {
      return;
   }


   CJAVal jv;
   jv["baseURL"]=baseURL;
   jv["applicationID"]=applicationID;
   jv["clientID"]=clientID;
   jv["sharedKey"]=sharedKey;
   jv["symbolID"]=trans_symbol;
   jv["instrument"]=trans_symbol;
   jv["limitPrice"]=price;
   jv["quantity"]=volume;
   jv["duration"]="day";
   jv["accountId"]="OZK2252.001";

   switch(lastOrderType)
      {
       case ORDER_TYPE_BUY:
        {
          jv["side"]="buy";
          jv["orderType"]="market";
        }
         break;
       case ORDER_TYPE_SELL:
        {
          jv["side"]="sell";
          jv["orderType"]="market";
        }
         break;
       case ORDER_TYPE_BUY_LIMIT:
        {
          jv["side"]="buy";
          jv["orderType"]="limit";
        }
         break;
       case ORDER_TYPE_SELL_LIMIT:
        {
          jv["side"]="sell";
          jv["orderType"]="limit";
        }
         break;
       case ORDER_TYPE_BUY_STOP:
        {
          jv["side"]="buy";
          jv["orderType"]="stop";
          jv["stopPrice"]="???";
        }
         break;
       case ORDER_TYPE_SELL_STOP:
        {
          jv["side"]="sell";
          jv["orderType"]="stop";
          jv["stopPrice"]="???";
        }
         break;
       case ORDER_TYPE_BUY_STOP_LIMIT:
        {
          jv["side"]="buy";
          jv["orderType"]="stop_limit";
          jv["stopPrice"]="???";
        }
         break;
       case ORDER_TYPE_SELL_STOP_LIMIT:
        {
          jv["side"]="sell";
          jv["orderType"]="stop_limit";
          jv["stopPrice"]="???";
        }
         break;
      }

   char data[];
   ArrayResize(data, StringToCharArray(jv.Serialize(), data, 0, WHOLE_ARRAY)-1);

   char res_data[];
   string res_headers=NULL;

   Print("dadad "+trans.type);

   switch(trans.type)
     {
      case TRADE_TRANSACTION_ORDER_UPDATE:
        {
           string url= StringFormat("%s/v3/orders/%d/place", sdkUrl, lastOrderID);
           int r=WebRequest("POST", url, "Content-Type: application/json\r\n", 5000, data, res_data, res_headers);
           PrintFormat("TRADE_TRANSACTION_ORDER_UPDATE: #%d %s modified: SL=%.5f TP=%.5f",lastOrderID,trans_symbol,trans.price_sl,trans.price_tp);

        }
      break;

      case TRADE_TRANSACTION_ORDER_DELETE:
        {
         string url= StringFormat("%s/v3/orders/%d/cancel", sdkUrl, lastOrderID);
         int r=WebRequest("POST", url, "Content-Type: application/json\r\n", 5000, data, res_data, res_headers);
         PrintFormat("TRADE_TRANSACTION_ORDER_DELETE: #%d %s modified: SL=%.5f TP=%.5f",lastOrderID,trans_symbol,trans.price_sl,trans.price_tp);
        }
      break;

     }

     Print("kind: ", trans.type);
}