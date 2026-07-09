package sdk

import (
	"net/http"
	"time"
)

// Client is a thin wrapper around Orion's HTTP API. Safe for
// concurrent use by multiple goroutines.
type Client struct {
	baseURL    string
	httpClient *http.Client
}

type Option func(*Client)

func WithHTTPClient(hc *http.Client) Option {
	return func(c *Client) { c.httpClient = hc }
}

func WithTimeout(d time.Duration) Option {
	return func(c *Client) { c.httpClient.Timeout = d }
}

// NewClient creates a Client pointed at baseURL, e.g.
// "http://localhost:8080". A 5-second timeout applies by default — a
// flag check must never be allowed to hang the calling application
// indefinitely just because Orion is slow or unreachable.
func NewClient(baseURL string, opts ...Option) *Client {
	c := &Client{
		baseURL:    baseURL,
		httpClient: &http.Client{Timeout: 5 * time.Second},
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}
