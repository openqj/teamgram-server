package persist

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"
)

func TestFeaturedStickerSetsPostgresPagination(t *testing.T) {
	dsn := os.Getenv("APIFULL_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("APIFULL_POSTGRES_DSN must point to an isolated PostgreSQL 18 database")
	}
	if err := OpenPostgres(dsn); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ClosePostgres() })

	db, err := stickerDB()
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	stamp := time.Now().UnixNano()
	ids := make([]int64, 3)
	for i := range ids {
		shortName := fmt.Sprintf("pagination_%d_%d", stamp, i)
		err = db.QueryRowContext(ctx, `INSERT INTO apifull_sticker_set
 (access_hash, owner_user_id, title, short_name, featured)
 VALUES ($1, $2, $3, $4, TRUE) RETURNING id`, stamp+int64(i), stamp, shortName, shortName).Scan(&ids[i])
		if err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		_, _ = db.ExecContext(context.Background(), `DELETE FROM apifull_sticker_set WHERE id IN ($1, $2, $3)`, ids[0], ids[1], ids[2])
	})

	rows, err := db.QueryContext(ctx, `SELECT s.id FROM apifull_sticker_set s
 LEFT JOIN apifull_sticker_user_set us ON us.set_id=s.id AND us.user_id=$1
 WHERE s.featured ORDER BY COALESCE(us.order_index, 2147483647), s.id`, stamp)
	if err != nil {
		t.Fatal(err)
	}
	ordered := make([]int64, 0)
	for rows.Next() {
		var id int64
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			t.Fatal(err)
		}
		ordered = append(ordered, id)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		t.Fatal(err)
	}
	if err = rows.Close(); err != nil {
		t.Fatal(err)
	}
	first := -1
	for i, id := range ordered {
		if id == ids[0] {
			first = i
			break
		}
	}
	if first < 0 || first+1 >= len(ordered) || ordered[first+1] != ids[1] {
		t.Fatalf("fixture ordering does not place the second inserted set after the first: ids=%v ordered=%v", ids, ordered)
	}
	featured, err := ListStickerSets(ctx, stamp, false, false, true, 100)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range ids {
		found := false
		for _, set := range featured {
			if set.ID == id {
				found = true
				if !set.Unread {
					t.Fatalf("new featured set %d is not unread", id)
				}
			}
		}
		if !found {
			t.Fatalf("featured set %d missing from listing", id)
		}
	}
	if err := SetFeaturedRead(ctx, stamp, []int64{ids[0]}); err != nil {
		t.Fatal(err)
	}
	featured, err = ListStickerSets(ctx, stamp, false, false, true, 100)
	if err != nil {
		t.Fatal(err)
	}
	for _, set := range featured {
		if set.ID == ids[0] && set.Unread {
			t.Fatalf("read featured set %d remains unread", ids[0])
		}
	}

	page, total, err := ListFeaturedStickerSets(ctx, stamp, int32(first+1), 1)
	if err != nil {
		t.Fatal(err)
	}
	if total != int64(len(ordered)) {
		t.Fatalf("total = %d, want %d", total, len(ordered))
	}
	if len(page) != 1 || page[0].ID != ids[1] {
		t.Fatalf("page IDs = %v, want [%d]", stickerSetIDs(page), ids[1])
	}
}

func TestRemoveStickerCompactsPositionsPostgres(t *testing.T) {
	dsn := os.Getenv("APIFULL_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("APIFULL_POSTGRES_DSN must point to an isolated PostgreSQL 18 database")
	}
	if err := OpenPostgres(dsn); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ClosePostgres() })
	db, err := stickerDB()
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	owner := time.Now().UnixNano()
	var setID int64
	if err := db.QueryRowContext(ctx, `INSERT INTO apifull_sticker_set
 (access_hash, owner_user_id, title, short_name) VALUES ($1,$2,'compact',$3) RETURNING id`,
		owner, owner, fmt.Sprintf("compact_%d", owner)).Scan(&setID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = db.ExecContext(context.Background(), `DELETE FROM apifull_sticker_set WHERE id=$1`, setID)
	})
	for i := int32(0); i < 3; i++ {
		if _, err := db.ExecContext(ctx, `INSERT INTO apifull_sticker
	 (set_id, access_hash, position, alt) VALUES ($1,$2,$3,$4)`, setID, owner+int64(i), i, fmt.Sprintf("%d", i)); err != nil {
			t.Fatal(err)
		}
	}
	var stickerID int64
	if err := db.QueryRowContext(ctx, `SELECT id FROM apifull_sticker WHERE set_id=$1 AND position=1`, setID).Scan(&stickerID); err != nil {
		t.Fatal(err)
	}
	if _, err := RemoveSticker(ctx, owner, stickerID); err != nil {
		t.Fatalf("RemoveSticker = %v", err)
	}
	rows, err := db.QueryContext(ctx, `SELECT position FROM apifull_sticker WHERE set_id=$1 ORDER BY position`, setID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var positions []int32
	for rows.Next() {
		var position int32
		if err := rows.Scan(&position); err != nil {
			t.Fatal(err)
		}
		positions = append(positions, position)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(positions) != "[0 1]" {
		t.Fatalf("positions = %v, want [0 1]", positions)
	}
}

func stickerSetIDs(sets []StickerSet) []int64 {
	ids := make([]int64, len(sets))
	for i := range sets {
		ids[i] = sets[i].ID
	}
	return ids
}
