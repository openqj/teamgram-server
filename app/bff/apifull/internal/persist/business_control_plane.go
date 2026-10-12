package persist

import (
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

// ErrBusinessControlPlaneUnavailable distinguishes an unprovisioned local
// control plane from an empty catalogue. Callers map both cases to the
// canonical provider-unavailable error, but keeping the distinction here
// prevents an in-memory fallback from becoming a production success path.
var ErrBusinessControlPlaneUnavailable = errors.New("apifull: business control plane requires PostgreSQL")

// BusinessControlRecord is a published, caller-scoped result owned by the
// deployment control plane. Result is a canonical Layer 229 constructor
// encoded as JSON. The API layer validates the decoded constructor before it
// reaches a client.
type BusinessControlRecord struct {
	Operation    string
	ScopeKey     string
	CallerUserID int64
	BotUserID    int64
	QueryID      int64
	State        string
	Result       json.RawMessage
	ExpiresAt    *time.Time
}

func businessControlPlaneStore() (*postgresStore, error) {
	store, ok := Default.(*postgresStore)
	if !ok || store == nil || store.db == nil {
		return nil, ErrBusinessControlPlaneUnavailable
	}
	return store, nil
}

func validateBusinessControlRecord(record BusinessControlRecord) error {
	if strings.TrimSpace(record.Operation) == "" || strings.TrimSpace(record.ScopeKey) == "" || record.CallerUserID < 0 {
		return errors.New("apifull: invalid business control-plane identity")
	}
	if len(record.Result) == 0 || !json.Valid(record.Result) || string(record.Result) == "null" {
		return errors.New("apifull: business control-plane result must be JSON")
	}
	if record.State == "" {
		record.State = "published"
	}
	switch record.State {
	case "published", "revoked":
	default:
		return errors.New("apifull: invalid business control-plane state")
	}
	return nil
}

// PutBusinessControlRecord publishes or replaces one explicit result. This
// is intentionally an administrative API; no request path creates a result.
// Replacing a row is atomic and therefore safe for a control-plane reload.
func PutBusinessControlRecord(record BusinessControlRecord) error {
	store, err := businessControlPlaneStore()
	if err != nil {
		return err
	}
	if err = validateBusinessControlRecord(record); err != nil {
		return err
	}
	if record.State == "" {
		record.State = "published"
	}
	_, err = store.db.Exec(`INSERT INTO apifull_business_control_plane
		(operation, scope_key, caller_user_id, bot_user_id, query_id, state, result, expires_at, updated_at)
		VALUES ($1, $2, $3, NULLIF($4, 0), NULLIF($5, 0), $6, $7::jsonb, $8, CURRENT_TIMESTAMP)
		ON CONFLICT (operation, scope_key, caller_user_id) DO UPDATE SET
		bot_user_id = EXCLUDED.bot_user_id, query_id = EXCLUDED.query_id,
		state = EXCLUDED.state, result = EXCLUDED.result, expires_at = EXCLUDED.expires_at,
		updated_at = CURRENT_TIMESTAMP`, record.Operation, record.ScopeKey, record.CallerUserID,
		record.BotUserID, record.QueryID, record.State, record.Result, record.ExpiresAt)
	return err
}

// LoadBusinessControlResult reads an explicit result for callerID, falling
// back to a published global record (caller_user_id=0). Revoked and expired
// rows are never exposed. The returned bytes remain JSON so the caller can
// decode the exact TL type it owns.
func LoadBusinessControlResult(operation, scopeKey string, callerID int64) (json.RawMessage, bool, error) {
	store, err := businessControlPlaneStore()
	if err != nil {
		return nil, false, err
	}
	if strings.TrimSpace(operation) == "" || strings.TrimSpace(scopeKey) == "" || callerID <= 0 {
		return nil, false, errors.New("apifull: invalid business control-plane lookup")
	}
	var raw []byte
	err = store.db.QueryRow(`SELECT result FROM apifull_business_control_plane
		WHERE operation = $1 AND scope_key = $2 AND caller_user_id IN ($3, 0)
			AND state = 'published' AND (expires_at IS NULL OR expires_at > CURRENT_TIMESTAMP)
		ORDER BY CASE WHEN caller_user_id = $3 THEN 0 ELSE 1 END
		LIMIT 1`, operation, scopeKey, callerID).Scan(&raw)
	if err == sql.ErrNoRows {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	if len(raw) == 0 || !json.Valid(raw) || string(raw) == "null" {
		return nil, false, errors.New("apifull: invalid published business control-plane result")
	}
	return json.RawMessage(raw), true, nil
}

// LoadBusinessControlResultForCaller reads only the row published for the
// authenticated caller. It deliberately does not fall back to the global
// caller_user_id=0 row: callback answers, WebView results, and mutations are
// authorization-bearing results and a catalogue row must never satisfy them.
func LoadBusinessControlResultForCaller(operation, scopeKey string, callerID int64) (json.RawMessage, bool, error) {
	store, err := businessControlPlaneStore()
	if err != nil {
		return nil, false, err
	}
	if strings.TrimSpace(operation) == "" || strings.TrimSpace(scopeKey) == "" || callerID <= 0 {
		return nil, false, errors.New("apifull: invalid caller-scoped business control-plane lookup")
	}
	var raw []byte
	err = store.db.QueryRow(`SELECT result FROM apifull_business_control_plane
		WHERE operation = $1 AND scope_key = $2 AND caller_user_id = $3
			AND state = 'published' AND (expires_at IS NULL OR expires_at > CURRENT_TIMESTAMP)
		LIMIT 1`, operation, scopeKey, callerID).Scan(&raw)
	if err == sql.ErrNoRows {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	if len(raw) == 0 || !json.Valid(raw) || string(raw) == "null" {
		return nil, false, errors.New("apifull: invalid published caller-scoped business control-plane result")
	}
	return json.RawMessage(raw), true, nil
}
