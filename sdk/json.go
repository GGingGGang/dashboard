package sdk

import (
	"context"
	"time"
)

type JSONRequest struct {
	Source string `json:"source"`
	URL    string `json:"url"`
	Filter string `json:"filter"`
}

type JSONResult struct {
	JSON       string    `json:"json"`
	Text       string    `json:"text"`
	Scalar     bool      `json:"scalar"`
	Count      int       `json:"count"`
	ObservedAt time.Time `json:"observedAt"`
}

type JSONSource interface {
	Provider
	ReadJSON(context.Context, JSONRequest) (JSONResult, error)
}
