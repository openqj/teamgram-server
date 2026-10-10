package persist

// PostgreSQL backed sticker catalog and per-user state.  Sticker RPCs must not
// use the process KV store in production: catalog rows and user mutations are
// kept in one database transaction so restart and concurrent requests preserve
// Telegram's ordering and idempotency semantics.

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

var ErrStickerProviderUnavailable = errors.New("sticker provider requires PostgreSQL")
var ErrStickerSetOwnerMismatch = errors.New("sticker set owner mismatch")
var ErrStickerDocumentAccessMismatch = errors.New("sticker document access hash mismatch")
var ErrStickerDocumentNotFound = errors.New("sticker document not found")

// ensureStickerSchema is used only by the application-owned OpenPostgres
// path (tests and local bootstrap). Production uses 031_apifull_stickers.sql
// and enters through OpenPostgresReadOnly, which preflights these relations.
func ensureStickerSchema(db *sql.DB) error {
	for _, stmt := range []string{
		`CREATE SEQUENCE IF NOT EXISTS apifull_sticker_set_id_seq AS BIGINT`,
		`CREATE SEQUENCE IF NOT EXISTS apifull_sticker_id_seq AS BIGINT`,
		`CREATE TABLE IF NOT EXISTS apifull_sticker_set (
 id BIGINT PRIMARY KEY DEFAULT nextval('apifull_sticker_set_id_seq'), access_hash BIGINT NOT NULL,
 owner_user_id BIGINT NOT NULL, title TEXT NOT NULL, short_name TEXT NOT NULL UNIQUE,
 masks BOOLEAN NOT NULL DEFAULT FALSE, emojis BOOLEAN NOT NULL DEFAULT FALSE,
 text_color BOOLEAN NOT NULL DEFAULT FALSE, animated BOOLEAN NOT NULL DEFAULT FALSE,
 videos BOOLEAN NOT NULL DEFAULT FALSE, creator BOOLEAN NOT NULL DEFAULT FALSE,
 archived BOOLEAN NOT NULL DEFAULT FALSE, featured BOOLEAN NOT NULL DEFAULT FALSE,
 thumb_document_id BIGINT NOT NULL DEFAULT 0, created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
 updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP)`,
		`CREATE TABLE IF NOT EXISTS apifull_sticker (
 id BIGINT PRIMARY KEY DEFAULT nextval('apifull_sticker_id_seq'), set_id BIGINT NOT NULL REFERENCES apifull_sticker_set(id) ON DELETE CASCADE,
 access_hash BIGINT NOT NULL, position INTEGER NOT NULL DEFAULT 0, alt TEXT NOT NULL DEFAULT '', keywords TEXT NOT NULL DEFAULT '',
 mask_coords JSONB, mime_type TEXT NOT NULL DEFAULT 'image/webp', size_bytes BIGINT NOT NULL DEFAULT 0,
 dc_id INTEGER NOT NULL DEFAULT 2, file_reference BYTEA NOT NULL DEFAULT ''::bytea,
 created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP, UNIQUE (set_id, position))`,
		`CREATE TABLE IF NOT EXISTS apifull_sticker_user_set (
 user_id BIGINT NOT NULL, set_id BIGINT NOT NULL REFERENCES apifull_sticker_set(id) ON DELETE CASCADE,
 installed BOOLEAN NOT NULL DEFAULT TRUE, archived BOOLEAN NOT NULL DEFAULT FALSE, unread BOOLEAN NOT NULL DEFAULT FALSE,
 order_index INTEGER NOT NULL DEFAULT 0, updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP, PRIMARY KEY (user_id, set_id))`,
		`CREATE TABLE IF NOT EXISTS apifull_sticker_user_recent (
 user_id BIGINT NOT NULL, sticker_id BIGINT NOT NULL REFERENCES apifull_sticker(id) ON DELETE CASCADE,
 attached BOOLEAN NOT NULL DEFAULT FALSE, saved_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
 PRIMARY KEY (user_id, sticker_id, attached))`,
		`CREATE TABLE IF NOT EXISTS apifull_sticker_user_favourite (
 user_id BIGINT NOT NULL, sticker_id BIGINT NOT NULL REFERENCES apifull_sticker(id) ON DELETE CASCADE,
 saved_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP, PRIMARY KEY (user_id, sticker_id))`,
	} {
		if _, err := db.Exec(stmt); err != nil {
			return err
		}
	}
	return nil
}

