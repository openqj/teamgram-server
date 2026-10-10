package core

import (
	"context"
	"errors"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	apifullDao "github.com/teamgram/teamgram-server/app/bff/apifull/internal/dao"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/persist"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/svc"
	sync_client "github.com/teamgram/teamgram-server/app/messenger/sync/client"
	syncpb "github.com/teamgram/teamgram-server/app/messenger/sync/sync"
	user_client "github.com/teamgram/teamgram-server/app/service/biz/user/client"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

type emojiStatusUserClient struct {
	user_client.UserClient
	request *userpb.TLUserUpdateEmojiStatus
	result  *mtproto.Bool
	err     error
}

func (c *emojiStatusUserClient) UserUpdateEmojiStatus(_ context.Context, in *userpb.TLUserUpdateEmojiStatus) (*mtproto.Bool, error) {
	c.request = in
	return c.result, c.err
}

type emojiStatusSyncClient struct {
	sync_client.SyncClient
	request *syncpb.TLSyncUpdatesNotMe
	err     error
}

func (c *emojiStatusSyncClient) SyncUpdatesNotMe(_ context.Context, in *syncpb.TLSyncUpdatesNotMe) (*mtproto.Void, error) {
	c.request = in
	return mtproto.EmptyVoid, c.err
}

func TestEmojiStatusRoundtrip(t *testing.T) {
	const userID = int64(987654321)
	client := &emojiStatusUserClient{result: mtproto.BoolTrue}
	syncer := &emojiStatusSyncClient{}
	c := &ApiFullCore{
		ctx:    context.Background(),
		MD:     &metadata.RpcMetadata{UserId: userID, PermAuthKeyId: 654321},
		svcCtx: &svc.ServiceContext{Dao: &apifullDao.Dao{UserClient: client, SyncClient: syncer}},
	}
	st := mtproto.MakeTLEmojiStatus(&mtproto.EmojiStatus{
		DocumentId:      42,
		Until_FLAGINT32: mtproto.MakeFlagsInt32(1234),
	}).To_EmojiStatus()
	if _, err := c.AccountUpdateEmojiStatus(&mtproto.TLAccountUpdateEmojiStatus{EmojiStatus: st}); err != nil {
		t.Fatal(err)
	}
	if client.request == nil || client.request.GetUserId() != userID || client.request.GetEmojiStatusDocumentId() != 42 || client.request.GetEmojiStatusUntil() != 1234 {
		t.Fatalf("user service update request: %+v", client.request)
	}
	if syncer.request == nil || syncer.request.GetUserId() != userID || syncer.request.GetPermAuthKeyId() != 654321 {
		t.Fatalf("sync update request: %+v", syncer.request)
	}
	statusUpdate := syncer.request.GetUpdates().GetUpdates()[0].To_UpdateUserEmojiStatus()
	if statusUpdate == nil || statusUpdate.GetData2().GetEmojiStatus().GetDocumentId() != 42 {
		t.Fatalf("sync update status: %+v", syncer.request.GetUpdates())
	}
	got, err := c.AccountGetRecentEmojiStatuses(&mtproto.TLAccountGetRecentEmojiStatuses{})
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || len(got.GetStatuses()) == 0 || got.GetStatuses()[0].GetDocumentId() != 42 {
		t.Fatalf("emoji status: %+v", got)
	}
}

func TestEmojiStatusRecentHashAndClear(t *testing.T) {
	const userID int64 = 987654326
	c := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: userID}}
	status := mtproto.MakeTLEmojiStatus(&mtproto.EmojiStatus{DocumentId: 123}).To_EmojiStatus()
	if err := saveRecentEmojiStatuses(userID, []*mtproto.EmojiStatus{status}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if raw, err := persist.Default.Get(recentEmojiStatusKey(userID)); err == nil {
			_, _ = persist.CompareAndDelete(recentEmojiStatusKey(userID), raw)
		}
	})

	full, err := c.AccountGetRecentEmojiStatuses(&mtproto.TLAccountGetRecentEmojiStatuses{})
	if err != nil {
		t.Fatal(err)
	}
	if full.GetPredicateName() != mtproto.Predicate_account_emojiStatuses || full.GetHash() == 0 || len(full.GetStatuses()) != 1 {
		t.Fatalf("full recent statuses = %#v", full)
	}
	notModified, err := c.AccountGetRecentEmojiStatuses(&mtproto.TLAccountGetRecentEmojiStatuses{Hash: full.GetHash()})
	if err != nil {
		t.Fatal(err)
	}
	if notModified.GetPredicateName() != mtproto.Predicate_account_emojiStatusesNotModified {
		t.Fatalf("same hash result = %#v, want notModified", notModified)
	}
	if ok, err := c.AccountClearRecentEmojiStatuses(nil); err != nil || !mtproto.FromBool(ok) {
		t.Fatalf("clear = (%v, %v)", ok, err)
	}
	cleared, err := c.AccountGetRecentEmojiStatuses(&mtproto.TLAccountGetRecentEmojiStatuses{Hash: full.GetHash()})
	if err != nil {
		t.Fatal(err)
	}
	if cleared.GetPredicateName() != mtproto.Predicate_account_emojiStatuses || len(cleared.GetStatuses()) != 0 {
		t.Fatalf("cleared statuses = %#v", cleared)
	}
}

