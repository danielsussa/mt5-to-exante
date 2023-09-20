//+------------------------------------------------------------------+
//|                                                         test.mq4 |
//|                      Copyright © 2006, MetaQuotes Software Corp. |
//|                                        https://www.metaquotes.net/ |
//+------------------------------------------------------------------+
#property copyright "Copyright © 2006, MetaQuotes Software Corp."
#property link      "https://www.metaquotes.net/"

//+------------------------------------------------------------------+
//| script program start function                                    |
//+------------------------------------------------------------------+
int start()
  {
//----
int x = 1;
while(Bid > 0 && x < 10)
   {
   Comment(" x = ",x," bid = ", Bid," ask = ",Ask);
   RefreshRates();
   Sleep(1000);
   x = x + 1;
   }
//----
   return(0);
}
//+------------------------------------------------------------------+