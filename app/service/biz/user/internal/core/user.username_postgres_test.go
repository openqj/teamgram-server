package core

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	userdao "github.com/teamgram/teamgram-server/app/service/biz/user/internal/dao"
	"github.com/teamgram/teamgram-server/app/service/biz/user/internal/svc"
	"github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

func TestUserPostgresUsernameOwnershipAndOrder(t *testing.T) {
	pg := userPostgresTest(t, 1)
	ctx := context.Background()
	ownerID := time.Now().UnixNano()
	otherID, channelID := ownerID+1, ownerID+2
	insertUserPostgresFixtures(t, pg, ownerID, otherID)
	if _, err := pg.Pool.Exec(ctx, `INSERT INTO apifull_channel(id,creator_user_id,access_hash,title,created_at) VALUES($1,$2,1,'Username owner',1)`, channelID, ownerID); err != nil {
		t.Fatal(err)
	}
	names := []string{fmt.Sprintf("basic_%d", ownerID), fmt.Sprintf("first_%d", ownerID), fmt.Sprintf("second_%d", ownerID), fmt.Sprintf("foreign_%d", ownerID), fmt.Sprintf("channel_%d", ownerID)}
	t.Cleanup(func() {
		_, _ = pg.Pool.Exec(ctx, `DELETE FROM username WHERE username=ANY($1::text[])`, names)
		_, _ = pg.Pool.Exec(ctx, `DELETE FROM apifull_channel WHERE id=$1`, channelID)
	})
	for i, name := range names {
		peerType, peerID := int32(mtproto.PEER_USER), ownerID
		if i == 3 {
			peerID = otherID
		} else if i == 4 {
			peerType, peerID = mtproto.PEER_CHANNEL, channelID
		}
		if _, err := pg.Pool.Exec(ctx, `INSERT INTO username(username,peer_type,peer_id,editable,active,order2) VALUES($1,$2,$3,$4,$5,$6)`, name, peerType, peerID, i == 0, i != 2, i); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := pg.Pool.Exec(ctx, `UPDATE users SET username=$2 WHERE id=$1`, ownerID, names[0]); err != nil {
		t.Fatal(err)
	}
	c := New(ctx, &svc.ServiceContext{Dao: &userdao.Dao{Postgres: pg}})
	c.MD = &metadata.RpcMetadata{UserId: ownerID}
	toggle := func(peerType int32, peerID int64, name string, active bool) error {
		t.Helper()
		got, err := c.UserToggleUsername(&user.TLUserToggleUsername{PeerType: peerType, PeerId: peerID, Username: name, Active: mtproto.ToBool(active)})
		if err == nil && !mtproto.FromBool(got) {
			t.Fatalf("successful toggle returned %v", got)
		}
		if err != nil && got != nil {
			t.Fatalf("failed toggle returned nonnil result %v", got)
		}
		return err
	}
	reorder := func(order []string) error {
		t.Helper()
		got, err := c.UserReorderUsernames(&user.TLUserReorderUsernames{PeerType: mtproto.PEER_USER, PeerId: ownerID, UsernameList: order})
		if err == nil && !mtproto.FromBool(got) {
			t.Fatalf("successful reorder returned %v", got)
		}
		if err != nil && got != nil {
			t.Fatalf("failed reorder returned nonnil result %v", got)
		}
		return err
	}
	if err := toggle(mtproto.PEER_USER, ownerID, names[3], false); !errors.Is(err, mtproto.ErrUsernameInvalid) {
		t.Fatalf("foreign username toggle error = %v", err)
	}
	if err := toggle(mtproto.PEER_USER, ownerID, names[4], false); !errors.Is(err, mtproto.ErrUsernameInvalid) {
		t.Fatalf("channel username as user toggle error = %v", err)
	}
	if err := toggle(mtproto.PEER_USER, ownerID, names[0], false); !errors.Is(err, mtproto.ErrUsernameInvalid) {
		t.Fatalf("editable username toggle error = %v", err)
	}
	if err := toggle(mtproto.PEER_USER, ownerID, names[1], true); !errors.Is(err, mtproto.ErrUsernameNotModified) {
		t.Fatalf("same username status error = %v", err)
	}
	if err := toggle(mtproto.PEER_USER, ownerID, strings.ToUpper(names[2]), true); err != nil {
		t.Fatal(err)
	}
	for _, order := range [][]string{{names[1]}, {names[0], names[1], names[3]}, {names[0], names[1], names[1]}} {
		if err := reorder(order); !errors.Is(err, mtproto.ErrOrderInvalid) {
			t.Fatalf("invalid order %v error = %v", order, err)
		}
	}
	if err := reorder([]string{strings.ToUpper(names[2]), names[1], names[0]}); err != nil {
		t.Fatal(err)
	}
	if err := reorder([]string{names[2], names[1], names[0]}); !errors.Is(err, mtproto.ErrUsernameNotModified) {
		t.Fatalf("same order error = %v", err)
	}
	immutable, err := c.UserGetImmutableUser(&user.TLUserGetImmutableUser{Id: ownerID})
	if err != nil || len(immutable.GetUser().GetUsernames()) != 3 {
		t.Fatalf("complete usernames = (%v, %v)", immutable, err)
	}
	for i, index := range []int{2, 1, 0} {
		if immutable.GetUser().GetUsernames()[i].GetUsername() != names[index] {
			t.Fatalf("username at %d = %v, want %s", i, immutable.GetUser().GetUsernames()[i], names[index])
		}
	}
	if err := toggle(mtproto.PEER_USER, ownerID, names[2], false); err != nil {
		t.Fatal(err)
	}
	if err := reorder([]string{names[0], names[1]}); err != nil {
		t.Fatal(err)
	}
	immutable, err = c.UserGetImmutableUser(&user.TLUserGetImmutableUser{Id: ownerID})
	if err != nil || len(immutable.GetUser().GetUsernames()) != 3 || immutable.GetUser().GetUsernames()[2].GetActive() {
		t.Fatalf("inactive usernames = (%v, %v)", immutable, err)
	}
	if err := toggle(mtproto.PEER_CHANNEL, channelID, names[4], false); err != nil {
		t.Fatal(err)
	}
	var foreignActive, basicActive bool
	if err := pg.Pool.QueryRow(ctx, `SELECT active FROM username WHERE username=$1`, names[3]).Scan(&foreignActive); err != nil {
		t.Fatal(err)
	}
	if err := pg.Pool.QueryRow(ctx, `SELECT active FROM username WHERE username=$1`, names[0]).Scan(&basicActive); err != nil {
		t.Fatal(err)
	}
	if !foreignActive || !basicActive {
		t.Fatal("foreign or basic username was deactivated")
	}
	if got, err := c.UserToggleUsername(&user.TLUserToggleUsername{PeerType: mtproto.PEER_USER, PeerId: ownerID, Username: names[1]}); got != nil || !errors.Is(err, mtproto.ErrInputRequestInvalid) {
		t.Fatalf("missing active Bool = (%v, %v)", got, err)
	}
}

func TestUserPostgresUsernameOrderRollback(t *testing.T) {
	pg := userPostgresTest(t, 1)
	ctx := context.Background()
	ownerID := time.Now().UnixNano()
	insertUserPostgresFixtures(t, pg, ownerID)
	if _, err := pg.Pool.Exec(ctx, `CREATE TEMP TABLE username (LIKE public.username INCLUDING ALL)`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = pg.Pool.Exec(ctx, `DROP TABLE pg_temp.username`) })
	if _, err := pg.Pool.Exec(ctx, `ALTER TABLE pg_temp.username ADD CHECK(username<>'rollback_first' OR order2<>1)`); err != nil {
		t.Fatal(err)
	}
	if _, err := pg.Pool.Exec(ctx, `INSERT INTO pg_temp.username(username,peer_type,peer_id,active,order2) VALUES('rollback_first',$1,$2,TRUE,0),('rollback_second',$1,$2,TRUE,1)`, mtproto.PEER_USER, ownerID); err != nil {
		t.Fatal(err)
	}
	c := New(ctx, &svc.ServiceContext{Dao: &userdao.Dao{Postgres: pg}})
	c.MD = &metadata.RpcMetadata{UserId: ownerID}
	if got, err := c.UserReorderUsernames(&user.TLUserReorderUsernames{PeerType: mtproto.PEER_USER, PeerId: ownerID, UsernameList: []string{"rollback_second", "rollback_first"}}); got != nil || err == nil {
		t.Fatalf("failing reorder = (%v, %v), want error", got, err)
	}
	var firstOrder, secondOrder int64
	if err := pg.Pool.QueryRow(ctx, `SELECT order2 FROM username WHERE username='rollback_first'`).Scan(&firstOrder); err != nil {
		t.Fatal(err)
	}
	if err := pg.Pool.QueryRow(ctx, `SELECT order2 FROM username WHERE username='rollback_second'`).Scan(&secondOrder); err != nil {
		t.Fatal(err)
	}
	if firstOrder != 0 || secondOrder != 1 {
		t.Fatalf("orders after failed transaction = (%d, %d), want (0, 1)", firstOrder, secondOrder)
	}
	if _, err := pg.Pool.Exec(ctx, `ALTER TABLE pg_temp.username ADD CHECK(username<>'rollback_second' OR active=TRUE)`); err != nil {
		t.Fatal(err)
	}
	if got, err := c.UserToggleUsername(&user.TLUserToggleUsername{PeerType: mtproto.PEER_USER, PeerId: ownerID, Username: "rollback_second", Active: mtproto.BoolFalse}); got != nil || err == nil {
		t.Fatalf("failing toggle = (%v, %v), want error", got, err)
	}
	var active bool
	if err := pg.Pool.QueryRow(ctx, `SELECT active FROM username WHERE username='rollback_second'`).Scan(&active); err != nil || !active {
		t.Fatalf("status after failed toggle = (%v, %v), want active", active, err)
	}
}

