package core

import (
	"context"
	"crypto/sha256"
	"errors"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/messenger/sync/internal/dao"
	"github.com/teamgram/teamgram-server/app/messenger/sync/internal/svc"
	"github.com/teamgram/teamgram-server/app/messenger/sync/internal/testutil"
	"github.com/teamgram/teamgram-server/app/messenger/sync/sync"
	chat_client "github.com/teamgram/teamgram-server/app/service/biz/chat/client"
	"github.com/teamgram/teamgram-server/app/service/biz/chat/chat"
	status_client "github.com/teamgram/teamgram-server/app/service/status/client"
	"github.com/teamgram/teamgram-server/app/service/status/status"
)

type syncStatusStub struct {
	status_client.StatusClient
	err      error
	calls    int
	sessions *status.UserSessionEntryList
	failUser int64
}

func (s *syncStatusStub) StatusGetUserOnlineSessions(_ context.Context, in *status.TLStatusGetUserOnlineSessions) (*status.UserSessionEntryList, error) {
	s.calls++
	if s.failUser != 0 && in.GetUserId() == s.failUser {
		return nil, errors.New("recipient unavailable")
	}
	return s.sessions, s.err
}

type syncChatStub struct {
	chat_client.ChatClient
	ids []int64
	err error
}

func (s *syncChatStub) ChatGetChatParticipantIdList(context.Context, *chat.TLChatGetChatParticipantIdList) (*chat.Vector_Long, error) {
	return &chat.Vector_Long{Datas: s.ids}, s.err
}

func newSyncPostgresCore(t *testing.T) (*SyncCore, *syncStatusStub) {
	t.Helper()
	pool := testutil.PostgresPool(t)
	testutil.SeedAuth(t, pool, 7, 91, mtproto.AuthKeyTypePerm, false)
	testutil.SeedAuth(t, pool, 7, -92, mtproto.AuthKeyTypePerm, false)
	statusStub := &syncStatusStub{sessions: &status.UserSessionEntryList{}}
	hash := sha256.Sum256([]byte("request"))
	ctx := dao.WithDeliveryReceipt(context.Background(), dao.DeliveryReceipt{ConsumerGroup: "sync-core", Topic: "Sync-T", Partition: 0, Offset: 1, RequestHash: hash[:]})
	return New(ctx, &svc.ServiceContext{Dao: &dao.Dao{Postgres: &dao.Postgres{Pool: pool}, StatusClient: statusStub}}), statusStub
}

func coreSeqUpdate() *mtproto.Update {
	return mtproto.MakeTLUpdatePeerSettings(&mtproto.Update{Peer_PEER: mtproto.MakePeerUser(8), Settings: mtproto.MakeTLPeerSettings(&mtproto.PeerSettings{}).To_PeerSettings()}).To_Update()
}

func TestPostgresSyncPersistenceFailureStopsFanout(t *testing.T) {
	c, statusStub := newSyncPostgresCore(t)
	if _, err := c.svcCtx.Dao.Pool.Exec(context.Background(), `ALTER TABLE auth_seq_updates ADD CONSTRAINT reject_sync CHECK(auth_id<>91)`); err != nil {
		t.Fatal(err)
	}
	if reply, err := c.SyncPushUpdates(&sync.TLSyncPushUpdates{UserId: 7, Updates: mtproto.MakeUpdatesByUpdates(coreSeqUpdate())}); err == nil || reply != nil {
		t.Fatalf("failed persistence reply=%v error=%v", reply, err)
	}
	if statusStub.calls != 0 {
		t.Fatalf("fanout started before update persistence, calls=%d", statusStub.calls)
	}
}

func TestPostgresSyncFailedPushReplaysCommittedSeq(t *testing.T) {
	c, statusStub := newSyncPostgresCore(t)
	statusStub.err = errors.New("status unavailable")
	request := &sync.TLSyncPushUpdates{UserId: 7, Updates: mtproto.MakeUpdatesByUpdates(coreSeqUpdate())}
	if reply, err := c.SyncPushUpdates(request); err == nil || reply != nil {
		t.Fatalf("failed fanout reply=%v error=%v", reply, err)
	}
	var delivered bool
	if err := c.svcCtx.Dao.Pool.QueryRow(context.Background(), `SELECT delivered FROM sync_delivery_receipts`).Scan(&delivered); err != nil || delivered {
		t.Fatalf("failed fanout receipt delivered=%v error=%v", delivered, err)
	}
	statusStub.err = nil
	if reply, err := c.SyncPushUpdates(request); err != nil || reply == nil {
		t.Fatalf("retry reply=%v error=%v", reply, err)
	}
	var rows, maxSeq int
	if err := c.svcCtx.Dao.Pool.QueryRow(context.Background(), `SELECT count(*),max(seq) FROM auth_seq_updates`).Scan(&rows, &maxSeq); err != nil || rows != 2 || maxSeq != 1 {
		t.Fatalf("retry seq rows=%d max=%d error=%v", rows, maxSeq, err)
	}
	if err := c.svcCtx.Dao.Pool.QueryRow(context.Background(), `SELECT delivered FROM sync_delivery_receipts`).Scan(&delivered); err != nil || !delivered {
		t.Fatalf("successful fanout receipt delivered=%v error=%v", delivered, err)
	}
	calls := statusStub.calls
	if _, err := c.SyncPushUpdates(request); err != nil || statusStub.calls != calls {
		t.Fatalf("completed replay performed fanout, calls=%d error=%v", statusStub.calls, err)
	}
}

