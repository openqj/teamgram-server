package core

import (
	"context"
	"errors"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/dialogs/internal/dao"
	"github.com/teamgram/teamgram-server/app/bff/dialogs/internal/svc"
	syncclient "github.com/teamgram/teamgram-server/app/messenger/sync/client"
	syncpb "github.com/teamgram/teamgram-server/app/messenger/sync/sync"
	dialogclient "github.com/teamgram/teamgram-server/app/service/biz/dialog/client"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/dialog"
	"github.com/zeromicro/go-zero/core/logx"
)

type markDialogUnreadDialogClientStub struct {
	dialogclient.DialogClient
	response *mtproto.Bool
	err      error
	request  *dialog.TLDialogMarkDialogUnread
}

func (s *markDialogUnreadDialogClientStub) DialogMarkDialogUnread(_ context.Context, in *dialog.TLDialogMarkDialogUnread) (*mtproto.Bool, error) {
	s.request = in
	return s.response, s.err
}

type markDialogUnreadSyncClientStub struct {
	syncclient.SyncClient
	requests []*syncpb.TLSyncUpdatesNotMe
	err      error
}

func (s *markDialogUnreadSyncClientStub) SyncUpdatesNotMe(_ context.Context, in *syncpb.TLSyncUpdatesNotMe) (*mtproto.Void, error) {
	s.requests = append(s.requests, in)
	return mtproto.EmptyVoid, s.err
}

func newMarkDialogUnreadTestCore(dialogs *markDialogUnreadDialogClientStub, syncStub *markDialogUnreadSyncClientStub) *DialogsCore {
	ctx := context.Background()
	return &DialogsCore{
		ctx: ctx,
		svcCtx: &svc.ServiceContext{Dao: &dao.Dao{
			DialogClient: dialogs,
			SyncClient:   syncStub,
		}},
		Logger: logx.WithContext(ctx),
		MD: &metadata.RpcMetadata{
			UserId:        42,
			PermAuthKeyId: 1234,
		},
	}
}

func markDialogUnreadRequest(unread bool) *mtproto.TLMessagesMarkDialogUnread {
	return &mtproto.TLMessagesMarkDialogUnread{
		Unread: unread,
		Peer: mtproto.MakeTLInputDialogPeer(&mtproto.InputDialogPeer{
			Peer: mtproto.MakeTLInputPeerChat(&mtproto.InputPeer{ChatId: 77}).To_InputPeer(),
		}).To_InputDialogPeer(),
	}
}

func TestMessagesMarkDialogUnreadSyncsSuccessfulMutationToOtherSessions(t *testing.T) {
	dialogs := &markDialogUnreadDialogClientStub{response: mtproto.BoolTrue}
	syncStub := &markDialogUnreadSyncClientStub{}
	core := newMarkDialogUnreadTestCore(dialogs, syncStub)

	got, err := core.MessagesMarkDialogUnread(markDialogUnreadRequest(true))
	if err != nil || got == nil || !mtproto.FromBool(got) {
		t.Fatalf("MessagesMarkDialogUnread() = (%v, %v), want true", got, err)
	}
	if dialogs.request == nil || dialogs.request.GetUserId() != 42 || dialogs.request.GetPeerType() != mtproto.PEER_CHAT || dialogs.request.GetPeerId() != 77 || !mtproto.FromBool(dialogs.request.GetUnreadMark()) {
		t.Fatalf("DialogMarkDialogUnread request = %v, want user 42, chat 77, unread true", dialogs.request)
	}
	if len(syncStub.requests) != 1 {
		t.Fatalf("SyncUpdatesNotMe calls = %d, want 1", len(syncStub.requests))
	}
	syncRequest := syncStub.requests[0]
	if syncRequest.GetUserId() != 42 || syncRequest.GetPermAuthKeyId() != 1234 {
		t.Fatalf("SyncUpdatesNotMe request = %v, want user 42 and current auth key 1234", syncRequest)
	}
	updates := syncRequest.GetUpdates().GetUpdates()
	if len(updates) != 1 || updates[0].GetPredicateName() != mtproto.Predicate_updateDialogUnreadMark {
		t.Fatalf("synced updates = %v, want one updateDialogUnreadMark", updates)
	}
	update := updates[0]
	peer := update.GetPeer_DIALOGPEER().GetPeer()
	if !update.GetUnread() || peer.GetPredicateName() != mtproto.Predicate_peerChat || peer.GetChatId() != 77 {
		t.Fatalf("updateDialogUnreadMark = %v, want unread chat 77", update)
	}
}