type StickerDocumentInput struct {
	ID         int64
	AccessHash int64
	Alt        string
	Keywords   string
	MimeType   string
	SizeBytes  int64
	DCID       int32
	FileRef    []byte
}

type StickerDocument struct {
	ID            int64
	AccessHash    int64
	SetID         int64
	SetAccessHash int64
	Position      int32
	Alt           string
	Keywords      string
	MimeType      string
	SizeBytes     int64
	DCID          int32
	FileRef       []byte
	Date          int32
}

type StickerSet struct {
	ID              int64
	AccessHash      int64
	OwnerUserID     int64
	Title           string
	ShortName       string
	Masks           bool
	Emojis          bool
	TextColor       bool
	Animated        bool
	Videos          bool
	Creator         bool
	Archived        bool
	Featured        bool
	ThumbDocumentID int64
	Installed       bool
	Unread          bool
	OrderIndex      int32
	Documents       []StickerDocument
}

type StickerUserSet struct {
	SetID      int64
	Installed  bool
	Archived   bool
	Unread     bool
	OrderIndex int32
}

func stickerDB() (*sql.DB, error) {
	store, ok := Default.(*postgresStore)
	if !ok || store == nil || store.db == nil {
		return nil, ErrStickerProviderUnavailable
	}
	return store.db, nil
}

func StickerProviderReady() bool {
	_, err := stickerDB()
	return err == nil
}

func normalizeStickerLimit(limit int32) int32 {
	if limit <= 0 || limit > 100 {
		return 100
	}
	return limit
}

func normalizeStickerName(name string) string {
	return strings.TrimSpace(name)
}

func lockStickerUserTx(ctx context.Context, tx *sql.Tx, userID int64) error {
	_, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, fmt.Sprintf("apifull-sticker-user:%d", userID))
	return err
}

func scanStickerSet(row interface{ Scan(...any) error }) (StickerSet, error) {
	var set StickerSet
	err := row.Scan(&set.ID, &set.AccessHash, &set.OwnerUserID, &set.Title,
		&set.ShortName, &set.Masks, &set.Emojis, &set.TextColor, &set.Animated,
		&set.Videos, &set.Creator, &set.Archived, &set.Featured,
		&set.ThumbDocumentID, &set.Installed, &set.Unread, &set.OrderIndex)
	return set, err
}

const stickerSetColumns = `s.id, s.access_hash, s.owner_user_id, s.title,
 s.short_name, s.masks, s.emojis, s.text_color, s.animated, s.videos,
 s.creator, COALESCE(us.archived, s.archived), s.featured, s.thumb_document_id,
 COALESCE(us.installed, FALSE), COALESCE(us.unread, s.featured), COALESCE(us.order_index, 0)`

type stickerQueryer interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

