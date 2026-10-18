package persist

import (
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"
)

const botCallbackQueryTTL = 2 * time.Minute

var (
	ErrBotCallbackQueryNotFound = errors.New("apifull: bot callback query not found or expired")
	ErrBotCallbackQueryConflict = errors.New("apifull: bot callback query already exists")
)

// BotCallbackQuery is the durable hand-off between a user's callback click
// and the bot's answerCallbackQuery call.
type BotCallbackQuery struct {
	QueryID      int64              `json:"query_id"`
	BotID        int64              `json:"bot_id"`
	UserID       int64              `json:"user_id"`
	PeerType     int32              `json:"peer_type"`
	PeerID       int64              `json:"peer_id"`
	MessageID    int32              `json:"message_id"`
	ChatInstance int64              `json:"chat_instance"`
	Data         []byte             `json:"data"`
	ExpiresAt    int64              `json:"expires_at"`
	Answer       *BotCallbackAnswer `json:"answer,omitempty"`
}

type BotCallbackAnswer struct {
	Alert     bool   `json:"alert,omitempty"`
	HasURL    bool   `json:"has_url,omitempty"`
	NativeUI  bool   `json:"native_ui,omitempty"`
	Message   string `json:"message,omitempty"`
	URL       string `json:"url,omitempty"`
	CacheTime int32  `json:"cache_time"`
}

func botCallbackQueryKey(queryID int64) string {
	return "bot-callback-query:" + strconv.FormatInt(queryID, 10)
}

func validateBotCallbackQuery(query BotCallbackQuery) error {
	if query.QueryID <= 0 || query.BotID <= 0 || query.UserID <= 0 || query.PeerType <= 0 || query.PeerID <= 0 || query.MessageID <= 0 || len(query.Data) == 0 {
		return errors.New("apifull: invalid bot callback query")
	}
	if query.ExpiresAt <= 0 {
		query.ExpiresAt = time.Now().UTC().Add(botCallbackQueryTTL).Unix()
	}
	return nil
}

func CreateBotCallbackQuery(query BotCallbackQuery) error {
	if query.ExpiresAt <= 0 {
		query.ExpiresAt = time.Now().UTC().Add(botCallbackQueryTTL).Unix()
	}
	if err := validateBotCallbackQuery(query); err != nil {
		return err
	}
	if query.ExpiresAt <= time.Now().UTC().Unix() {
		return ErrBotCallbackQueryNotFound
	}
	return Update(botCallbackQueryKey(query.QueryID), func(raw string) (string, error) {
		if strings.TrimSpace(raw) != "" {
			var existing BotCallbackQuery
			if err := json.Unmarshal([]byte(raw), &existing); err != nil {
				return "", err
			}
			if existing.ExpiresAt > time.Now().UTC().Unix() {
				return "", ErrBotCallbackQueryConflict
			}
		}
		encoded, err := json.Marshal(query)
		if err != nil {
			return "", err
		}
		return string(encoded), nil
	})
}

func FindBotCallbackQuery(queryID int64) (*BotCallbackQuery, error) {
	if queryID <= 0 {
		return nil, ErrBotCallbackQueryNotFound
	}
	raw, err := Default.Get(botCallbackQueryKey(queryID))
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(raw) == "" {
		return nil, ErrBotCallbackQueryNotFound
	}
	var query BotCallbackQuery
	if err := json.Unmarshal([]byte(raw), &query); err != nil {
		return nil, err
	}
	if query.ExpiresAt <= time.Now().UTC().Unix() {
		return nil, ErrBotCallbackQueryNotFound
	}
	query.Data = append([]byte(nil), query.Data...)
	return &query, nil
}

func SetBotCallbackAnswer(botID, queryID int64, answer BotCallbackAnswer) error {
	if botID <= 0 || queryID <= 0 || answer.CacheTime < 0 {
		return ErrBotCallbackQueryNotFound
	}
	return Update(botCallbackQueryKey(queryID), func(raw string) (string, error) {
		if strings.TrimSpace(raw) == "" {
			return "", ErrBotCallbackQueryNotFound
		}
		var query BotCallbackQuery
		if err := json.Unmarshal([]byte(raw), &query); err != nil {
			return "", err
		}
		if query.ExpiresAt <= time.Now().UTC().Unix() || query.BotID != botID {
			return "", ErrBotCallbackQueryNotFound
		}
		answerCopy := answer
		query.Answer = &answerCopy
		encoded, err := json.Marshal(query)
		if err != nil {
			return "", err
		}
		return string(encoded), nil
	})
}
