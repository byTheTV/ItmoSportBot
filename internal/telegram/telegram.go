package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

const api = "https://api.telegram.org"

func truncateForLog(b []byte, max int) string {
	if len(b) <= max {
		return string(b)
	}
	return string(b[:max]) + "…"
}

type Bot struct {
	token  string
	http   *http.Client
	offset int64
}

func NewBot(token string, hc *http.Client) *Bot {
	if hc == nil {
		hc = &http.Client{Timeout: 65 * time.Second}
	}
	return &Bot{token: token, http: hc}
}

func (b *Bot) base() string {
	return api + "/bot" + b.token
}

// SendMessage шлёт текст; длинные сообщения режутся (лимит Telegram ~4096).
func (b *Bot) SendMessage(ctx context.Context, chatID int64, text string) error {
	const max = 3900
	text = strings.TrimSpace(text)
	for text != "" {
		chunk := text
		if len(chunk) > max {
			chunk = text[:max]
			text = text[max:]
		} else {
			text = ""
		}
		if err := b.sendOne(ctx, chatID, chunk); err != nil {
			return err
		}
	}
	return nil
}

func (b *Bot) sendOne(ctx context.Context, chatID int64, text string) error {
	body := map[string]any{
		"chat_id": chatID,
		"text":    text,
	}
	raw, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, b.base()+"/sendMessage", bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := b.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("telegram sendMessage: %s: %s", resp.Status, string(respBody))
	}
	var wrap struct {
		Ok          bool            `json:"ok"`
		Description string          `json:"description"`
		Result      json.RawMessage `json:"result"`
	}
	if json.Unmarshal(respBody, &wrap) != nil || !wrap.Ok {
		return fmt.Errorf("telegram: %s", string(respBody))
	}
	return nil
}

// Incoming — входящее сообщение с текстом (личка / группа).
type Incoming struct {
	ChatID   int64
	Text     string
	Username string // логин без @; может быть пустым, если в настройках Telegram скрыт
}

// PollUpdates long-poll; для каждого входящего сообщения вызывает onMessage.
func (b *Bot) PollUpdates(ctx context.Context, onMessage func(Incoming)) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		u := fmt.Sprintf("%s/getUpdates?timeout=50&offset=%d", b.base(), b.offset)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		if err != nil {
			return err
		}
		resp, err := b.http.Do(req)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			time.Sleep(2 * time.Second)
			continue
		}
		respBody, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		var wrap struct {
			Ok          bool   `json:"ok"`
			Description string `json:"description"`
			Result      []struct {
				UpdateID int64 `json:"update_id"`
				Message  *struct {
					Chat struct {
						ID int64 `json:"id"`
					} `json:"chat"`
					From *struct {
						Username string `json:"username"`
					} `json:"from"`
					Text string `json:"text"`
				} `json:"message"`
			} `json:"result"`
		}
		if err := json.Unmarshal(respBody, &wrap); err != nil {
			log.Printf("telegram getUpdates: parse JSON: %v; body=%q", err, truncateForLog(respBody, 500))
			time.Sleep(2 * time.Second)
			continue
		}
		if !wrap.Ok {
			log.Printf("telegram getUpdates: ok=false status=%s desc=%q body=%q", resp.Status, wrap.Description, truncateForLog(respBody, 500))
			time.Sleep(2 * time.Second)
			continue
		}
		for _, up := range wrap.Result {
			if up.UpdateID >= b.offset {
				b.offset = up.UpdateID + 1
			}
			if up.Message == nil || up.Message.Text == "" {
				continue
			}
			un := ""
			if up.Message.From != nil {
				un = up.Message.From.Username
			}
			go onMessage(Incoming{
				ChatID:   up.Message.Chat.ID,
				Text:     strings.TrimSpace(up.Message.Text),
				Username: un,
			})
		}
	}
}

func ParseCommand(text string) (cmd string, args []string) {
	text = strings.TrimSpace(text)
	if text == "" {
		return "", nil
	}
	if i := strings.IndexByte(text, ' '); i >= 0 {
		cmd = strings.ToLower(strings.TrimSpace(text[:i]))
		args = strings.Fields(text[i+1:])
	} else {
		cmd = strings.ToLower(text)
	}
	if len(cmd) > 0 && cmd[0] == '/' {
		cmd = cmd[1:]
	}
	if i := strings.IndexByte(cmd, '@'); i >= 0 {
		cmd = cmd[:i]
	}
	return cmd, args
}
