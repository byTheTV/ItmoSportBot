package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"itmosportbot/internal/buildings"
)

const (
	DefaultTokenURL   = "https://id.itmo.ru/auth/realms/itmo/protocol/openid-connect/token"
	DefaultSignURL    = "https://my.itmo.ru/api/sport/sign/schedule/lessons"
	DefaultClientID   = "student-personal-cabinet"
	DefaultBuildingID = 273 // корпус на Ломоносова (см. my.itmo); смени при необходимости
)

type Telegram struct {
	BotToken         string  `json:"bot_token"`
	DefaultNotifyIDs []int64 `json:"default_notify_chat_ids,omitempty"`
	AdminChatIDs     []int64 `json:"admin_chat_ids,omitempty"`
}

type User struct {
	Name           string `json:"name"`
	RefreshToken   string `json:"refresh_token"`
	Priority       int    `json:"priority"`
	TelegramChatID int64  `json:"telegram_chat_id,omitempty"`
}

type App struct {
	ClientID                 string
	TokenURL                 string
	SignURL                  string
	BuildingIDs              []int64
	BuildingsSource          string // откуда взяли список: config | файл | default
	TokenKey                 string // AES-256: 64 hex или строка ≥32 символа; см. PE_TOKEN_KEY
	RecurringPollSlow        time.Duration
	RecurringPollFast        time.Duration
	RecurringHorizonDays     int
	RecurringFastWindowStart string // HH:MM МСК
	RecurringFastWindowEnd   string
	SQLitePath               string
	Telegram                 Telegram
	Users                    []User
}

type fileConfig struct {
	ClientID                 string   `json:"client_id"`
	TokenURL                 string   `json:"token_url"`
	SignURL                  string   `json:"sign_url"`
	BuildingID               int64    `json:"building_id"`
	BuildingIDs              []int64  `json:"building_ids"`
	BuildingsFile            string   `json:"buildings_file"`
	RecurringPollMS          int      `json:"recurring_poll_ms"` // устар.: интервал «медленного» опроса, если нет recurring_poll_slow_ms
	RecurringPollSlowMS      int      `json:"recurring_poll_slow_ms"`
	RecurringFastPollMS      int      `json:"recurring_fast_poll_ms"`
	RecurringHorizonDays     int      `json:"recurring_horizon_days"`
	RecurringFastWindowStart string   `json:"recurring_fast_window_start"`
	RecurringFastWindowEnd   string   `json:"recurring_fast_window_end"`
	SQLitePath               string   `json:"sqlite_path"`
	TokenKey                 string   `json:"token_key"`
	Telegram                 Telegram `json:"telegram"`
	Users                    []User   `json:"users"`
}

func LoadFile(path string) (*App, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	var f fileConfig
	if err := json.Unmarshal(raw, &f); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	if f.Telegram.BotToken == "" {
		return nil, fmt.Errorf("config: telegram.bot_token обязателен")
	}
	for i := range f.Users {
		if f.Users[i].RefreshToken == "" {
			return nil, fmt.Errorf("config: users[%d] (%s): пустой refresh_token", i, f.Users[i].Name)
		}
	}
	sort.SliceStable(f.Users, func(i, j int) bool {
		return f.Users[i].Priority < f.Users[j].Priority
	})

	clientID := f.ClientID
	if clientID == "" {
		clientID = DefaultClientID
	}
	tokenURL := f.TokenURL
	if tokenURL == "" {
		tokenURL = DefaultTokenURL
	}
	signURL := f.SignURL
	if signURL == "" {
		signURL = DefaultSignURL
	}
	slowMs := f.RecurringPollSlowMS
	if slowMs <= 0 {
		slowMs = f.RecurringPollMS
	}
	if slowMs <= 0 {
		slowMs = 60000
	}
	if slowMs < 5000 {
		slowMs = 5000
	}
	fastMs := f.RecurringFastPollMS
	if fastMs <= 0 {
		fastMs = 20000
	}
	if fastMs < 5000 {
		fastMs = 5000
	}
	horizon := f.RecurringHorizonDays
	if horizon <= 0 {
		horizon = 18
	}
	if horizon > 120 {
		horizon = 120
	}
	winStart := strings.TrimSpace(f.RecurringFastWindowStart)
	winEnd := strings.TrimSpace(f.RecurringFastWindowEnd)
	if winStart == "" {
		winStart = "23:50"
	}
	if winEnd == "" {
		winEnd = "00:15"
	}
	var bids []int64
	var bsrc string
	if len(f.BuildingIDs) > 0 {
		bids = append(bids, f.BuildingIDs...)
		bsrc = "config:building_ids"
	} else if f.BuildingID > 0 {
		bids = []int64{f.BuildingID}
		bsrc = "config:building_id"
	} else {
		bp := resolveBuildingsPath(path, f.BuildingsFile)
		if loaded, err := buildings.Load(bp); err == nil {
			bids = loaded
			bsrc = bp
		} else {
			bids = []int64{DefaultBuildingID}
			bsrc = "default(273)"
		}
	}
	sqlitePath := f.SQLitePath
	if sqlitePath == "" {
		sqlitePath = ""
	}
	return &App{
		ClientID:                 clientID,
		TokenURL:                 tokenURL,
		SignURL:                  signURL,
		BuildingIDs:              bids,
		BuildingsSource:          bsrc,
		TokenKey:                 strings.TrimSpace(f.TokenKey),
		RecurringPollSlow:        time.Duration(slowMs) * time.Millisecond,
		RecurringPollFast:        time.Duration(fastMs) * time.Millisecond,
		RecurringHorizonDays:     horizon,
		RecurringFastWindowStart: winStart,
		RecurringFastWindowEnd:   winEnd,
		SQLitePath:               sqlitePath,
		Telegram:                 f.Telegram,
		Users:                    f.Users,
	}, nil
}

func resolveBuildingsPath(configPath, fromJSON string) string {
	if p := os.Getenv("PE_BUILDINGS"); p != "" {
		return filepath.Clean(p)
	}
	dir := filepath.Dir(configPath)
	if dir == "" {
		dir = "."
	}
	if fromJSON != "" {
		if filepath.IsAbs(fromJSON) {
			return filepath.Clean(fromJSON)
		}
		return filepath.Clean(filepath.Join(dir, fromJSON))
	}
	return filepath.Clean(filepath.Join(dir, "buildings.json"))
}

func ConfigPath() string {
	if p := os.Getenv("PE_CONFIG"); p != "" {
		return p
	}
	return "config.json"
}

// AdminIDs — кто может /admin (только admin_chat_ids).
func (a *App) AdminIDs() []int64 {
	return append([]int64(nil), a.Telegram.AdminChatIDs...)
}

func (a *App) IsAdmin(chatID int64) bool {
	for _, id := range a.Telegram.AdminChatIDs {
		if id == chatID {
			return true
		}
	}
	return false
}
