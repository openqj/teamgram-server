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
	Payload            []byte          `json:"payload,omitempty"`
	Currency           string          `json:"currency,omitempty"`
	Amount             int64           `json:"amount,omitempty"`
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
	return createBotPaymentQuery(query, "")
}

// CreateBotPaymentQueryAndDelivery atomically creates a shipping query and its
// durable shipping_query delivery intent. It requires the PostgreSQL path so
// a process restart cannot lose the initial bot update.
func CreateBotPaymentQueryAndDelivery(query BotPaymentQuery) error {
	return createBotPaymentQuery(query, BotPaymentOutboxEventShippingQuery)
}

func createBotPaymentQuery(query BotPaymentQuery, deliveryEvent string) error {
	if query.QueryID <= 0 || query.BotUserID <= 0 || query.BuyerUserID <= 0 || !validBotPaymentStage(query.Stage) {
		return ErrBotPaymentQueryConflict
	}
	if deliveryEvent != "" && !validBotPaymentDeliveryEvent(deliveryEvent, query.Stage) {
		return ErrBotPaymentQueryState
	}
	if len(query.Payload) > 0 {
		var compact bytes.Buffer
		if err := json.Compact(&compact, query.Payload); err != nil {
			return ErrBotPaymentQueryConflict
		}
		query.Payload = compact.Bytes()
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
	if store, ok := botPaymentPostgresStore(); ok {
		return createBotPaymentQueryPostgres(store, query, deliveryEvent)
	}
	if deliveryEvent != "" {
		return ErrBotPaymentOutboxRequiresPostgres
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
// after the payment flow has consumed the shipping answer. An optional
// canonical context replaces Payload in the same transaction, allowing the
// payment flow to persist the selected shipping option and requested info
// before the precheckout callback is delivered. The context is retained by
// the callback outbox for restart-safe delivery.
func AdvanceBotPaymentQuery(queryID, botUserID int64, stage string, contextJSON ...[]byte) error {
	if queryID <= 0 || botUserID <= 0 || stage != BotPaymentStagePrecheckoutPending {
		return ErrBotPaymentQueryState
	}
	var payload []byte
	if len(contextJSON) > 1 {
		return ErrBotPaymentQueryState
	}
	if len(contextJSON) == 1 && len(contextJSON[0]) > 0 {
		var compact bytes.Buffer
		if err := json.Compact(&compact, contextJSON[0]); err != nil {
			return ErrBotPaymentQueryState
		}
		payload = compact.Bytes()
	}
	if store, ok := botPaymentPostgresStore(); ok {
		return advanceBotPaymentQueryPostgres(store, queryID, botUserID, stage, payload, "", 0, false)
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
			if len(payload) > 0 && !jsonEqual(query.Payload, payload) {
				return "", ErrBotPaymentQueryState
			}
			return raw, nil
		}
		if query.Stage != BotPaymentStageShippingAnswered {
			return "", ErrBotPaymentQueryState
		}
		if len(payload) > 0 {
			query.Payload = append(query.Payload[:0], payload...)
		}
		query.Stage = stage
		query.UpdatedAt = time.Now().UTC().Unix()
		return encodeBotPaymentQuery(query)
	})
}

// AdvanceBotPaymentQueryWithContext advances a shipping query and atomically
// records the complete precheckout context plus its precheckout_query delivery
// intent. Payload must be canonical JSON containing the invoice, requested
// information, shipping address and selected option.
func AdvanceBotPaymentQueryWithContext(queryID, botUserID int64, payload []byte, currency string, amount int64) error {
	if queryID <= 0 || botUserID <= 0 || amount < 0 || len(payload) == 0 {
		return ErrBotPaymentQueryState
	}
	var compact bytes.Buffer
	if err := json.Compact(&compact, payload); err != nil {
		return ErrBotPaymentQueryState
	}
	store, ok := botPaymentPostgresStore()
	if !ok {
		return ErrBotPaymentOutboxRequiresPostgres
	}
	return advanceBotPaymentQueryPostgres(store, queryID, botUserID, BotPaymentStagePrecheckoutPending,
		compact.Bytes(), strings.TrimSpace(currency), amount, true)
}

// AdvanceBotPaymentQueryAndDelivery is the explicit name used by payment
// producers when they need the stage transition and precheckout delivery
// intent to commit together.
func AdvanceBotPaymentQueryAndDelivery(queryID, botUserID int64, payload []byte, currency string, amount int64) error {
	return AdvanceBotPaymentQueryWithContext(queryID, botUserID, payload, currency, amount)
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
	if store, ok := botPaymentPostgresStore(); ok {
		return setBotShippingResultsPostgres(store, botUserID, queryID, errorText, options)
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
	if store, ok := botPaymentPostgresStore(); ok {
		return setBotPrecheckoutResultsPostgres(store, botUserID, queryID, success, errorText)
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
	if store, ok := botPaymentPostgresStore(); ok {
		return loadBotPaymentQueryPostgres(store, queryID)
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