func TestUserPostgresUsernameManagementPermissions(t *testing.T) {
	pg := userPostgresTest(t, 2)
	ctx := context.Background()
	ownerID := time.Now().UnixNano()
	botID, otherID, adminID, channelID := ownerID+1, ownerID+2, ownerID+3, ownerID+4
	insertUserPostgresFixtures(t, pg, ownerID, botID, otherID, adminID)
	if _, err := pg.Pool.Exec(ctx, `UPDATE users SET user_type=$2,is_bot=TRUE WHERE id=$1`, botID, user.UserTypeBot); err != nil {
		t.Fatal(err)
	}
	if err := pg.Store.Bots.InsertRegistryTx(ctx, pg.Pool, botID, ownerID, 0, fmt.Sprintf("permission-%d", botID)); err != nil {
		t.Fatal(err)
	}
	if _, err := pg.Pool.Exec(ctx, `INSERT INTO apifull_channel(id,creator_user_id,access_hash,title,created_at) VALUES($1,$2,1,'Admin permissions',1)`, channelID, ownerID); err != nil {
		t.Fatal(err)
	}
	if _, err := pg.Pool.Exec(ctx, `INSERT INTO apifull_channel_member(channel_id,user_id,joined_at,admin_rights) VALUES($1,$2,1,'{"change_info":true}')`, channelID, adminID); err != nil {
		t.Fatal(err)
	}
	names := []string{fmt.Sprintf("bot_permission_%d", ownerID), fmt.Sprintf("channel_permission_%d", ownerID)}
	t.Cleanup(func() {
		_, _ = pg.Pool.Exec(ctx, `DELETE FROM username WHERE username=ANY($1::text[])`, names)
		_, _ = pg.Pool.Exec(ctx, `DELETE FROM apifull_channel_member WHERE channel_id=$1`, channelID)
		_, _ = pg.Pool.Exec(ctx, `DELETE FROM apifull_channel WHERE id=$1`, channelID)
		_, _ = pg.Pool.Exec(ctx, `DELETE FROM bots WHERE bot_id=$1`, botID)
	})
	if _, err := pg.Pool.Exec(ctx, `INSERT INTO username(username,peer_type,peer_id,editable,active) VALUES($1,$2,$3,FALSE,TRUE),($4,$5,$6,FALSE,TRUE)`, names[0], mtproto.PEER_USER, botID, names[1], mtproto.PEER_CHANNEL, channelID); err != nil {
		t.Fatal(err)
	}
	c := New(ctx, &svc.ServiceContext{Dao: &userdao.Dao{Postgres: pg}})
	botToggle := &user.TLUserToggleUsername{PeerType: mtproto.PEER_USER, PeerId: botID, Username: names[0], Active: mtproto.BoolFalse}
	if got, err := c.UserToggleUsername(botToggle); got != nil || !errors.Is(err, mtproto.ErrAuthKeyUnregistered) {
		t.Fatalf("unauthenticated toggle = (%v, %v)", got, err)
	}
	c.MD = &metadata.RpcMetadata{UserId: otherID}
	if got, err := c.UserToggleUsername(botToggle); got != nil || !errors.Is(err, mtproto.ErrForbiddenUserBotInvalid) {
		t.Fatalf("other user bot toggle = (%v, %v)", got, err)
	}
	c.MD.UserId = ownerID
	if got, err := c.UserToggleUsername(botToggle); err != nil || !mtproto.FromBool(got) {
		t.Fatalf("creator bot toggle = (%v, %v)", got, err)
	}
	c.MD.UserId = otherID
	if got, err := c.UserDeactivateAllChannelUsernames(&user.TLUserDeactivateAllChannelUsernames{ChannelId: channelID}); got != nil || !errors.Is(err, mtproto.ErrChatAdminRequired) {
		t.Fatalf("nonmember channel deactivate = (%v, %v)", got, err)
	}
	c.MD.UserId = adminID
	if got, err := c.UserDeactivateAllChannelUsernames(&user.TLUserDeactivateAllChannelUsernames{ChannelId: channelID}); err != nil || !mtproto.FromBool(got) {
		t.Fatalf("admin channel deactivate = (%v, %v)", got, err)
	}
	channelToggle := &user.TLUserToggleUsername{PeerType: mtproto.PEER_CHANNEL, PeerId: channelID, Username: names[1], Active: mtproto.BoolTrue}
	if _, err := pg.Pool.Exec(ctx, `UPDATE apifull_channel_member SET admin_rights='' WHERE channel_id=$1 AND user_id=$2`, channelID, adminID); err != nil {
		t.Fatal(err)
	}
	if got, err := c.UserToggleUsername(channelToggle); got != nil || !errors.Is(err, mtproto.ErrChatAdminRequired) {
		t.Fatalf("revoked admin toggle = (%v, %v)", got, err)
	}
	if _, err := pg.Pool.Exec(ctx, `UPDATE apifull_channel_member SET admin_rights='{"change_info":true}',banned_rights='{"view_messages":true}' WHERE channel_id=$1 AND user_id=$2`, channelID, adminID); err != nil {
		t.Fatal(err)
	}
	if got, err := c.UserToggleUsername(channelToggle); got != nil || !errors.Is(err, mtproto.ErrChatAdminRequired) {
		t.Fatalf("kicked admin toggle = (%v, %v)", got, err)
	}
	c.MD.UserId = ownerID
	if got, err := c.UserToggleUsername(channelToggle); err != nil || !mtproto.FromBool(got) {
		t.Fatalf("creator channel toggle = (%v, %v)", got, err)
	}
}
