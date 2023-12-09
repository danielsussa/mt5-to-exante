package exante

import (
	"errors"
	"fmt"
	"github.com/dgrijalva/jwt-go"
	"github.com/go-resty/resty/v2"
	"github.com/peterbourgon/diskv/v3"
	"net/http"
	"net/http/httputil"
	"time"
)

// Standard jwt-go claims does not support multiple audience
type claimsWithMultiAudSupport struct {
	Aud []string `json:"aud"`
	jwt.StandardClaims
}

type ErrorResponse struct {
	Message string
}

func (e ErrorResponse) Error() string {
	return fmt.Sprintf("error: %s", e.Message)
}

type ReplaceOrderResponse struct {
	OrderId string
}

type Api struct {
	BaseURL       string `json:"baseURL"`
	ApplicationID string `json:"applicationID"`
	ClientID      string `json:"clientID"`
	SharedKey     string `json:"sharedKey"`
	cli           *resty.Client
	d             *diskv.Diskv
	jwt           string
}

func NewApi(baseUrl, appID, cliID, sharedKey string) *Api {
	client := resty.New()

	// Retries are configured per client
	client.
		// Set retry count to non zero to enable retries
		SetRetryCount(5).
		// You can override initial retry wait time.
		// Default is 100 milliseconds.
		SetRetryWaitTime(5 * time.Second).
		// MaxWaitTime can be overridden as well.
		// Default is 2 seconds.
		SetRetryMaxWaitTime(20 * time.Second).
		// SetRetryAfter sets callback to calculate wait time between retries.
		// Default (nil) implies exponential backoff with jitter
		SetRetryAfter(func(client *resty.Client, resp *resty.Response) (time.Duration, error) {
			return 0, errors.New("quota exceeded")
		})

	d := diskv.New(diskv.Options{
		BasePath:     fmt.Sprintf("orders-%s", appID),
		Transform:    func(s string) []string { return []string{} },
		CacheSizeMax: 1024 * 1024,
	})

	return &Api{
		BaseURL:       baseUrl,
		ApplicationID: appID,
		ClientID:      cliID,
		SharedKey:     sharedKey,
		cli:           client,
		d:             d,
	}
}

var Scopes = []string{
	"crossrates", "change", "crossrates", "summary",
	"symbols", "feed", "ohlc", "orders", "transactions",
	"accounts",
}

func (a Api) Jwt() string {
	token, err := jwt.Parse(a.jwt, func(token *jwt.Token) (interface{}, error) {
		return []byte(a.SharedKey), nil
	})
	if err == nil && token.Valid {
		return a.jwt
	}

	now := time.Now()
	jwtExpiresAt := now.Add(time.Minute * 10).Unix()
	jwtIssueAt := now.Unix()

	claims := claimsWithMultiAudSupport{
		Scopes,
		jwt.StandardClaims{
			Issuer:    a.ClientID,
			Subject:   a.ApplicationID,
			IssuedAt:  jwtIssueAt,
			ExpiresAt: jwtExpiresAt,
		},
	}

	token = jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, _ := token.SignedString([]byte(a.SharedKey))
	a.jwt = tokenString
	return tokenString
}

func (a Api) Bearer() string {
	return fmt.Sprintf("Bearer %s", a.Jwt())
}

// ReplaceOrderPayload method optional payload
type ReplaceOrderPayload struct {
	Action     string                 `json:"action"`
	Parameters ReplaceOrderParameters `json:"parameters,omitempty"`
}

type ReplaceOrderParameters struct {
	Quantity      string `json:"quantity,omitempty"`
	LimitPrice    string `json:"limitPrice,omitempty"`
	StopPrice     string `json:"stopPrice,omitempty"`
	PriceDistance string `json:"priceDistance,omitempty"`
}

type CancelOrderPayload struct {
	Action string `json:"action"`
}

// CancelOrder cancel trading order
func (a Api) CancelOrder(orderID string) error {
	var errRes []ErrorResponse

	url := fmt.Sprintf("%s/trade/3.0/orders/%s", a.BaseURL, orderID)
	resp, err := a.cli.R().
		SetError(&errRes).
		SetBody(CancelOrderPayload{
			Action: "cancel",
		}).
		SetHeader("Authorization", a.Bearer()).
		Post(url)
	if err != nil {
		return err
	}

	res, _ := httputil.DumpRequest(resp.Request.RawRequest, true)
	fmt.Print(string(res))

	if resp.StatusCode() >= http.StatusInternalServerError {
		return fmt.Errorf("internal server error")
	}

	if resp.IsError() {
		if len(errRes) == 0 {
			return fmt.Errorf("error: %s", string(resp.Body()))
		}
		return errRes[0]
	}

	return nil
}

