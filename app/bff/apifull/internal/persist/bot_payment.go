package persist

import (
	"bytes"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"
)

// Bot payment queries are the durable hand-off between the payment flow and
// the bot's messages.setBot*Results callbacks. The query producer is owned by
// the payment flow; callbacks only mutate an existing, caller-owned query.
const (
	BotPaymentStageShippingPending     = "shipping_pending"
	BotPaymentStageShippingAnswered    = "shipping_answered"
	BotPaymentStagePrecheckoutPending  = "precheckout_pending"
	BotPaymentStagePrecheckoutAnswered = "precheckout_answered"
)

var (
	ErrBotPaymentQueryNotFound = errors.New("apifull: bot payment query not found")
	ErrBotPaymentQueryConflict = errors.New("apifull: bot payment query conflicts with existing query")
	ErrBotPaymentQueryExpired  = errors.New("apifull: bot payment query expired")
	ErrBotPaymentQueryOwner    = errors.New("apifull: bot payment query owner mismatch")
	ErrBotPaymentQueryState    = errors.New("apifull: bot payment query is not awaiting this result")
	ErrBotPaymentQueryCorrupt  = errors.New("apifull: bot payment query state is corrupt")
)

// BotPaymentQuery is intentionally JSON-compatible so the process store can
// retain the exact Layer 229 shipping option payload without a second codec.
// ExpiresAt is a Unix timestamp in UTC seconds.
type BotPaymentQuery struct {
	QueryID            int64           `json:"query_id"`
	BotUserID          int64           `json:"bot_user_id"`
	BuyerUserID        int64           `json:"buyer_user_id"`
	Stage              string          `json:"stage"`
	ShippingError      string          `json:"shipping_error,omitempty"`
	ShippingOptions    json.RawMessage `json:"shipping_options,omitempty"`
	PrecheckoutSuccess *bool           `json:"precheckout_success,omitempty"`
	PrecheckoutError   string          `json:"precheckout_error,omitempty"`
	ExpiresAt          int64           `json:"expires_at"`
	UpdatedAt          int64           `json:"updated_at"`
}

func botPaymentQueryKey(queryID int64) string {
	return "bot-payment-query:" + strconv.FormatInt(queryID, 10)
}

func validBotPaymentStage(stage string) bool {
	switch stage {
	case BotPaymentStageShippingPending, BotPaymentStageShippingAnswered,
		BotPaymentStagePrecheckoutPending, BotPaymentStagePrecheckoutAnswered:
		return true
	default:
		return false
	}
}

// CreateBotPaymentQuery inserts a pending payment query. Replays with the
// same identity are idempotent; a query ID cannot be rebound to another bot or
// buyer. The caller controls the stage transition into precheckout.
func CreateBotPaymentQuery(query BotPaymentQuery) error {
	if query.QueryID <= 0 || query.BotUserID <= 0 || query.BuyerUserID <= 0 || !validBotPaymentStage(query.Stage) {
		return ErrBotPaymentQueryConflict
	}
	if query.ExpiresAt <= 0 {
		query.ExpiresAt = time.Now().UTC().Add(5 * time.Minute).Unix()
	}
	if query.ExpiresAt <= time.Now().UTC().Unix() {
		return ErrBotPaymentQueryExpired
	}
	query.UpdatedAt = time.Now().UTC().Unix()
	if len(query.ShippingOptions) == 0 {
		query.ShippingOptions = json.RawMessage("[]")
	}
	if !json.Valid(query.ShippingOptions) {
		return ErrBotPaymentQueryConflict
	}
	encoded, err := json.Marshal(query)
	if err != nil {
		return err
	}
	return Update(botPaymentQueryKey(query.QueryID), func(raw string) (string, error) {
		if strings.TrimSpace(raw) == "" {
			return string(encoded), nil
		}
		var current BotPaymentQuery
		if err := json.Unmarshal([]byte(raw), &current); err != nil {
			return "", err
		}
		if current.BotUserID != query.BotUserID || current.BuyerUserID != query.BuyerUserID {
			return "", ErrBotPaymentQueryConflict
		}
		return raw, nil
	})
}

// AdvanceBotPaymentQuery moves a shipping query to the precheckout stage
// after the payment flow has consumed the shipping answer. It is kept
// separate from the callback so payment settlement can perform its own
// transactional checks before asking the bot to approve precheckout.
func AdvanceBotPaymentQuery(queryID, botUserID int64, stage string) error {
	if queryID <= 0 || botUserID <= 0 || stage != BotPaymentStagePrecheckoutPending {
		return ErrBotPaymentQueryState
	}
	return Update(botPaymentQueryKey(queryID), func(raw string) (string, error) {
		query, err := decodeBotPaymentQuery(raw)
		if err != nil {
			return "", err
		}
		if query.BotUserID != botUserID {
			return "", ErrBotPaymentQueryOwner
		}
		if query.ExpiresAt <= time.Now().UTC().Unix() {
			return "", ErrBotPaymentQueryExpired
		}
		if query.Stage == BotPaymentStagePrecheckoutPending || query.Stage == BotPaymentStagePrecheckoutAnswered {
			return raw, nil
		}
		if query.Stage != BotPaymentStageShippingAnswered {
			return "", ErrBotPaymentQueryState
		}
		query.Stage = stage
		query.UpdatedAt = time.Now().UTC().Unix()
		return encodeBotPaymentQuery(query)
	})
}

