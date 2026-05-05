package model

import "time"

type ChatSession struct {
	ID        string    `bson:"_id"`
	UserID    string    `bson:"user_id"`
	CreatedAt time.Time `bson:"created_at"`
	UpdatedAt time.Time `bson:"updated_at"`
}
