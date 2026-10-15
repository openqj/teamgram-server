package channelview

import (
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/domain"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/persist"
)

func TestPrivateChannelInvitePeekReadLifecyclePostgres(t *testing.T) {
	dsn := os.Getenv("APIFULL_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("APIFULL_POSTGRES_DSN is not configured")
	}
	if err := domain.OpenPostgresReadOnly(dsn); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = domain.Close() })
	cleanupDB, err := persist.OpenPostgresDB(dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cleanupDB.Close() })

	channelID := time.Now().UnixNano()
	ownerID := channelID + 1
	outsiderID := channelID + 2
	now := time.Now().Unix()
	link := fmt.Sprintf("peek-%d", channelID)
	deleted := false
	t.Cleanup(func() {
		if deleted {
			return
		}
		for _, query := range []string{
			`DELETE FROM apifull_channel_peek_access WHERE channel_id=?`,
			`DELETE FROM chat_invite_participants WHERE chat_id=?`,
			`DELETE FROM chat_invites WHERE chat_id=?`,
			`DELETE FROM apifull_channel_delivery_recipient WHERE delivery_id IN (SELECT id FROM apifull_channel_delivery_outbox WHERE channel_id=?)`,
			`DELETE FROM apifull_channel_delivery_outbox WHERE channel_id=?`,
			`DELETE FROM apifull_channel_event WHERE channel_id=?`,
			`DELETE FROM apifull_channel_message WHERE channel_id=?`,
			`DELETE FROM apifull_channel_message_seq WHERE channel_id=?`,
			`DELETE FROM apifull_channel_member WHERE channel_id=?`,
			`DELETE FROM apifull_channel WHERE id=?`,
		} {
			if _, cleanupErr := cleanupDB.Exec(query, channelID); cleanupErr != nil {
				t.Errorf("cleanup peek fixture: %v", cleanupErr)
			}
		}
	})

	if err = domain.SaveChannel(domain.Channel{
		ID: channelID, AccessHash: channelID, Creator: ownerID,
		Title: "private-preview", Broadcast: true,
	}); err != nil {
		t.Fatal(err)
	}
	invite, err := domain.CreateChannelInvite(domain.ChannelInvite{
		ChannelID: channelID, AdminID: ownerID, Link: link,
		ExpireDate: now + 300, Date: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = Post(ownerID, channelID, "preview message", now); err != nil {
		t.Fatal(err)
	}

	peek, err := CheckInvite(outsiderID, invite.Link)
	if err != nil {
		t.Fatalf("check private invite: %v", err)
	}
	if peek.GetPredicateName() != mtproto.Predicate_chatInvitePeek {
		t.Fatalf("invite predicate = %q, want %q", peek.GetPredicateName(), mtproto.Predicate_chatInvitePeek)
	}
	if peek.GetExpires() <= int32(now) || peek.GetExpires() > int32(invite.ExpireDate) {
		t.Fatalf("peek expiry = %d, want (%d,%d]", peek.GetExpires(), now, invite.ExpireDate)
	}
	if peek.GetChat() == nil || !peek.GetChat().GetLeft() {
		t.Fatalf("peek chat = %v, want left=true", peek.GetChat())
	}

	input := mtproto.MakeTLInputPeerChannel(&mtproto.InputPeer{
		ChannelId: channelID, AccessHash: channelID,
	}).To_InputPeer()
	history, err := HistoryForInputPeer(outsiderID, input, 0, 20)
	if err != nil || len(history.GetMessages()) != 1 || !history.GetChats()[0].GetLeft() {
		t.Fatalf("preview history = messages:%d chats:%v err:%v", len(history.GetMessages()), history.GetChats(), err)
	}
	search, err := SearchForInputPeer(outsiderID, input, "preview", 0, 0, 0, 0, 0, 0, 0, 20)
	if err != nil || len(search.GetMessages()) != 1 {
		t.Fatalf("preview search = messages:%d err:%v", len(search.GetMessages()), err)
	}
	if _, err = ValidateReadInputChannel(outsiderID, mtproto.MakeTLInputChannel(&mtproto.InputChannel{
		ChannelId: channelID, AccessHash: channelID,
	}).To_InputChannel()); err != nil {
		t.Fatalf("preview channels.getMessages validation: %v", err)
	}
	if _, err = ValidateChannelDifferenceInputChannel(outsiderID, mtproto.MakeTLInputChannel(&mtproto.InputChannel{
		ChannelId: channelID, AccessHash: channelID,
	}).To_InputChannel()); err != nil {
		t.Fatalf("preview channel difference validation: %v", err)
	}

	if _, err = domain.UpdateChannelInvite(channelID, invite.Link, "", domain.ChannelInviteUpdate{Revoked: true}, time.Now().Unix()); err != nil {
		t.Fatalf("revoke preview invite: %v", err)
	}
	if _, err = CheckInvite(outsiderID, invite.Link); !errors.Is(err, mtproto.ErrInviteHashExpired) {
		t.Fatalf("revoked preview invite error = %v, want INVITE_HASH_EXPIRED", err)
	}

	if _, err = cleanupDB.Exec(`UPDATE apifull_channel_peek_access SET expires_at=? WHERE channel_id=? AND user_id=?`, now-1, channelID, outsiderID); err != nil {
		t.Fatal(err)
	}
	if _, err = HistoryForInputPeer(outsiderID, input, 0, 20); !errors.Is(err, mtproto.ErrChannelPrivate) {
		t.Fatalf("expired preview history error = %v, want CHANNEL_PRIVATE", err)
	}
	if _, err = SearchForInputPeer(outsiderID, input, "preview", 0, 0, 0, 0, 0, 0, 0, 20); !errors.Is(err, mtproto.ErrChannelPrivate) {
		t.Fatalf("expired preview search error = %v, want CHANNEL_PRIVATE", err)
	}
	if _, err = ValidateChannelDifferenceInputChannel(outsiderID, mtproto.MakeTLInputChannel(&mtproto.InputChannel{
		ChannelId: channelID, AccessHash: channelID,
	}).To_InputChannel()); !errors.Is(err, mtproto.ErrUserNotParticipant) {
		t.Fatalf("expired preview difference error = %v, want USER_NOT_PARTICIPANT", err)
	}

	if err = domain.DeleteChannel(ownerID, channelID); err != nil {
		t.Fatal(err)
	}
	deleted = true
	var grants int
	if err = cleanupDB.QueryRow(`SELECT COUNT(*) FROM apifull_channel_peek_access WHERE channel_id=?`, channelID).Scan(&grants); err != nil {
		t.Fatal(err)
	}
	if grants != 0 {
		t.Fatalf("peek grants after channel deletion = %d, want 0", grants)
	}
}