// SetBotShippingResults atomically records a bot's shipping response. A
// repeated identical response is idempotent; conflicting retries are rejected.
func SetBotShippingResults(botUserID, queryID int64, errorText string, options []byte) error {
	if botUserID <= 0 || queryID <= 0 {
		return ErrBotPaymentQueryNotFound
	}
	if len(options) == 0 {
		options = []byte("[]")
	}
	if !json.Valid(options) {
		return ErrBotPaymentQueryState
	}
	var compact bytes.Buffer
	if err := json.Compact(&compact, options); err != nil {
		return ErrBotPaymentQueryState
	}
	options = compact.Bytes()
	return Update(botPaymentQueryKey(queryID), func(raw string) (string, error) {
		query, err := decodeBotPaymentQuery(raw)
		if err != nil {
			return "", err
		}
		if query.BotUserID != botUserID {
			return "", ErrBotPaymentQueryOwner
		}
		if query.ExpiresAt <= time.Now().UTC().Unix() {
			return "", ErrBotPaymentQueryExpired
		}
		if query.Stage == BotPaymentStageShippingAnswered {
			if query.ShippingError == errorText && bytes.Equal(query.ShippingOptions, options) {
				return raw, nil
			}
			return "", ErrBotPaymentQueryState
		}
		if query.Stage != BotPaymentStageShippingPending {
			return "", ErrBotPaymentQueryState
		}
		query.Stage = BotPaymentStageShippingAnswered
		query.ShippingError = errorText
		query.ShippingOptions = append(query.ShippingOptions[:0], options...)
		query.UpdatedAt = time.Now().UTC().Unix()
		return encodeBotPaymentQuery(query)
	})
}

// SetBotPrecheckoutResults atomically records a bot's precheckout response.
func SetBotPrecheckoutResults(botUserID, queryID int64, success bool, errorText string) error {
	if botUserID <= 0 || queryID <= 0 {
		return ErrBotPaymentQueryNotFound
	}
	return Update(botPaymentQueryKey(queryID), func(raw string) (string, error) {
		query, err := decodeBotPaymentQuery(raw)
		if err != nil {
			return "", err
		}
		if query.BotUserID != botUserID {
			return "", ErrBotPaymentQueryOwner
		}
		if query.ExpiresAt <= time.Now().UTC().Unix() {
			return "", ErrBotPaymentQueryExpired
		}
		if query.Stage == BotPaymentStagePrecheckoutAnswered {
			if query.PrecheckoutSuccess != nil && *query.PrecheckoutSuccess == success && query.PrecheckoutError == errorText {
				return raw, nil
			}
			return "", ErrBotPaymentQueryState
		}
		if query.Stage != BotPaymentStagePrecheckoutPending {
			return "", ErrBotPaymentQueryState
		}
		query.Stage = BotPaymentStagePrecheckoutAnswered
		query.PrecheckoutSuccess = &success
		query.PrecheckoutError = errorText
		query.UpdatedAt = time.Now().UTC().Unix()
		return encodeBotPaymentQuery(query)
	})
}

func LoadBotPaymentQuery(queryID int64) (BotPaymentQuery, bool, error) {
	if queryID <= 0 {
		return BotPaymentQuery{}, false, ErrBotPaymentQueryNotFound
	}
	raw, err := Default.Get(botPaymentQueryKey(queryID))
	if err != nil {
		return BotPaymentQuery{}, false, err
	}
	if strings.TrimSpace(raw) == "" {
		return BotPaymentQuery{}, false, nil
	}
	query, err := decodeBotPaymentQuery(raw)
	if err != nil {
		return BotPaymentQuery{}, false, err
	}
	if query.ExpiresAt <= time.Now().UTC().Unix() {
		return BotPaymentQuery{}, false, nil
	}
	return query, true, nil
}

func decodeBotPaymentQuery(raw string) (BotPaymentQuery, error) {
	if strings.TrimSpace(raw) == "" {
		return BotPaymentQuery{}, ErrBotPaymentQueryNotFound
	}
	var query BotPaymentQuery
	if err := json.Unmarshal([]byte(raw), &query); err != nil || query.QueryID <= 0 {
		return BotPaymentQuery{}, ErrBotPaymentQueryCorrupt
	}
	return query, nil
}

func encodeBotPaymentQuery(query BotPaymentQuery) (string, error) {
	encoded, err := json.Marshal(query)
	return string(encoded), err
}
