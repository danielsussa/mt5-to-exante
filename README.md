# mt5-to-exante

This project enable integration between MetaTrader HomeBroker and Exante's APIs.

Exante API documentation:
https://exante.eu/pt/all-apis/http-api/

The project goal is to provide full integration and allow the use of MetaTrader HomeBroker with Exante's services.

## Installation

### Configure and RUN SDK:
1. Go to the latest tag [link](https://github.com/danielsussa/mt5-to-exante/tags)
2. download `mt-to-exante-sdk.exe`
3. download `production.env` and `developer.env`
4. replace `*.env` values with your own API KEYS
5. run app with command line argument `mt-to-exante-sdk.exe production`

### Configure MT5 scrips:
1. download `JAson.mqh` and `mt-to-exante.mq4`
2. copy `JAson.mqh` to folder `...\MQL5\Include\JAson.mqh`
3. copy `mt-to-exante.mq4` to folder `...\MQL5\Experts\mt-to-exante.mq4`
4. allow MT5 to call external services:
![image](src/mt5-url.PNG)
5. Add Expert to target chart