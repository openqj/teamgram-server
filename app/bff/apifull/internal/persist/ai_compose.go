package persist

import (
	"encoding/json"
	"fmt"
)

// AITone is the durable representation used by the Layer 229 aicompose
// methods. The APIFull PostgreSQL store owns these rows; the in-memory and
// legacy blob paths below exist only for unit tests and transitional callers.
type AITone struct {
	ID            int64  `json:"id"`
	Title         string `json:"title,omitempty"`
	Prompt        string `json:"prompt,omitempty"`
	Tone          string `json:"tone,omitempty"`
	Slug          string `json:"slug,omitempty"`
	EmojiID       int64  `json:"emoji_id,omitempty"`
	AccessHash    int64  `json:"access_hash,omitempty"`
	Creator       bool   `json:"creator,omitempty"`
	Saved         bool   `json:"saved,omitempty"`
	DisplayAuthor bool   `json:"display_author,omitempty"`
}

func aiToneKey(userID int64) string {
	return fmt.Sprintf("b5:%d:", userID)
}

// LoadAITones reads a user's tones from the authoritative PostgreSQL table.
// The blob fallback keeps package-level unit tests that intentionally use the
// memory Store working; production APIFull always installs postgresStore.
func LoadAITones(userID int64) ([]AITone, error) {
	if store, ok := Default.(*postgresStore); ok {
		return store.loadAITones(userID)
	}
	return loadAITonesBlob(userID)
}

// MutateAITones applies one read-modify-write operation under a PostgreSQL
// transaction. A per-user advisory lock is required because SELECT FOR UPDATE
// cannot lock an absent user's first row. The callback runs while the lock is
// held, so create, update, save, and delete cannot lose concurrent changes.
func MutateAITones(userID int64, mutate func([]AITone) ([]AITone, error)) ([]AITone, error) {
	if store, ok := Default.(*postgresStore); ok {
		return store.mutateAITones(userID, mutate)
	}
	current, err := loadAITonesBlob(userID)
	if err != nil {
		return nil, err
	}
	next, err := mutate(current)
	if err != nil {
		return nil, err
	}
	if err := saveAITonesBlob(userID, next); err != nil {
		return nil, err
	}
	return next, nil
}

func (s *postgresStore) loadAITones(userID int64) ([]AITone, error) {
	rows, err := s.db.Query(`SELECT id, title, prompt, tone, slug, emoji_id,
		access_hash, creator, saved, display_author
		FROM apifull_ai_compose_tone WHERE user_id = $1 ORDER BY id`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	tones := make([]AITone, 0)
	for rows.Next() {
		var tone AITone
		if err := rows.Scan(&tone.ID, &tone.Title, &tone.Prompt, &tone.Tone, &tone.Slug,
			&tone.EmojiID, &tone.AccessHash, &tone.Creator, &tone.Saved, &tone.DisplayAuthor); err != nil {
			return nil, err
		}
		tones = append(tones, tone)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return tones, nil
}

func (s *postgresStore) mutateAITones(userID int64, mutate func([]AITone) ([]AITone, error)) ([]AITone, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err = tx.Exec(`SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`,
		fmt.Sprintf("aicompose-tone:%d", userID)); err != nil {
		return nil, err
	}
	rows, err := tx.Query(`SELECT id, title, prompt, tone, slug, emoji_id,
		access_hash, creator, saved, display_author
		FROM apifull_ai_compose_tone WHERE user_id = $1 ORDER BY id FOR UPDATE`, userID)
	if err != nil {
		return nil, err
	}
	current := make([]AITone, 0)
	for rows.Next() {
		var tone AITone
		if err = rows.Scan(&tone.ID, &tone.Title, &tone.Prompt, &tone.Tone, &tone.Slug,
			&tone.EmojiID, &tone.AccessHash, &tone.Creator, &tone.Saved, &tone.DisplayAuthor); err != nil {
			_ = rows.Close()
			return nil, err
		}
		current = append(current, tone)
	}
	if err = rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	if err = rows.Close(); err != nil {
		return nil, err
	}
	next, err := mutate(current)
	if err != nil {
		return nil, err
	}
	if _, err = tx.Exec(`DELETE FROM apifull_ai_compose_tone WHERE user_id = $1`, userID); err != nil {
		return nil, err
	}
	for _, tone := range next {
		if _, err = tx.Exec(`INSERT INTO apifull_ai_compose_tone
			(user_id, id, title, prompt, tone, slug, emoji_id, access_hash,
			 creator, saved, display_author)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`,
			userID, tone.ID, tone.Title, tone.Prompt, tone.Tone, tone.Slug,
			tone.EmojiID, tone.AccessHash, tone.Creator, tone.Saved, tone.DisplayAuthor); err != nil {
			return nil, err
		}
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return next, nil
}

func loadAITonesBlob(userID int64) ([]AITone, error) {
	raw, err := Default.Get(aiToneKey(userID))
	if err != nil || raw == "" {
		return nil, err
	}
	if raw[0] != '[' && raw[0] != '{' {
		return []AITone{{ID: 1, Title: raw, Creator: true, AccessHash: 1}}, nil
	}
	var tones []AITone
	if err = json.Unmarshal([]byte(raw), &tones); err != nil {
		return []AITone{{ID: 1, Title: raw, Creator: true, AccessHash: 1}}, nil
	}
	return tones, nil
}

func saveAITonesBlob(userID int64, tones []AITone) error {
	if tones == nil {
		tones = []AITone{}
	}
	raw, err := json.Marshal(tones)
	if err != nil {
		return err
	}
	return Default.Set(aiToneKey(userID), string(raw))
}