type OrderSentTypeV3 struct {
	AccountID      string  `json:"accountId"`
	Instrument     string  `json:"instrument"`
	Side           string  `json:"side"`
	Quantity       string  `json:"quantity"`
	Duration       string  `json:"duration"`
	ClientTag      string  `json:"clientTag,omitempty"`
	OcoGroup       string  `json:"ocoGroup,omitempty"`
	LimitPrice     string  `json:"limitPrice,omitempty"`
	IfDoneParentID string  `json:"ifDoneParentId,omitempty"`
	OrderType      string  `json:"orderType"`
	TakeProfit     *string `json:"takeProfit,omitempty"`
	StopLoss       *string `json:"stopLoss,omitempty"`
	SymbolID       string  `json:"symbolId"`
}

func (a Api) PlaceOrderV3(req *OrderSentTypeV3) ([]OrderV3, error) {

	var result []OrderV3
	var errRes []ErrorResponse

	resp, err := a.cli.R().
		SetResult(&result).
		SetError(&errRes).
		SetBody(req).
		SetHeader("Authorization", a.Bearer()).
		Post(fmt.Sprintf("%s/trade/3.0/orders", a.BaseURL))

	if err != nil {
		return nil, err
	}

	if resp.StatusCode() >= http.StatusInternalServerError {
		return nil, fmt.Errorf("internal server error")
	}

	if resp.IsError() {
		if len(errRes) == 0 {
			return nil, fmt.Errorf("error: %s", string(resp.Body()))
		}
		return nil, errRes[0]
	}

	return result, nil
}

func (a Api) GetActiveOrdersByID(orderID string) ([]OrderV3, error) {
	orders, err := a.GetActiveOrdersV3()
	if err != nil {
		return nil, err
	}

	orders, _ = getActiveOrdersByID(orders, orderID)
	return orders, nil
}

func (a Api) GetOrdersByID(orderID string) ([]OrderV3, error) {
	orders, err := a.GetOrdersV3()
	if err != nil {
		return nil, err
	}

	orders, _ = getOrdersByID(orders, orderID)
	return orders, nil
}

func (a Api) GetFilledOrderByID(orderID string) (OrderV3, bool, error) {
	orders, err := a.GetOrdersV3()
	if err != nil {
		return OrderV3{}, false, err
	}

	order, hasOrder := getActiveOrderByID(orders, orderID)
	if order.OrderState.Status == "filled" {
		return order, hasOrder, nil
	}
	return order, false, nil
}

func (a Api) GetActiveOrderByID(orderID string) (OrderV3, bool, error) {
	orders, err := a.GetActiveOrdersV3()
	if err != nil {
		return OrderV3{}, false, err
	}

	order, hasOrder := getActiveOrderByID(orders, orderID)
	return order, hasOrder, nil
}

func getActiveOrderByID(orders []OrderV3, orderID string) (OrderV3, bool) {
	for _, order := range orders {
		if order.ClientTag == orderID {
			return order, true
		}
	}

	return OrderV3{}, false
}

func getActiveOrdersByID(orders []OrderV3, orderID string) ([]OrderV3, bool) {
	newOrdersList := make([]OrderV3, 0)
	hasActiveOrders := false
	for _, order := range orders {
		if order.ClientTag == orderID {
			newOrdersList = append(newOrdersList, order)
			hasActiveOrders = true
		}
	}

	return newOrdersList, hasActiveOrders
}

func getOrdersByID(orders []OrderV3, orderID string) ([]OrderV3, bool) {
	newOrdersList := make([]OrderV3, 0)
	hasActiveOrders := false
	for _, order := range orders {
		if order.ClientTag == orderID {
			newOrdersList = append(newOrdersList, order)
			hasActiveOrders = true
		}
	}

	return newOrdersList, hasActiveOrders
}

// OrderV3 model
type OrderV3 struct {
	OrderState      OrderState      `json:"orderState"`
	OrderParameters OrderParameters `json:"orderParameters"`

	OrderID               string `json:"orderId"`
	PlaceTime             string `json:"placeTime"`
	AccountID             string `json:"accountId"`
	ClientTag             string `json:"clientTag"`
	CurrentModificationID string `json:"currentModificationId"`
	ExanteAccount         string `json:"exanteAccount"`
	Username              string `json:"username"`
}

