package httplib

import (
	"fmt"
	"github.com/dgrijalva/jwt-go"
	"github.com/go-resty/resty/v2"
	"net/http"
	"time"
)

type Api struct {
	BaseURL       string `json:"baseURL"`
	ApplicationID string `json:"applicationID"`
	ClientID      string `json:"clientID"`
	SharedKey     string `json:"sharedKey"`
}

func (a Api) Jwt() string {
	now := time.Now()
	jwtExpiresAt := now.Add(time.Second * 60).Unix()
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

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, _ := token.SignedString([]byte(a.SharedKey))
	return fmt.Sprintf("Bearer %s", tokenString)
}

// ReplaceOrderPayload method optional payload
type ReplaceOrderPayload struct {
	Action     string                 `json:"action"`
	Parameters ReplaceOrderParameters `json:"parameters,omitempty"`
}

// CancelOrder cancel trading order
func (a Api) CancelOrder(orderID string) error {
	var errRes []ErrorResponse

	resp, err := resty.New().R().
		SetError(&errRes).
		SetBody(ReplaceOrderPayload{
			Action: "cancel",
		}).
		SetHeader("Authorization", a.Jwt()).
		Get(fmt.Sprintf("%s/trade/3.0/orders/%s", a.BaseURL, orderID))

	if err != nil {
		return err
	}

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

func (a Api) PlaceOrderV3(req *OrderSentTypeV3) ([]OrderSentTypeV3Response, error) {

	var result []OrderSentTypeV3Response
	var errRes []ErrorResponse

	resp, err := resty.New().R().
		SetResult(&result).
		SetError(&errRes).
		SetBody(req).
		SetHeader("Authorization", a.Jwt()).
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

func (a Api) GetActiveOrder(orderID string) (OrderV3, bool, error) {
	orders, err := a.GetActiveOrdersV3()
	if err != nil {
		return OrderV3{}, false, err
	}

	order, hasOrder := getActiveOrder(orders, orderID)
	return order, hasOrder, nil
}

func getActiveOrder(orders []OrderV3, orderID string) (OrderV3, bool) {
	for _, order := range orders {
		if order.ClientTag == orderID {
			return order, true
		}
	}

	return OrderV3{}, false
}

// GetActiveOrdersV3 return the list of active trading orders
func (a Api) GetActiveOrdersV3() (OrdersV3, error) {

	var result OrdersV3
	var errRes []ErrorResponse

	resp, err := resty.New().R().
		SetResult(&result).
		SetError(&errRes).
		SetHeader("Authorization", a.Jwt()).
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

func (a Api) ReplaceOrder(orderID string, req ReplaceOrderPayload) (*ReplaceOrderResponse, error) {

	var result *ReplaceOrderResponse
	var errRes []ErrorResponse

	resp, err := resty.New().R().
		SetResult(&result).
		SetError(&errRes).
		SetBody(req).
		SetHeader("Authorization", a.Jwt()).
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