func TestEmojiStatusUpdatePropagatesUserServiceError(t *testing.T) {
	const userID = int64(987654322)
	const prior = `{"predicate_name":"emojiStatus","document_id":7}`
	if err := persist.Default.Set(emojiStatusKey(userID), prior); err != nil {
		t.Fatal(err)
	}
	wantErr := errors.New("user service unavailable")
	client := &emojiStatusUserClient{err: wantErr}
	syncer := &emojiStatusSyncClient{}
	c := &ApiFullCore{
		ctx:    context.Background(),
		MD:     &metadata.RpcMetadata{UserId: userID},
		svcCtx: &svc.ServiceContext{Dao: &apifullDao.Dao{UserClient: client, SyncClient: syncer}},
	}
	st := mtproto.MakeTLEmojiStatus(&mtproto.EmojiStatus{DocumentId: 42}).To_EmojiStatus()
	if _, err := c.AccountUpdateEmojiStatus(&mtproto.TLAccountUpdateEmojiStatus{EmojiStatus: st}); !errors.Is(err, wantErr) {
		t.Fatalf("error = %v, want %v", err, wantErr)
	}
	if got, err := persist.Default.Get(emojiStatusKey(userID)); err != nil || got != prior {
		t.Fatalf("active status after failed update = %q, err = %v", got, err)
	}
	if syncer.request != nil {
		t.Fatalf("failed user update was pushed: %+v", syncer.request)
	}
}

func TestEmojiStatusUpdateRejectsUnsupportedCollectible(t *testing.T) {
	client := &emojiStatusUserClient{result: mtproto.BoolTrue}
	syncer := &emojiStatusSyncClient{}
	c := &ApiFullCore{
		MD:     &metadata.RpcMetadata{UserId: 987654323},
		svcCtx: &svc.ServiceContext{Dao: &apifullDao.Dao{UserClient: client, SyncClient: syncer}},
	}
	st := mtproto.MakeTLEmojiStatusCollectible(&mtproto.EmojiStatus{CollectibleId: 10, DocumentId: 42}).To_EmojiStatus()
	if _, err := c.AccountUpdateEmojiStatus(&mtproto.TLAccountUpdateEmojiStatus{EmojiStatus: st}); !errors.Is(err, mtproto.ErrMethodNotImpl) {
		t.Fatalf("error = %v, want METHOD_NOT_IMPL", err)
	}
	if client.request != nil {
		t.Fatalf("unsupported collectible reached user service: %+v", client.request)
	}
	if syncer.request != nil {
		t.Fatalf("unsupported collectible was pushed: %+v", syncer.request)
	}
}

func TestEmojiStatusUpdatePropagatesSyncError(t *testing.T) {
	wantErr := errors.New("sync unavailable")
	client := &emojiStatusUserClient{result: mtproto.BoolTrue}
	syncer := &emojiStatusSyncClient{err: wantErr}
	c := &ApiFullCore{
		MD:     &metadata.RpcMetadata{UserId: 987654324},
		svcCtx: &svc.ServiceContext{Dao: &apifullDao.Dao{UserClient: client, SyncClient: syncer}},
	}
	st := mtproto.MakeTLEmojiStatus(&mtproto.EmojiStatus{DocumentId: 99}).To_EmojiStatus()
	if _, err := c.AccountUpdateEmojiStatus(&mtproto.TLAccountUpdateEmojiStatus{EmojiStatus: st}); !errors.Is(err, wantErr) {
		t.Fatalf("error = %v, want %v", err, wantErr)
	}
	if client.request == nil || syncer.request == nil {
		t.Fatalf("expected persistent update before sync failure: user=%+v sync=%+v", client.request, syncer.request)
	}
}

func TestEmojiStatusMethodsFailClosedWithoutCatalogOrBackend(t *testing.T) {
	if persist.StickerProviderReady() {
		t.Skip("PostgreSQL emoji catalog is configured; catalog-backed behavior is covered by emoji_status_catalog_postgres_test.go")
	}
	c := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: 987654325}}
	tests := []struct {
		name string
		call func() (bool, error)
	}{
		{
			name: "default statuses",
			call: func() (bool, error) {
				result, err := c.AccountGetDefaultEmojiStatuses(&mtproto.TLAccountGetDefaultEmojiStatuses{})
				return result != nil, err
			},
		},
		{
			name: "channel default statuses",
			call: func() (bool, error) {
				result, err := c.AccountGetChannelDefaultEmojiStatuses(&mtproto.TLAccountGetChannelDefaultEmojiStatuses{})
				return result != nil, err
			},
		},
		{
			name: "channel restricted emojis",
			call: func() (bool, error) {
				result, err := c.AccountGetChannelRestrictedStatusEmojis(&mtproto.TLAccountGetChannelRestrictedStatusEmojis{})
				return result != nil, err
			},
		},
		{
			name: "collectible statuses",
			call: func() (bool, error) {
				result, err := c.AccountGetCollectibleEmojiStatuses(&mtproto.TLAccountGetCollectibleEmojiStatuses{})
				return result != nil && len(result.GetStatuses()) == 0, err
			},
		},
		{
			name: "channel status update",
			call: func() (bool, error) {
				result, err := c.ChannelsUpdateEmojiStatus(&mtproto.TLChannelsUpdateEmojiStatus{})
				return result != nil, err
			},
		},
		{
			name: "bot target status update",
			call: func() (bool, error) {
				result, err := c.BotsUpdateUserEmojiStatus(&mtproto.TLBotsUpdateUserEmojiStatus{})
				return result != nil, err
			},
		},
		{
			name: "bot status permission update",
			call: func() (bool, error) {
				result, err := c.BotsToggleUserEmojiStatusPermission(&mtproto.TLBotsToggleUserEmojiStatusPermission{})
				return result != nil, err
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.name == "collectible statuses" {
				if result, err := tt.call(); !result || err != nil {
					t.Fatalf("call() = (%v, %v), want empty success", result, err)
				}
				return
			}
			if result, err := tt.call(); result || !errors.Is(err, mtproto.ErrMethodNotImpl) {
				t.Fatalf("call() = (%v, %v), want METHOD_NOT_IMPL", result, err)
			}
		})
	}
}
