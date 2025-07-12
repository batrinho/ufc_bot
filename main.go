package main

import (
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"ufc_bot/model"
	"ufc_bot/networking"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

var (
	bot               *tgbotapi.BotAPI
	subscriptions     = make(map[string]*Subscription)
	subscriptionsLock sync.RWMutex
)

const botToken = "7440150663:AAHLGdabG0pWnHGDs9vAhLTx4EEv4LXktR0"

type Subscription struct {
	ChatIDs    map[int64]bool
	EventTime  time.Time
	FightLabel string
}

func main() {
	initBot()
	go pollSubscriptions()
	handleUpdates()
}

func initBot() {
	var err error
	bot, err = tgbotapi.NewBotAPI(botToken)
	if err != nil {
		log.Fatal("Bot init failed:", err)
	}
	log.Println("Bot running as:", bot.Self.UserName)
}

func handleUpdates() {
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 30
	updates := bot.GetUpdatesChan(u)

	for update := range updates {
		switch {
		case update.Message != nil:
			handleMessage(update.Message)
		case update.CallbackQuery != nil:
			handleCallback(update.CallbackQuery)
		}
	}
}

func handleMessage(msg *tgbotapi.Message) {
	chatID := msg.Chat.ID
	switch msg.Text {
	case "/start":
		showMainMenu(chatID)
	default:
		bot.Send(tgbotapi.NewMessage(chatID, "Unknown command. Use /start"))
	}
}

func showMainMenu(chatID int64) {
	markup := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("📅 Select a fight to be notified for", "action_subscribe"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("👀 See all fights I have selected", "action_view"),
		),
	)
	msg := tgbotapi.NewMessage(chatID, "Welcome! What would you like to do?")
	msg.ReplyMarkup = markup
	bot.Send(msg)
}

func handleCallback(cb *tgbotapi.CallbackQuery) {
	chatID := cb.Message.Chat.ID
	data := cb.Data

	switch {
	case data == "action_subscribe":
		event, err := networking.FetchEventData()
		if err != nil {
			log.Println("Failed to fetch event:", err)
			return
		}
		sendFightSelection(chatID, event)

	case data == "action_view":
		showUserSubscriptions(chatID)

	case strings.Count(data, "|") == 2:
		parts := strings.Split(data, "|")
		eventID, fightID, label := parts[0], parts[1], parts[2]
		event, err := networking.FetchEventByID(eventID)
		if err != nil {
			log.Println("Event fetch error:", err)
			return
		}
		eventTime, err := parseEventTime(event.Date)
		if err != nil {
			log.Println("Time parse error:", err)
			return
		}
		statusURL := buildStatusURL(eventID, fightID)

		subscriptionsLock.Lock()
		if subscriptions[statusURL] == nil {
			subscriptions[statusURL] = &Subscription{
				ChatIDs:    map[int64]bool{},
				EventTime:  eventTime,
				FightLabel: label,
			}
		}
		subscriptions[statusURL].ChatIDs[chatID] = true
		subscriptionsLock.Unlock()

		bot.Send(tgbotapi.NewMessage(chatID, "✅ Subscribed"))

	default:
		log.Println("Unknown callback:", data)
	}

	bot.Request(tgbotapi.NewCallback(cb.ID, ""))
}

func showUserSubscriptions(chatID int64) {
	subscriptionsLock.RLock()
	defer subscriptionsLock.RUnlock()

	var lines []string
	for _, sub := range subscriptions {
		if sub.ChatIDs[chatID] {
			t := sub.EventTime.Format("Jan 02 15:04 MST")
			lines = append(lines, fmt.Sprintf("• %s at %s", sub.FightLabel, t))
		}
	}
	if len(lines) == 0 {
		bot.Send(tgbotapi.NewMessage(chatID, "😔 You haven’t subscribed to any fights."))
		return
	}

	msg := fmt.Sprintf("📋 Your Subscriptions:\n%s", strings.Join(lines, "\n"))
	bot.Send(tgbotapi.NewMessage(chatID, msg))
}

func sendFightSelection(chatID int64, event *model.Event) {
	msg := tgbotapi.NewMessage(chatID, fmt.Sprintf("Select a fight: *%s*", event.Name))
	msg.ParseMode = "Markdown"

	type btn struct {
		index   int
		label   string
		fightID string
	}
	results := make(chan btn, len(event.Fights))

	for i, f := range event.Fights {
		go func(i int, fight model.Fight) {
			results <- btn{i, getFightLabel(fight), fight.ID}
		}(i, f)
	}

	rows := make([][]tgbotapi.InlineKeyboardButton, len(event.Fights))
	for i := 0; i < len(event.Fights); i++ {
		b := <-results
		data := fmt.Sprintf("%s|%s|%s", event.ID, b.fightID, b.label)
		rows[b.index] = tgbotapi.NewInlineKeyboardRow(tgbotapi.NewInlineKeyboardButtonData(b.label, data))
	}

	for i, j := 0, len(rows)-1; i < j; i, j = i+1, j-1 {
		rows[i], rows[j] = rows[j], rows[i]
	}

	msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(rows...)
	bot.Send(msg)
}

func getFightLabel(fight model.Fight) string {
	if len(fight.FighterUrls) < 2 {
		return "Unknown Fight"
	}

	type nameResult struct {
		name string
		err  error
	}
	ch := make(chan nameResult, 2)

	for _, url := range []string{fight.FighterUrls[0].Url.Url, fight.FighterUrls[1].Url.Url} {
		go func(url string) {
			name, err := networking.FetchFighterName(url)
			ch <- nameResult{name, err}
		}(url)
	}
	a := <-ch
	b := <-ch
	if a.err != nil || b.err != nil {
		return "Fight"
	}
	return fmt.Sprintf("%s vs %s", a.name, b.name)
}

func pollSubscriptions() {
	for {
		now := time.Now().UTC()
		var active [][2]string

		subscriptionsLock.RLock()
		for url, sub := range subscriptions {
			if now.After(sub.EventTime) {
				active = append(active, [2]string{url, sub.FightLabel})
			}
		}
		subscriptionsLock.RUnlock()

		for _, entry := range active {
			url, label := entry[0], entry[1]
			status, err := networking.FetchFightStatus(url)
			if err != nil {
				log.Println("Status fetch failed:", err)
				continue
			}

			if status.Type.Name == "STATUS_FIGHTERS_WALKING" {
				subscriptionsLock.RLock()
				for chatID := range subscriptions[url].ChatIDs {
					msg := tgbotapi.NewMessage(chatID, fmt.Sprintf("🚨 Fighters walking out: %s", label))
					bot.Send(msg)
				}
				subscriptionsLock.RUnlock()

				subscriptionsLock.Lock()
				delete(subscriptions, url)
				subscriptionsLock.Unlock()
			}
		}
		time.Sleep(30 * time.Second)
	}
}

func parseEventTime(raw string) (time.Time, error) {
	layout := "2006-01-02T15:04Z"
	return time.Parse(layout, raw)
}

func buildStatusURL(eventID, fightID string) string {
	return fmt.Sprintf("http://sports.core.api.espn.com/v2/sports/mma/leagues/ufc/events/%s/competitions/%s/status?lang=en&region=us", eventID, fightID)
}
