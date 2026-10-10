package persist

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
)

// EmojiKeywordRecord is the PostgreSQL representation of one Layer 229
// emoji keyword.  The catalog is provider-owned; APIFull only serves rows.
type EmojiKeywordRecord struct {
	LangCode  string
	Keyword   string
	Emoticons []string
}

type EmojiLanguageRecord struct {
	LangCode string
	Version  int32
	URL      string
}

// EmojiDocumentRecord contains the media fields needed to construct a
// custom-emoji Document constructor without consulting the legacy KV store.
type EmojiDocumentRecord struct {
	ID         int64
	AccessHash int64
	Date       int32
	MimeType   string
	SizeBytes  int64
	DCID       int32
	FileRef    []byte
	Alt        string
	SetID      int64
}

type EmojiGroupRecord struct {
	Kind        string
	Title       string
	IconEmojiID int64
	Emoticons   []string
}

// CollectibleEmojiStatusRecord is the provider-owned representation of one
// collectible emoji status available to a user. The status document itself is
// stored in apifull_emoji_document; this table carries the ownership and the
// collectible metadata that is not part of a normal custom emoji document.
type CollectibleEmojiStatusRecord struct {
	OwnerUserID       int64
	CollectibleID     int64
	DocumentID        int64
	Title             string
	Slug              string
	PatternDocumentID int64
	CenterColor       int32
	EdgeColor         int32
	PatternColor      int32
	TextColor         int32
}

func ensureEmojiSchema(db *sql.DB) error {
	for _, stmt := range []string{
		`CREATE TABLE IF NOT EXISTS apifull_emoji_language (
 lang_code TEXT PRIMARY KEY, version INTEGER NOT NULL DEFAULT 1,
 url TEXT NOT NULL DEFAULT '', updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP)`,
		`CREATE TABLE IF NOT EXISTS apifull_emoji_keyword (
 lang_code TEXT NOT NULL REFERENCES apifull_emoji_language(lang_code) ON DELETE CASCADE,
 keyword TEXT NOT NULL, emoticons JSONB NOT NULL DEFAULT '[]'::jsonb,
 PRIMARY KEY (lang_code, keyword))`,
		`CREATE INDEX IF NOT EXISTS apifull_emoji_keyword_lang_idx ON apifull_emoji_keyword (lang_code, keyword)`,
		`CREATE TABLE IF NOT EXISTS apifull_emoji_document (
 id BIGINT PRIMARY KEY, access_hash BIGINT NOT NULL, date INTEGER NOT NULL DEFAULT 0,
 mime_type TEXT NOT NULL DEFAULT 'application/x-tgsticker', size_bytes BIGINT NOT NULL DEFAULT 0,
 dc_id INTEGER NOT NULL DEFAULT 2, file_reference BYTEA NOT NULL DEFAULT ''::bytea,
 alt TEXT NOT NULL DEFAULT '', set_id BIGINT NOT NULL DEFAULT 0,
 featured BOOLEAN NOT NULL DEFAULT FALSE, profile BOOLEAN NOT NULL DEFAULT FALSE,
			status BOOLEAN NOT NULL DEFAULT FALSE, group_photo BOOLEAN NOT NULL DEFAULT FALSE,
			background BOOLEAN NOT NULL DEFAULT FALSE, channel_status BOOLEAN NOT NULL DEFAULT FALSE,
			restricted_status BOOLEAN NOT NULL DEFAULT FALSE,
			updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP)`,
		`ALTER TABLE apifull_emoji_document ADD COLUMN IF NOT EXISTS group_photo BOOLEAN NOT NULL DEFAULT FALSE`,
		`ALTER TABLE apifull_emoji_document ADD COLUMN IF NOT EXISTS background BOOLEAN NOT NULL DEFAULT FALSE`,
		`ALTER TABLE apifull_emoji_document ADD COLUMN IF NOT EXISTS channel_status BOOLEAN NOT NULL DEFAULT FALSE`,
		`ALTER TABLE apifull_emoji_document ADD COLUMN IF NOT EXISTS restricted_status BOOLEAN NOT NULL DEFAULT FALSE`,
		`CREATE INDEX IF NOT EXISTS apifull_emoji_document_alt_idx ON apifull_emoji_document (alt, id)`,
		`CREATE TABLE IF NOT EXISTS apifull_collectible_emoji_status (
 owner_user_id BIGINT NOT NULL, collectible_id BIGINT NOT NULL CHECK (collectible_id > 0),
 document_id BIGINT NOT NULL CHECK (document_id > 0), title TEXT NOT NULL DEFAULT '',
 slug TEXT NOT NULL DEFAULT '', pattern_document_id BIGINT NOT NULL DEFAULT 0,
 center_color INTEGER NOT NULL DEFAULT 0, edge_color INTEGER NOT NULL DEFAULT 0,
 pattern_color INTEGER NOT NULL DEFAULT 0, text_color INTEGER NOT NULL DEFAULT 0,
 updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
 PRIMARY KEY (owner_user_id, collectible_id),
 UNIQUE (owner_user_id, document_id))`,
		`CREATE INDEX IF NOT EXISTS apifull_collectible_emoji_status_owner_idx
 ON apifull_collectible_emoji_status (owner_user_id, collectible_id)`,
		`CREATE TABLE IF NOT EXISTS apifull_emoji_group (
 kind TEXT NOT NULL, title TEXT NOT NULL, icon_emoji_id BIGINT NOT NULL DEFAULT 0,
 emoticons JSONB NOT NULL DEFAULT '[]'::jsonb, position INTEGER NOT NULL DEFAULT 0,
 PRIMARY KEY (kind, title))`,
		`CREATE INDEX IF NOT EXISTS apifull_emoji_group_order_idx ON apifull_emoji_group (kind, position, title)`,
		// Keep a deterministic baseline language available on a fresh database;
		// deployments may add provider languages and replace the URL/version.
		`INSERT INTO apifull_emoji_language (lang_code, version, url) VALUES ('en', 1, 'https://telegram.org/emoji') ON CONFLICT (lang_code) DO NOTHING`,
	} {
		if _, err := db.Exec(stmt); err != nil {
			return err
		}
	}
	return nil
}