func TestPostgresSyncNotMeExcludesPermanentAuthorization(t *testing.T) {
	c, _ := newSyncPostgresCore(t)
	if _, err := c.SyncUpdatesNotMe(&sync.TLSyncUpdatesNotMe{UserId: 7, PermAuthKeyId: 91, Updates: mtproto.MakeUpdatesByUpdates(coreSeqUpdate())}); err != nil {
		t.Fatal(err)
	}
	var authID int64
	if err := c.svcCtx.Dao.Pool.QueryRow(context.Background(), `SELECT auth_id FROM auth_seq_updates`).Scan(&authID); err != nil || authID != -92 {
		t.Fatalf("notMe auth=%d error=%v", authID, err)
	}
}

func TestPostgresSyncStaleSessionPreservesPendingReceipt(t *testing.T) {
	c, statusStub := newSyncPostgresCore(t)
	statusStub.sessions.UserSessions = []*status.SessionEntry{{PermAuthKeyId: 91, AuthKeyId: 55, Gateway: "removed-session"}}
	request := &sync.TLSyncPushUpdates{UserId: 7, Updates: mtproto.MakeUpdatesByUpdates(coreSeqUpdate())}
	if reply, err := c.SyncPushUpdates(request); err == nil || reply != nil {
		t.Fatalf("stale session reply=%v error=%v", reply, err)
	}
	var delivered bool
	var date int32
	if err := c.svcCtx.Dao.Pool.QueryRow(context.Background(), `SELECT delivered FROM sync_delivery_receipts`).Scan(&delivered); err != nil || delivered {
		t.Fatalf("stale session receipt delivered=%v error=%v", delivered, err)
	}
	if err := c.svcCtx.Dao.Pool.QueryRow(context.Background(), `SELECT date2 FROM auth_seq_updates WHERE auth_id=91`).Scan(&date); err != nil {
		t.Fatal(err)
	}
	statusStub.sessions.UserSessions = nil
	if _, err := c.SyncPushUpdates(request); err != nil {
		t.Fatalf("offline retry failed: %v", err)
	}
	var rows int
	var replayDate int32
	if err := c.svcCtx.Dao.Pool.QueryRow(context.Background(), `SELECT count(*),max(date2) FROM auth_seq_updates WHERE auth_id=91`).Scan(&rows, &replayDate); err != nil || rows != 1 || replayDate != date {
		t.Fatalf("session retry rows=%d date=%d originalDate=%d error=%v", rows, replayDate, date, err)
	}
}

func TestPostgresSyncBroadcastReceiptIsPerRecipient(t *testing.T) {
	c, statusStub := newSyncPostgresCore(t)
	testutil.SeedAuth(t, c.svcCtx.Dao.Pool, 8, 81, mtproto.AuthKeyTypePerm, false)
	testutil.SeedAuth(t, c.svcCtx.Dao.Pool, 9, 82, mtproto.AuthKeyTypePerm, false)
	c.svcCtx.Dao.ChatClient = &syncChatStub{ids: []int64{7, 8, 9}}
	statusStub.failUser = 9
	request := &sync.TLSyncBroadcastUpdates{BroadcastType: sync.BroadcastTypeChat, ChatId: 5, ExcludeIdList: []int64{8}, Updates: mtproto.MakeUpdatesByUpdates(coreSeqUpdate())}
	if reply, err := c.SyncBroadcastUpdates(request); err == nil || reply != nil {
		t.Fatalf("partial broadcast reply=%v error=%v", reply, err)
	}
	var complete, pending, excluded int
	if err := c.svcCtx.Dao.Pool.QueryRow(context.Background(), `SELECT count(*) FILTER(WHERE delivered),count(*) FILTER(WHERE NOT delivered),count(*) FILTER(WHERE user_id=8) FROM sync_delivery_receipts`).Scan(&complete, &pending, &excluded); err != nil || complete != 1 || pending != 1 || excluded != 0 {
		t.Fatalf("receipt complete=%d pending=%d excluded=%d error=%v", complete, pending, excluded, err)
	}
	statusStub.failUser = 0
	if _, err := c.SyncBroadcastUpdates(request); err != nil {
		t.Fatal(err)
	}
	var rows, maxSeq int
	if err := c.svcCtx.Dao.Pool.QueryRow(context.Background(), `SELECT count(*),max(seq) FROM auth_seq_updates`).Scan(&rows, &maxSeq); err != nil || rows != 3 || maxSeq != 1 {
		t.Fatalf("broadcast replay rows=%d maxSeq=%d error=%v", rows, maxSeq, err)
	}
	if statusStub.calls != 3 {
		t.Fatalf("successful recipient re-fanned out, calls=%d", statusStub.calls)
	}
}

func TestPostgresSyncPtsFailureStopsFanout(t *testing.T) {
	c, statusStub := newSyncPostgresCore(t)
	if _, err := c.svcCtx.Dao.Pool.Exec(context.Background(), `ALTER TABLE user_pts_updates ADD CONSTRAINT reject_sync_pts CHECK(user_id<>7)`); err != nil {
		t.Fatal(err)
	}
	update := mtproto.MakeTLUpdateDeleteMessages(&mtproto.Update{Messages: []int32{1}, Pts_INT32: 1, PtsCount: 1}).To_Update()
	if reply, err := c.SyncPushUpdates(&sync.TLSyncPushUpdates{UserId: 7, Updates: mtproto.MakeUpdatesByUpdates(update)}); err == nil || reply != nil || statusStub.calls != 0 {
		t.Fatalf("PTS failure reached fanout, reply=%v error=%v calls=%d", reply, err, statusStub.calls)
	}
}
