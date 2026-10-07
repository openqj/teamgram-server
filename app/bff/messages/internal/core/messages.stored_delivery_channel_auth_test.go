package core

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/apifull/channelview"
	"github.com/teamgram/teamgram-server/app/bff/messages/internal/dao"
	"github.com/teamgram/teamgram-server/app/bff/messages/internal/svc"
	sync_client "github.com/teamgram/teamgram-server/app/messenger/sync/client"
	syncpb "github.com/teamgram/teamgram-server/app/messenger/sync/sync"
	"github.com/zeromicro/go-zero/core/logx"
)

type storedChannelSyncClient struct {
	sync_client.SyncClient
	calls []*syncpb.TLSyncPushUpdatesIfNot
}

func (s *storedChannelSyncClient) SyncPushUpdatesIfNot(_ context.Context, in *syncpb.TLSyncPushUpdatesIfNot) (*mtproto.Void, error) {
	s.calls = append(s.calls, in)
	return mtproto.EmptyVoid, nil
}

func TestStoredChannelDeliveryValidatesInputPeer(t *testing.T) {
	dsn := os.Getenv("APIFULL_MYSQL_DSN")
	if dsn == "" {
		t.Skip("APIFULL_MYSQL_DSN is not configured")
	}
	if err := channelview.Open(dsn); err != nil {
		t.Fatal("open channel store:", err)
	}
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Fatal("open channel store database:", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	channelID := time.Now().UnixNano()
	const owner int64 = 98001
	const member int64 = 98002
	const outsider int64 = 98003
	if _, err = db.Exec(`INSERT INTO apifull_channel
		(id, access_hash, creator_user_id, title, broadcast, megagroup, created_at)
		VALUES (?,?,?,?,?,?,?)`, channelID, channelID, owner, "stored-channel-delivery", 1, 0, time.Now().Unix()); err != nil {
		t.Fatal("insert channel fixture:", err)
	}
	if _, err = db.Exec(`INSERT INTO apifull_channel_member (channel_id, user_id, invited_by_user_id, joined_at)
		VALUES (?,?,?,?)`, channelID, member, owner, time.Now().Unix()); err != nil {
		t.Fatal("insert channel member fixture:", err)
	}
	t.Cleanup(func() {
		for _, query := range []string{
			`DELETE FROM apifull_channel_message_hidden WHERE channel_id=?`,
			`DELETE FROM apifull_channel_message WHERE channel_id=?`,
			`DELETE FROM apifull_channel_message_seq WHERE channel_id=?`,
			`DELETE FROM apifull_channel_read_state WHERE channel_id=?`,
			`DELETE FROM apifull_channel_member WHERE channel_id=?`,
			`DELETE FROM apifull_channel WHERE id=?`,
		} {
			if _, err := db.Exec(query, channelID); err != nil {
				t.Errorf("clean up channel fixture: %v", err)
			}
		}
	})

	input := mtproto.MakeTLInputPeerChannel(&mtproto.InputPeer{ChannelId: channelID, AccessHash: channelID}).To_InputPeer()
	syncer := &storedChannelSyncClient{}
	coreFor := func(userID int64) *MessagesCore {
		return &MessagesCore{
			ctx:    context.Background(),
			svcCtx: &svc.ServiceContext{Dao: &dao.Dao{SyncClient: syncer}},
			MD:     &metadata.RpcMetadata{UserId: userID, PermAuthKeyId: 99101},
			Logger: logx.WithContext(context.Background()),
		}
	}
	ownerCore := coreFor(owner)
	request := &mtproto.TLMessagesSendMessage{Peer: input, Message: "stored text", RandomId: 70001}
	updates, err := ownerCore.MessagesSendMessage(request)
	if err != nil || updates == nil || len(updates.GetUpdates()) != 1 || updates.GetUpdates()[0].GetPredicateName() != mtproto.Predicate_updateNewChannelMessage {
		t.Fatalf("valid channel text send: updates=%+v err=%v", updates, err)
	}
	messageID := updates.GetUpdates()[0].GetMessage_MESSAGE().GetId()
	retried, err := ownerCore.MessagesSendMessage(request)
	if err != nil || retried == nil || len(retried.GetUpdates()) != 1 || retried.GetUpdates()[0].GetMessage_MESSAGE().GetId() != messageID {
		t.Fatalf("channel retry did not reuse message id %d: updates=%+v err=%v", messageID, retried, err)
	}
	conflicting := *request
	conflicting.Message = "different text"
	if _, err = ownerCore.MessagesSendMessage(&conflicting); !errors.Is(err, mtproto.ErrRandomIdDuplicate) {
		t.Fatalf("reused random id with changed text: got %v, want RANDOM_ID_DUPLICATE", err)
	}

	history, err := channelview.HistoryForInputPeer(member, input, 0, 20)
	if err != nil {
		t.Fatal("member reads stored channel messages:", err)
	}
	texts := make(map[string]struct{}, len(history.GetMessages()))
	for _, message := range history.GetMessages() {
		texts[message.GetMessage()] = struct{}{}
	}
	if _, ok := texts["stored text"]; !ok {
		t.Fatalf("member history omitted stored text: %+v", history.GetMessages())
	}

	if _, err = coreFor(outsider).MessagesSendMessage(&mtproto.TLMessagesSendMessage{Peer: input, Message: "outsider write", RandomId: 70002}); !errors.Is(err, mtproto.ErrUserNotParticipant) {
		t.Fatalf("outsider channel send: got %v, want USER_NOT_PARTICIPANT", err)
	}
	badHash := mtproto.MakeTLInputPeerChannel(&mtproto.InputPeer{ChannelId: channelID, AccessHash: channelID + 1}).To_InputPeer()
	if _, err = ownerCore.MessagesSendMessage(&mtproto.TLMessagesSendMessage{Peer: badHash, Message: "bad hash write", RandomId: 70003}); !errors.Is(err, mtproto.ErrChannelInvalid) {
		t.Fatalf("bad-hash channel send: got %v, want CHANNEL_INVALID", err)
	}
	if history, err = channelview.HistoryForInputPeer(member, input, 0, 20); err != nil || len(history.GetMessages()) != 1 {
		t.Fatalf("rejected sends changed stored messages: history=%+v err=%v", history, err)
	}
}

func TestPushChannelUpdatesFansOutToVisibleMembers(t *testing.T) {
	dsn := os.Getenv("APIFULL_MYSQL_DSN")
	if dsn == "" {
		t.Skip("APIFULL_MYSQL_DSN is not configured")
	}
	if err := channelview.Open(dsn); err != nil {
		t.Fatal("open channel store:", err)
	}
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Fatal("open channel store database:", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	channelID := time.Now().UnixNano()
	const owner, visibleMember, hiddenMember int64 = 98101, 98102, 98103
	if _, err = db.Exec(`INSERT INTO apifull_channel
		(id, access_hash, creator_user_id, title, broadcast, megagroup, created_at)
		VALUES (?,?,?,?,?,?,?)`, channelID, channelID, owner, "channel-update-fanout", 1, 0, time.Now().Unix()); err != nil {
		t.Fatal("insert channel fixture:", err)
	}
	if _, err = db.Exec(`INSERT INTO apifull_channel_member (channel_id, user_id, invited_by_user_id, joined_at)
		VALUES (?,?,?,?), (?,?,?,?)`, channelID, visibleMember, owner, time.Now().Unix(), channelID, hiddenMember, owner, time.Now().Unix()); err != nil {
		t.Fatal("insert channel member fixtures:", err)
	}
	if _, err = db.Exec(`UPDATE apifull_channel_member SET banned_rights=? WHERE channel_id=? AND user_id=?`, `{"view_messages":true}`, channelID, hiddenMember); err != nil {
		t.Fatal("hide channel member fixture:", err)
	}
	t.Cleanup(func() {
		for _, query := range []string{
			`DELETE FROM apifull_channel_member WHERE channel_id=?`,
			`DELETE FROM apifull_channel WHERE id=?`,
		} {
			if _, err := db.Exec(query, channelID); err != nil {
				t.Errorf("clean up channel fixture: %v", err)
			}
		}
	})

	syncer := &storedChannelSyncClient{}
	ctx := context.Background()
	core := &MessagesCore{
		ctx:    ctx,
		svcCtx: &svc.ServiceContext{Dao: &dao.Dao{SyncClient: syncer}},
		MD:     &metadata.RpcMetadata{UserId: owner, PermAuthKeyId: 99101},
		Logger: logx.WithContext(ctx),
	}
	updates := mtproto.MakeUpdatesByUpdates()
	if err = core.pushChannelUpdates(channelID, updates); err != nil {
		t.Fatal("push channel updates:", err)
	}
	if len(syncer.calls) != 2 {
		t.Fatalf("fanout calls = %d, want owner and visible member", len(syncer.calls))
	}
	got := map[int64]struct{}{}
	for _, call := range syncer.calls {
		got[call.GetUserId()] = struct{}{}
		if len(call.GetExcludes()) != 1 || call.GetExcludes()[0] != 99101 {
			t.Fatalf("fanout excludes = %v, want current perm auth key", call.GetExcludes())
		}
		if len(call.GetUpdates().GetChats()) != 1 || call.GetUpdates().GetChats()[0].GetId() != channelID {
			t.Fatalf("fanout channel entities = %v, want channel %d", call.GetUpdates().GetChats(), channelID)
		}
	}
	if _, ok := got[owner]; !ok {
		t.Errorf("fanout omitted owner %d", owner)
	}
	if _, ok := got[visibleMember]; !ok {
		t.Errorf("fanout omitted visible member %d", visibleMember)
	}
	if _, ok := got[hiddenMember]; ok {
		t.Errorf("fanout included member barred from viewing channel %d", hiddenMember)
	}
}
