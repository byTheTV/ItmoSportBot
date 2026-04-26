package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"itmosportbot/internal/config"
	"itmosportbot/internal/myitmo"
	"itmosportbot/internal/recurring"
	"itmosportbot/internal/schedule"
	"itmosportbot/internal/store"
	"itmosportbot/internal/telegram"
)

const defaultNewUserPriority = 100

// helpStringsRev — смотри в лог при старте; если на сервере другое число, там старый бинарник.
const helpStringsRev = "8"

func main() {
	configPath := flag.String("config", config.ConfigPath(), "путь к config.json")
	flag.Parse()

	app, err := config.LoadFile(*configPath)
	if err != nil {
		log.Fatal(err)
	}
	if len(app.Telegram.AdminChatIDs) == 0 {
		log.Printf("предупреждение: telegram.admin_chat_ids пуст — команда /admin никому недоступна")
	}

	store.SetTokenKey(app.TokenKey)
	if !store.TokenEncryptionConfigured() {
		log.Printf("предупреждение: ни PE_TOKEN_KEY, ни token_key в config — refresh-токены в SQLite без шифрования (префикс PLAIN1:). Задайте 64 hex (openssl rand -hex 32) или строку от 32 символов; в env приоритетнее config.")
	}

	dbPath := sqliteDBPath(*configPath, app.SQLitePath)
	st, err := store.Open(dbPath)
	if err != nil {
		log.Fatalf("sqlite %q: %v", dbPath, err)
	}
	defer st.Close()

	if err := st.ImportFromConfig(app.Users); err != nil {
		log.Fatalf("импорт users: %v", err)
	}
	var firstChat int64
	for _, u := range app.Users {
		if u.TelegramChatID != 0 {
			firstChat = u.TelegramChatID
			break
		}
	}
	if firstChat != 0 {
		if err := st.ImportRecurringFile(recurringStorePath(*configPath), firstChat); err != nil {
			log.Printf("импорт recurring json: %v", err)
		}
	}

	shared := sharedHTTPClient()

	notifyDefault := append([]int64(nil), app.Telegram.DefaultNotifyIDs...)

	tgBot := telegram.NewBot(app.Telegram.BotToken, nil)
	var tgMu sync.Mutex
	send := func(chatID int64, text string) {
		tgMu.Lock()
		defer tgMu.Unlock()
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := tgBot.SendMessage(ctx, chatID, text); err != nil {
			log.Printf("telegram send -> %d: %v", chatID, err)
		}
	}

	recWorker := &recurring.Worker{
		DB:              st,
		SharedHTTP:      shared,
		TokenURL:        app.TokenURL,
		ClientID:        app.ClientID,
		ConfigBids:      app.BuildingIDs,
		SignURL:         app.SignURL,
		PollSlow:        app.RecurringPollSlow,
		PollFast:        app.RecurringPollFast,
		FastWindowStart: app.RecurringFastWindowStart,
		FastWindowEnd:   app.RecurringFastWindowEnd,
		HorizonDays:     app.RecurringHorizonDays,
		OnSuccess: func(lessonID int64, userName string, telegramChatID int64) {
			msg := fmt.Sprintf("Автозапись (шаблон) на занятие id=%d успешна (аккаунт: %s).", lessonID, userName)
			seen := make(map[int64]struct{})
			if telegramChatID != 0 {
				seen[telegramChatID] = struct{}{}
				send(telegramChatID, msg)
			}
			for _, id := range notifyDefault {
				if _, ok := seen[id]; ok {
					continue
				}
				seen[id] = struct{}{}
				send(id, msg)
			}
		},
		OnTokenExpired: func(telegramChatID int64, userName string, details string) {
			msg := "Не удалось обновить токен ITMO для автозаписи. " +
				"Нужно заново привязать аккаунт: /link <refresh_token>. " +
				"Это уведомление отправляется один раз до успешной авторизации."
			if details != "" {
				msg += "\nДиагностика: " + truncateForMessage(details, 260)
			}
			if telegramChatID != 0 {
				send(telegramChatID, msg)
			}
			log.Printf("recurring: token expired chat=%d user=%s: %s", telegramChatID, userName, details)
		},
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	log.Printf("pewatch: sqlite=%q, здания (%s)=%v, recurring=%v/%v МСК [%s–%s], horizon=%d дн., config %q, help_rev=%s",
		dbPath, app.BuildingsSource, app.BuildingIDs,
		app.RecurringPollFast, app.RecurringPollSlow,
		app.RecurringFastWindowStart, app.RecurringFastWindowEnd,
		app.RecurringHorizonDays, *configPath, helpStringsRev)

	go recWorker.Run(ctx)

	go func() {
		_ = tgBot.PollUpdates(ctx, func(in telegram.Incoming) {
			chatID := in.ChatID
			text := in.Text
			tgUser := in.Username
			if tgUser != "" {
				_ = st.UpdateTelegramUsername(chatID, tgUser)
			}
			cmd, args := telegram.ParseCommand(text)
			if cmd == "admin" {
				if !app.IsAdmin(chatID) {
					send(chatID, "Команда только для администраторов. Ваш chat_id должен быть в telegram.admin_chat_ids в config.")
					return
				}
				handleAdmin(st, send, chatID, args)
				return
			}
			linked, err := st.HasLinkedITMO(chatID)
			if err != nil {
				log.Printf("HasLinkedITMO: %v", err)
				send(chatID, "Ошибка базы данных. Попробуйте позже или сообщите администратору.")
				return
			}
			if !linked {
				switch cmd {
				case "help", "start":
					send(chatID, helpText())
				case "link":
					if len(args) < 1 {
						send(chatID, "Использование: /link <refresh_token>\nОдин пробел после /link, затем токен из DevTools -> Network (my.itmo.ru, заголовки set-cookie auth._refresh_token.itmoId= - это он и есть)")
						return
					}
					token := strings.TrimSpace(args[0])
					handleLink(ctx, app, st, recWorker, send, shared, chatID, token, tgUser)
				default:
					send(chatID, "Сначала привяжите аккаунт: отправьте /link и refresh_token из my.itmo в одном сообщении (см. /help).")
				}
				return
			}

			listClient, err := clientForChat(ctx, st, app, shared, chatID)
			if err != nil || listClient == nil {
				send(chatID, "Не удалось создать клиент API. Проверьте токен: /link <новый refresh_token>")
				return
			}

			switch cmd {
			case "help", "start":
				send(chatID, helpText())
			case "link":
				if len(args) < 1 {
					send(chatID, "Использование: /link <refresh_token> — обновить токен ITMO (см. /help).")
					return
				}
				token := strings.TrimSpace(args[0])
				handleLink(ctx, app, st, recWorker, send, shared, chatID, token, tgUser)
			case "schedule":
				if len(args) < 1 {
					send(chatID, "Использование: /schedule YYYY-MM-DD")
					return
				}
				date := args[0]
				sctx, cancel := context.WithTimeout(ctx, 180*time.Second)
				allBids := schedule.UnionBuildingIDs(app.BuildingIDs)
				mergedFromFilters := false
				if fbody, err := listClient.ScheduleFilters(sctx); err != nil {
					log.Printf("schedule filters: %v", err)
				} else if fb, err := schedule.ParseFilterBuildingIDs(fbody); err != nil {
					log.Printf("parse schedule filters: %v", err)
				} else if len(fb) > 0 {
					allBids = schedule.UnionBuildingIDs(app.BuildingIDs, fb)
					mergedFromFilters = true
				}
				parts, loadFailed := schedule.FetchBuildingSchedules(sctx, listClient, date, allBids, 10)
				cancel()
				if len(parts) == 0 {
					send(chatID, fmt.Sprintf("Ни один корпус не загрузился. Проверь сеть и токен.\nОшибки по id: %s", joinInt64(loadFailed)))
					return
				}
				raw, err := schedule.MergeSchedules(parts)
				if err != nil {
					send(chatID, "Сборка расписания: "+err.Error())
					return
				}
				lctx, cancel2 := context.WithTimeout(ctx, 90*time.Second)
				limChunks := schedule.FetchBuildingLimits(lctx, listClient, allBids, 10)
				cancel2()
				limRaw, _ := schedule.MergeLimits(limChunks)
				if len(limRaw) == 0 {
					limRaw = []byte("{}")
				}
				msgs, err := schedule.FormatDayMessages(date, raw, limRaw, app.BuildingIDs, loadFailed, mergedFromFilters, len(allBids))
				if err != nil {
					send(chatID, "Разбор ответа: "+err.Error())
					return
				}
				for _, m := range msgs {
					send(chatID, m)
					time.Sleep(80 * time.Millisecond)
				}
			case "add":
				handleRecurringAdd(ctx, chatID, st, listClient, app, args, send)
			case "list":
				handleRecurringList(chatID, st, send)
			case "remove":
				handleRecurringRemove(chatID, st, args, send)
			case "lead":
				handleLead(chatID, st, args, send)
			case "recurring":
				if len(args) < 1 {
					send(chatID, "Используйте: /add /list /remove (см. /help). Старый вид: /recurring add|list|remove …")
					return
				}
				switch strings.ToLower(strings.TrimSpace(args[0])) {
				case "add":
					handleRecurringAdd(ctx, chatID, st, listClient, app, args[1:], send)
				case "list":
					handleRecurringList(chatID, st, send)
				case "remove":
					handleRecurringRemove(chatID, st, args[1:], send)
				default:
					send(chatID, "Используйте: /add /list /remove (см. /help).")
				}
			default:
				send(chatID, "Неизвестная команда. /help")
			}
		})
	}()

	<-ctx.Done()
	log.Println("shutdown")
}

func handleAdmin(st *store.DB, send func(int64, string), chatID int64, args []string) {
	if len(args) < 1 || strings.EqualFold(strings.TrimSpace(args[0]), "help") {
		send(chatID, adminHelpText())
		return
	}
	switch strings.ToLower(strings.TrimSpace(args[0])) {
	case "users":
		users, err := st.ListAllUsers()
		if err != nil {
			send(chatID, "Ошибка БД: "+err.Error())
			return
		}
		if len(users) == 0 {
			send(chatID, "Пользователей в БД нет.")
			return
		}
		var b strings.Builder
		b.WriteString("Пользователи (priority: меньше — раньше в очереди автозаписи):\n")
		for _, u := range users {
			link := "нет ITMO"
			if u.Linked {
				link = "ITMO привязан"
			}
			at := u.TelegramUsername
			if at != "" {
				at = "@" + at
			} else {
				at = "нет @username"
			}
			nTpl, _ := st.CountRecurringTemplates(u.TelegramChatID)
			fmt.Fprintf(&b, "\n· id=%d chat=%d %s priority=%d · %s · шаблонов: %d · %s\n",
				u.ID, u.TelegramChatID, at, u.Priority, link, nTpl, u.DisplayName)
		}
		b.WriteString("\n/admin setpriority <chat_id> <priority> — сменить приоритет.")
		send(chatID, strings.TrimSpace(b.String()))
	case "setpriority":
		if len(args) < 3 {
			send(chatID, "Использование: /admin setpriority <telegram_chat_id> <priority>\nМеньше priority — раньше обрабатываются шаблоны и попытки записи.")
			return
		}
		cid, err := strconv.ParseInt(strings.TrimSpace(args[1]), 10, 64)
		if err != nil {
			send(chatID, "Некорректный chat_id.")
			return
		}
		prio, err := strconv.Atoi(strings.TrimSpace(args[2]))
		if err != nil {
			send(chatID, "Некорректный priority.")
			return
		}
		if err := st.SetPriority(cid, prio); err != nil {
			send(chatID, err.Error())
			return
		}
		send(chatID, fmt.Sprintf("priority для chat_id=%d установлен: %d", cid, prio))
	default:
		send(chatID, adminHelpText())
	}
}

func handleRecurringAdd(ctx context.Context, chatID int64, st *store.DB, listClient *myitmo.Client, app *config.App, args []string, send func(int64, string)) {
	if len(args) < 1 {
		send(chatID, "Использование: /add <lesson_id> [YYYY-MM-DD]")
		return
	}
	lid, err := strconv.ParseInt(strings.TrimSpace(args[0]), 10, 64)
	if err != nil || lid <= 0 {
		send(chatID, "Некорректный lesson_id.")
		return
	}
	hint := ""
	if len(args) >= 2 {
		hint = strings.TrimSpace(args[1])
	}
	actx, acancel := context.WithTimeout(ctx, 200*time.Second)
	raw, err := mergedScheduleForTemplate(actx, listClient, app.BuildingIDs, hint)
	acancel()
	if err != nil {
		send(chatID, "Не удалось найти занятие: "+err.Error())
		return
	}
	occ, err := schedule.FindOccurrenceByLessonID(raw, lid)
	if err != nil {
		send(chatID, err.Error())
		return
	}
	tpl := recurring.NewTemplate(lid, *occ)
	if err := st.InsertRecurringTemplate(chatID, tpl.ToStore()); err != nil {
		send(chatID, "Сохранение: "+err.Error())
		return
	}
	send(chatID, formatNewRecurringTemplate(tpl))
}

func handleRecurringList(chatID int64, st *store.DB, send func(int64, string)) {
	rows, err := st.ListRecurringTemplates(chatID)
	if err != nil {
		send(chatID, "Чтение шаблонов: "+err.Error())
		return
	}
	list := make([]recurring.Template, len(rows))
	for i := range rows {
		list[i] = recurring.TemplateFromStore(rows[i])
	}
	if len(list) == 0 {
		send(chatID, "Шаблонов нет. /add <lesson_id>")
		return
	}
	var b strings.Builder
	b.WriteString("Шаблоны (автозапись после 00:00 МСК за 14 дней до занятия):\n")
	for i, t := range list {
		f := t.Fingerprint
		bid := ""
		if f.BuildingID > 0 {
			bid = fmt.Sprintf(" · корпус %d", f.BuildingID)
		}
		fmt.Fprintf(&b, "\n%d) шаблон id=%s · эталон lesson_id=%d%s\n   %s–%s  %s · %s\n   %s\n   👤 %s\n",
			i+1, t.ID, t.SourceLessonID, bid,
			f.TimeSlotStart, f.TimeSlotEnd, f.SectionName, weekdayNameRu(f.Weekday),
			f.RoomName, shortTeacher(f.TeacherFIO))
		if len(t.SignedLessonIDs) > 0 {
			fmt.Fprintf(&b, "   уже записывались на id: %s\n", joinInt64(t.SignedLessonIDs))
		}
	}
	send(chatID, strings.TrimSpace(b.String()))
}

func handleRecurringRemove(chatID int64, st *store.DB, args []string, send func(int64, string)) {
	if len(args) < 1 {
		send(chatID, "Использование: /remove <номер из /list>")
		return
	}
	n, err := strconv.Atoi(strings.TrimSpace(args[0]))
	if err != nil || n < 1 {
		send(chatID, "Некорректный номер.")
		return
	}
	if err := st.DeleteRecurringTemplateByOneBasedIndex(chatID, n); err != nil {
		send(chatID, err.Error())
		return
	}
	send(chatID, "Удалено.")
}

func handleLead(chatID int64, st *store.DB, args []string, send func(int64, string)) {
	if len(args) == 0 {
		h, err := st.MinLeadHours(chatID, recurring.DefaultMinLeadHours)
		if err != nil {
			send(chatID, err.Error())
			return
		}
		var b strings.Builder
		fmt.Fprintf(&b, "Минимальный запас до начала пары для автозаписи: %d ч.\n", h)
		if h == 0 {
			b.WriteString("Ограничение отключено (запись вплотную к началу, если API разрешает).\n")
		}
		b.WriteString("Задать: /lead ЧАСЫ (целое 0…720; 0 = без ограничения).")
		send(chatID, strings.TrimSpace(b.String()))
		return
	}
	h, err := strconv.Atoi(strings.TrimSpace(args[0]))
	if err != nil {
		send(chatID, "Нужно целое число часов, например: /lead 48")
		return
	}
	if err := st.SetMinLeadHours(chatID, h); err != nil {
		send(chatID, err.Error())
		return
	}
	if h == 0 {
		send(chatID, "Сохранено: ограничение по запасу времени до пары отключено.")
		return
	}
	send(chatID, fmt.Sprintf("Сохранено: автозапись только если до начала пары не меньше %d ч.", h))
}

func handleLink(ctx context.Context, app *config.App, st *store.DB, w *recurring.Worker, send func(int64, string), shared *http.Client, chatID int64, token string, tgUsername string) {
	vctx, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()
	cli := myitmo.NewClient("verify", app.TokenURL, app.ClientID, token, shared)
	if err := cli.EnsureAccessToken(vctx); err != nil {
		send(chatID, "Токен не принят (проверьте refresh_token): "+err.Error())
		return
	}
	prio := st.UserPriorityOrDefault(chatID, defaultNewUserPriority)
	if err := st.UpsertUser(chatID, "", prio, &token); err != nil {
		send(chatID, "БД: "+err.Error())
		return
	}
	if tgUsername != "" {
		_ = st.UpdateTelegramUsername(chatID, tgUsername)
	}
	w.InvalidateClient(chatID)
	send(chatID, "ITMO привязан. /help — команды. Приоритет по умолчанию 100 (меньше — раньше в очереди); меняет админ.")
}

func clientForChat(ctx context.Context, st *store.DB, app *config.App, shared *http.Client, chatID int64) (*myitmo.Client, error) {
	tok, err := st.RefreshToken(chatID)
	if err != nil || tok == "" {
		return nil, fmt.Errorf("нет токена")
	}
	cli := myitmo.NewClient("tg", app.TokenURL, app.ClientID, tok, shared)
	return cli, nil
}

func clientForStore(st *store.DB, u store.User, app *config.App, shared *http.Client) (*myitmo.Client, error) {
	tok, err := st.RefreshToken(u.TelegramChatID)
	if err != nil || tok == "" {
		return nil, fmt.Errorf("нет токена")
	}
	name := u.DisplayName
	if name == "" {
		name = "tg"
	}
	return myitmo.NewClient(name, app.TokenURL, app.ClientID, tok, shared), nil
}

func sqliteDBPath(configPath, fromConfig string) string {
	if p := os.Getenv("PE_SQLITE"); p != "" {
		return p
	}
	if fromConfig != "" {
		return fromConfig
	}
	return filepath.Join(filepath.Dir(configPath), "pewatch.sqlite")
}

func helpText() string {
	return strings.TrimSpace(`/schedule ДАТА — расписание YYYY-MM-DD
/add <lesson_id> [дата] — шаблон автозаписи
/list — список шаблонов
/remove <номер> — удалить шаблон (номер из /list)
/lead [часы] — минимальный запас до начала пары для автозаписи (по умолчанию 36; 0 = без ограничения)
/link ТОКЕН — ITMO (my.itmo → DevTools → Network → refresh_token)
/help — это сообщение`)
}

func adminHelpText() string {
	return strings.TrimSpace(`/admin users
/admin setpriority <chat_id> <число>
/admin help`)
}

func recurringStorePath(configPath string) string {
	if p := os.Getenv("PE_RECURRING_STORE"); p != "" {
		return p
	}
	dir := filepath.Dir(configPath)
	return filepath.Join(dir, "recurring_templates.json")
}

func mergedScheduleForTemplate(ctx context.Context, client *myitmo.Client, configBids []int64, hintDate string) ([]byte, error) {
	loc, err := time.LoadLocation("Europe/Moscow")
	if err != nil {
		loc = time.UTC
	}
	now := time.Now().In(loc)
	var start, end time.Time
	if hintDate != "" {
		t, err := time.ParseInLocation("2006-01-02", hintDate, loc)
		if err != nil {
			return nil, fmt.Errorf("дата: %w", err)
		}
		start = t.AddDate(0, 0, -10)
		end = t.AddDate(0, 0, 35)
	} else {
		start = now
		end = now.AddDate(0, 0, 42)
	}
	sctx, cancel := context.WithTimeout(ctx, 180*time.Second)
	defer cancel()
	allBids := schedule.UnionBuildingIDs(configBids)
	if raw, err := client.ScheduleFilters(sctx); err == nil {
		if fb, err := schedule.ParseFilterBuildingIDs(raw); err == nil && len(fb) > 0 {
			allBids = schedule.UnionBuildingIDs(configBids, fb)
		}
	}
	parts, failed := schedule.FetchBuildingSchedulesRange(sctx, client, start.Format("2006-01-02"), end.Format("2006-01-02"), allBids, 10)
	if len(parts) == 0 {
		return nil, fmt.Errorf("расписание не загрузилось (ошибки по корпусам: %s)", joinInt64(failed))
	}
	return schedule.MergeSchedules(parts)
}

func formatNewRecurringTemplate(t recurring.Template) string {
	f := t.Fingerprint
	bid := ""
	if f.BuildingID > 0 {
		bid = fmt.Sprintf(" · корпус %d", f.BuildingID)
	}
	return fmt.Sprintf(strings.TrimSpace(`
Шаблон сохранён id=%s
Эталон lesson_id=%d · запись откроется в 00:00 МСК за 14 дней до дня занятия.
Совпадение: %s–%s, %s, %s, %s, день недели=%s%s
`),
		t.ID, t.SourceLessonID,
		f.TimeSlotStart, f.TimeSlotEnd, f.SectionName, f.RoomName, shortTeacher(f.TeacherFIO), weekdayNameRu(f.Weekday), bid)
}

func shortTeacher(fio string) string {
	fio = strings.TrimSpace(fio)
	if i := strings.IndexByte(fio, ' '); i > 0 {
		return fio[:i]
	}
	return fio
}

func truncateForMessage(s string, max int) string {
	s = strings.TrimSpace(s)
	if max <= 0 || len(s) <= max {
		return s
	}
	return s[:max] + "…"
}

func weekdayNameRu(wd int) string {
	names := []string{"вс", "пн", "вт", "ср", "чт", "пт", "сб"}
	if wd >= 0 && wd < len(names) {
		return names[wd]
	}
	return "?"
}

func joinInt64(ids []int64) string {
	var b strings.Builder
	for i, id := range ids {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(strconv.FormatInt(id, 10))
	}
	return b.String()
}

func sharedHTTPClient() *http.Client {
	tr := http.DefaultTransport.(*http.Transport).Clone()
	tr.MaxIdleConns = 32
	tr.MaxIdleConnsPerHost = 32
	tr.IdleConnTimeout = 90 * time.Second
	return &http.Client{Timeout: 25 * time.Second, Transport: tr}
}
