//+------------------------------------------------------------------+
//|                                                mt5-to-exante.mq5 |
//|                                  Copyright 2023, MetaQuotes Ltd. |
//|                                             https://www.mql5.com |
//+------------------------------------------------------------------+
#property service
#property copyright "Copyright 2023, MetaQuotes Ltd."
#property link      "https://www.mql5.com"
#property version   "1.00"
#include <jason.mqh>
//+------------------------------------------------------------------+
//| Service program start function                                   |
//+------------------------------------------------------------------+
void OnStart(){
   while (true) {
      CJAVal req;

      AllOrdersRequest(&req);
      AllPositionsRequest(&req);
      AllHistoryOrdersRequest(&req);
      AllHistoryPositionsRequest(&req);

      CJAVal response = callApi("http://127.0.0.1:1323/sync", &req);
      string journal = response["journalF"].ToStr();
      if (StringLen(journal) > 0) {
         Print(journal);
      }

      Sleep(1000);
   }



   //Print(req.Serialize());
   //CJAVal action = callApi("http://127.0.0.1:1323/sync", &req);

}

void createRequest() {
}
//+------------------------------------------------------------------+

void AllOrdersRequest(CJAVal* req) {

   for(int i=0;i<OrdersTotal();i++) {
      ResetLastError();
      //--- copy into the cache, the order by its number in the list
      ulong ticket=OrderGetTicket(i);
      if(ticket!=0){

         CJAVal orderReq;
         orderReq["state"]=convertState(OrderGetInteger(ORDER_STATE));
         orderReq["ticket"]= IntegerToString(OrderGetInteger(ORDER_TICKET));
         orderReq["symbol"]=OrderGetString(ORDER_SYMBOL);
         orderReq["volume"]=OrderGetDouble(ORDER_VOLUME_INITIAL);
         orderReq["type"]=convertType(OrderGetInteger(ORDER_TYPE));
         orderReq["price"]=OrderGetDouble(ORDER_PRICE_OPEN);
         orderReq["stopLoss"]=OrderGetDouble(ORDER_SL);
         orderReq["takeProfit"]=OrderGetDouble(ORDER_TP);
         orderReq["updatedAt"]=formatDatetime(OrderGetInteger(ORDER_TIME_SETUP));

         req["activeOrders"].Add(orderReq);

      }
   }
}

void AllPositionsRequest(CJAVal* req) {
   for(int i=0;i<PositionsTotal();i++) {
      ResetLastError();
      //--- copy into the cache, the order by its number in the list
      ulong ticket=PositionGetTicket(i);
      if(ticket!=0){

         CJAVal positionReq;
         positionReq["state"]=EnumToString(ORDER_STATE_FILLED);
         positionReq["symbol"]=PositionGetString(POSITION_SYMBOL);
         positionReq["ticket"]=IntegerToString(PositionGetInteger(POSITION_TICKET));
         positionReq["positionTicket"]=IntegerToString(PositionGetInteger(POSITION_IDENTIFIER));
         positionReq["type"]=convertType(PositionGetInteger(POSITION_TYPE));
         positionReq["updatedAt"]=formatDatetime(PositionGetInteger(POSITION_TIME_UPDATE));
         positionReq["volume"]=PositionGetDouble(POSITION_VOLUME);
         positionReq["price"]=PositionGetDouble(POSITION_PRICE_CURRENT);
         positionReq["stopLoss"]=PositionGetDouble(POSITION_SL);
         positionReq["takeProfit"]=PositionGetDouble(POSITION_TP);

         req["activePositions"].Add(positionReq);

      }
   }
}

void AllHistoryOrdersRequest(CJAVal* req) {

   HistorySelect(TimeCurrent()-60, TimeCurrent());
   ulong ticket=0;
   for(int i=0;i<HistoryOrdersTotal();i++) {
      ResetLastError();

      if((ticket=HistoryOrderGetTicket(i))>0){

         CJAVal orderReq;


         orderReq["state"]=convertState(HistoryOrderGetInteger(ticket,ORDER_STATE));
         orderReq["ticket"]= IntegerToString(HistoryOrderGetInteger(ticket,ORDER_TICKET));
         orderReq["symbol"]=HistoryOrderGetString(ticket,ORDER_SYMBOL);
         orderReq["volume"]=HistoryOrderGetDouble(ticket,ORDER_VOLUME_INITIAL);
         orderReq["type"]=convertType(HistoryOrderGetInteger(ticket,ORDER_TYPE));
         orderReq["price"]=HistoryOrderGetDouble(ticket,ORDER_PRICE_OPEN);
         orderReq["stopLoss"]=HistoryOrderGetDouble(ticket,ORDER_SL);
         orderReq["takeProfit"]=HistoryOrderGetDouble(ticket,ORDER_TP);
         orderReq["updatedAt"]=formatDatetime(HistoryOrderGetInteger(ticket,ORDER_TIME_SETUP));


         req["recentInactiveOrders"].Add(orderReq);



         //CJAVal response = callApi("http://127.0.0.1:1323/OrderChange", &jsonReq);
        }
     }
}