type OrderParameters struct {
	Side           string `json:"side"`
	Duration       string `json:"duration"`
	Quantity       string `json:"quantity"`
	Instrument     string `json:"instrument"`
	SymbolId       string `json:"symbolId"`
	OrderType      string `json:"orderType"`
	OcoGroup       string `json:"ocoGroup"`
	IfDoneParentID string `json:"ifDoneParentId"`
	LimitPrice     string `json:"limitPrice"`
	StopPrice      string `json:"stopPrice"`
	PriceDistance  string `json:"priceDistance"`
	PartQuantity   string `json:"partQuantity"`
	PlaceInterval  string `json:"placeInterval"`
}

type OrderState struct {
	Fills []OrderFill `json:"fills"`

	Status     string `json:"status"`
	LastUpdate string `json:"lastUpdate"`
}

type OrderFill struct {
	Quantity string `json:"quantity"`
	Price    string `json:"price"`
	Time     string `json:"timestamp"`
	Position int    `json:"position"`
}

// OrdersV3 model
type OrdersV3 []OrderV3

// GetActiveOrdersV3 return the list of active trading orders
func (a Api) GetActiveOrdersV3() (OrdersV3, error) {

	var result OrdersV3
	var errRes []ErrorResponse

	resp, err := a.cli.R().
		SetResult(&result).
		SetError(&errRes).
		SetHeader("Authorization", a.Bearer()).
		Get(fmt.Sprintf("%s/trade/3.0/orders/active", a.BaseURL))

	if err != nil {
		return nil, err
	}

	if resp.StatusCode() >= http.StatusInternalServerError {
		return nil, fmt.Errorf("internal server error")
	}

	if resp.IsError() {
		if len(errRes) == 0 {
			return nil, fmt.Errorf("error: %s", string(resp.Body()))
		}
		return nil, errRes[0]
	}

	return result, nil
}

func (a Api) GetOrdersV3() (OrdersV3, error) {

	var result OrdersV3
	var errRes []ErrorResponse

	resp, err := a.cli.R().
		SetResult(&result).
		SetError(&errRes).
		SetHeader("Authorization", a.Bearer()).
		Get(fmt.Sprintf("%s/trade/3.0/orders", a.BaseURL))

	if err != nil {
		return nil, err
	}

	if resp.StatusCode() >= http.StatusInternalServerError {
		return nil, fmt.Errorf("internal server error")
	}

	if resp.IsError() {
		if len(errRes) == 0 {
			return nil, fmt.Errorf("error: %s", string(resp.Body()))
		}
		return nil, errRes[0]
	}

	return result, nil
}

func (a Api) ReplaceOrder(orderID string, req ReplaceOrderPayload) (*ReplaceOrderResponse, error) {

	var result *ReplaceOrderResponse
	var errRes []ErrorResponse

	resp, err := a.cli.R().
		SetResult(&result).
		SetError(&errRes).
		SetBody(req).
		SetHeader("Authorization", a.Bearer()).
		Post(fmt.Sprintf("%s/trade/3.0/orders/%s", a.BaseURL, orderID))

	if err != nil {
		return nil, err
	}

	if resp.StatusCode() >= http.StatusInternalServerError {
		return nil, fmt.Errorf("internal server error")
	}

	if resp.IsError() {
		if len(errRes) == 0 {
			return nil, fmt.Errorf("error: %s", string(resp.Body()))
		}
		return nil, errRes[0]
	}

	return result, nil
}

type UserAccount struct {
	Status    string `json:"status"`
	AccountID string `json:"accountId"`
}

type UserAccounts []UserAccount

func (a Api) GetUserAccounts() (*UserAccounts, error) {
	var result *UserAccounts
	var errRes []ErrorResponse

	resp, err := a.cli.R().
		SetResult(&result).
		SetError(&errRes).
		SetHeader("Authorization", a.Bearer()).
		Get(fmt.Sprintf("%s/md/3.0/accounts", a.BaseURL))

	if err != nil {
		return nil, err
	}

	if resp.StatusCode() >= http.StatusInternalServerError {
		return nil, fmt.Errorf("internal server error")
	}

	if resp.IsError() {
		if len(errRes) == 0 {
			return nil, fmt.Errorf("error: %s", string(resp.Body()))
		}
		return nil, errRes[0]
	}

	return result, nil
}
