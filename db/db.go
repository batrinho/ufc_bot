package db

import (
	"database/sql"
	"log"
	"time"

	"ufc_bot/model"

	_ "github.com/mattn/go-sqlite3"
)

var DB *sql.DB

func InitDB(path string) {
	var err error
	DB, err = sql.Open("sqlite3", path)
	if err != nil {
		log.Fatal("Failed to open DB:", err)
	}

	createTables := `
	CREATE TABLE IF NOT EXISTS subscriptions (
		url TEXT PRIMARY KEY,
		event_time TEXT,
		fight_label TEXT
	);

	CREATE TABLE IF NOT EXISTS chat_subscriptions (
		url TEXT,
		chat_id INTEGER,
		PRIMARY KEY (url, chat_id),
		FOREIGN KEY (url) REFERENCES subscriptions(url) ON DELETE CASCADE
	);
	`

	_, err = DB.Exec(createTables)
	if err != nil {
		log.Fatal("Failed to create tables:", err)
	}
}

func InsertSubscription(url, fightLabel string, eventTime time.Time) error {
	_, err := DB.Exec(`INSERT OR IGNORE INTO subscriptions(url, event_time, fight_label) VALUES (?, ?, ?)`, url, eventTime.Format(time.RFC3339), fightLabel)
	return err
}

func AddChatSubscription(url string, chatID int64) error {
	_, err := DB.Exec(`INSERT OR IGNORE INTO chat_subscriptions(url, chat_id) VALUES (?, ?)`, url, chatID)
	return err
}

func GetDueSubscriptions() ([]struct {
	URL        string
	FightLabel string
}, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	rows, err := DB.Query(`
		SELECT url, fight_label FROM subscriptions
		WHERE event_time <= ?
	`, now)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []struct {
		URL        string
		FightLabel string
	}
	for rows.Next() {
		var url, label string
		if err := rows.Scan(&url, &label); err != nil {
			return nil, err
		}
		result = append(result, struct {
			URL        string
			FightLabel string
		}{url, label})
	}
	return result, nil
}

func GetChatIDsForURL(url string) ([]int64, error) {
	rows, err := DB.Query(`SELECT chat_id FROM chat_subscriptions WHERE url = ?`, url)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, nil
}

func RemoveSubscription(url string) error {
	_, err := DB.Exec(`DELETE FROM subscriptions WHERE url = ?`, url)
	return err
}

func GetSubscriptionsForChat(chatID int64) ([]model.Subscription, error) {
	rows, err := DB.Query(`
		SELECT subscriptions.fight_label, subscriptions.event_time 
		FROM subscriptions
		JOIN chat_subscriptions ON subscriptions.url = chat_subscriptions.url
		WHERE chat_subscriptions.chat_id = ?
	`, chatID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var subs []model.Subscription
	for rows.Next() {
		var s model.Subscription
		var timeStr string
		if err := rows.Scan(&s.FightLabel, &timeStr); err != nil {
			return nil, err
		}
		s.EventTime, err = time.Parse(time.RFC3339, timeStr)
		if err != nil {
			return nil, err
		}
		subs = append(subs, s)
	}
	return subs, nil
}

func RemoveUserSubscription(chatID int64, fightLabel string) error {
	_, err := DB.Exec(`
		DELETE FROM chat_subscriptions 
		WHERE chat_id = ? 
		AND url IN (SELECT url FROM subscriptions WHERE fight_label = ?)
	`, chatID, fightLabel)
	return err
}

func CleanupSubscriptions() error {
	_, err := DB.Exec(`
		DELETE FROM subscriptions 
		WHERE url NOT IN (SELECT url FROM chat_subscriptions)
	`)
	return err
}
