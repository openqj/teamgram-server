package core

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
	userdao "github.com/teamgram/teamgram-server/app/service/biz/user/internal/dao"
	"github.com/teamgram/teamgram-server/app/service/biz/user/internal/svc"
	"github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

func TestUserPostgresBlockWaitsForSendPermissionLock(t *testing.T) {
	pg := userPostgresTest(t, 3)
	ctx := context.Background()
	ownerID := time.Now().UnixNano()
	peerID := ownerID + 1
	insertUserPostgresFixtures(t, pg, ownerID, peerID)
	d := &userdao.Dao{Postgres: pg}
	for _, block := range []bool{true, false} {
		t.Run(map[bool]string{true: "block", false: "unblock"}[block], func(t *testing.T) {
			tx, err := pg.Pool.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = tx.Rollback(ctx) }()
			if _, err := tx.Exec(ctx, `SELECT id FROM users WHERE id=ANY($1::bigint[]) ORDER BY id FOR SHARE`, []int64{ownerID, peerID}); err != nil {
				t.Fatal(err)
			}
			blockedCtx, cancel := context.WithTimeout(ctx, 150*time.Millisecond)
			defer cancel()
			if block {
				err = d.BlockUser(blockedCtx, ownerID, peerID)
			} else {
				err = d.UnBlockUser(blockedCtx, ownerID, peerID)
			}
			if !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("mutation while send holds permission lock = %v, want deadline exceeded", err)
			}
			stored, err := d.CheckBlocked(ctx, ownerID, peerID)
			if err != nil || stored == block {
				t.Fatalf("block during lock = (%v, %v), want %v", stored, err, !block)
			}
			if err := tx.Commit(ctx); err != nil {
				t.Fatal(err)
			}
			if block {
				err = d.BlockUser(ctx, ownerID, peerID)
			} else {
				err = d.UnBlockUser(ctx, ownerID, peerID)
			}
			if err != nil {
				t.Fatal(err)
			}
		})
	}
	c := New(ctx, &svc.ServiceContext{Dao: d})
	if got, err := c.UserUnBlockPeer(&user.TLUserUnBlockPeer{UserId: ownerID + 2, PeerType: mtproto.PEER_USER, PeerId: peerID}); got != nil || !errors.Is(err, mtproto.ErrUserIdInvalid) {
		t.Fatalf("missing unblock owner = (%v, %v)", got, err)
	}
	if _, err := pg.Pool.Exec(ctx, `UPDATE users SET deleted=TRUE WHERE id=$1`, ownerID); err != nil {
		t.Fatal(err)
	}
	if got, err := c.UserUnBlockPeer(&user.TLUserUnBlockPeer{UserId: ownerID, PeerType: mtproto.PEER_USER, PeerId: peerID}); got != nil || !errors.Is(err, mtproto.ErrUserIdInvalid) {
		t.Fatalf("deleted unblock owner = (%v, %v)", got, err)
	}
}
