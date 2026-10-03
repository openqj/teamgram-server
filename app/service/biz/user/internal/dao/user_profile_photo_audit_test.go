package dao

import (
	"context"
	"database/sql"
	"os"
	"strconv"
	"testing"
	"time"

	mysql "github.com/go-sql-driver/mysql"
	marmotaCache "github.com/teamgram/marmota/pkg/stores/cache"
	"github.com/teamgram/marmota/pkg/stores/sqlc"
	"github.com/teamgram/marmota/pkg/stores/sqlx"
)

type profilePhotoAuditCache struct {
	marmotaCache.BatchCache
}

func (*profilePhotoAuditCache) DelCtx(context.Context, ...string) error { return nil }

func TestProfilePhotoMutationsAuditDatabase(t *testing.T) {
	dsn := os.Getenv("APIFULL_MYSQL_DSN")
	if dsn == "" {
		t.Skip("set APIFULL_MYSQL_DSN to the isolated 127.0.0.1:13306/teamgram_audit database")
	}
	dsnConfig, err := mysql.ParseDSN(dsn)
	if err != nil {
		t.Fatalf("parse APIFULL_MYSQL_DSN: %v", err)
	}
	if dsnConfig.DBName != "teamgram_audit" || dsnConfig.Net != "tcp" || dsnConfig.Addr != "127.0.0.1:13306" {
		t.Fatalf("refusing non-audit MySQL target %q at %q", dsnConfig.DBName, dsnConfig.Addr)
	}

	adminDB, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = adminDB.Close() })
	if err = adminDB.Ping(); err != nil {
		t.Fatalf("connect isolated audit database: %v", err)
	}

	wrappedDB, err := sqlx.Open(&sqlx.Config{DSN: dsn})
	if err != nil {
		t.Fatalf("open isolated audit database: %v", err)
	}
	cache := &profilePhotoAuditCache{}
	testDAO := &Dao{
		Mysql:      newMysqlDao(wrappedDB),
		CachedConn: sqlc.NewConnWithCache(wrappedDB, cache),
	}

	baseID := time.Now().UnixNano()
	seed := func(userID, mainPhotoID int64, photos ...int64) {
		t.Helper()
		t.Cleanup(func() {
			_, _ = adminDB.Exec("DELETE FROM user_profile_photos WHERE user_id = ?", userID)
			_, _ = adminDB.Exec("DELETE FROM users WHERE id = ?", userID)
		})
		if _, err := adminDB.Exec("INSERT INTO users (id, access_hash, phone, country_code, photo_id) VALUES (?, ?, ?, ?, ?)",
			userID, userID, "audit"+strconv.FormatInt(userID, 10), "ZZ", mainPhotoID); err != nil {
			t.Fatalf("seed audit user %d: %v", userID, err)
		}
		for i, photoID := range photos {
			if _, err := adminDB.Exec("INSERT INTO user_profile_photos (user_id, photo_id, date2, deleted) VALUES (?, ?, ?, 0)",
				userID, photoID, int64(i+1)); err != nil {
				t.Fatalf("seed profile photo %d for user %d: %v", photoID, userID, err)
			}
		}
	}
	readMainPhoto := func(userID int64) int64 {
		t.Helper()
		var photoID int64
		if err := adminDB.QueryRow("SELECT photo_id FROM users WHERE id = ?", userID).Scan(&photoID); err != nil {
			t.Fatal(err)
		}
		return photoID
	}
	readDeleted := func(userID, photoID int64) bool {
		t.Helper()
		var deleted bool
		if err := adminDB.QueryRow("SELECT deleted FROM user_profile_photos WHERE user_id = ? AND photo_id = ?", userID, photoID).Scan(&deleted); err != nil {
			t.Fatal(err)
		}
		return deleted
	}

	t.Run("update and clear select the newest remaining photo", func(t *testing.T) {
		userID := baseID + 1
		seed(userID, 100, 100, 90)

		mainID, err := testDAO.UpdateProfilePhoto(context.Background(), userID, 110)
		if err != nil || mainID != 110 || readMainPhoto(userID) != 110 {
			t.Fatalf("set profile photo = (%d, %v), stored main = %d, want 110", mainID, err, readMainPhoto(userID))
		}
		mainID, err = testDAO.UpdateProfilePhoto(context.Background(), userID, 0)
		if err != nil || mainID != 90 || readMainPhoto(userID) != 90 || !readDeleted(userID, 110) {
			t.Fatalf("clear profile photo = (%d, %v), stored main = %d, cleared row = %v; want main 90 and cleared row", mainID, err, readMainPhoto(userID), readDeleted(userID, 110))
		}
	})

	t.Run("deleting current photo selects a remaining photo", func(t *testing.T) {
		userID := baseID + 2
		seed(userID, 200, 200, 190)

		mainID, err := testDAO.DeleteProfilePhotos(context.Background(), userID, []int64{200})
		if err != nil || mainID != 190 || readMainPhoto(userID) != 190 || !readDeleted(userID, 200) {
			t.Fatalf("delete current photo = (%d, %v), stored main = %d, deleted row = %v; want main 190", mainID, err, readMainPhoto(userID), readDeleted(userID, 200))
		}
	})

	t.Run("deleting last photo clears the main photo", func(t *testing.T) {
		userID := baseID + 3
		seed(userID, 300, 300)

		mainID, err := testDAO.DeleteProfilePhotos(context.Background(), userID, []int64{300})
		if err != nil || mainID != 0 || readMainPhoto(userID) != 0 || !readDeleted(userID, 300) {
			t.Fatalf("delete last photo = (%d, %v), stored main = %d, deleted row = %v; want main 0", mainID, err, readMainPhoto(userID), readDeleted(userID, 300))
		}
	})

}

func TestUpdateProfilePhotoReturnsStorageError(t *testing.T) {
	db, err := sqlx.Open(&sqlx.Config{DSN: "audit_test@tcp(127.0.0.1:1)/teamgram_audit?timeout=250ms"})
	if err != nil {
		t.Fatalf("open unreachable local test database: %v", err)
	}
	testDAO := &Dao{
		Mysql:      newMysqlDao(db),
		CachedConn: sqlc.NewConnWithCache(db, &profilePhotoAuditCache{}),
	}
	if _, err = testDAO.UpdateProfilePhoto(context.Background(), 42, 99); err == nil {
		t.Fatal("UpdateProfilePhoto() succeeded when the local database connection was refused")
	}
}