func emojiDB() (*sql.DB, error) {
	store, ok := Default.(*postgresStore)
	if !ok || store == nil || store.db == nil {
		return nil, ErrStickerProviderUnavailable
	}
	return store.db, nil
}

func loadEmojiKeywords(ctx context.Context, langCode string) ([]EmojiKeywordRecord, int32, error) {
	db, err := emojiDB()
	if err != nil {
		return nil, 0, err
	}
	langCode = strings.TrimSpace(langCode)
	if langCode == "" {
		langCode = "en"
	}
	var version int32
	if err = db.QueryRowContext(ctx, `SELECT version FROM apifull_emoji_language WHERE lang_code=$1`, langCode).Scan(&version); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return []EmojiKeywordRecord{}, 0, nil
		}
		return nil, 0, err
	}
	rows, err := db.QueryContext(ctx, `SELECT keyword, emoticons FROM apifull_emoji_keyword WHERE lang_code=$1 ORDER BY keyword`, langCode)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	keywords := make([]EmojiKeywordRecord, 0)
	for rows.Next() {
		var row EmojiKeywordRecord
		var raw []byte
		if err = rows.Scan(&row.Keyword, &raw); err != nil {
			return nil, 0, err
		}
		if len(raw) != 0 && string(raw) != "null" && !json.Valid(raw) {
			return nil, 0, errors.New("apifull: invalid emoji keyword emoticons json")
		}
		if len(raw) != 0 {
			_ = json.Unmarshal(raw, &row.Emoticons)
		}
		if row.Emoticons == nil {
			row.Emoticons = []string{}
		}
		row.LangCode = langCode
		keywords = append(keywords, row)
	}
	return keywords, version, rows.Err()
}

func loadEmojiLanguages(ctx context.Context, requested []string) ([]EmojiLanguageRecord, error) {
	db, err := emojiDB()
	if err != nil {
		return nil, err
	}
	seen := make(map[string]struct{}, len(requested))
	out := make([]EmojiLanguageRecord, 0, len(requested))
	for _, code := range requested {
		code = strings.TrimSpace(code)
		if code == "" {
			continue
		}
		if _, ok := seen[code]; ok {
			continue
		}
		seen[code] = struct{}{}
		var row EmojiLanguageRecord
		if err = db.QueryRowContext(ctx, `SELECT lang_code, version, url FROM apifull_emoji_language WHERE lang_code=$1`, code).Scan(&row.LangCode, &row.Version, &row.URL); err == sql.ErrNoRows {
			continue
		} else if err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, nil
}

func loadEmojiURL(ctx context.Context, langCode string) (string, error) {
	db, err := emojiDB()
	if err != nil {
		return "", err
	}
	var url string
	err = db.QueryRowContext(ctx, `SELECT url FROM apifull_emoji_language WHERE lang_code=$1`, strings.TrimSpace(langCode)).Scan(&url)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return url, err
}

func loadCustomEmojiDocuments(ctx context.Context, ids []int64) ([]EmojiDocumentRecord, error) {
	db, err := emojiDB()
	if err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return []EmojiDocumentRecord{}, nil
	}
	rows, err := db.QueryContext(ctx, `SELECT id, access_hash, date, mime_type, size_bytes, dc_id, file_reference, alt, set_id
 FROM apifull_emoji_document WHERE id = ANY($1::bigint[]) ORDER BY array_position($1::bigint[], id)`, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]EmojiDocumentRecord, 0, len(ids))
	for rows.Next() {
		var row EmojiDocumentRecord
		if err = rows.Scan(&row.ID, &row.AccessHash, &row.Date, &row.MimeType, &row.SizeBytes, &row.DCID, &row.FileRef, &row.Alt, &row.SetID); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func searchCustomEmojiDocuments(ctx context.Context, emoticon string) ([]EmojiDocumentRecord, error) {
	db, err := emojiDB()
	if err != nil {
		return nil, err
	}
	emoticon = strings.TrimSpace(emoticon)
	rows, err := db.QueryContext(ctx, `SELECT id, access_hash, date, mime_type, size_bytes, dc_id, file_reference, alt, set_id
 FROM apifull_emoji_document WHERE ($1='' OR alt=$1) ORDER BY id LIMIT 100`, emoticon)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]EmojiDocumentRecord, 0)
	for rows.Next() {
		var row EmojiDocumentRecord
		if err = rows.Scan(&row.ID, &row.AccessHash, &row.Date, &row.MimeType, &row.SizeBytes, &row.DCID, &row.FileRef, &row.Alt, &row.SetID); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func loadEmojiGroups(ctx context.Context, kind string) ([]EmojiGroupRecord, error) {
	db, err := emojiDB()
	if err != nil {
		return nil, err
	}
	rows, err := db.QueryContext(ctx, `SELECT title, icon_emoji_id, emoticons FROM apifull_emoji_group WHERE kind=$1 ORDER BY position, title`, kind)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]EmojiGroupRecord, 0)
	for rows.Next() {
		var row EmojiGroupRecord
		var raw []byte
		if err = rows.Scan(&row.Title, &row.IconEmojiID, &raw); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(raw, &row.Emoticons)
		if row.Emoticons == nil {
			row.Emoticons = []string{}
		}
		row.Kind = kind
		out = append(out, row)
	}
	return out, rows.Err()
}

