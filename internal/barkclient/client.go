package barkclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"
)

// Client is a thin wrapper over the Bark server HTTP API.
type Client struct {
	baseURL *url.URL
	token   string
	http    *http.Client
}

// New creates a Bark API client.
func New(rawURL, token string, timeout time.Duration) (*Client, error) {
	if rawURL == "" {
		return nil, fmt.Errorf("base url is required")
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return nil, err
	}
	if parsed.Scheme == "" {
		return nil, fmt.Errorf("base url must include scheme")
	}
	parsed.Path = strings.TrimSuffix(parsed.Path, "/")
	return &Client{
		baseURL: parsed,
		token:   token,
		http: &http.Client{
			Timeout: timeout,
		},
	}, nil
}

// Ping checks Bark server health.
func (c *Client) Ping(ctx context.Context) (*CommonResponse[map[string]any], error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.resolve("/ping"), nil)
	if err != nil {
		return nil, err
	}
	c.decorate(req)
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ping failed: %s", resp.Status)
	}
	var payload CommonResponse[map[string]any]
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}
	return &payload, nil
}

// Register ensures the Bark server knows the device token and returns latest device key.
func (c *Client) Register(ctx context.Context, deviceToken, key string) (*CommonResponse[RegisterData], error) {
	u := c.resolve("/register")
	values := url.Values{}
	values.Set("devicetoken", deviceToken)
	if key != "" {
		values.Set("key", key)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u+"?"+values.Encode(), nil)
	if err != nil {
		return nil, err
	}
	c.decorate(req)
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("register http status %s", resp.Status)
	}
	var payload CommonResponse[RegisterData]
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}
	return &payload, nil
}

// SendEncryptedPush posts ciphertext to Bark server push endpoint.
func (c *Client) SendEncryptedPush(ctx context.Context, deviceKey, ciphertext, iv string) (*CommonResponse[struct{}], error) {
	body, err := json.Marshal(map[string]string{
		"ciphertext": ciphertext,
		"iv":         iv,
	})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.resolve("/"+deviceKey), bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	c.decorate(req)
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("push http status %s", resp.Status)
	}
	var payload CommonResponse[struct{}]
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}
	return &payload, nil
}

func (c *Client) resolve(p string) string {
	u := *c.baseURL
	u.Path = path.Join(c.baseURL.Path, p)
	if !strings.HasSuffix(p, "/") && strings.HasSuffix(u.Path, "/") {
		u.Path = u.Path[:len(u.Path)-1]
	}
	return u.String()
}

func (c *Client) decorate(req *http.Request) {
	if c.token != "" {
		req.Header.Set("API-TOKEN", c.token)
	}
}

// BaseURL returns the configured Bark server URL without trailing slash.
func (c *Client) BaseURL() string {
	return strings.TrimRight(c.baseURL.String(), "/")
}

// DeviceEndpoint returns the push endpoint for the provided device key.
func (c *Client) DeviceEndpoint(deviceKey string) string {
	return fmt.Sprintf("%s/%s", c.BaseURL(), deviceKey)
}

// CommonResponse models Bark server standard response.
type CommonResponse[T any] struct {
	Code      int    `json:"code"`
	Message   string `json:"message"`
	Timestamp int64  `json:"timestamp"`
	Data      T      `json:"data"`
}

// RegisterData is returned by Bark /register.
type RegisterData struct {
	Key         string `json:"key"`
	DeviceKey   string `json:"device_key"`
	DeviceToken string `json:"device_token"`
}