void AllHistoryPositionsRequest(CJAVal* req) {

   HistorySelect(TimeCurrent()-600, TimeCurrent());
   ulong ticket=0;
   for(int i=0;i<HistoryDealsTotal();i++) {
      ResetLastError();

      if((ticket=HistoryDealGetTicket(i))>0){

         CJAVal jsonReq;

         jsonReq["positionTicket"] = IntegerToString(HistoryDealGetInteger(ticket,DEAL_POSITION_ID));
         jsonReq["ticket"] = IntegerToString(HistoryDealGetInteger(ticket,DEAL_ORDER));
         jsonReq["entry"] = convertDealEntry(HistoryDealGetInteger(ticket,DEAL_ENTRY));
         jsonReq["reason"] = convertDealReason(HistoryDealGetInteger(ticket,DEAL_REASON));
         jsonReq["state"] = convertState(HistoryOrderGetInteger(ticket, ORDER_STATE));

         jsonReq["symbol"]=HistoryDealGetString(ticket, DEAL_SYMBOL);
         jsonReq["type"]=convertType(HistoryDealGetInteger(ticket, DEAL_TYPE));
         jsonReq["volume"]=HistoryDealGetDouble(ticket, DEAL_VOLUME);
         jsonReq["price"]=HistoryDealGetDouble(ticket, DEAL_PRICE);
         jsonReq["stopLoss"]=HistoryDealGetDouble(ticket, DEAL_SL);
         jsonReq["takeProfit"]=HistoryDealGetDouble(ticket, DEAL_TP);

         req["recentInactivePositions"].Add(jsonReq);



         //CJAVal response = callApi("http://127.0.0.1:1323/OrderChange", &jsonReq);
       }
     }
}

string formatDatetime(datetime val) {
// 2006-01-02T15:04:05-0700
   MqlDateTime date;
   TimeToStruct(val,date);


   return StringFormat("%d-%02d-%02dT%02d:%02d:%02dZ",
      date.year, date.mon, date.day, date.hour, date.min, date.sec
   );
}

string convertState(int state) {

   switch (state) {
      case ORDER_STATE_PLACED:
         return EnumToString(ORDER_STATE_PLACED);
      case ORDER_STATE_CANCELED:
         return EnumToString(ORDER_STATE_CANCELED);
      case ORDER_STATE_FILLED:
         return EnumToString(ORDER_STATE_FILLED);
      case ORDER_STATE_STARTED:
         return EnumToString(ORDER_STATE_STARTED);
      case ORDER_STATE_REQUEST_CANCEL:
         return EnumToString(ORDER_STATE_REQUEST_CANCEL);
   }
   return "";
}

string convertDealEntry(int entry) {

   switch (entry) {
      case DEAL_ENTRY_IN:
         return EnumToString(DEAL_ENTRY_IN);
      case DEAL_ENTRY_OUT:
         return EnumToString(DEAL_ENTRY_OUT);
      case DEAL_ENTRY_INOUT:
         return EnumToString(DEAL_ENTRY_INOUT);
      case DEAL_ENTRY_OUT_BY:
         return EnumToString(DEAL_ENTRY_OUT_BY);
   }
   return "";
}

string convertDealReason(int reason) {

   switch (reason) {
      case DEAL_REASON_CLIENT:
         return EnumToString(DEAL_REASON_CLIENT);
      case DEAL_REASON_SL:
         return EnumToString(DEAL_REASON_SL);
      case DEAL_REASON_TP:
         return EnumToString(DEAL_REASON_TP);
      case DEAL_REASON_VMARGIN:
         return EnumToString(DEAL_REASON_VMARGIN);
   }
   return "";
}

string convertType(int type) {

   switch (type) {
      case ORDER_TYPE_BUY:
         return EnumToString(ORDER_TYPE_BUY);
      case ORDER_TYPE_BUY_LIMIT:
         return EnumToString(ORDER_TYPE_BUY_LIMIT);
      case ORDER_TYPE_BUY_STOP:
         return EnumToString(ORDER_TYPE_BUY_STOP);
      case ORDER_TYPE_SELL:
         return EnumToString(ORDER_TYPE_SELL);
      case ORDER_TYPE_SELL_LIMIT:
         return EnumToString(ORDER_TYPE_SELL_LIMIT);
   }
   return "";
}

CJAVal callApi(string url, CJAVal* request) {
   char data[];
   ArrayResize(data, StringToCharArray(request.Serialize(), data, 0, WHOLE_ARRAY)-1);

   char res_data[];
   string res_headers=NULL;

   int r=WebRequest("POST", url, "Content-Type: application/json\r\n", 5000, data, res_data, res_headers);
   if (r > 399) {
      Print("SDK offline");
      Sleep(5000);
   }

   CJAVal response;
   string res_string=CharArrayToString(res_data,0,ArraySize(res_data),CP_UTF8);
   response.Deserialize(res_string,CP_UTF8);

   return response;
}