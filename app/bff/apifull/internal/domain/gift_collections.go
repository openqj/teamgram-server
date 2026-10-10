package domain

import (
	"database/sql"
	"errors"
	"hash/fnv"
	"strings"
)

var (
	ErrGiftCollectionNotFound = errors.New("gift collection not found")
	ErrGiftCollectionInvalid  = errors.New("gift collection invalid")
)

type GiftCollection struct {
	ID       int64
	OwnerID  int64
	Title    string
	GiftIDs  []int64
	GiftsCnt int32
	Hash     int64
}

type GiftRef struct {
	ID   int64
	Slug string
}

func ResolveOwnedGift(owner int64, ref GiftRef) (int64, error) {
	if db == nil || owner <= 0 || (ref.ID <= 0 && strings.TrimSpace(ref.Slug) == "") {
		return 0, ErrGiftCollectionInvalid
	}
	query := `SELECT id FROM apifull_gift WHERE to_user=$1 AND saved=1`
	args := []any{owner}
	if ref.ID > 0 {
		query += ` AND id=$2`
		args = append(args, ref.ID)
	} else {
		query += ` AND slug=$2 ORDER BY id LIMIT 1`
		args = append(args, ref.Slug)
	}
	var id int64
	if err := db.QueryRow(query, args...).Scan(&id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, ErrGiftCollectionInvalid
		}
		return 0, err
	}
	return id, nil
}

func CreateGiftCollection(owner int64, title string, giftIDs []int64) (GiftCollection, error) {
	if db == nil || owner <= 0 || strings.TrimSpace(title) == "" {
		return GiftCollection{}, ErrGiftCollectionInvalid
	}
	tx, err := db.Begin()
	if err != nil {
		return GiftCollection{}, err
	}
	defer func() { _ = tx.Rollback() }()
	var collection GiftCollection
	if err = tx.QueryRow(`INSERT INTO apifull_gift_collection (owner_user_id, title, position)
		VALUES ($1,$2,COALESCE((SELECT max(position)+1 FROM apifull_gift_collection WHERE owner_user_id=$1),0))
		RETURNING id`, owner, title).Scan(&collection.ID); err != nil {
		return GiftCollection{}, err
	}
	for i, giftID := range giftIDs {
		var ok bool
		if err = tx.QueryRow(`SELECT EXISTS(SELECT 1 FROM apifull_gift WHERE id=$1 AND to_user=$2 AND saved=1)`, giftID, owner).Scan(&ok); err != nil {
			return GiftCollection{}, err
		}
		if !ok {
			return GiftCollection{}, ErrGiftCollectionInvalid
		}
		if _, err = tx.Exec(`INSERT INTO apifull_gift_collection_item (collection_id, gift_id, position) VALUES ($1,$2,$3)`, collection.ID, giftID, i); err != nil {
			return GiftCollection{}, err
		}
	}
	if err = tx.Commit(); err != nil {
		return GiftCollection{}, err
	}
	return loadGiftCollection(owner, collection.ID)
}

func UpdateGiftCollection(owner int64, id int64, title *string, deleteIDs, addIDs, orderIDs []int64) (GiftCollection, error) {
	if db == nil || owner <= 0 || id <= 0 {
		return GiftCollection{}, ErrGiftCollectionInvalid
	}
	tx, err := db.Begin()
	if err != nil {
		return GiftCollection{}, err
	}
	defer func() { _ = tx.Rollback() }()
	var lockedID int64
	if err = tx.QueryRow(`SELECT id FROM apifull_gift_collection WHERE id=$1 AND owner_user_id=$2 FOR UPDATE`, id, owner).Scan(&lockedID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return GiftCollection{}, ErrGiftCollectionNotFound
		}
		return GiftCollection{}, err
	}
	if title != nil {
		if strings.TrimSpace(*title) == "" {
			return GiftCollection{}, ErrGiftCollectionInvalid
		}
		if _, err = tx.Exec(`UPDATE apifull_gift_collection SET title=$1 WHERE id=$2 AND owner_user_id=$3`, *title, id, owner); err != nil {
			return GiftCollection{}, err
		}
	}
	for _, giftID := range deleteIDs {
		if _, err = tx.Exec(`DELETE FROM apifull_gift_collection_item WHERE collection_id=$1 AND gift_id=$2`, id, giftID); err != nil {
			return GiftCollection{}, err
		}
	}
	for _, giftID := range addIDs {
		var ok bool
		if err = tx.QueryRow(`SELECT EXISTS(SELECT 1 FROM apifull_gift WHERE id=$1 AND to_user=$2 AND saved=1)`, giftID, owner).Scan(&ok); err != nil {
			return GiftCollection{}, err
		}
		if !ok {
			return GiftCollection{}, ErrGiftCollectionInvalid
		}
		if _, err = tx.Exec(`INSERT INTO apifull_gift_collection_item (collection_id, gift_id, position)
			VALUES ($1,$2,COALESCE((SELECT max(position)+1 FROM apifull_gift_collection_item WHERE collection_id=$1),0))
			ON CONFLICT (collection_id, gift_id) DO NOTHING`, id, giftID); err != nil {
			return GiftCollection{}, err
		}
	}
	if len(orderIDs) > 0 {
		for i, giftID := range orderIDs {
			result, updateErr := tx.Exec(`UPDATE apifull_gift_collection_item SET position=$1 WHERE collection_id=$2 AND gift_id=$3`, i, id, giftID)
			if updateErr != nil {
				return GiftCollection{}, updateErr
			}
			if affected, affectedErr := result.RowsAffected(); affectedErr != nil || affected == 0 {
				return GiftCollection{}, ErrGiftCollectionInvalid
			}
		}
	}
	if err = tx.Commit(); err != nil {
		return GiftCollection{}, err
	}
	return loadGiftCollection(owner, id)
}

