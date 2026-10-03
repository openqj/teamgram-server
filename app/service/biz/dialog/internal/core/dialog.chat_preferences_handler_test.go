package core

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/teamgram/marmota/pkg/stores/cache"
	"github.com/teamgram/marmota/pkg/stores/sqlc"
	"github.com/teamgram/marmota/pkg/stores/sqlx"
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/dialog"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/internal/dal/dao/mysql_dao"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/internal/dao"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/internal/svc"
	"github.com/zeromicro/go-zero/core/logx"
)

type dialogPreferencesAuditCache struct {
	cache.BatchCache
}

func (c *dialogPreferencesAuditCache) DelCtx(context.Context, ...string) error { return nil }

func dialogPreferencesAuditDB(t *testing.T) *sqlx.DB {
	t.Helper()
	dsn := os.Getenv("APIFULL_MYSQL_DSN")
	if dsn == "" {
		t.Skip("APIFULL_MYSQL_DSN must point to the isolated teamgram_audit database")
	}
	config, err := mysql.ParseDSN(dsn)
	if err != nil {
		t.Fatal(err)
	}
	if config.DBName != "teamgram_audit" || config.Addr != "127.0.0.1:13306" {
		t.Fatalf("refusing non-audit MySQL target %q at %q", config.DBName, config.Addr)
	}
	return sqlx.NewMySQL(&sqlx.Config{DSN: dsn})
}

func ensureDialogPreferenceColumns(t *testing.T, db *sqlx.DB) {
	t.Helper()
	ctx := context.Background()
	for _, statement := range []string{
		"ALTER TABLE dialogs ADD COLUMN theme_emoticon varchar(255) NOT NULL DEFAULT ''",
		"ALTER TABLE dialogs ADD COLUMN wallpaper_id bigint NOT NULL DEFAULT 0",
		"ALTER TABLE dialogs ADD COLUMN wallpaper_overridden tinyint(1) NOT NULL DEFAULT 0",
	} {
		if _, err := db.Exec(ctx, statement); err != nil {
			if !sqlx.IsDuplicate(err) {
				// MySQL reports duplicate-column as 1060, which is not the
				// duplicate-key helper's 1062. The information-schema check
				// below handles that case without masking other errors.
				var present int
				column := ""
				if statement == "ALTER TABLE dialogs ADD COLUMN theme_emoticon varchar(255) NOT NULL DEFAULT ''" {
					column = "theme_emoticon"
				} else if statement == "ALTER TABLE dialogs ADD COLUMN wallpaper_id bigint NOT NULL DEFAULT 0" {
					column = "wallpaper_id"
				} else {
					column = "wallpaper_overridden"
				}
				if queryErr := db.QueryRowPartial(ctx, &present, "SELECT COUNT(*) FROM information_schema.columns WHERE table_schema = DATABASE() AND table_name = 'dialogs' AND column_name = ?", column); queryErr != nil || present == 0 {
					t.Fatalf("ensure dialogs.%s: %v", column, err)
				}
			}
		}
	}
}

func TestDialogChatPreferencesPersistAndPropagateErrors(t *testing.T) {
	db := dialogPreferencesAuditDB(t)
	ctx := context.Background()
	ensureDialogPreferenceColumns(t, db)
	userID := time.Now().UnixNano()
	peerID := userID + 1
	const peerType int32 = mtproto.PEER_USER
	for _, row := range [][3]interface{}{{userID, peerType, peerID}, {peerID, peerType, userID}} {
		if _, err := db.Exec(ctx, "INSERT INTO dialogs (user_id, peer_type, peer_id, ttl_period, theme_emoticon, wallpaper_id, wallpaper_overridden) VALUES (?, ?, ?, 0, '', 0, 0)", row[0], row[1], row[2]); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		_, _ = db.Exec(ctx, "DELETE FROM dialogs WHERE (user_id = ? AND peer_id = ?) OR (user_id = ? AND peer_id = ?)", userID, peerID, peerID, userID)
	})
	d := &dao.Dao{
		Mysql:      &dao.Mysql{DB: db, DialogsDAO: mysql_dao.NewDialogsDAO(db)},
		CachedConn: sqlc.NewConnWithCache(db, &dialogPreferencesAuditCache{}),
	}
	core := &DialogCore{ctx: ctx, svcCtx: &svc.ServiceContext{Dao: d}, Logger: logx.WithContext(ctx)}
	if result, err := core.DialogSetChatTheme(&dialog.TLDialogSetChatTheme{UserId: userID, PeerType: peerType, PeerId: peerID, ThemeEmoticon: "🍀"}); err != nil || !mtproto.FromBool(result) {
		t.Fatalf("set chat theme = (%v, %v), want BoolTrue", result, err)
	}
	if result, err := core.DialogSetChatWallpaper(&dialog.TLDialogSetChatWallpaper{UserId: userID, PeerType: peerType, PeerId: peerID, WallpaperId: 9001, WallpaperOverridden: true}); err != nil || !mtproto.FromBool(result) {
		t.Fatalf("set chat wallpaper = (%v, %v), want BoolTrue", result, err)
	}
	var row struct {
		Theme      string `db:"theme_emoticon"`
		Wallpaper  int64  `db:"wallpaper_id"`
		Overridden bool   `db:"wallpaper_overridden"`
	}
	if err := db.QueryRowPartial(ctx, &row, "SELECT theme_emoticon, wallpaper_id, wallpaper_overridden FROM dialogs WHERE user_id = ? AND peer_type = ? AND peer_id = ?", userID, peerType, peerID); err != nil {
		t.Fatal(err)
	}
	if row.Theme != "🍀" || row.Wallpaper != 9001 || !row.Overridden {
		t.Fatalf("persisted chat preferences = %+v, want theme 🍀 and wallpaper 9001/true", row)
	}

	errDB, err := sqlx.Open(&sqlx.Config{DSN: "ttl:ttl@tcp(127.0.0.1:1)/teamgram_dialog_error?timeout=50ms&readTimeout=50ms&writeTimeout=50ms"})
	if err != nil {
		t.Fatal(err)
	}
	errCore := &DialogCore{ctx: ctx, svcCtx: &svc.ServiceContext{Dao: &dao.Dao{
		Mysql:      &dao.Mysql{DB: errDB, DialogsDAO: mysql_dao.NewDialogsDAO(errDB)},
		CachedConn: sqlc.NewConnWithCache(errDB, &dialogPreferencesAuditCache{}),
	}}, Logger: logx.WithContext(ctx)}
	if result, err := errCore.DialogSetChatTheme(&dialog.TLDialogSetChatTheme{UserId: userID, PeerType: peerType, PeerId: peerID, ThemeEmoticon: "x"}); result != nil || err == nil {
		t.Fatalf("theme storage error = (%v, %v), want nil and error", result, err)
	}
	if result, err := errCore.DialogSetChatWallpaper(&dialog.TLDialogSetChatWallpaper{UserId: userID, PeerType: peerType, PeerId: peerID, WallpaperId: 9002}); result != nil || err == nil {
		t.Fatalf("wallpaper storage error = (%v, %v), want nil and error", result, err)
	}
}
