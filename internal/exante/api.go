package httplib

import (
	"errors"
	"fmt"
	"github.com/dgrijalva/jwt-go"
	"github.com/go-resty/resty/v2"
	"net/http"
	"net/http/httputil"
	"time"
)

type Api struct {
	BaseURL       string `json:"baseURL"`
	ApplicationID string `json:"applicationID"`
	ClientID      string `json:"clientID"`
	SharedKey     string `json:"sharedKey"`
	cli           *resty.Client
}

func NewApi(baseUrl, appID, cliID, sharedKey string) Api {
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

	return Api{
		BaseURL:       baseUrl,
		ApplicationID: appID,
		ClientID:      cliID,
		SharedKey:     sharedKey,
		cli:           client,
	}
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
		SetHeader("Authorization", a.Jwt()).
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

func (a Api) PlaceOrderV3(req *OrderSentTypeV3) ([]OrderSentTypeV3Response, error) {

	var result []OrderSentTypeV3Response
	var errRes []ErrorResponse

	resp, err := a.cli.R().
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

	resp, err := a.cli.R().
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

	resp, err := a.cli.R().
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
