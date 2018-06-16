package forge

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

const URL = "https://forge.laravel.com/api/v1"

var (
	ErrBadRequest      = errors.New("Valid data was given but the request has failed.")
	ErrInvalidAPIKey   = errors.New("No valid API Key was given.")
	ErrNotFound        = errors.New("The request resource could not be found.")
	ErrInvalidData     = errors.New("The payload has missing required parameters or invalid data was given.")
	ErrTooManyAttempts = errors.New("Too many attempts.")
	ErrInternal        = errors.New("Request failed due to an internal error in Forge.")
	ErrMaintenance     = errors.New("Forge is offline for maintenance.")
)

type Time struct {
	time.Time
}

func (t *Time) UnmarshalJSON(b []byte) error {
	parsed, err := time.Parse(`"2006-01-02 15:04:05"`, string(b))
	if err != nil {
		return err
	}
	t.Time = parsed
	return nil
}

type Client struct {
	apiKey string
	hc     *http.Client
}

func New(apiKey string) *Client {
	return &Client{
		apiKey: apiKey,
		hc:     &http.Client{},
	}
}

func (c *Client) Servers() *Servers {
	return &Servers{c: c}
}

func (c *Client) Sites(serverId int) *Sites {
	return &Sites{c: c, serverId: serverId}
}

func (c *Client) Keys(serverId int) *Keys {
	return &Keys{c: c, serverId: serverId}
}

func (c *Client) Env(serverId, siteId int) *Env {
	return &Env{
		c:        c,
		serverId: serverId,
		siteId:   siteId,
	}
}

func (c *Client) Do(ctx context.Context, req *Request, result interface{}) error {
	var body io.Reader
	if req.Body != nil {
		b, err := json.Marshal(req.Body)
		if err != nil {
			return err
		}
		body = bytes.NewReader(b)
	}
	httpReq, err := http.NewRequest(req.Method, URL+req.Path, body)
	if err != nil {
		return err
	}
	httpReq = httpReq.WithContext(ctx)
	httpReq.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.apiKey))
	httpReq.Header.Set("Accept", "application/json")
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := c.hc.Do(httpReq)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return forgeError(resp.StatusCode)
	}
	if result == nil {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(result)
}

type Request struct {
	Method string
	Path   string
	Body   interface{}
}

func NewRequest(method, path string, body interface{}) *Request {
	return &Request{
		Method: method,
		Path:   path,
		Body:   body,
	}
}

func forgeError(statusCode int) error {
	switch statusCode {
	case 400:
		return ErrBadRequest
	case 401:
		return ErrInvalidAPIKey
	case 404:
		return ErrNotFound
	case 422:
		return ErrInvalidData
	case 429:
		return ErrTooManyAttempts
	case 500:
		return ErrInternal
	case 503:
		return ErrMaintenance
	}
	return fmt.Errorf("unknown status code %d returned", statusCode)
}