func TestMessagesMarkDialogUnreadSyncsClearedMark(t *testing.T) {
	dialogs := &markDialogUnreadDialogClientStub{response: mtproto.BoolTrue}
	syncStub := &markDialogUnreadSyncClientStub{}
	core := newMarkDialogUnreadTestCore(dialogs, syncStub)

	got, err := core.MessagesMarkDialogUnread(markDialogUnreadRequest(false))
	if err != nil || got == nil || !mtproto.FromBool(got) {
		t.Fatalf("MessagesMarkDialogUnread() = (%v, %v), want true", got, err)
	}
	if len(syncStub.requests) != 1 {
		t.Fatalf("SyncUpdatesNotMe calls = %d, want 1", len(syncStub.requests))
	}
	updates := syncStub.requests[0].GetUpdates().GetUpdates()
	if len(updates) != 1 || updates[0].GetPredicateName() != mtproto.Predicate_updateDialogUnreadMark || updates[0].GetUnread() {
		t.Fatalf("synced updates = %v, want one cleared updateDialogUnreadMark", updates)
	}
}

func TestMessagesMarkDialogUnreadDoesNotSyncUnsuccessfulMutation(t *testing.T) {
	mutationErr := errors.New("dialog update failed")
	for _, tc := range []struct {
		name     string
		response *mtproto.Bool
		err      error
		wantErr  error
	}{
		{name: "service error", err: mutationErr, wantErr: mutationErr},
		{name: "false response", response: mtproto.BoolFalse},
		{name: "missing response", wantErr: mtproto.ErrInternalServerError},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dialogs := &markDialogUnreadDialogClientStub{response: tc.response, err: tc.err}
			syncStub := &markDialogUnreadSyncClientStub{}
			core := newMarkDialogUnreadTestCore(dialogs, syncStub)

			got, err := core.MessagesMarkDialogUnread(markDialogUnreadRequest(false))
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("MessagesMarkDialogUnread() error = %v, want %v", err, tc.wantErr)
			}
			if tc.response != nil && got != tc.response {
				t.Fatalf("MessagesMarkDialogUnread() response = %v, want %v", got, tc.response)
			}
			if len(syncStub.requests) != 0 {
				t.Fatalf("SyncUpdatesNotMe calls = %d, want 0", len(syncStub.requests))
			}
		})
	}
}

func TestMessagesMarkDialogUnreadRejectsParentPeer(t *testing.T) {
	dialogs := &markDialogUnreadDialogClientStub{response: mtproto.BoolTrue}
	syncStub := &markDialogUnreadSyncClientStub{}
	core := newMarkDialogUnreadTestCore(dialogs, syncStub)
	request := markDialogUnreadRequest(true)
	request.ParentPeer = mtproto.MakeTLInputPeerChat(&mtproto.InputPeer{ChatId: 88}).To_InputPeer()

	got, err := core.MessagesMarkDialogUnread(request)
	if !errors.Is(err, mtproto.ErrMethodNotImpl) || got != nil {
		t.Fatalf("MessagesMarkDialogUnread() = (%v, %v), want METHOD_NOT_IMPL", got, err)
	}
	if dialogs.request != nil || len(syncStub.requests) != 0 {
		t.Fatalf("dialog request = %v, sync calls = %d; want no persistence or sync", dialogs.request, len(syncStub.requests))
	}
}

func TestMessagesMarkDialogUnreadPropagatesSyncFailure(t *testing.T) {
	syncErr := errors.New("sync unavailable")
	dialogs := &markDialogUnreadDialogClientStub{response: mtproto.BoolTrue}
	syncStub := &markDialogUnreadSyncClientStub{err: syncErr}
	core := newMarkDialogUnreadTestCore(dialogs, syncStub)

	got, err := core.MessagesMarkDialogUnread(markDialogUnreadRequest(true))
	if !errors.Is(err, syncErr) || got != nil {
		t.Fatalf("MessagesMarkDialogUnread() = (%v, %v), want propagated sync error", got, err)
	}
	if len(syncStub.requests) != 1 {
		t.Fatalf("SyncUpdatesNotMe calls = %d, want 1", len(syncStub.requests))
	}
}
