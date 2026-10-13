package persist

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// InlineBotQuery is the durable hand-off between messages.getInlineBotResults
// and messages.setInlineBotResults. Results are kept as JSON because the
// input and output TL constructors intentionally have different nested media
// types; the API layer validates and converts those values at the boundary.
type InlineBotQuery struct {
	QueryID       int64              `json:"query_id"`
	BotID         int64              `json:"bot_id"`
	RequesterID   int64              `json:"requester_id"`
	PeerKey       string             `json:"peer_key"`
	PeerType      string             `json:"peer_type,omitempty"`
	HasGeo        bool               `json:"has_geo,omitempty"`
	GeoLat        float64            `json:"geo_lat,omitempty"`
	GeoLong       float64            `json:"geo_long,omitempty"`
	GeoAccuracy   int32              `json:"geo_accuracy,omitempty"`
	Query         string             `json:"query"`
	Offset        string             `json:"offset"`
	Gallery       bool               `json:"gallery"`
	Private       bool               `json:"private"`
	CacheTime     int32              `json:"cache_time"`
	NextOffset    string             `json:"next_offset,omitempty"`
	SwitchPM      *InlineBotSwitchPM `json:"switch_pm,omitempty"`
	SwitchWebview *InlineBotWebView  `json:"switch_webview,omitempty"`
	Results       []json.RawMessage  `json:"results"`
	Ready         bool               `json:"ready"`
	ExpiresAt     int64              `json:"expires_at"`
	UpdatedAt     int64              `json:"updated_at"`
	DeliveryState string             `json:"delivery_state,omitempty"`
	ClaimToken    string             `json:"claim_token,omitempty"`
	LeaseUntil    int64              `json:"lease_until,omitempty"`
	RetryAt       int64              `json:"retry_at,omitempty"`
	LastError     string             `json:"last_error,omitempty"`
	Attempts      int                `json:"attempts,omitempty"`
	DeliveredAt   int64              `json:"delivered_at,omitempty"`
}

// InlineBotSwitchPM and InlineBotWebView are the JSON-safe subset of the
// corresponding MTProto constructors kept with a durable inline query. The
// generated protobuf values contain runtime state that should not be stored.
type InlineBotSwitchPM struct {
	Text       string `json:"text"`
	StartParam string `json:"start_param"`
}

type InlineBotWebView struct {
	Text string `json:"text"`
	URL  string `json:"url"`
}

var (
	ErrInlineQueryNotFound = errors.New("apifull: inline bot query not found or expired")
	ErrInlineQueryConflict = errors.New("apifull: inline bot query already has different results")
)

func inlineBotQueryKey(botID int64) string {
	return "inline-bot-query:" + strconv.FormatInt(botID, 10)
}

// CreateInlineBotQuery allocates a pending query under the per-bot store
// lock. A caller can safely retry with the same query ID without creating a
// duplicate row.
func CreateInlineBotQuery(query InlineBotQuery) error {
	if query.QueryID <= 0 || query.BotID <= 0 || query.RequesterID <= 0 || strings.TrimSpace(query.PeerKey) == "" {
		return errors.New("apifull: invalid inline bot query identity")
	}
	if query.ExpiresAt <= 0 {
		query.ExpiresAt = time.Now().UTC().Add(5 * time.Minute).Unix()
	}
	if query.DeliveryState == "" {
		query.DeliveryState = InlineBotQueryStatePending
	}
	if !validInlineBotQueryDeliveryState(query.DeliveryState) {
		return ErrInlineQueryState
	}
	query.Ready = false
	query.Results = []json.RawMessage{}
	query.UpdatedAt = time.Now().UTC().Unix()
	query.RetryAt = query.UpdatedAt
	query.ClaimToken = ""
	query.LeaseUntil = 0
	query.LastError = ""
	query.Attempts = 0
	query.DeliveredAt = 0
	return Update(inlineBotQueryKey(query.BotID), func(raw string) (string, error) {
		items, err := decodeInlineQueries(raw)
		if err != nil {
			return "", err
		}
		now := time.Now().UTC().Unix()
		kept := items[:0]
		for _, item := range items {
			if item.ExpiresAt > now {
				kept = append(kept, item)
			}
		}
		for _, item := range kept {
			if item.QueryID == query.QueryID {
				if item.BotID != query.BotID || item.RequesterID != query.RequesterID {
					return "", ErrInlineQueryConflict
				}
				return encodeInlineQueries(kept)
			}
		}
		kept = append(kept, query)
		return encodeInlineQueries(kept)
	})
}

