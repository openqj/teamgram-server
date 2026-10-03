package core

import (
	"context"
	"errors"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	bffdao "github.com/teamgram/teamgram-server/app/bff/savedmessagedialogs/internal/dao"
	"github.com/teamgram/teamgram-server/app/bff/savedmessagedialogs/internal/svc"
	syncclient "github.com/teamgram/teamgram-server/app/messenger/sync/client"
	syncpb "github.com/teamgram/teamgram-server/app/messenger/sync/sync"
	dialogclient "github.com/teamgram/teamgram-server/app/service/biz/dialog/client"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/dialog"
)

type toggleSavedDialogPinDialogClient struct {
	dialogclient.DialogClient
	response *mtproto.Bool
	err      error
	calls    int
}

func (d *toggleSavedDialogPinDialogClient) DialogToggleSavedDialogPin(context.Context, *dialog.TLDialogToggleSavedDialogPin) (*mtproto.Bool, error) {
	d.calls++
	return d.response, d.err
}

type toggleSavedDialogPinSyncClient struct {
	syncclient.SyncClient
	requests []*syncpb.TLSyncUpdatesNotMe
	reply    *mtproto.Void
	err      error
}

func (s *toggleSavedDialogPinSyncClient) SyncUpdatesNotMe(_ context.Context, in *syncpb.TLSyncUpdatesNotMe) (*mtproto.Void, error) {
	s.requests = append(s.requests, in)
	return s.reply, s.err
}

func toggleSavedDialogPinRequest(channelID int64) *mtproto.TLMessagesToggleSavedDialogPin {
	return &mtproto.TLMessagesToggleSavedDialogPin{
		Pinned: true,
		Peer: mtproto.MakeTLInputDialogPeer(&mtproto.InputDialogPeer{
			Peer: mtproto.MakeTLInputPeerChannel(&mtproto.InputPeer{ChannelId: channelID, AccessHash: 9001}).To_InputPeer(),
		}).To_InputDialogPeer(),
	}
}

func newToggleSavedDialogPinTestCore(dialogs *toggleSavedDialogPinDialogClient, syncer *toggleSavedDialogPinSyncClient) *SavedMessageDialogsCore {
	core := New(context.Background(), &svc.ServiceContext{Dao: &bffdao.Dao{
		DialogClient: dialogs,
		SyncClient:   syncer,
	}})
	core.MD = &metadata.RpcMetadata{UserId: 42, PermAuthKeyId: 777}
	return core
}

func TestMessagesToggleSavedDialogPinHydratesChannelForSync(t *testing.T) {
	dialogs := &toggleSavedDialogPinDialogClient{response: mtproto.BoolTrue}
	syncer := &toggleSavedDialogPinSyncClient{reply: mtproto.EmptyVoid}
	core := newToggleSavedDialogPinTestCore(dialogs, syncer)
	core.channelChatsByID = func(_ int64, ids []int64) []*mtproto.Chat {
		if len(ids) != 1 || ids[0] != 77 {
			t.Fatalf("channel ids = %v, want [77]", ids)
		}
		return []*mtproto.Chat{mtproto.MakeTLChannel(&mtproto.Chat{Id: 77, Title: "Saved"}).To_Chat()}
	}

	got, err := core.MessagesToggleSavedDialogPin(toggleSavedDialogPinRequest(77))
	if err != nil || !mtproto.FromBool(got) {
		t.Fatalf("MessagesToggleSavedDialogPin() = (%v, %v), want true", got, err)
	}
	if dialogs.calls != 1 || len(syncer.requests) != 1 {
		t.Fatalf("provider calls = dialog %d, sync %d; want one each", dialogs.calls, len(syncer.requests))
	}
	updates := syncer.requests[0].GetUpdates()
	if len(updates.GetChats()) != 1 || updates.GetChats()[0].GetId() != 77 {
		t.Fatalf("synced chats = %v, want channel 77", updates.GetChats())
	}
	if len(updates.GetUpdates()) != 1 || updates.GetUpdates()[0].GetPredicateName() != mtproto.Predicate_updateSavedDialogPinned {
		t.Fatalf("synced updates = %v, want one updateSavedDialogPinned", updates.GetUpdates())
	}
}

func TestMessagesToggleSavedDialogPinPropagatesProviderFailures(t *testing.T) {
	dialogErr := errors.New("dialog unavailable")
	dialogs := &toggleSavedDialogPinDialogClient{err: dialogErr}
	syncer := &toggleSavedDialogPinSyncClient{reply: mtproto.EmptyVoid}
	core := newToggleSavedDialogPinTestCore(dialogs, syncer)
	if _, err := core.MessagesToggleSavedDialogPin(toggleSavedDialogPinRequest(77)); !errors.Is(err, dialogErr) {
		t.Fatalf("dialog error = %v, want %v", err, dialogErr)
	}
	if len(syncer.requests) != 0 {
		t.Fatalf("sync calls after dialog failure = %d, want 0", len(syncer.requests))
	}

	dialogs.err = nil
	dialogs.response = mtproto.BoolTrue
	syncErr := errors.New("sync unavailable")
	syncer.err = syncErr
	if _, err := core.MessagesToggleSavedDialogPin(toggleSavedDialogPinRequest(77)); !errors.Is(err, syncErr) {
		t.Fatalf("sync error = %v, want %v", err, syncErr)
	}
}

func TestMessagesToggleSavedDialogPinReturnsFalseWithoutSync(t *testing.T) {
	dialogs := &toggleSavedDialogPinDialogClient{response: mtproto.BoolFalse}
	syncer := &toggleSavedDialogPinSyncClient{reply: mtproto.EmptyVoid}
	core := newToggleSavedDialogPinTestCore(dialogs, syncer)

	got, err := core.MessagesToggleSavedDialogPin(toggleSavedDialogPinRequest(77))
	if err != nil || mtproto.FromBool(got) {
		t.Fatalf("MessagesToggleSavedDialogPin() = (%v, %v), want false,nil", got, err)
	}
	if len(syncer.requests) != 0 {
		t.Fatalf("sync calls after false provider reply = %d, want 0", len(syncer.requests))
	}
}

func TestMessagesToggleSavedDialogPinRejectsUnauthenticatedOrNilRequest(t *testing.T) {
	dialogs := &toggleSavedDialogPinDialogClient{response: mtproto.BoolTrue}
	syncer := &toggleSavedDialogPinSyncClient{reply: mtproto.EmptyVoid}
	core := newToggleSavedDialogPinTestCore(dialogs, syncer)
	core.MD = &metadata.RpcMetadata{}
	if _, err := core.MessagesToggleSavedDialogPin(toggleSavedDialogPinRequest(77)); err != mtproto.ErrAuthKeyUnregistered {
		t.Fatalf("unauthenticated error = %v, want %v", err, mtproto.ErrAuthKeyUnregistered)
	}
	core.MD.UserId = 42
	if _, err := core.MessagesToggleSavedDialogPin(nil); err != mtproto.ErrInputRequestInvalid {
		t.Fatalf("nil request error = %v, want %v", err, mtproto.ErrInputRequestInvalid)
	}
}
