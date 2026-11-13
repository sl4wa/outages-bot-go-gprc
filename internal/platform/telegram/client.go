// Package telegram is an HTTP adapter, independent of the bot and notifier domains.
package telegram

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

type Chat struct {
	ID int64 `json:"id"`
}
type Message struct {
	Chat Chat   `json:"chat"`
	Text string `json:"text"`
}
type Update struct {
	UpdateID int64    `json:"update_id"`
	Message  *Message `json:"message"`
}

type Error struct {
	StatusCode  int
	RetryAfter  int
	Description string
}

// Deliberately omit Telegram's response text, which can contain message data.
func (e *Error) Error() string {
	return fmt.Sprintf("telegram request failed (status %d)", e.StatusCode)
}

type Client struct {
	baseURL string
	http    *http.Client
}

func New(token, baseURL string, client *http.Client) (*Client, error) {
	if strings.TrimSpace(token) == "" {
		return nil, fmt.Errorf("TELEGRAM_BOT_TOKEN is required")
	}
	if baseURL == "" {
		baseURL = "https://api.telegram.org"
	}
	if client == nil {
		client = &http.Client{Timeout: 75 * time.Second}
	}
	return &Client{baseURL: strings.TrimRight(baseURL, "/") + "/bot" + token + "/", http: client}, nil
}

func (c *Client) call(ctx context.Context, method string, payload any, out any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+method, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("invalid Telegram endpoint")
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		// net/url.Error contains the token-bearing URL. Never propagate it.
		return fmt.Errorf("telegram transport failed")
	}
	defer resp.Body.Close()
	var envelope struct {
		OK          bool            `json:"ok"`
		Result      json.RawMessage `json:"result"`
		ErrorCode   int             `json:"error_code"`
		Description string          `json:"description"`
		Parameters  struct {
			RetryAfter int `json:"retry_after"`
		} `json:"parameters"`
	}
	err = json.NewDecoder(io.LimitReader(resp.Body, 8<<20)).Decode(&envelope)
	if err != nil {
		if resp.StatusCode != http.StatusOK {
			return &Error{StatusCode: resp.StatusCode}
		}
		return fmt.Errorf("invalid Telegram response")
	}
	if resp.StatusCode != http.StatusOK || !envelope.OK {
		code := envelope.ErrorCode
		if code == 0 {
			code = resp.StatusCode
		}
		// Treat forbidden descriptions as blocked or deactivated chats too.
		if resp.StatusCode == http.StatusForbidden || strings.HasPrefix(strings.ToLower(envelope.Description), "forbidden") {
			code = http.StatusForbidden
		}
		return &Error{StatusCode: code, RetryAfter: envelope.Parameters.RetryAfter, Description: envelope.Description}
	}
	if out != nil {
		if err := json.Unmarshal(envelope.Result, out); err != nil {
			return fmt.Errorf("invalid Telegram result")
		}
	}
	return nil
}

func (c *Client) GetUpdates(ctx context.Context, offset int64) ([]Update, error) {
	ctx, cancel := context.WithTimeout(ctx, 75*time.Second)
	defer cancel()
	var updates []Update
	err := c.call(ctx, "getUpdates", map[string]any{"offset": offset, "timeout": 60, "allowed_updates": []string{"message"}}, &updates)
	return updates, err
}

func (c *Client) Send(ctx context.Context, chatID int64, text string) error {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	return c.call(ctx, "sendMessage", map[string]any{"chat_id": chatID, "text": text, "parse_mode": "HTML"}, nil)
}

func (c *Client) SendReply(ctx context.Context, chatID int64, text string, keyboard [][]string, removeKeyboard bool) error {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	payload := map[string]any{"chat_id": chatID, "text": text}
	if removeKeyboard {
		payload["reply_markup"] = map[string]any{"remove_keyboard": true}
	} else if len(keyboard) > 0 {
		payload["reply_markup"] = map[string]any{"keyboard": keyboard, "resize_keyboard": true, "one_time_keyboard": true}
	}
	return c.call(ctx, "sendMessage", payload, nil)
}
