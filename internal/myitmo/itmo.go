package myitmo

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const (
	scheduleSignBase = "https://my.itmo.ru/api/sport/sign/schedule"
	limitsSignBase   = "https://my.itmo.ru/api/sport/sign/schedule/limits"
	filtersSignURL   = "https://my.itmo.ru/api/sport/sign/schedule/filters"
)

// Client — OAuth (ITMO: x-www-form-urlencoded) + запросы к my.itmo.
type Client struct {
	tokenURL string
	clientID string
	refresh  string
	name     string
	http     *http.Client
	mu       sync.RWMutex
	access   string
	expiry   time.Time
}

func NewClient(name, tokenURL, clientID, refreshToken string, shared *http.Client) *Client {
	if shared == nil {
		shared = defaultHTTPClient()
	}
	if clientID == "" {
		clientID = "student-personal-cabinet"
	}
	return &Client{
		name:     name,
		tokenURL: tokenURL,
		clientID: clientID,
		refresh:  refreshToken,
		http:     shared,
	}
}

func defaultHTTPClient() *http.Client {
	tr := http.DefaultTransport.(*http.Transport).Clone()
	tr.MaxIdleConns = 32
	tr.MaxIdleConnsPerHost = 32
	tr.IdleConnTimeout = 90 * time.Second
	return &http.Client{Timeout: 25 * time.Second, Transport: tr}
}

func (c *Client) Name() string { return c.name }

func (c *Client) EnsureAccessToken(ctx context.Context) error {
	if c.refresh == "" || c.tokenURL == "" {
		return fmt.Errorf("refresh_token и token_url обязательны")
	}
	c.mu.RLock()
	valid := c.access != "" && time.Now().Before(c.expiry.Add(-30*time.Second))
	c.mu.RUnlock()
	if valid {
		return nil
	}

	form := url.Values{}
	form.Set("client_id", c.clientID)
	form.Set("grant_type", "refresh_token")
	form.Set("refresh_token", c.refresh)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("token (%s): %s: %s", c.name, resp.Status, string(b))
	}
	var tok struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int    `json:"expires_in"`
	}
	if err := json.Unmarshal(b, &tok); err != nil {
		return fmt.Errorf("parse token (%s): %w; body=%s", c.name, err, string(b))
	}
	if tok.AccessToken == "" {
		return fmt.Errorf("no access_token (%s): %s", c.name, string(b))
	}
	sec := tok.ExpiresIn
	if sec <= 0 {
		sec = 300
	}
	exp := time.Now().Add(time.Duration(sec) * time.Second)

	c.mu.Lock()
	c.access = tok.AccessToken
	if tok.RefreshToken != "" {
		c.refresh = tok.RefreshToken
	}
	c.expiry = exp
	c.mu.Unlock()
	return nil
}

func (c *Client) bearer() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return "Bearer " + c.access
}

func (c *Client) GetJSON(ctx context.Context, url string) ([]byte, error) {
	if err := c.EnsureAccessToken(ctx); err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", c.bearer())
	req.Header.Set("Accept", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("GET %s (%s): %s: %s", url, c.name, resp.Status, string(b))
	}
	return b, nil
}

// ScheduleAvailable — GET расписания (актуальный API: /api/sport/sign/schedule).
func (c *Client) ScheduleAvailable(ctx context.Context, dateStart, dateEnd string, buildingID int64) ([]byte, error) {
	u := fmt.Sprintf("%s?building_id=%d&date_start=%s&date_end=%s",
		scheduleSignBase, buildingID,
		url.QueryEscape(dateStart), url.QueryEscape(dateEnd))
	return c.GetJSON(ctx, u)
}

// ScheduleLimits — лимиты для того же API.
func (c *Client) ScheduleLimits(ctx context.Context, buildingID int64) ([]byte, error) {
	u := fmt.Sprintf("%s?building_id=%d", limitsSignBase, buildingID)
	return c.GetJSON(ctx, u)
}

// ScheduleFilters — справочник фильтров (в т.ч. все building_id для расписания).
func (c *Client) ScheduleFilters(ctx context.Context) ([]byte, error) {
	return c.GetJSON(ctx, filtersSignURL)
}

// SignURLWithBuilding добавляет building_id к URL записи (если > 0).
func SignURLWithBuilding(signBase string, buildingID int64) string {
	if buildingID <= 0 {
		return signBase
	}
	sep := "?"
	if strings.Contains(signBase, "?") {
		sep = "&"
	}
	return fmt.Sprintf("%s%sbuilding_id=%d", signBase, sep, buildingID)
}

// SignForLesson — POST [lessonID] на URL записи (query building_id — см. SignURLWithBuilding).
func (c *Client) SignForLesson(ctx context.Context, signURL string, lessonID int64) (body []byte, status int, err error) {
	if err := c.EnsureAccessToken(ctx); err != nil {
		return nil, 0, err
	}
	payload, err := json.Marshal([]int64{lessonID})
	if err != nil {
		return nil, 0, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, signURL, bytes.NewReader(payload))
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Authorization", c.bearer())
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, err
	}
	return b, resp.StatusCode, nil
}

// SignSuccess — error_code == 0 или ответ без error_code, но с result и без error_message.
func SignSuccess(body []byte) bool {
	var v map[string]any
	if json.Unmarshal(body, &v) != nil {
		return false
	}
	if ec, ok := v["error_code"]; ok {
		switch x := ec.(type) {
		case float64:
			return x == 0
		case int:
			return x == 0
		case int64:
			return x == 0
		default:
			return false
		}
	}
	if msg, ok := v["error_message"].(string); ok && msg != "" {
		return false
	}
	_, ok := v["result"]
	return ok
}