func loadStickerDocuments(ctx context.Context, q stickerQueryer, setID int64) ([]StickerDocument, error) {
	rows, err := q.QueryContext(ctx, `SELECT s.id, s.access_hash, s.set_id, ss.access_hash, s.position, s.alt,
 s.keywords, s.mime_type, s.size_bytes, s.dc_id, s.file_reference,
 EXTRACT(EPOCH FROM s.created_at)::bigint FROM apifull_sticker s
	JOIN apifull_sticker_set ss ON ss.id=s.set_id
 WHERE s.set_id=$1 ORDER BY s.position, s.id`, setID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	docs := make([]StickerDocument, 0)
	for rows.Next() {
		var d StickerDocument
		var date int64
		if err = rows.Scan(&d.ID, &d.AccessHash, &d.SetID, &d.SetAccessHash, &d.Position, &d.Alt,
			&d.Keywords, &d.MimeType, &d.SizeBytes, &d.DCID, &d.FileRef, &date); err != nil {
			return nil, err
		}
		d.Date = int32(date)
		docs = append(docs, d)
	}
	return docs, rows.Err()
}

func loadStickerSetTx(ctx context.Context, tx *sql.Tx, userID, setID int64, shortName string) (*StickerSet, error) {
	where := "s.id = $2"
	var arg any = setID
	if setID == 0 {
		where, arg = "s.short_name = $2", shortName
	}
	row := tx.QueryRowContext(ctx, `SELECT `+stickerSetColumns+`
 FROM apifull_sticker_set s
 LEFT JOIN apifull_sticker_user_set us ON us.set_id=s.id AND us.user_id=$1
 WHERE `+where, userID, arg)
	set, err := scanStickerSet(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	set.Documents, err = loadStickerDocuments(ctx, tx, set.ID)
	if err != nil {
		return nil, err
	}
	return &set, nil
}

func GetStickerSet(ctx context.Context, userID, setID int64, shortName string) (*StickerSet, error) {
	db, err := stickerDB()
	if err != nil {
		return nil, err
	}
	tx, err := db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	set, err := loadStickerSetTx(ctx, tx, userID, setID, normalizeStickerName(shortName))
	if err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return set, nil
}

func ListStickerSets(ctx context.Context, userID int64, installedOnly, archivedOnly, featuredOnly bool, limit int32) ([]StickerSet, error) {
	db, err := stickerDB()
	if err != nil {
		return nil, err
	}
	limit = normalizeStickerLimit(limit)
	query := `SELECT ` + stickerSetColumns + ` FROM apifull_sticker_set s
 LEFT JOIN apifull_sticker_user_set us ON us.set_id=s.id AND us.user_id=$1 WHERE 1=1`
	args := []any{userID}
	if installedOnly {
		query += ` AND COALESCE(us.installed,FALSE)`
	}
	if archivedOnly {
		query += ` AND COALESCE(us.archived,FALSE)`
	}
	if featuredOnly {
		query += ` AND s.featured`
	}
	query += ` ORDER BY COALESCE(us.order_index, 2147483647), s.id LIMIT $2`
	args = append(args, limit)
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	sets := make([]StickerSet, 0, limit)
	for rows.Next() {
		set, scanErr := scanStickerSet(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		sets = append(sets, set)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	if err = rows.Close(); err != nil {
		return nil, err
	}
	for i := range sets {
		sets[i].Documents, err = loadStickerDocuments(ctx, db, sets[i].ID)
		if err != nil {
			return nil, err
		}
	}
	return sets, nil
}

func ListFeaturedStickerSets(ctx context.Context, userID int64, offset, limit int32) ([]StickerSet, int64, error) {
	db, err := stickerDB()
	if err != nil {
		return nil, 0, err
	}
	limit = normalizeStickerLimit(limit)
	tx, err := db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return nil, 0, err
	}
	defer tx.Rollback()

	var total int64
	if err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM apifull_sticker_set WHERE featured`).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := tx.QueryContext(ctx, `SELECT `+stickerSetColumns+` FROM apifull_sticker_set s
 LEFT JOIN apifull_sticker_user_set us ON us.set_id=s.id AND us.user_id=$1
 WHERE s.featured
 ORDER BY COALESCE(us.order_index, 2147483647), s.id LIMIT $2 OFFSET $3`, userID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	sets := make([]StickerSet, 0, limit)
	for rows.Next() {
		set, scanErr := scanStickerSet(rows)
		if scanErr != nil {
			rows.Close()
			return nil, 0, scanErr
		}
		sets = append(sets, set)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return nil, 0, err
	}
	if err = rows.Close(); err != nil {
		return nil, 0, err
	}
	for i := range sets {
		sets[i].Documents, err = loadStickerDocuments(ctx, tx, sets[i].ID)
		if err != nil {
			return nil, 0, err
		}
	}
	if err = tx.Commit(); err != nil {
		return nil, 0, err
	}
	return sets, total, nil
}

func SearchStickerSets(ctx context.Context, userID int64, queryText string, emojiOnly bool, limit int32) ([]StickerSet, error) {
	db, err := stickerDB()
	if err != nil {
		return nil, err
	}
	limit = normalizeStickerLimit(limit)
	query := `SELECT ` + stickerSetColumns + ` FROM apifull_sticker_set s
 LEFT JOIN apifull_sticker_user_set us ON us.set_id=s.id AND us.user_id=$1
 WHERE (lower(s.title) LIKE lower($2) OR lower(s.short_name) LIKE lower($2))`
	args := []any{userID, "%" + queryText + "%"}
	if emojiOnly {
		query += ` AND s.emojis`
	}
	query += ` ORDER BY s.id LIMIT $3`
	args = append(args, limit)
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	sets := make([]StickerSet, 0, limit)
	for rows.Next() {
		set, scanErr := scanStickerSet(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		sets = append(sets, set)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	if err = rows.Close(); err != nil {
		return nil, err
	}
	for i := range sets {
		sets[i].Documents, err = loadStickerDocuments(ctx, db, sets[i].ID)
		if err != nil {
			return nil, err
		}
	}
	return sets, nil
}

func CreateStickerSet(ctx context.Context, ownerID int64, title, shortName string, masks, emojis, textColor, animated, videos bool, docs []StickerDocumentInput) (*StickerSet, error) {
	db, err := stickerDB()
	if err != nil {
		return nil, err
	}
	title, shortName = strings.TrimSpace(title), normalizeStickerName(shortName)
	if ownerID <= 0 || title == "" || shortName == "" {
		return nil, errors.New("sticker set owner, title, and short name are required")
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var set StickerSet
	set.Creator, set.OwnerUserID = true, ownerID
	set.Masks, set.Emojis, set.TextColor = masks, emojis, textColor
	set.Animated, set.Videos, set.Title, set.ShortName = animated, videos, title, shortName
	set.AccessHash = ownerID ^ int64(time.Now().UnixNano())
	err = tx.QueryRowContext(ctx, `INSERT INTO apifull_sticker_set
 (access_hash, owner_user_id, title, short_name, masks, emojis, text_color, animated, videos, creator)
 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,TRUE) RETURNING id`, set.AccessHash, ownerID,
		title, shortName, masks, emojis, textColor, animated, videos).Scan(&set.ID)
	if err != nil {
		return nil, err
	}
	for i, input := range docs {
		if input.ID <= 0 {
			input.ID = 0
		}
		if input.AccessHash == 0 {
			input.AccessHash = input.ID
		}
		if input.MimeType == "" {
			input.MimeType = "image/webp"
		}
		if input.DCID == 0 {
			input.DCID = 2
		}
		if input.FileRef == nil {
			input.FileRef = []byte{}
		}
		var doc StickerDocument
		err = tx.QueryRowContext(ctx, `INSERT INTO apifull_sticker
 (id, set_id, access_hash, position, alt, keywords, mime_type, size_bytes, dc_id, file_reference)
 VALUES (CASE WHEN $1=0 THEN nextval('apifull_sticker_id_seq') ELSE $1 END,
 $2,$3,$4,$5,$6,$7,$8,$9,$10) RETURNING id`, input.ID, set.ID,
			input.AccessHash, i, input.Alt, input.Keywords, input.MimeType,
			input.SizeBytes, input.DCID, input.FileRef).Scan(&doc.ID)
		if err != nil {
			return nil, err
		}
		doc.SetID, doc.SetAccessHash, doc.Position, doc.AccessHash, doc.Alt, doc.Keywords = set.ID, set.AccessHash, int32(i), input.AccessHash, input.Alt, input.Keywords
		doc.MimeType, doc.SizeBytes, doc.DCID, doc.FileRef, doc.Date = input.MimeType, input.SizeBytes, input.DCID, input.FileRef, int32(time.Now().Unix())
		set.Documents = append(set.Documents, doc)
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return &set, nil
}

func mutateStickerSet(ctx context.Context, setID, ownerID int64, fn func(*sql.Tx) error) (*StickerSet, error) {
	db, err := stickerDB()
	if err != nil {
		return nil, err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var actualOwner int64
	if err = tx.QueryRowContext(ctx, `SELECT owner_user_id FROM apifull_sticker_set WHERE id=$1 FOR UPDATE`, setID).Scan(&actualOwner); err == sql.ErrNoRows {
		return nil, sql.ErrNoRows
	} else if err != nil {
		return nil, err
	}
	if actualOwner != ownerID {
		return nil, ErrStickerSetOwnerMismatch
	}
	if err = fn(tx); err != nil {
		return nil, err
	}
	set, err := loadStickerSetTx(ctx, tx, ownerID, setID, "")
	if err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return set, nil
}

func UpdateStickerSetTitle(ctx context.Context, ownerID, setID int64, title string) (*StickerSet, error) {
	return mutateStickerSet(ctx, setID, ownerID, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `UPDATE apifull_sticker_set SET title=$1, updated_at=CURRENT_TIMESTAMP WHERE id=$2`, strings.TrimSpace(title), setID)
		return err
	})
}

func DeleteStickerSet(ctx context.Context, ownerID, setID int64) error {
	db, err := stickerDB()
	if err != nil {
		return err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `DELETE FROM apifull_sticker_set WHERE id=$1 AND owner_user_id=$2`, setID, ownerID)
	if err != nil {
		return err
	}
	if count, e := result.RowsAffected(); e != nil {
		return e
	} else if count == 0 {
		return sql.ErrNoRows
	}
	return tx.Commit()
}

func SetStickerSetThumb(ctx context.Context, ownerID, setID, documentID int64) (*StickerSet, error) {
	return mutateStickerSet(ctx, setID, ownerID, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `UPDATE apifull_sticker_set SET thumb_document_id=$1, updated_at=CURRENT_TIMESTAMP WHERE id=$2`, documentID, setID)
		return err
	})
}

func AddSticker(ctx context.Context, ownerID, setID int64, input StickerDocumentInput) (*StickerSet, error) {
	return mutateStickerSet(ctx, setID, ownerID, func(tx *sql.Tx) error {
		var next int32
		if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(position)+1,0) FROM apifull_sticker WHERE set_id=$1`, setID).Scan(&next); err != nil {
			return err
		}
		if input.MimeType == "" {
			input.MimeType = "image/webp"
		}
		if input.DCID == 0 {
			input.DCID = 2
		}
		if input.FileRef == nil {
			input.FileRef = []byte{}
		}
		_, err := tx.ExecContext(ctx, `INSERT INTO apifull_sticker
 (id,set_id,access_hash,position,alt,keywords,mime_type,size_bytes,dc_id,file_reference)
 VALUES (CASE WHEN $1=0 THEN nextval('apifull_sticker_id_seq') ELSE $1 END,$2,$3,$4,$5,$6,$7,$8,$9,$10)`,
			input.ID, setID, input.AccessHash, next, input.Alt, input.Keywords, input.MimeType,
			input.SizeBytes, input.DCID, input.FileRef)
		return err
	})
}

