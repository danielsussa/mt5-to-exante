//+------------------------------------------------------------------+
//|                                                         tick.mq5 |
//|                                  Copyright 2023, MetaQuotes Ltd. |
//|                                             https://www.mql5.com |
//+------------------------------------------------------------------+
#property copyright "Copyright 2023, MetaQuotes Ltd."
#property version   "1.00"
#include <jason.mqh>


//--- input parameters
input string         sdkUrl="http://127.0.0.1:1323";
input string         accoundID="QJO2251.001";

//+------------------------------------------------------------------+
//| Expert initialization function                                   |
//+------------------------------------------------------------------+


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
   double            priceSl   =trans.price_sl;
   double            priceTp   =trans.price_tp;
   double            volume   =request.volume;
   double            price   =request.price;
   ENUM_ORDER_TYPE  lastOrderType =trans.order_type;
   ENUM_ORDER_STATE lastOrderState=trans.order_state;
   string trans_symbol=request.symbol;


   PrintFormat("[t_order=%d/res_order=%d/position=%d] ot: %s t: %s d: %s rt: %s",trans.order,result.order,request.position,EnumToString(lastOrderType),EnumToString(trans.type),EnumToString(lastOrderState),EnumToString(request.action));



   CJAVal jv;
   jv["symbolID"]=trans_symbol;
   jv["instrument"]=trans_symbol;
   jv["limitPrice"]=price;
   jv["quantity"]=volume;
   jv["duration"]="good_till_cancel";
   jv["accountId"]=accoundID;

   switch(lastOrderType)
      {
       case ORDER_TYPE_BUY:
        {
          jv["side"]="buy";
        }
         break;
       case ORDER_TYPE_SELL:
        {
          jv["side"]="sell";
        }
         break;
       case ORDER_TYPE_BUY_LIMIT:
        {
          jv["side"]="buy";
        }
         break;
       case ORDER_TYPE_SELL_LIMIT:
        {
          jv["side"]="sell";
        }
         break;
       case ORDER_TYPE_BUY_STOP:
        {
          jv["side"]="buy";

        }
         break;
       case ORDER_TYPE_SELL_STOP:
        {
          jv["side"]="sell";
        }
         break;
       case ORDER_TYPE_BUY_STOP_LIMIT:
        {
          jv["side"]="buy";
        }
         break;
       case ORDER_TYPE_SELL_STOP_LIMIT:
        {
          jv["side"]="sell";
        }
         break;
      }


   bool isLimitOrder =
      request.action == TRADE_ACTION_PENDING &&
      trans.position == NULL;

   bool changeLimitOrder =
      request.action == TRADE_ACTION_MODIFY &&
      trans.position == NULL;

   bool isMarketOrder =
      request.action == TRADE_ACTION_DEAL &&
      request.position == NULL;

   bool isClosePosition =
      request.action == TRADE_ACTION_DEAL &&
      request.position != NULL;

   bool isCancelOrder =
      request.action == TRADE_ACTION_REMOVE;

   bool isChangeTpSl =
      request.action == TRADE_ACTION_SLTP &&
      request.position != NULL;



   if (isLimitOrder) {
      jv["stopPrice"]=request.sl;
      jv["takeProfit"]=request.tp;
      jv["orderType"]="limit";
      PrintFormat("LIMIT ORDER: #%d %s modified: SL=%.5f TP=%.5f",result.order,trans_symbol,request.sl,request.tp);

      string url= StringFormat("%s/v3/orders/%d/place", sdkUrl, result.order);
      performOrder(url, &jv);

   }
   if (isMarketOrder) {
      jv["stopPrice"]=request.sl;
      jv["takeProfit"]=request.tp;
      jv["orderType"]="market";
      PrintFormat("MARKET_ORDER: #%d %s modified: SL=%.5f TP=%.5f",result.order,trans_symbol,request.sl,request.tp);

      string url= StringFormat("%s/v3/orders/%d/place", sdkUrl, result.order);
      performOrder(url, &jv);
   }
   if (changeLimitOrder) {
      jv["stopPrice"]=request.sl;
      jv["takeProfit"]=request.tp;
      jv["orderType"]="limit";
      PrintFormat("CHANGE_LIMIT: #%d %s modified: SL=%.5f TP=%.5f",result.order,trans_symbol,request.sl,request.tp);

      string url= StringFormat("%s/v3/orders/%d/place", sdkUrl, result.order);
      performOrder(url, &jv);
   }
   if (isClosePosition) {
      PrintFormat("CLOSE_POSITION: #%d %s",request.position,trans_symbol);

      string url= StringFormat("%s/v3/positions/%d/close", sdkUrl, request.position);
      performOrder(url, &jv);
   }
   if (isCancelOrder) {
      PrintFormat("CANCEL ORDER: #%d %s modified: SL=%.5f TP=%.5f",result.order,trans_symbol,trans.price_sl,trans.price_tp);

      string url= StringFormat("%s/v3/orders/%d/cancel", sdkUrl, result.order);
      performOrder(url, &jv);
   }
   if (isChangeTpSl) {
      jv["stopLoss"]=request.sl;
      jv["takeProfit"]=request.tp;
      PrintFormat("CHANGE_TPLS: #%d %s modified: SL=%.5f TP=%.5f",request.position,trans_symbol,request.sl,request.tp);

      string url= StringFormat("%s/v3/positions/%d/tpls", sdkUrl, request.position);
      performOrder(url, &jv);
   }

}

void performOrder(string url, CJAVal* jv) {
   char data[];
   ArrayResize(data, StringToCharArray(jv.Serialize(), data, 0, WHOLE_ARRAY)-1);

   char res_data[];
   string res_headers=NULL;

   int r=WebRequest("POST", url, "Content-Type: application/json\r\n", 5000, data, res_data, res_headers);
}
