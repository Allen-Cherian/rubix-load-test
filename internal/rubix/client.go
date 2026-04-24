package rubix

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Client is a minimal HTTP client for a single rubix node.
// It is safe for concurrent use.
type Client struct {
	baseURL string
	http    *http.Client
}

// NewClient builds a client pointing at http://addr:port.
func NewClient(addr string, port int, timeout time.Duration) *Client {
	if timeout <= 0 {
		timeout = 2 * time.Minute
	}
	return &Client{
		baseURL: fmt.Sprintf("http://%s:%d", addr, port),
		http: &http.Client{
			Timeout: timeout,
		},
	}
}

// doJSON posts body (if non-nil) to path and decodes the JSON response into out.
// Path may contain {key} segments; pathParams supplies their replacements.
func (c *Client) doJSON(method, path string, pathParams map[string]string, body, out interface{}) error {
	for k, v := range pathParams {
		path = strings.ReplaceAll(path, "{"+k+"}", v)
	}

	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshal body: %w", err)
		}
		reader = bytes.NewReader(b)
	}

	req, err := http.NewRequest(method, c.baseURL+path, reader)
	if err != nil {
		return fmt.Errorf("new request: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("http do: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read body: %w", err)
	}
	if resp.StatusCode >= 400 {
		return fmt.Errorf("http %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("decode response (%s): %w", truncate(string(raw), 200), err)
	}
	return nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
