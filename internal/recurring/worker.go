package recurring

import (
	"context"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"itmosportbot/internal/myitmo"
	"itmosportbot/internal/schedule"
	"itmosportbot/internal/store"
)

// Worker периодически тянет расписание и для каждого пользователя (по priority) пытается записаться на его шаблоны.
type Worker struct {
	DB         *store.DB
	SharedHTTP *http.Client
	TokenURL   string
	ClientID   string
	ConfigBids []int64
	SignURL    string
	// PollSlow — интервал между тиками вне окна полуночи (МСК).
	PollSlow time.Duration
	// PollFast — интервал в окне recurring_fast_window_* (обычно ~20 с у 00:00 МСК).
	PollFast        time.Duration
	FastWindowStart string
	FastWindowEnd   string
	HorizonDays     int
	OnSuccess       func(lessonID int64, userName string, telegramChatID int64)
	// OnTokenExpired вызывается один раз на пользователя при явной auth-ошибке token/refresh.
	OnTokenExpired func(telegramChatID int64, userName string, details string)

	mu          sync.Mutex
	clientCache map[int64]*myitmo.Client
	tokenAlert  map[int64]struct{}
}

func (w *Worker) InvalidateClient(chatID int64) {
	w.mu.Lock()
	defer w.mu.Unlock()
	delete(w.clientCache, chatID)
}

