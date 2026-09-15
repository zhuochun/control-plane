package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type Client struct {
	BaseURL string
	HTTP    *http.Client
}
type Error struct {
	Status    int
	Code      string
	Message   string
	Retryable bool
	Details   any
}

func (e *Error) Error() string { return e.Message }

func New(base string) *Client {
	return &Client{BaseURL: strings.TrimRight(base, "/"), HTTP: &http.Client{Timeout: 30 * time.Second}}
}
func (c *Client) Do(ctx context.Context, method, path string, body any) (json.RawMessage, error) {
	var input io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		input = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+"/api/v1"+path, input)
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	response, err := c.HTTP.Do(req)
	if err != nil {
		return nil, &Error{Code: "server_unavailable", Message: fmt.Sprintf("aicp server unavailable; run aicp serve: %v", err)}
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, 8<<20))
	if err != nil {
		return nil, err
	}
	if response.StatusCode >= 400 {
		var envelope struct {
			Error struct {
				Code      string `json:"code"`
				Message   string `json:"message"`
				Retryable bool   `json:"retryable"`
				Details   any    `json:"details"`
			} `json:"error"`
		}
		if json.Unmarshal(data, &envelope) == nil && envelope.Error.Message != "" {
			return nil, &Error{Status: response.StatusCode, Code: envelope.Error.Code, Message: envelope.Error.Message, Retryable: envelope.Error.Retryable, Details: envelope.Error.Details}
		}
		return nil, &Error{Status: response.StatusCode, Code: "http_error", Message: fmt.Sprintf("server returned %d", response.StatusCode)}
	}
	return data, nil
}
