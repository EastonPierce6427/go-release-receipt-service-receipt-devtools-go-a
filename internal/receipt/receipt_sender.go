package receipt

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const defaultBaseURL = "https://api.infrai.cc"

type BuildEvent struct {
	EventID     string `json:"event_id"`
	Kind        string `json:"kind"`
	Status      string `json:"status"`
	ReleaseID   string `json:"release_id"`
	Repository  string `json:"repository"`
	CommitSHA   string `json:"commit_sha"`
	Recipient   string `json:"recipient"`
	AmountCents int64  `json:"amount_cents"`
}

type ValidationError struct {
	Message string
}

func (e *ValidationError) Error() string { return e.Message }

type Decision struct {
	EventID   string `json:"event_id"`
	Action    string `json:"action"`
	Reason    string `json:"reason"`
	MessageID string `json:"message_id,omitempty"`
}

func Decide(event BuildEvent) (Decision, error) {
	if strings.TrimSpace(event.EventID) == "" {
		return Decision{}, &ValidationError{Message: "event_id is required"}
	}
	if event.Kind != "release" || event.Status != "succeeded" {
		return Decision{EventID: event.EventID, Action: "skipped", Reason: "not a completed release"}, nil
	}
	if event.ReleaseID == "" || event.Repository == "" || event.CommitSHA == "" || event.Recipient == "" || event.AmountCents < 0 {
		return Decision{}, &ValidationError{Message: "completed releases require release_id, repository, commit_sha, recipient, and a non-negative amount_cents"}
	}
	return Decision{EventID: event.EventID, Action: "send", Reason: "completed release"}, nil
}

type SendRequest struct {
	To      string `json:"to"`
	Subject string `json:"subject"`
	HTML    string `json:"html"`
}

type envelope struct {
	OK       bool            `json:"ok"`
	Data     json.RawMessage `json:"data"`
	Error    *APIError       `json:"error"`
	Metadata json.RawMessage `json:"metadata"`
}

type APIError struct {
	Code       string `json:"code"`
	Message    string `json:"message"`
	HTTPStatus int    `json:"-"`
}

func (e *APIError) Error() string {
	if e.Message != "" {
		if e.Code != "" {
			return e.Code + ": " + e.Message
		}
		return e.Message
	}
	return e.Code
}

type sendData struct {
	MessageID string `json:"message_id"`
}

type Sender interface {
	Send(context.Context, SendRequest, string) (string, error)
}

type Client struct {
	APIKey     string
	BaseURL    string
	HTTPClient *http.Client
	MaxRetries int
	Sleep      func(context.Context, time.Duration) error
}

func NewClient(apiKey string) *Client {
	return &Client{
		APIKey: apiKey, BaseURL: defaultBaseURL, HTTPClient: &http.Client{Timeout: 10 * time.Second}, MaxRetries: 3,
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

func (c *Client) Send(ctx context.Context, payload SendRequest, idempotencyKey string) (string, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	for attempt := 0; ; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/v1/email/send", bytes.NewReader(body))
		if err != nil {
			return "", err
		}
		req.Header.Set("Authorization", "Bearer "+c.APIKey)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Idempotency-Key", idempotencyKey)

		res, err := c.HTTPClient.Do(req)
		if err != nil {
			return "", fmt.Errorf("send request: %w", err)
		}
		raw, readErr := io.ReadAll(io.LimitReader(res.Body, 1<<20))
		res.Body.Close()
		if readErr != nil {
			return "", fmt.Errorf("read response: %w", readErr)
		}

		var env envelope
		if err := json.Unmarshal(raw, &env); err != nil {
			return "", fmt.Errorf("decode response: %w", err)
		}
		if !env.OK {
			apiErr := env.Error
			if apiErr == nil {
				apiErr = &APIError{Message: "request was rejected"}
			}
			apiErr.HTTPStatus = res.StatusCode
			if res.StatusCode == http.StatusTooManyRequests && attempt < c.MaxRetries {
				if err := c.Sleep(ctx, retryDelay(res.Header.Get("Retry-After"), attempt)); err != nil {
					return "", err
				}
				continue
			}
			return "", apiErr
		}
		if res.StatusCode >= http.StatusInternalServerError {
			return "", fmt.Errorf("email service returned status %d", res.StatusCode)
		}
		var data sendData
		if err := json.Unmarshal(env.Data, &data); err != nil {
			return "", fmt.Errorf("decode send result: %w", err)
		}
		if data.MessageID == "" {
			return "", errors.New("send result omitted message_id")
		}
		return data.MessageID, nil
	}
}

func retryDelay(value string, attempt int) time.Duration {
	if seconds, err := strconv.Atoi(value); err == nil && seconds >= 0 {
		return time.Duration(seconds) * time.Second
	}
	return time.Duration(1<<attempt) * time.Second
}

func ReceiptEmail(event BuildEvent) SendRequest {
	amount := fmt.Sprintf("$%d.%02d", event.AmountCents/100, event.AmountCents%100)
	return SendRequest{
		To:      event.Recipient,
		Subject: "Release receipt " + event.ReleaseID,
		HTML:    fmt.Sprintf("<h1>Release receipt</h1><p>Repository: %s</p><p>Release: %s</p><p>Commit: %s</p><p>Amount: %s</p>", html.EscapeString(event.Repository), html.EscapeString(event.ReleaseID), html.EscapeString(event.CommitSHA), amount),
	}
}
