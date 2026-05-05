package model

import "time"

type Step struct {
	ID          string `bson:"id"`
	Title       string `bson:"title"`
	Description string `bson:"description"`
	Result      string `bson:"result,omitempty"`
}

type Plan struct {
	ID        string    `bson:"_id"`
	SessionID string    `bson:"session_id,omitempty"`
	Goal      string    `bson:"goal"`
	Steps     []Step    `bson:"steps"`
	CreatedAt time.Time `bson:"created_at"`
	UpdatedAt time.Time `bson:"updated_at"`
}