func ReorderGiftCollections(owner int64, order []int64) error {
	if db == nil || owner <= 0 || len(order) == 0 {
		return ErrGiftCollectionInvalid
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	for i, id := range order {
		result, e := tx.Exec(`UPDATE apifull_gift_collection SET position=$1 WHERE id=$2 AND owner_user_id=$3`, i, id, owner)
		if e != nil {
			return e
		}
		if n, _ := result.RowsAffected(); n == 0 {
			return ErrGiftCollectionNotFound
		}
	}
	return tx.Commit()
}

func DeleteGiftCollection(owner int64, id int64) error {
	if db == nil || owner <= 0 || id <= 0 {
		return ErrGiftCollectionInvalid
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	result, err := tx.Exec(`DELETE FROM apifull_gift_collection WHERE id=$1 AND owner_user_id=$2`, id, owner)
	if err != nil {
		return err
	}
	if n, _ := result.RowsAffected(); n == 0 {
		return ErrGiftCollectionNotFound
	}
	return tx.Commit()
}

func ListGiftCollections(owner int64) ([]GiftCollection, int64, error) {
	if db == nil || owner <= 0 {
		return nil, 0, ErrGiftCollectionInvalid
	}
	rows, err := db.Query(`SELECT c.id, c.title, i.gift_id FROM apifull_gift_collection c
		LEFT JOIN apifull_gift_collection_item i ON i.collection_id=c.id
		WHERE c.owner_user_id=$1 ORDER BY c.position, c.id, i.position, i.gift_id`, owner)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	byID := make(map[int64]*GiftCollection)
	var order []int64
	for rows.Next() {
		var id int64
		var giftID sql.NullInt64
		var title string
		if err = rows.Scan(&id, &title, &giftID); err != nil {
			return nil, 0, err
		}
		collection := byID[id]
		if collection == nil {
			collection = &GiftCollection{ID: id, OwnerID: owner, Title: title}
			byID[id] = collection
			order = append(order, id)
		}
		if giftID.Valid && giftID.Int64 > 0 {
			collection.GiftIDs = append(collection.GiftIDs, giftID.Int64)
		}
	}
	if err = rows.Err(); err != nil {
		return nil, 0, err
	}
	out := make([]GiftCollection, 0, len(order))
	hashes := make([]int64, 0, len(order))
	for _, id := range order {
		collection := *byID[id]
		collection.GiftsCnt = int32(len(collection.GiftIDs))
		collection.Hash = giftCollectionHash(collection)
		out = append(out, collection)
		hashes = append(hashes, collection.Hash)
	}
	return out, giftCollectionListHash(hashes), nil
}

func loadGiftCollection(owner, id int64) (GiftCollection, error) {
	collections, _, err := ListGiftCollections(owner)
	if err != nil {
		return GiftCollection{}, err
	}
	for _, collection := range collections {
		if collection.ID == id {
			return collection, nil
		}
	}
	return GiftCollection{}, ErrGiftCollectionNotFound
}

func giftCollectionHash(collection GiftCollection) int64 {
	h := fnv.New64a()
	var idBytes [8]byte
	for i := range idBytes {
		idBytes[i] = byte(collection.ID >> (8 * i))
	}
	_, _ = h.Write(idBytes[:])
	_, _ = h.Write([]byte(collection.Title))
	var b [8]byte
	for _, id := range collection.GiftIDs {
		for i := range b {
			b[i] = byte(id >> (8 * i))
		}
		_, _ = h.Write(b[:])
	}
	value := int64(h.Sum64() & 0x7fffffffffffffff)
	if value == 0 {
		value = collection.ID
	}
	return value
}

func giftCollectionListHash(values []int64) int64 {
	var result int64
	for _, value := range values {
		result = ((result * 20261) + 0x80000000 + value) % 0x80000000
	}
	return result
}
