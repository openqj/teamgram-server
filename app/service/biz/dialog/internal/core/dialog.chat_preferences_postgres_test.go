package core

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/dialog"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/internal/dao"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/internal/svc"
	"github.com/teamgram/teamgram-server/pkg/storage/postgres"
)

func TestDialogChatPreferencesPostgresRoundTrip(t *testing.T) {
	dsn := os.Getenv("DIALOG_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("DIALOG_POSTGRES_DSN must point to an isolated PostgreSQL 18 test database")
	}
	pg, err := dao.NewPostgres(postgres.Config{DSN: dsn})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pg.Close)

	ctx := context.Background()
	var version int
	if err := pg.Pool.QueryRow(ctx, `SELECT current_setting('server_version_num')::integer`).Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version/10000 != 18 {
		t.Fatalf("PostgreSQL version is %d, want 18.x", version)
	}

	userID := time.Now().UnixNano()
	peerID := userID + 1
	const peerType int32 = mtproto.PEER_USER
	for _, row := range [][2]int64{{userID, peerID}, {peerID, userID}} {
		if _, err := pg.Pool.Exec(ctx, `INSERT INTO dialogs (user_id, peer_type, peer_id, peer_dialog_id) VALUES ($1, $2, $3, $3)`, row[0], peerType, row[1]); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		_, _ = pg.Pool.Exec(ctx, `DELETE FROM dialogs WHERE (user_id = $1 AND peer_id = $2) OR (user_id = $2 AND peer_id = $1)`, userID, peerID)
	})

	c := &DialogCore{ctx: ctx, svcCtx: &svc.ServiceContext{Dao: &dao.Dao{Postgres: pg}}}
	if result, err := c.DialogSetChatTheme(&dialog.TLDialogSetChatTheme{UserId: userID, PeerType: peerType, PeerId: peerID, ThemeEmoticon: "🍀"}); err != nil || !mtproto.FromBool(result) {
		t.Fatalf("set theme = (%v, %v), want true", result, err)
	}
	if result, err := c.DialogSetChatWallpaper(&dialog.TLDialogSetChatWallpaper{UserId: userID, PeerType: peerType, PeerId: peerID, WallpaperId: 9001, WallpaperOverridden: true}); err != nil || !mtproto.FromBool(result) {
		t.Fatalf("set wallpaper = (%v, %v), want true", result, err)
	}

	rows, err := pg.Pool.Query(ctx, `SELECT user_id, theme_emoticon, wallpaper_id, wallpaper_overridden FROM dialogs WHERE (user_id = $1 AND peer_id = $2) OR (user_id = $2 AND peer_id = $1) ORDER BY user_id`, userID, peerID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	count := 0
	for rows.Next() {
		var gotUser, wallpaperID int64
		var theme string
		var overridden bool
		if err := rows.Scan(&gotUser, &theme, &wallpaperID, &overridden); err != nil {
			t.Fatal(err)
		}
		if theme != "🍀" || wallpaperID != 9001 || !overridden {
			t.Fatalf("row for user %d = theme %q wallpaper %d/%v", gotUser, theme, wallpaperID, overridden)
		}
		count++
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("preference rows = %d, want 2", count)
	}

	if result, err := c.DialogSetChatWallpaper(&dialog.TLDialogSetChatWallpaper{UserId: userID, PeerType: peerType, PeerId: peerID, WallpaperId: 9002}); err != nil || !mtproto.FromBool(result) {
		t.Fatalf("set caller-only wallpaper = (%v, %v), want true", result, err)
	}
	var callerWallpaper, peerWallpaper int64
	if err := pg.Pool.QueryRow(ctx, `SELECT wallpaper_id FROM dialogs WHERE user_id = $1 AND peer_type = $2 AND peer_id = $3`, userID, peerType, peerID).Scan(&callerWallpaper); err != nil {
		t.Fatal(err)
	}
	if err := pg.Pool.QueryRow(ctx, `SELECT wallpaper_id FROM dialogs WHERE user_id = $1 AND peer_type = $2 AND peer_id = $3`, peerID, peerType, userID).Scan(&peerWallpaper); err != nil {
		t.Fatal(err)
	}
	if callerWallpaper != 9002 || peerWallpaper != 9001 {
		t.Fatalf("caller-only wallpaper state = %d/%d, want 9002/9001", callerWallpaper, peerWallpaper)
	}

	if result, err := c.DialogSetChatWallpaper(&dialog.TLDialogSetChatWallpaper{UserId: userID, PeerType: peerType, PeerId: peerID, WallpaperId: 0, WallpaperOverridden: true}); err != nil || !mtproto.FromBool(result) {
		t.Fatalf("revert wallpaper = (%v, %v), want true", result, err)
	}
	var wallpaperID, peerWallpaperID int64
	var overridden, peerOverridden bool
	if err := pg.Pool.QueryRow(ctx, `SELECT wallpaper_id, wallpaper_overridden FROM dialogs WHERE user_id = $1 AND peer_type = $2 AND peer_id = $3`, userID, peerType, peerID).Scan(&wallpaperID, &overridden); err != nil {
		t.Fatal(err)
	}
	if err := pg.Pool.QueryRow(ctx, `SELECT wallpaper_id, wallpaper_overridden FROM dialogs WHERE user_id = $1 AND peer_type = $2 AND peer_id = $3`, peerID, peerType, userID).Scan(&peerWallpaperID, &peerOverridden); err != nil {
		t.Fatal(err)
	}
	if wallpaperID != 0 || overridden || peerWallpaperID != 0 || peerOverridden {
		t.Fatalf("reverted wallpaper = %d/%v and %d/%v, want 0/false for both", wallpaperID, overridden, peerWallpaperID, peerOverridden)
	}
}
