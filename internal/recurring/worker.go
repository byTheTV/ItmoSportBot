package recurring

import (
	"context"
	"log"
	"net/http"
	"sync"
	"time"

	"itmosportbot/internal/myitmo"
	"itmosportbot/internal/schedule"
	"itmosportbot/internal/store"
)

// Worker периодически тянет расписание и для каждого пользователя (по priority) пытается записаться на его шаблоны.
type Worker struct {
	DB          *store.DB
	SharedHTTP  *http.Client
	TokenURL    string
	ClientID    string
	ConfigBids  []int64
	SignURL     string
	SignBids    []int64
	Interval    time.Duration
	HorizonDays int
	OnSuccess   func(lessonID int64, userName string, telegramChatID int64)

	mu          sync.Mutex
	clientCache map[int64]*myitmo.Client
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

func (w *Worker) Run(ctx context.Context) {
	if w.Interval <= 0 {
		w.Interval = 20 * time.Second
	}
	if w.HorizonDays <= 0 {
		w.HorizonDays = 42
	}
	t := time.NewTicker(w.Interval)
	defer t.Stop()
	for {
		w.tick(ctx)
		select {
		case <-ctx.Done():
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
	listClient, err := w.clientFor(users[0])
	if err != nil || listClient == nil {
		return
	}
	tctx, cancel := context.WithTimeout(ctx, 150*time.Second)
	allBids := schedule.UnionBuildingIDs(w.ConfigBids)
	if raw, err := listClient.ScheduleFilters(tctx); err == nil {
		if fb, err := schedule.ParseFilterBuildingIDs(raw); err == nil && len(fb) > 0 {
			allBids = schedule.UnionBuildingIDs(w.ConfigBids, fb)
		}
	}
	loc, err := time.LoadLocation("Europe/Moscow")
	if err != nil {
		loc = time.FixedZone("MSK", 3*3600)
	}
	now := time.Now().In(loc)
	start := now.Format("2006-01-02")
	end := now.AddDate(0, 0, w.HorizonDays).Format("2006-01-02")
	parts, _ := schedule.FetchBuildingSchedulesRange(tctx, listClient, start, end, allBids, 10)
	cancel()
	if len(parts) == 0 {
		return
	}
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
		blob, err := w.DB.GetRecurringBlob(u.TelegramChatID)
		if err != nil {
			log.Printf("recurring: blob %d: %v", u.TelegramChatID, err)
			continue
		}
		templates, err := DecodeTemplatesFileJSON(blob)
		if err != nil || len(templates) == 0 {
			continue
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
				if lessonStart.Sub(now) < MinLeadBeforeLesson {
					continue
				}
				ok, _, uname := myitmo.TrySignLesson(signCtx, []*myitmo.Client{cli}, w.SignURL, w.SignBids, oc.LessonID)
				if !ok {
					continue
				}
				newBlob, err := AppendSignedToJSON(blob, tpl.ID, oc.LessonID)
				if err != nil {
					log.Printf("recurring: AppendSignedToJSON: %v", err)
					continue
				}
				if err := w.DB.SetRecurringBlob(u.TelegramChatID, newBlob); err != nil {
					log.Printf("recurring: save blob: %v", err)
					continue
				}
				blob = newBlob
				signedSet[oc.LessonID] = struct{}{}
				if w.OnSuccess != nil {
					w.OnSuccess(oc.LessonID, uname, u.TelegramChatID)
				}
				log.Printf("recurring: запись id=%d шаблон=%s chat=%d", oc.LessonID, tpl.ID, u.TelegramChatID)
			}
		}
	}
}
