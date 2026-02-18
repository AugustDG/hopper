package model

import "time"

type Project struct {
	Path       string    `json:"path"`
	Name       string    `json:"name"`
	LastSeen   time.Time `json:"last_seen"`
	LastOpened time.Time `json:"last_opened,omitempty"`
	OpenCount  int       `json:"open_count,omitempty"`
}