// SetInlineBotResults atomically publishes the result set for one pending
// query. A second identical submission is idempotent; a conflicting retry is
// rejected so a bot cannot silently replace a response another worker saw.
func SetInlineBotResults(botID, queryID int64, gallery, private bool, cacheTime int32, nextOffset string, results []json.RawMessage) error {
	return SetInlineBotResultsWithSwitches(botID, queryID, gallery, private, cacheTime, nextOffset, nil, nil, results)
}

// SetInlineBotResultsWithSwitches publishes the full inline result envelope,
// including the optional switch-to-private-chat and Mini App buttons.
func SetInlineBotResultsWithSwitches(botID, queryID int64, gallery, private bool, cacheTime int32, nextOffset string, switchPM *InlineBotSwitchPM, switchWebview *InlineBotWebView, results []json.RawMessage) error {
	if botID <= 0 || queryID <= 0 || cacheTime < 0 || len(results) > 50 {
		return errors.New("apifull: invalid inline bot results")
	}
	if strings.TrimSpace(nextOffset) == "" {
		nextOffset = ""
	}
	return Update(inlineBotQueryKey(botID), func(raw string) (string, error) {
		items, err := decodeInlineQueries(raw)
		if err != nil {
			return "", err
		}
		now := time.Now().UTC().Unix()
		for i := range items {
			item := &items[i]
			if item.ExpiresAt <= now {
				continue
			}
			if item.QueryID != queryID {
				continue
			}
			if item.BotID != botID {
				return "", ErrInlineQueryConflict
			}
			if item.Ready {
				if item.Gallery == gallery && item.Private == private && item.CacheTime == cacheTime &&
					item.NextOffset == nextOffset && inlineSwitchesEqual(item.SwitchPM, switchPM) &&
					inlineWebviewsEqual(item.SwitchWebview, switchWebview) && jsonEqualSlices(item.Results, results) {
					return encodeInlineQueries(items)
				}
				return "", ErrInlineQueryConflict
			}
			item.Gallery = gallery
			item.Private = private
			item.CacheTime = cacheTime
			item.NextOffset = nextOffset
			item.SwitchPM = cloneInlineSwitchPM(switchPM)
			item.SwitchWebview = cloneInlineBotWebView(switchWebview)
			item.Results = cloneRawMessages(results)
			item.Ready = true
			item.UpdatedAt = now
			// Keep pending state available through the advertised cache period,
			// while retaining a bounded upper lifetime for abandoned queries.
			ttl := int64(cacheTime)
			if ttl < 300 {
				ttl = 300
			}
			if ttl > 3600 {
				ttl = 3600
			}
			item.ExpiresAt = now + ttl
			return encodeInlineQueries(items)
		}
		return "", ErrInlineQueryNotFound
	})
}

// SetInlineBotResultsForBot publishes results only within the authenticated
// bot's partition. Query IDs are opaque to clients, but ownership must still
// be enforced if a bot replays another bot's ID.
func SetInlineBotResultsForBot(botID, queryID int64, gallery, private bool, cacheTime int32, nextOffset string, results []json.RawMessage) error {
	return SetInlineBotResults(botID, queryID, gallery, private, cacheTime, nextOffset, results)
}

func SetInlineBotResultsForBotWithSwitches(botID, queryID int64, gallery, private bool, cacheTime int32, nextOffset string, switchPM *InlineBotSwitchPM, switchWebview *InlineBotWebView, results []json.RawMessage) error {
	return SetInlineBotResultsWithSwitches(botID, queryID, gallery, private, cacheTime, nextOffset, switchPM, switchWebview, results)
}