func RemoveSticker(ctx context.Context, ownerID, stickerID int64) (*StickerSet, error) {
	db, err := stickerDB()
	if err != nil {
		return nil, err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var setID int64
	var oldPosition int32
	if err = tx.QueryRowContext(ctx, `SELECT set_id, position FROM apifull_sticker WHERE id=$1 FOR UPDATE`, stickerID).Scan(&setID, &oldPosition); err != nil {
		return nil, err
	}
	var owner int64
	if err = tx.QueryRowContext(ctx, `SELECT owner_user_id FROM apifull_sticker_set WHERE id=$1`, setID).Scan(&owner); err != nil || owner != ownerID {
		if err == nil {
			err = ErrStickerSetOwnerMismatch
		}
		return nil, err
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM apifull_sticker WHERE id=$1`, stickerID); err != nil {
		return nil, err
	}
	// Move the affected rows out of the unique (set_id, position) range before
	// compacting them. PostgreSQL checks the unique constraint per statement.
	if _, err = tx.ExecContext(ctx, `UPDATE apifull_sticker SET position=-position-1 WHERE set_id=$1 AND position>$2`, setID, oldPosition); err != nil {
		return nil, err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE apifull_sticker SET position=-position-2 WHERE set_id=$1 AND position<0`, setID); err != nil {
		return nil, err
	}
	set, err := loadStickerSetTx(ctx, tx, ownerID, setID, "")
	if err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return set, nil
}

func MoveSticker(ctx context.Context, ownerID, stickerID int64, position int32) (*StickerSet, error) {
	return mutateStickerForSet(ctx, ownerID, stickerID, func(tx *sql.Tx, setID int64) error {
		rows, err := tx.QueryContext(ctx, `SELECT id FROM apifull_sticker WHERE set_id=$1 ORDER BY position, id`, setID)
		if err != nil {
			return err
		}
		defer rows.Close()
		ordered := make([]int64, 0)
		for rows.Next() {
			var id int64
			if err = rows.Scan(&id); err != nil {
				return err
			}
			ordered = append(ordered, id)
		}
		if err = rows.Err(); err != nil {
			return err
		}
		if len(ordered) == 0 {
			return sql.ErrNoRows
		}
		from := -1
		for i, id := range ordered {
			if id == stickerID {
				from = i
				break
			}
		}
		if from < 0 {
			return sql.ErrNoRows
		}
		if position < 0 {
			position = 0
		}
		if position >= int32(len(ordered)) {
			position = int32(len(ordered) - 1)
		}
		if int32(from) != position {
			item := ordered[from]
			ordered = append(ordered[:from], ordered[from+1:]...)
			at := int(position)
			ordered = append(ordered, 0)
			copy(ordered[at+1:], ordered[at:])
			ordered[at] = item
		}

		// Move every row to a unique temporary negative position first. This
		// avoids transient collisions with the unique (set_id, position) key
		// while assigning the final contiguous order below.
		if _, err = tx.ExecContext(ctx, `UPDATE apifull_sticker SET position=-position-1 WHERE set_id=$1`, setID); err != nil {
			return err
		}
		for i, id := range ordered {
			if _, err = tx.ExecContext(ctx, `UPDATE apifull_sticker SET position=$1 WHERE id=$2 AND set_id=$3`, i, id, setID); err != nil {
				return err
			}
		}
		return nil
	})
}

func mutateStickerForSet(ctx context.Context, ownerID, stickerID int64, fn func(*sql.Tx, int64) error) (*StickerSet, error) {
	db, err := stickerDB()
	if err != nil {
		return nil, err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var setID, owner int64
	if err = tx.QueryRowContext(ctx, `SELECT s.set_id,ss.owner_user_id FROM apifull_sticker s JOIN apifull_sticker_set ss ON ss.id=s.set_id WHERE s.id=$1 FOR UPDATE`, stickerID).Scan(&setID, &owner); err != nil {
		return nil, err
	}
	if owner != ownerID {
		return nil, ErrStickerSetOwnerMismatch
	}
	if err = fn(tx, setID); err != nil {
		return nil, err
	}
	set, err := loadStickerSetTx(ctx, tx, ownerID, setID, "")
	if err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return set, nil
}

func SetInstalled(ctx context.Context, userID, setID int64, installed, archived bool) error {
	db, err := stickerDB()
	if err != nil {
		return err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err = lockStickerUserTx(ctx, tx, userID); err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO apifull_sticker_user_set
 (user_id,set_id,installed,archived,unread,order_index) VALUES ($1,$2,$3,$4,FALSE,
 COALESCE((SELECT MAX(order_index)+1 FROM apifull_sticker_user_set WHERE user_id=$1),0))
 ON CONFLICT (user_id,set_id) DO UPDATE SET installed=EXCLUDED.installed,
 archived=EXCLUDED.archived, updated_at=CURRENT_TIMESTAMP`, userID, setID, installed, archived)
	if err != nil {
		return err
	}
	return tx.Commit()
}

func ReorderStickerSets(ctx context.Context, userID int64, ids []int64) error {
	db, err := stickerDB()
	if err != nil {
		return err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err = lockStickerUserTx(ctx, tx, userID); err != nil {
		return err
	}
	for i, id := range ids {
		if _, err = tx.ExecContext(ctx, `UPDATE apifull_sticker_user_set SET order_index=$1, installed=TRUE, updated_at=CURRENT_TIMESTAMP WHERE user_id=$2 AND set_id=$3`, i, userID, id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func SetFeaturedRead(ctx context.Context, userID int64, ids []int64) error {
	db, err := stickerDB()
	if err != nil {
		return err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err = lockStickerUserTx(ctx, tx, userID); err != nil {
		return err
	}
	for _, id := range ids {
		if _, err = tx.ExecContext(ctx, `INSERT INTO apifull_sticker_user_set (user_id,set_id,installed,archived,unread,order_index)
 VALUES ($1,$2,FALSE,FALSE,FALSE,0) ON CONFLICT (user_id,set_id) DO UPDATE SET unread=FALSE, updated_at=CURRENT_TIMESTAMP`, userID, id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func SaveRecentSticker(ctx context.Context, userID, stickerID int64, attached, unsave bool) error {
	return SaveRecentStickerWithAccessHash(ctx, userID, stickerID, 0, attached, unsave)
}

// SaveRecentStickerWithAccessHash records recent state only for the exact
// Telegram document access hash supplied by the caller. The zero-hash wrapper
// remains for internal callers that already resolved the document.
func SaveRecentStickerWithAccessHash(ctx context.Context, userID, stickerID, accessHash int64, attached, unsave bool) error {
	db, err := stickerDB()
	if err != nil {
		return err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err = lockStickerUserTx(ctx, tx, userID); err != nil {
		return err
	}
	if accessHash != 0 {
		var storedHash int64
		if err = tx.QueryRowContext(ctx, `SELECT access_hash FROM apifull_sticker WHERE id=$1`, stickerID).Scan(&storedHash); err != nil {
			if err == sql.ErrNoRows {
				return ErrStickerDocumentNotFound
			}
			return err
		}
		if storedHash != accessHash {
			return ErrStickerDocumentAccessMismatch
		}
	}
	if unsave {
		_, err = tx.ExecContext(ctx, `DELETE FROM apifull_sticker_user_recent WHERE user_id=$1 AND sticker_id=$2 AND attached=$3`, userID, stickerID, attached)
	} else {
		_, err = tx.ExecContext(ctx, `INSERT INTO apifull_sticker_user_recent (user_id,sticker_id,attached)
 VALUES ($1,$2,$3) ON CONFLICT (user_id,sticker_id,attached) DO UPDATE SET saved_at=CURRENT_TIMESTAMP`, userID, stickerID, attached)
	}
	if err != nil {
		return err
	}
	return tx.Commit()
}

func ClearRecentStickers(ctx context.Context, userID int64, attached bool) error {
	db, err := stickerDB()
	if err != nil {
		return err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err = lockStickerUserTx(ctx, tx, userID); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM apifull_sticker_user_recent WHERE user_id=$1 AND attached=$2`, userID, attached); err != nil {
		return err
	}
	return tx.Commit()
}

func SaveFavouriteSticker(ctx context.Context, userID, stickerID int64, unfave bool) error {
	return SaveFavouriteStickerWithAccessHash(ctx, userID, stickerID, 0, unfave)
}

// SaveFavouriteStickerWithAccessHash records favourite state only for the
// exact Telegram document access hash supplied by the caller.
func SaveFavouriteStickerWithAccessHash(ctx context.Context, userID, stickerID, accessHash int64, unfave bool) error {
	db, err := stickerDB()
	if err != nil {
		return err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err = lockStickerUserTx(ctx, tx, userID); err != nil {
		return err
	}
	if accessHash != 0 {
		var storedHash int64
		if err = tx.QueryRowContext(ctx, `SELECT access_hash FROM apifull_sticker WHERE id=$1`, stickerID).Scan(&storedHash); err != nil {
			if err == sql.ErrNoRows {
				return ErrStickerDocumentNotFound
			}
			return err
		}
		if storedHash != accessHash {
			return ErrStickerDocumentAccessMismatch
		}
	}
	if unfave {
		_, err = tx.ExecContext(ctx, `DELETE FROM apifull_sticker_user_favourite WHERE user_id=$1 AND sticker_id=$2`, userID, stickerID)
	} else {
		_, err = tx.ExecContext(ctx, `INSERT INTO apifull_sticker_user_favourite (user_id,sticker_id) VALUES ($1,$2) ON CONFLICT DO NOTHING`, userID, stickerID)
	}
	if err != nil {
		return err
	}
	return tx.Commit()
}

func listStickerDocuments(ctx context.Context, query string, args ...any) ([]StickerDocument, error) {
	db, err := stickerDB()
	if err != nil {
		return nil, err
	}
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var docs []StickerDocument
	for rows.Next() {
		var d StickerDocument
		var date int64
		if err = rows.Scan(&d.ID, &d.AccessHash, &d.SetID, &d.SetAccessHash, &d.Position, &d.Alt, &d.Keywords, &d.MimeType, &d.SizeBytes, &d.DCID, &d.FileRef, &date); err != nil {
			return nil, err
		}
		d.Date = int32(date)
		docs = append(docs, d)
	}
	return docs, rows.Err()
}

func ListRecentStickers(ctx context.Context, userID int64, attached bool, limit int32) ([]StickerDocument, error) {
	limit = normalizeStickerLimit(limit)
	return listStickerDocuments(ctx, `SELECT s.id,s.access_hash,s.set_id,ss.access_hash,s.position,s.alt,s.keywords,s.mime_type,s.size_bytes,s.dc_id,s.file_reference,
 EXTRACT(EPOCH FROM r.saved_at)::bigint FROM apifull_sticker s JOIN apifull_sticker_set ss ON ss.id=s.set_id JOIN apifull_sticker_user_recent r ON r.sticker_id=s.id
 WHERE r.user_id=$1 AND r.attached=$2 ORDER BY r.saved_at DESC LIMIT $3`, userID, attached, limit)
}

func ListFavouriteStickers(ctx context.Context, userID int64, limit int32) ([]StickerDocument, error) {
	limit = normalizeStickerLimit(limit)
	return listStickerDocuments(ctx, `SELECT s.id,s.access_hash,s.set_id,ss.access_hash,s.position,s.alt,s.keywords,s.mime_type,s.size_bytes,s.dc_id,s.file_reference,
 EXTRACT(EPOCH FROM s.created_at)::bigint FROM apifull_sticker s JOIN apifull_sticker_set ss ON ss.id=s.set_id JOIN apifull_sticker_user_favourite f ON f.sticker_id=s.id
 WHERE f.user_id=$1 ORDER BY f.saved_at DESC LIMIT $2`, userID, limit)
}

func SearchStickerDocuments(ctx context.Context, userID int64, queryText, emoticon string, offset, limit int32) ([]StickerDocument, error) {
	limit = normalizeStickerLimit(limit)
	if offset < 0 {
		offset = 0
	}
	return listStickerDocuments(ctx, `SELECT DISTINCT s.id,s.access_hash,s.set_id,ss.access_hash,s.position,s.alt,s.keywords,s.mime_type,s.size_bytes,s.dc_id,s.file_reference,
 EXTRACT(EPOCH FROM s.created_at)::bigint FROM apifull_sticker s JOIN apifull_sticker_set ss ON ss.id=s.set_id
 WHERE (lower(s.alt) LIKE lower($1) OR lower(s.keywords) LIKE lower($1) OR lower(ss.title) LIKE lower($1) OR lower(ss.short_name) LIKE lower($1))
 AND ($2='' OR s.alt=$2) ORDER BY s.id OFFSET $3 LIMIT $4`, "%"+queryText+"%", emoticon, offset, limit)
}

func StickerSetsForDocument(ctx context.Context, userID, documentID, accessHash int64) ([]StickerSet, error) {
	db, err := stickerDB()
	if err != nil {
		return nil, err
	}
	rows, err := db.QueryContext(ctx, `SELECT DISTINCT ss.id FROM apifull_sticker_set ss
	JOIN apifull_sticker s ON s.set_id=ss.id WHERE s.id=$1 AND s.access_hash=$2`, documentID, accessHash)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err = rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		var storedHash int64
		err = db.QueryRowContext(ctx, `SELECT access_hash FROM apifull_sticker WHERE id=$1`, documentID).Scan(&storedHash)
		if err == sql.ErrNoRows {
			return nil, ErrStickerDocumentNotFound
		}
		if err != nil {
			return nil, err
		}
		return nil, ErrStickerDocumentAccessMismatch
	}
	sets := make([]StickerSet, 0, len(ids))
	for _, id := range ids {
		set, getErr := GetStickerSet(ctx, userID, id, "")
		if getErr != nil {
			return nil, getErr
		}
		if set != nil {
			sets = append(sets, *set)
		}
	}
	return sets, nil
}

func StickerShortNameAvailable(ctx context.Context, shortName string) (bool, error) {
	db, err := stickerDB()
	if err != nil {
		return false, err
	}
	var exists bool
	err = db.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM apifull_sticker_set WHERE short_name=$1)`, normalizeStickerName(shortName)).Scan(&exists)
	return !exists, err
}

func ChangeSticker(ctx context.Context, ownerID, stickerID int64, alt, keywords string) (*StickerSet, error) {
	return mutateStickerForSet(ctx, ownerID, stickerID, func(tx *sql.Tx, setID int64) error {
		_, err := tx.ExecContext(ctx, `UPDATE apifull_sticker SET alt=$1, keywords=$2 WHERE id=$3 AND set_id=$4`, alt, keywords, stickerID, setID)
		return err
	})
}

func ReplaceSticker(ctx context.Context, ownerID, stickerID int64, input StickerDocumentInput) (*StickerSet, error) {
	return mutateStickerForSet(ctx, ownerID, stickerID, func(tx *sql.Tx, setID int64) error {
		if input.AccessHash == 0 {
			input.AccessHash = input.ID
		}
		if input.MimeType == "" {
			input.MimeType = "image/webp"
		}
		if input.DCID == 0 {
			input.DCID = 2
		}
		if input.FileRef == nil {
			input.FileRef = []byte{}
		}
		_, err := tx.ExecContext(ctx, `UPDATE apifull_sticker SET access_hash=$1, alt=$2, keywords=$3,
 mime_type=$4, size_bytes=$5, dc_id=$6, file_reference=$7 WHERE id=$8 AND set_id=$9`,
			input.AccessHash, input.Alt, input.Keywords, input.MimeType, input.SizeBytes,
			input.DCID, input.FileRef, stickerID, setID)
		return err
	})
}
