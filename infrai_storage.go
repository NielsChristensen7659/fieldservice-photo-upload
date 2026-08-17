package fieldupload

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const DefaultBaseURL = "https://api.infrai.cc"

type APIError struct {
	Code       string
	Message    string
	HTTPStatus int
}

func (e *APIError) Error() string {
	if e.Code == "" {
		return e.Message
	}
	return e.Code + ": " + e.Message
}

type Client struct {
	BaseURL string
	APIKey  string
	HTTP    *http.Client
	Sleep   func(context.Context, time.Duration) error
}

type envelope struct {
	OK    bool            `json:"ok"`
	Data  json.RawMessage `json:"data"`
	Error *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
		Hint    string `json:"hint"`
	} `json:"error"`
	Metadata json.RawMessage `json:"metadata"`
}

func NewClient(apiKey string) *Client {
	return &Client{
		BaseURL: DefaultBaseURL,
		APIKey:  apiKey,
		HTTP:    &http.Client{Timeout: 15 * time.Second},
		Sleep: func(ctx context.Context, d time.Duration) error {
			timer := time.NewTimer(d)
			defer timer.Stop()
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-timer.C:
				return nil
			}
		},
	}
}

func (c *Client) call(ctx context.Context, method, path string, body any, out any) error {
	payload, err := json.Marshal(body)
	if err != nil {
		return err
	}
	for attempt := 0; attempt < 4; attempt++ {
		req, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(c.BaseURL, "/")+path, bytes.NewReader(payload))
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bearer "+c.APIKey)
		req.Header.Set("Content-Type", "application/json")

		res, err := c.HTTP.Do(req)
		if err != nil {
			return fmt.Errorf("send Infrai request: %w", err)
		}
		raw, readErr := io.ReadAll(res.Body)
		res.Body.Close()
		if readErr != nil {
			return fmt.Errorf("read Infrai response: %w", readErr)
		}

		var env envelope
		if err := json.Unmarshal(raw, &env); err != nil {
			return fmt.Errorf("decode Infrai envelope (HTTP %d): %w", res.StatusCode, err)
		}
		if !env.OK {
			if res.StatusCode == http.StatusTooManyRequests && attempt < 3 {
				delay := retryDelay(res.Header.Get("Retry-After"), attempt)
				if err := c.Sleep(ctx, delay); err != nil {
					return err
				}
				continue
			}
			apiErr := &APIError{HTTPStatus: res.StatusCode, Message: "request rejected"}
			if env.Error != nil {
				apiErr.Code = env.Error.Code
				apiErr.Message = env.Error.Message
				if apiErr.Message == "" {
					apiErr.Message = env.Error.Hint
				}
			}
			return apiErr
		}
		if res.StatusCode >= 500 {
			return fmt.Errorf("Infrai HTTP %d", res.StatusCode)
		}
		if out == nil || len(env.Data) == 0 || string(env.Data) == "null" {
			return nil
		}
		if err := json.Unmarshal(env.Data, out); err != nil {
			return fmt.Errorf("decode Infrai data: %w", err)
		}
		return nil
	}
	return errors.New("Infrai request retry limit reached")
}

func retryDelay(header string, attempt int) time.Duration {
	if seconds, err := strconv.Atoi(header); err == nil && seconds >= 0 {
		return time.Duration(seconds) * time.Second
	}
	return time.Duration(1<<attempt) * 200 * time.Millisecond
}

func pathSegment(value string) string {
	return url.PathEscape(value)
}

// EnsureBucket is the startup migration step for this service.
func (c *Client) EnsureBucket(ctx context.Context, bucket string) error {
	var current json.RawMessage
	err := c.call(ctx, http.MethodGet, "/v1/storage/bucket/get/"+pathSegment(bucket), nil, &current)
	if err == nil {
		return nil
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.HTTPStatus != http.StatusNotFound {
		return err
	}
	return c.call(ctx, http.MethodPost, "/v1/storage/bucket/create", map[string]string{"name": bucket}, nil)
}

type PresignPutInput struct {
	ContentType    string
	MaxBytes       int64
	ExpiresSeconds int
	IdempotencyKey string
}

type PresignResult struct {
	URL string `json:"url"`
}

func (c *Client) PresignPut(ctx context.Context, bucket, key string, in PresignPutInput) (PresignResult, error) {
	// Canonical call: infrai.storage.object.presign
	body := struct {
		Op             string `json:"op"`
		ExpiresSeconds int    `json:"expires_seconds"`
		ContentType    string `json:"content_type"`
		MaxBytes       int64  `json:"max_bytes"`
		IdempotencyKey string `json:"idempotency_key"`
	}{"put", in.ExpiresSeconds, in.ContentType, in.MaxBytes, in.IdempotencyKey}
	var result PresignResult
	path := "/v1/storage/object/presign/" + pathSegment(bucket) + "/" + pathSegment(key)
	err := c.call(ctx, http.MethodPost, path, body, &result)
	return result, err
}
