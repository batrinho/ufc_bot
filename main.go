package main

import (
	"fmt"
	"log"
	"strings"
	"time"

	"ufc_bot/db"
	"ufc_bot/model"
	"ufc_bot/networking"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

var bot *tgbotapi.BotAPI

const botToken = "7440150663:AAHLGdabG0pWnHGDs9vAhLTx4EEv4LXktR0"

func main() {
	db.InitDB("ufc.db")
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
			if update.Message.IsCommand() {
				switch update.Message.Command() {
				case "start":
					showMainMenu(update.Message.Chat.ID)
				default:
					bot.Send(tgbotapi.NewMessage(update.Message.Chat.ID, "❌ Unknown command. Try /start"))
				}
			} else {
				handleUnknownMessage(update.Message.Chat.ID)
			}
		case update.CallbackQuery != nil:
			cb := update.CallbackQuery
			chatID := cb.Message.Chat.ID

			switch {
			case cb.Data == "action_start":
				showMainMenu(chatID)
			case cb.Data == "action_subscribe":
				handleCommand(chatID)
			case cb.Data == "action_view":
				handleViewSubscriptions(chatID)
			case cb.Data == "action_remove":
				handleRemoveSubscription(chatID)
			case strings.HasPrefix(cb.Data, "remove|"):
				fightLabel := strings.Split(cb.Data, "|")[1]
				err := db.RemoveUserSubscription(chatID, fightLabel)
				if err != nil {
					bot.Send(tgbotapi.NewMessage(chatID, "❌ Failed to remove subscription."))
				} else {
					bot.Send(tgbotapi.NewMessage(chatID, "✅ Subscription removed successfully!"))
				}
			default:
				handleCallback(cb)
			}
		}
	}
}

func handleUnknownMessage(chatID int64) {
	msg := tgbotapi.NewMessage(chatID, "🤖 I don't understand that. Please go to main menu.")
	msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("Main Menu", "action_start"),
		),
	)
	bot.Send(msg)
}

func handleCommand(chatID int64) {
	event, err := networking.FetchEventData()
	if err != nil {
		log.Println("Failed to fetch event:", err)
		return
	}
	sendFightSelection(chatID, event)
}

func handleCallback(cb *tgbotapi.CallbackQuery) {
	chatID := cb.Message.Chat.ID
	params := strings.Split(cb.Data, "|")
	if len(params) != 3 {
		log.Println("Malformed callback data:", cb.Data)
		return
	}
	eventID, fightID, label := params[0], params[1], params[2]
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
	status, err := networking.FetchFightStatus(statusURL)
	if err != nil {
		log.Println("Error fetching status during subscription:", err)
		return
	}

	switch status.Type.Name {
	case "STATUS_FINAL":
		bot.Send(tgbotapi.NewMessage(chatID, "❌ This fight is already over."))
		return
	case "STATUS_FIGHTERS_WALKING":
		bot.Send(tgbotapi.NewMessage(chatID, "🚨 Fighters are walking out now!"))
		return
	default:
		if status.Type.Name != "STATUS_SCHEDULED" && status.Type.Name != "STATUS_PREFIGHT" {
			bot.Send(tgbotapi.NewMessage(chatID, "🔥 The fight is happening right now!"))
			return
		}
	}

	db.InsertSubscription(statusURL, label, eventTime)
	db.AddChatSubscription(statusURL, chatID)
	bot.Send(tgbotapi.NewMessage(chatID, "✅ Subscribed!"))
}

func sendFightSelection(chatID int64, event *model.Event) {
	msg := tgbotapi.NewMessage(chatID, fmt.Sprintf("Select a fight: *%s*", event.Name))
	msg.ParseMode = "Markdown"

	type buttonData struct {
		index   int
		label   string
		fightID string
	}

	results := make(chan buttonData, len(event.Fights))
	for i, f := range event.Fights {
		go func(i int, fight model.Fight) {
			results <- buttonData{i, getFightLabel(fight), fight.ID}
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
		due, err := db.GetDueSubscriptions()
		if err != nil {
			log.Println("Failed to fetch due subscriptions:", err)
			time.Sleep(30 * time.Second)
			continue
		}

		for _, sub := range due {
			status, err := networking.FetchFightStatus(sub.URL)
			if err != nil {
				log.Println("Status fetch failed:", err)
				continue
			}
			if status.Type.Name == "STATUS_FIGHTERS_WALKING" {
				chatIDs, err := db.GetChatIDsForURL(sub.URL)
				if err != nil {
					log.Println("Failed to get chat IDs:", err)
					continue
				}
				for _, id := range chatIDs {
					bot.Send(tgbotapi.NewMessage(id, fmt.Sprintf("🚨 Fighters walking out: %s", sub.FightLabel)))
				}
				db.RemoveSubscription(sub.URL)
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

func showMainMenu(chatID int64) {
	markup := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("📅 Select a fight to be notified for", "action_subscribe"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("👀 See all fights I have selected", "action_view"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("❌ Remove a fight", "action_remove"),
		),
	)
	msg := tgbotapi.NewMessage(chatID, "Welcome! What would you like to do?")
	msg.ReplyMarkup = markup
	bot.Send(msg)
}

func handleViewSubscriptions(chatID int64) {
	subs, err := db.GetSubscriptionsForChat(chatID)
	if err != nil {
		bot.Send(tgbotapi.NewMessage(chatID, "Failed to load your subscriptions."))
		return
	}
	if len(subs) == 0 {
		bot.Send(tgbotapi.NewMessage(chatID, "You have no active subscriptions."))
		return
	}
	var msgText strings.Builder
	msgText.WriteString("📌 Your current fight subscriptions:\n")
	for _, s := range subs {
		msgText.WriteString(fmt.Sprintf("- %s at %s UTC\n", s.FightLabel, s.EventTime.Format("02 Jan 15:04")))
	}
	bot.Send(tgbotapi.NewMessage(chatID, msgText.String()))
}

func handleRemoveSubscription(chatID int64) {
	subs, err := db.GetSubscriptionsForChat(chatID)
	if err != nil {
		bot.Send(tgbotapi.NewMessage(chatID, "❌ Failed to load subscriptions."))
		return
	}
	if len(subs) == 0 {
		bot.Send(tgbotapi.NewMessage(chatID, "You have no active subscriptions to remove."))
		return
	}

	var rows [][]tgbotapi.InlineKeyboardButton
	for _, sub := range subs {
		btn := tgbotapi.NewInlineKeyboardButtonData(
			fmt.Sprintf("❌ %s (%s)", sub.FightLabel, sub.EventTime.Format("Jan 2")),
			fmt.Sprintf("remove|%s", sub.FightLabel),
		)
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(btn))
	}

	rows = append(rows, tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("🔙 Cancel", "action_start"),
	))

	msg := tgbotapi.NewMessage(chatID, "Select a subscription to remove:")
	msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(rows...)
	bot.Send(msg)
}
