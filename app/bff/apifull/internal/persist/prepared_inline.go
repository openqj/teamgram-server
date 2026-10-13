package persist

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// PreparedInlineMessage is the durable representation of a prepared inline
// result. Result and peer types stay as JSON so their full Layer 229
// constructors survive a process restart without a lossy hand-written model.
type PreparedInlineMessage struct {
	ID           string            `json:"id"`
	BotID        int64             `json:"bot_id"`
	TargetUserID int64             `json:"target_user_id,omitempty"`
	Result       json.RawMessage   `json:"result"`
	PeerTypes    []json.RawMessage `json:"peer_types"`
	CacheTime    int32             `json:"cache_time"`
	ExpiresAt    int64             `json:"expires_at"`
	UpdatedAt    int64             `json:"updated_at"`
}

var (
	ErrPreparedInlineNotFound = errors.New("apifull: prepared inline message not found")
	ErrPreparedInlineConflict = errors.New("apifull: prepared inline message conflicts with existing result")
)

func preparedInlineKey(botID int64, id string) string {
	return "prepared-inline:" + strconv.FormatInt(botID, 10) + ":" + id
}

func SavePreparedInlineMessage(message PreparedInlineMessage) error {
	if message.BotID <= 0 || strings.TrimSpace(message.ID) == "" || len(message.Result) == 0 || !json.Valid(message.Result) {
		return errors.New("apifull: invalid prepared inline message")
	}
	if len(message.PeerTypes) > 16 {
		return errors.New("apifull: too many prepared inline peer types")
	}
	for _, peerType := range message.PeerTypes {
		if len(peerType) == 0 || !json.Valid(peerType) {
			return errors.New("apifull: invalid prepared inline peer type")
		}
	}
	now := time.Now().UTC().Unix()
	if message.ExpiresAt <= now {
		return errors.New("apifull: prepared inline message is already expired")
	}
	if message.CacheTime < 0 {
		return errors.New("apifull: invalid prepared inline cache time")
	}
	message.Result = append(json.RawMessage(nil), message.Result...)
	message.PeerTypes = cloneRawMessages(message.PeerTypes)
	message.UpdatedAt = now
	return Update(preparedInlineKey(message.BotID, message.ID), func(raw string) (string, error) {
		if strings.TrimSpace(raw) != "" {
			var existing PreparedInlineMessage
			if err := json.Unmarshal([]byte(raw), &existing); err != nil {
				return "", fmt.Errorf("apifull: decode prepared inline message: %w", err)
			}
			if existing.BotID != message.BotID || existing.ID != message.ID {
				return "", ErrPreparedInlineConflict
			}
			if string(existing.Result) != string(message.Result) || !jsonEqualSlices(existing.PeerTypes, message.PeerTypes) ||
				existing.TargetUserID != message.TargetUserID || existing.CacheTime != message.CacheTime {
				return "", ErrPreparedInlineConflict
			}
			return raw, nil
		}
		encoded, err := json.Marshal(message)
		if err != nil {
			return "", err
		}
		return string(encoded), nil
	})
}

func LoadPreparedInlineMessage(botID int64, id string) (*PreparedInlineMessage, error) {
	if botID <= 0 || strings.TrimSpace(id) == "" {
		return nil, ErrPreparedInlineNotFound
	}
	raw, err := Default.Get(preparedInlineKey(botID, id))
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(raw) == "" {
		return nil, ErrPreparedInlineNotFound
	}
	var message PreparedInlineMessage
	if err = json.Unmarshal([]byte(raw), &message); err != nil || message.ID != id || message.BotID != botID || len(message.Result) == 0 || !json.Valid(message.Result) {
		return nil, ErrPreparedInlineNotFound
	}
	if message.ExpiresAt <= time.Now().UTC().Unix() {
		return nil, ErrPreparedInlineNotFound
	}
	message.Result = append(json.RawMessage(nil), message.Result...)
	message.PeerTypes = cloneRawMessages(message.PeerTypes)
	return &message, nil
}
