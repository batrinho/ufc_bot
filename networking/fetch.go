package networking

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"ufc_bot/model"
)

const espnAPIEndpoint = "http://sports.core.api.espn.com/v2/sports/mma/leagues/ufc/events"

func fetchData[T any](endpoint string, target *T) error {
	resp, err := http.Get(endpoint)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	return json.Unmarshal(body, target)
}

func fetchEventURL() (string, error) {
	var data model.EventURLs
	if err := fetchData(espnAPIEndpoint, &data); err != nil {
		return "", err
	}
	if len(data.Urls) == 0 {
		return "", fmt.Errorf("no events found")
	}
	return data.Urls[0].Url, nil
}

func FetchEventData() (*model.Event, error) {
	var event model.Event
	eventURL, err := fetchEventURL()
	if err != nil {
		return nil, err
	}
	if err := fetchData(eventURL, &event); err != nil {
		return nil, err
	}
	return &event, nil
}

func FetchEventByID(eventID string) (*model.Event, error) {
	url := fmt.Sprintf("http://sports.core.api.espn.com/v2/sports/mma/leagues/ufc/events/%s", eventID)
	var data model.Event
	if err := fetchData(url, &data); err != nil {
		return nil, err
	}
	return &data, nil
}

func FetchFighterName(url string) (string, error) {
	var fighter model.Fighter
	if err := fetchData(url, &fighter); err != nil {
		return "", err
	}
	return fighter.Name, nil
}

func FetchFightStatus(url string) (*model.FightStatus, error) {
	var status model.FightStatus
	if err := fetchData(url, &status); err != nil {
		return nil, err
	}
	return &status, nil
}