// SetInlineBotResultsByQuery updates the query identified by the Telegram
// query_id. The setInlineBotResults request is sent by the bot and does not
// carry the bot ID, so callers must not use the authenticated user ID as a
// storage partition key.
func SetInlineBotResultsByQuery(queryID int64, gallery, private bool, cacheTime int32, nextOffset string, results []json.RawMessage) error {
	return SetInlineBotResultsByQueryWithSwitches(queryID, gallery, private, cacheTime, nextOffset, nil, nil, results)
}

// SetInlineBotResultsByQueryForBotWithSwitches is the ownership-aware form of
// setInlineBotResults. The TL request carries only query_id, so the caller's
// authenticated bot must be checked against the bot that created the query
// before any result envelope is committed.
func SetInlineBotResultsByQueryForBotWithSwitches(botID, queryID int64, gallery, private bool, cacheTime int32, nextOffset string, switchPM *InlineBotSwitchPM, switchWebview *InlineBotWebView, results []json.RawMessage) error {
	if botID <= 0 {
		return errors.New("apifull: invalid inline bot owner")
	}
	return setInlineBotResultsByQuery(botID, queryID, gallery, private, cacheTime, nextOffset, switchPM, switchWebview, results)
}

// SetInlineBotResultsByQueryWithSwitches updates the query identified by the
// Telegram query_id and retains the complete response envelope.
func SetInlineBotResultsByQueryWithSwitches(queryID int64, gallery, private bool, cacheTime int32, nextOffset string, switchPM *InlineBotSwitchPM, switchWebview *InlineBotWebView, results []json.RawMessage) error {
	return setInlineBotResultsByQuery(0, queryID, gallery, private, cacheTime, nextOffset, switchPM, switchWebview, results)
}

func setInlineBotResultsByQuery(expectedBotID, queryID int64, gallery, private bool, cacheTime int32, nextOffset string, switchPM *InlineBotSwitchPM, switchWebview *InlineBotWebView, results []json.RawMessage) error {
	if queryID <= 0 || cacheTime < 0 || len(results) > 50 {
		return errors.New("apifull: invalid inline bot results")
	}
	if store, ok := Default.(*postgresStore); ok {
		return store.setInlineBotResultsByQuery(expectedBotID, queryID, gallery, private, cacheTime, nextOffset, switchPM, switchWebview, results)
	}
	if store, ok := Default.(*mem); ok {
		store.mu.Lock()
		defer store.mu.Unlock()
		for key, raw := range store.m {
			if !strings.HasPrefix(key, "inline-bot-query:") {
				continue
			}
			items, err := decodeInlineQueries(raw)
			if err != nil {
				return err
			}
			if err := applyInlineBotResultsForBot(items, expectedBotID, queryID, gallery, private, cacheTime, nextOffset, switchPM, switchWebview, results); err != nil {
				if errors.Is(err, ErrInlineQueryNotFound) {
					continue
				}
				return err
			}
			encoded, err := encodeInlineQueries(items)
			if err != nil {
				return err
			}
			store.m[key] = encoded
			return nil
		}
		return ErrInlineQueryNotFound
	}
	return ErrInlineQueryNotFound
}

