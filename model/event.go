package model

type Url struct {
	Url string `json:"$ref"`
}

type Event struct {
	ID     string  `json:"id"`
	Date   string  `json:"date"`
	Name   string  `json:"name"`
	Fights []Fight `json:"competitions"`
}

type EventURLs struct {
	Urls []Url `json:"items"`
}

type Fight struct {
	ID          string       `json:"id"`
	FighterUrls []FighterUrl `json:"competitors"`
	StatusUrl   Url          `json:"status"`
}

type FighterUrl struct {
	Url Url `json:"athlete"`
}

type Fighter struct {
	Name string `json:"fullName"`
}

type FightStatus struct {
	Type struct {
		Name string `json:"name"`
	} `json:"type"`
}
