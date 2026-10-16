// Copyright 2026 Teamgram Authors
// All rights reserved.

package botapi

// PreparedInlineMessage describes an inline result saved for a Mini App user.
type PreparedInlineMessage struct {
	ID             string `json:"id"`
	ExpirationDate int64  `json:"expiration_date"`
}

// PreparedKeyboardButton describes a keyboard button saved for a Mini App
// user.
type PreparedKeyboardButton struct {
	ID string `json:"id"`
}