func (s *postgresStore) setInlineBotResultsByQuery(expectedBotID, queryID int64, gallery, private bool, cacheTime int32, nextOffset string, switchPM *InlineBotSwitchPM, switchWebview *InlineBotWebView, results []json.RawMessage) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	rows, err := tx.Query(`SELECT k, v FROM apifull_kv WHERE k LIKE 'inline-bot-query:%' FOR UPDATE`)
	if err != nil {
		return err
	}
	type row struct{ key, value string }
	var candidates []row
	for rows.Next() {
		var item row
		if err := rows.Scan(&item.key, &item.value); err != nil {
			rows.Close()
			return err
		}
		candidates = append(candidates, item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	found := 0
	for _, candidate := range candidates {
		items, err := decodeInlineQueries(candidate.value)
		if err != nil {
			return err
		}
		if err := applyInlineBotResultsForBot(items, expectedBotID, queryID, gallery, private, cacheTime, nextOffset, switchPM, switchWebview, results); err != nil {
			if errors.Is(err, ErrInlineQueryNotFound) {
				continue
			}
			return err
		}
		found++
		if found > 1 {
			return ErrInlineQueryConflict
		}
		encoded, err := encodeInlineQueries(items)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(`UPDATE apifull_kv SET v = $1 WHERE k = $2`, encoded, candidate.key); err != nil {
			return err
		}
	}
	if found == 0 {
		return ErrInlineQueryNotFound
	}
	return tx.Commit()
}

func applyInlineBotResults(items []InlineBotQuery, queryID int64, gallery, private bool, cacheTime int32, nextOffset string, switchPM *InlineBotSwitchPM, switchWebview *InlineBotWebView, results []json.RawMessage) error {
	return applyInlineBotResultsForBot(items, 0, queryID, gallery, private, cacheTime, nextOffset, switchPM, switchWebview, results)
}

func applyInlineBotResultsForBot(items []InlineBotQuery, expectedBotID, queryID int64, gallery, private bool, cacheTime int32, nextOffset string, switchPM *InlineBotSwitchPM, switchWebview *InlineBotWebView, results []json.RawMessage) error {
	now := time.Now().UTC().Unix()
	for i := range items {
		item := &items[i]
		if item.QueryID != queryID {
			continue
		}
		if expectedBotID > 0 && item.BotID != expectedBotID {
			return ErrInlineQueryConflict
		}
		if item.ExpiresAt <= now {
			return ErrInlineQueryNotFound
		}
		if item.Ready {
			if item.Gallery == gallery && item.Private == private && item.CacheTime == cacheTime &&
				item.NextOffset == nextOffset && inlineSwitchesEqual(item.SwitchPM, switchPM) &&
				inlineWebviewsEqual(item.SwitchWebview, switchWebview) && jsonEqualSlices(item.Results, results) {
				return nil
			}
			return ErrInlineQueryConflict
		}
		item.Gallery = gallery
		item.Private = private
		item.CacheTime = cacheTime
		item.NextOffset = nextOffset
		item.SwitchPM = cloneInlineSwitchPM(switchPM)
		item.SwitchWebview = cloneInlineBotWebView(switchWebview)
		item.Results = cloneRawMessages(results)
		item.Ready = true
		item.UpdatedAt = now
		ttl := int64(cacheTime)
		if ttl < 300 {
			ttl = 300
		}
		if ttl > 3600 {
			ttl = 3600
		}
		item.ExpiresAt = now + ttl
		return nil
	}
	return ErrInlineQueryNotFound
}

func cloneInlineSwitchPM(value *InlineBotSwitchPM) *InlineBotSwitchPM {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func cloneInlineBotWebView(value *InlineBotWebView) *InlineBotWebView {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func inlineSwitchesEqual(left, right *InlineBotSwitchPM) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return left.Text == right.Text && left.StartParam == right.StartParam
}

func inlineWebviewsEqual(left, right *InlineBotWebView) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return left.Text == right.Text && left.URL == right.URL
}

// FindInlineBotQuery returns the newest pending or ready result for a request.
// It is intentionally scoped to requester, bot, query and offset so one
// user's private result cannot leak into another inline session. Returning a
// pending query is important: the bot must receive the same query ID when the
// client retries before the bot has published its results.
func FindInlineBotQuery(botID, requesterID int64, peerKey, query, offset string) (*InlineBotQuery, error) {
	if botID <= 0 || requesterID <= 0 || strings.TrimSpace(peerKey) == "" {
		return nil, errors.New("apifull: invalid inline bot query lookup")
	}
	raw, err := Default.Get(inlineBotQueryKey(botID))
	if err != nil {
		return nil, err
	}
	items, err := decodeInlineQueries(raw)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC().Unix()
	var found *InlineBotQuery
	for i := range items {
		item := items[i]
		if item.ExpiresAt <= now || item.RequesterID != requesterID || item.PeerKey != peerKey || item.Query != query || item.Offset != offset {
			continue
		}
		if found == nil || item.UpdatedAt > found.UpdatedAt {
			copy := item
			copy.Results = cloneRawMessages(item.Results)
			found = &copy
		}
	}
	if found == nil {
		return nil, nil
	}
	return found, nil
}

// FindInlineBotQueryByID resolves one query_id for the requesting user. The
// query_id is intentionally opaque and is not partitioned by the caller's
// bot identity, so the lookup must inspect the PostgreSQL-owned bot
// partitions while retaining requester ownership and expiry checks.
func FindInlineBotQueryByID(queryID, requesterID int64) (*InlineBotQuery, error) {
	if queryID <= 0 || requesterID <= 0 {
		return nil, errors.New("apifull: invalid inline bot query identity")
	}
	now := time.Now().UTC().Unix()
	if store, ok := Default.(*postgresStore); ok {
		rows, err := store.db.Query(`SELECT v FROM apifull_kv WHERE k LIKE 'inline-bot-query:%'`)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		var found *InlineBotQuery
		for rows.Next() {
			var raw string
			if err := rows.Scan(&raw); err != nil {
				return nil, err
			}
			items, err := decodeInlineQueries(raw)
			if err != nil {
				return nil, err
			}
			for i := range items {
				item := items[i]
				if item.QueryID != queryID || item.RequesterID != requesterID || item.ExpiresAt <= now {
					continue
				}
				if found != nil && found.BotID != item.BotID {
					return nil, ErrInlineQueryConflict
				}
				copy := item
				copy.Results = cloneRawMessages(item.Results)
				found = &copy
			}
		}
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return found, nil
	}
	if store, ok := Default.(*mem); ok {
		store.mu.Lock()
		defer store.mu.Unlock()
		var found *InlineBotQuery
		for key, raw := range store.m {
			if !strings.HasPrefix(key, "inline-bot-query:") {
				continue
			}
			items, err := decodeInlineQueries(raw)
			if err != nil {
				return nil, err
			}
			for i := range items {
				item := items[i]
				if item.QueryID != queryID || item.RequesterID != requesterID || item.ExpiresAt <= now {
					continue
				}
				if found != nil && found.BotID != item.BotID {
					return nil, ErrInlineQueryConflict
				}
				copy := item
				copy.Results = cloneRawMessages(item.Results)
				found = &copy
			}
		}
		return found, nil
	}
	return nil, ErrInlineQueryNotFound
}

func decodeInlineQueries(raw string) ([]InlineBotQuery, error) {
	if strings.TrimSpace(raw) == "" {
		return []InlineBotQuery{}, nil
	}
	var items []InlineBotQuery
	if err := json.Unmarshal([]byte(raw), &items); err != nil {
		return nil, fmt.Errorf("apifull: decode inline bot queries: %w", err)
	}
	if items == nil {
		items = []InlineBotQuery{}
	}
	return items, nil
}

func encodeInlineQueries(items []InlineBotQuery) (string, error) {
	if items == nil {
		items = []InlineBotQuery{}
	}
	raw, err := json.Marshal(items)
	if err != nil {
		return "", fmt.Errorf("apifull: encode inline bot queries: %w", err)
	}
	return string(raw), nil
}

func cloneRawMessages(in []json.RawMessage) []json.RawMessage {
	out := make([]json.RawMessage, len(in))
	for i := range in {
		out[i] = append(json.RawMessage(nil), in[i]...)
	}
	return out
}

func jsonEqualSlices(a, b []json.RawMessage) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		var left, right any
		if json.Unmarshal(a[i], &left) != nil || json.Unmarshal(b[i], &right) != nil {
			return string(a[i]) == string(b[i])
		}
		la, _ := json.Marshal(left)
		ra, _ := json.Marshal(right)
		if string(la) != string(ra) {
			return false
		}
	}
	return true
}