func (w *Worker) clientFor(u store.User) (*myitmo.Client, error) {
	tok, err := w.DB.RefreshToken(u.TelegramChatID)
	if err != nil {
		return nil, err
	}
	if tok == "" {
		return nil, nil
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.clientCache == nil {
		w.clientCache = make(map[int64]*myitmo.Client)
	}
	if c, ok := w.clientCache[u.TelegramChatID]; ok {
		return c, nil
	}
	name := u.DisplayName
	if name == "" {
		name = "tg"
	}
	c := myitmo.NewClient(name, w.TokenURL, w.ClientID, tok, w.SharedHTTP)
	w.clientCache[u.TelegramChatID] = c
	return c, nil
}

func isTokenAuthError(err error) bool {
	if err == nil {
		return false
	}
	s := strings.ToLower(err.Error())
	if strings.Contains(s, "invalid_grant") || strings.Contains(s, "invalid_token") {
		return true
	}
	if strings.Contains(s, "unauthorized") || strings.Contains(s, "401") || strings.Contains(s, "403") {
		return true
	}
	// Ошибка рефреша обычно приходит из token endpoint.
	return strings.Contains(s, "refresh_token") && strings.Contains(s, "token")
}

func (w *Worker) noteTokenAuthFailure(u store.User, err error) {
	if !isTokenAuthError(err) {
		return
	}
	w.mu.Lock()
	if w.tokenAlert == nil {
		w.tokenAlert = make(map[int64]struct{})
	}
	if _, ok := w.tokenAlert[u.TelegramChatID]; ok {
		w.mu.Unlock()
		return
	}
	w.tokenAlert[u.TelegramChatID] = struct{}{}
	w.mu.Unlock()

	if w.OnTokenExpired != nil {
		name := u.DisplayName
		if name == "" {
			name = "tg"
		}
		w.OnTokenExpired(u.TelegramChatID, name, err.Error())
	}
}

func (w *Worker) clearTokenAuthFailure(chatID int64) {
	w.mu.Lock()
	if w.tokenAlert != nil {
		delete(w.tokenAlert, chatID)
	}
	w.mu.Unlock()
}

func (w *Worker) Run(ctx context.Context) {
	if w.PollSlow <= 0 {
		w.PollSlow = 60 * time.Second
	}
	if w.PollFast <= 0 {
		w.PollFast = 20 * time.Second
	}
	if w.HorizonDays <= 0 {
		w.HorizonDays = 18
	}
	if strings.TrimSpace(w.FastWindowStart) == "" {
		w.FastWindowStart = "23:50"
	}
	if strings.TrimSpace(w.FastWindowEnd) == "" {
		w.FastWindowEnd = "00:15"
	}
	loc := mskLoc()
	for {
		w.tick(ctx)
		select {
		case <-ctx.Done():
			return
		default:
		}
		d := NextPollInterval(time.Now().In(loc), w.PollFast, w.PollSlow, w.FastWindowStart, w.FastWindowEnd)
		if d < 5*time.Second {
			d = 5 * time.Second
		}
		t := time.NewTimer(d)
		select {
		case <-ctx.Done():
			if !t.Stop() {
				<-t.C
			}
			return
		case <-t.C:
		}
	}
}

func (w *Worker) tick(ctx context.Context) {
	users, err := w.DB.ListUsersWithTokensOrdered()
	if err != nil {
		log.Printf("recurring: list users: %v", err)
		return
	}
	if len(users) == 0 {
		return
	}
	tctx, cancel := context.WithTimeout(ctx, 150*time.Second)
	allBids := schedule.UnionBuildingIDs(w.ConfigBids)
	var bidMu sync.Mutex
	var wg sync.WaitGroup
	for _, u := range users {
		u := u
		wg.Add(1)
		go func() {
			defer wg.Done()
			cli, err := w.clientFor(u)
			if err != nil || cli == nil {
				return
			}
			raw, err := cli.ScheduleFilters(tctx)
			if err != nil {
				w.noteTokenAuthFailure(u, err)
				return
			}
			w.clearTokenAuthFailure(u.TelegramChatID)
			fb, err := schedule.ParseFilterBuildingIDs(raw)
			if err != nil || len(fb) == 0 {
				return
			}
			bidMu.Lock()
			allBids = schedule.UnionBuildingIDs(allBids, fb)
			bidMu.Unlock()
		}()
	}
	wg.Wait()
	loc, err := time.LoadLocation("Europe/Moscow")
	if err != nil {
		loc = time.FixedZone("MSK", 3*3600)
	}
	now := time.Now().In(loc)
	start := now.Format("2006-01-02")
	end := now.AddDate(0, 0, w.HorizonDays).Format("2006-01-02")
	var parts []schedule.BuildingPart
	var failed []int64
	var fetchedBy int64
	for _, u := range users {
		listClient, err := w.clientFor(u)
		if err != nil || listClient == nil {
			continue
		}
		if err := listClient.EnsureAccessToken(tctx); err != nil {
			w.noteTokenAuthFailure(u, err)
			continue
		}
		w.clearTokenAuthFailure(u.TelegramChatID)
		p, f := schedule.FetchBuildingSchedulesRange(tctx, listClient, start, end, allBids, 10)
		if len(p) == 0 {
			failed = f
			continue
		}
		parts = p
		fetchedBy = u.TelegramChatID
		break
	}
	cancel()
	if len(parts) == 0 {
		log.Printf("recurring: schedules range %s..%s не загрузились ни для одного пользователя (bids=%d, failed=%v)", start, end, len(allBids), failed)
		return
	}
	log.Printf("recurring: расписание загружено через chat_id=%d, корпусов=%d", fetchedBy, len(allBids))
	raw, err := schedule.MergeSchedules(parts)
	if err != nil {
		log.Printf("recurring: merge: %v", err)
		return
	}
	occs, err := schedule.ParseOccurrences(raw)
	if err != nil {
		log.Printf("recurring: parse: %v", err)
		return
	}
	todayStart := CalendarDateMSK(now)
	signCtx, signCancel := context.WithTimeout(ctx, 90*time.Second)
	defer signCancel()

	for _, u := range users {
		cli, err := w.clientFor(u)
		if err != nil || cli == nil {
			continue
		}
		if err := cli.EnsureAccessToken(signCtx); err != nil {
			w.noteTokenAuthFailure(u, err)
			continue
		}
		w.clearTokenAuthFailure(u.TelegramChatID)
		rows, err := w.DB.ListRecurringTemplates(u.TelegramChatID)
		if err != nil {
			log.Printf("recurring: list templates %d: %v", u.TelegramChatID, err)
			continue
		}
		if len(rows) == 0 {
			continue
		}
		templates := make([]Template, len(rows))
		for i := range rows {
			templates[i] = TemplateFromStore(rows[i])
		}
		for _, tpl := range templates {
			signedSet := make(map[int64]struct{}, len(tpl.SignedLessonIDs))
			for _, id := range tpl.SignedLessonIDs {
				signedSet[id] = struct{}{}
			}
			for _, oc := range occs {
				if !tpl.Fingerprint.Matches(oc) {
					continue
				}
				if _, ok := signedSet[oc.LessonID]; ok {
					continue
				}
				if len(oc.Date) < 10 {
					continue
				}
				openAt, err := SignOpensAtMSK(oc.Date)
				if err != nil {
					continue
				}
				if now.Before(openAt) {
					continue
				}
				lessonDay, err := time.ParseInLocation("2006-01-02", oc.Date[:10], loc)
				if err != nil {
					continue
				}
				lessonDay = CalendarDateMSK(lessonDay)
				if todayStart.After(lessonDay) {
					continue
				}
				lessonStart, err := LessonStartMSK(oc, loc)
				if err != nil {
					continue
				}
				if u.MinLeadHours > 0 {
					minLead := time.Duration(u.MinLeadHours) * time.Hour
					if lessonStart.Sub(now) < minLead {
						continue
					}
				}
				signBids := BuildingIDsForSign(tpl.Fingerprint, allBids)
				ok, _, uname := myitmo.TrySignLesson(signCtx, []*myitmo.Client{cli}, w.SignURL, signBids, oc.LessonID)
				if !ok {
					continue
				}
				if err := w.DB.AppendSignedLesson(u.TelegramChatID, tpl.ID, oc.LessonID); err != nil {
					log.Printf("recurring: AppendSignedLesson: %v", err)
					continue
				}
				signedSet[oc.LessonID] = struct{}{}
				if w.OnSuccess != nil {
					w.OnSuccess(oc.LessonID, uname, u.TelegramChatID)
				}
				log.Printf("recurring: запись id=%d шаблон=%s chat=%d", oc.LessonID, tpl.ID, u.TelegramChatID)
			}
		}
	}
}