func LoadEmojiKeywords(ctx context.Context, langCode string) ([]EmojiKeywordRecord, int32, error) {
	return loadEmojiKeywords(ctx, langCode)
}

func LoadEmojiLanguages(ctx context.Context, requested []string) ([]EmojiLanguageRecord, error) {
	return loadEmojiLanguages(ctx, requested)
}

func LoadEmojiURL(ctx context.Context, langCode string) (string, error) {
	return loadEmojiURL(ctx, langCode)
}

func LoadCustomEmojiDocuments(ctx context.Context, ids []int64) ([]EmojiDocumentRecord, error) {
	return loadCustomEmojiDocuments(ctx, ids)
}

func SearchCustomEmojiDocuments(ctx context.Context, emoticon string) ([]EmojiDocumentRecord, error) {
	return searchCustomEmojiDocuments(ctx, emoticon)
}

func LoadEmojiDocumentIDs(ctx context.Context, kind string) ([]int64, error) {
	db, err := emojiDB()
	if err != nil {
		return nil, err
	}
	column := map[string]string{
		"profile": "profile", "status": "status", "group": "group_photo", "featured": "featured",
		"background": "background", "channel_status": "channel_status", "restricted_status": "restricted_status",
	}[kind]
	if column == "" {
		return []int64{}, nil
	}
	rows, err := db.QueryContext(ctx, `SELECT id FROM apifull_emoji_document WHERE `+column+` ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := make([]int64, 0)
	for rows.Next() {
		var id int64
		if err = rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// LoadCollectibleEmojiStatuses reads only the provider-owned statuses for the
// authenticated user. An empty result is intentionally preserved so callers
// can fail closed when no collectible provider has populated the table.
func LoadCollectibleEmojiStatuses(ctx context.Context, ownerUserID int64) ([]CollectibleEmojiStatusRecord, error) {
	db, err := emojiDB()
	if err != nil {
		return nil, err
	}
	if ownerUserID <= 0 {
		return nil, errors.New("apifull: invalid collectible emoji owner")
	}
	rows, err := db.QueryContext(ctx, `SELECT owner_user_id, collectible_id, document_id,
 title, slug, pattern_document_id, center_color, edge_color, pattern_color, text_color
 FROM apifull_collectible_emoji_status WHERE owner_user_id=$1 ORDER BY collectible_id`, ownerUserID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]CollectibleEmojiStatusRecord, 0)
	for rows.Next() {
		var row CollectibleEmojiStatusRecord
		if err = rows.Scan(&row.OwnerUserID, &row.CollectibleID, &row.DocumentID,
			&row.Title, &row.Slug, &row.PatternDocumentID, &row.CenterColor,
			&row.EdgeColor, &row.PatternColor, &row.TextColor); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

// EmojiDocumentInCatalog reports whether a document is explicitly published
// for the requested catalog. Callers use this to reject arbitrary document
// IDs at mutation boundaries.
func EmojiDocumentInCatalog(ctx context.Context, id int64, kind string) (bool, error) {
	if id <= 0 {
		return false, nil
	}
	db, err := emojiDB()
	if err != nil {
		return false, err
	}
	column := map[string]string{"status": "status", "channel_status": "channel_status"}[kind]
	if column == "" {
		return false, errors.New("apifull: invalid emoji catalog")
	}
	var found bool
	err = db.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM apifull_emoji_document WHERE id=$1 AND `+column+`)`, id).Scan(&found)
	return found, err
}

func LoadEmojiGroups(ctx context.Context, kind string) ([]EmojiGroupRecord, error) {
	return loadEmojiGroups(ctx, kind)
}
