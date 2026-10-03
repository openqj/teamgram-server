package core

import (
	"context"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	bffdao "github.com/teamgram/teamgram-server/app/bff/savedmessagedialogs/internal/dao"
	"github.com/teamgram/teamgram-server/app/bff/savedmessagedialogs/internal/svc"
	dialogclient "github.com/teamgram/teamgram-server/app/service/biz/dialog/client"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/dialog"
)

type savedHistoryDialogClient struct {
	dialogclient.DialogClient
	calls    int
	request  *dialog.TLDialogInsertOrUpdateDialog
	response *mtproto.Bool
	err      error
}

func (d *savedHistoryDialogClient) DialogMarkSavedHistoryRead(_ context.Context, in *dialog.TLDialogInsertOrUpdateDialog) (*mtproto.Bool, error) {
	d.calls++
	d.request = in
	return d.response, d.err
}

func TestMessagesReadSavedHistoryValidatesAndPersistsRequest(t *testing.T) {
	const userID int64 = 910021
	dialogs := &savedHistoryDialogClient{response: mtproto.BoolTrue}
	c := New(context.Background(), &svc.ServiceContext{Dao: &bffdao.Dao{DialogClient: dialogs}})
	c.MD = &metadata.RpcMetadata{UserId: userID}

	got, err := c.MessagesReadSavedHistory(&mtproto.TLMessagesReadSavedHistory{
		ParentPeer: mtproto.MakeTLInputPeerSelf(nil).To_InputPeer(),
		Peer:       mtproto.MakeTLInputPeerChat(&mtproto.InputPeer{ChatId: 910022}).To_InputPeer(),
		MaxId:      456,
	})
	if err != nil || !mtproto.FromBool(got) {
		t.Fatalf("MessagesReadSavedHistory() = (%v, %v), want true", got, err)
	}
	if dialogs.calls != 1 || dialogs.request == nil {
		t.Fatalf("dialog RPC calls = %d, request = %v; want one request", dialogs.calls, dialogs.request)
	}
	if dialogs.request.GetUserId() != userID || dialogs.request.GetPeerType() != mtproto.PEER_CHAT || dialogs.request.GetPeerId() != 910022 || dialogs.request.GetReadInboxMaxId().GetValue() != 456 {
		t.Fatalf("dialog RPC request = %v, want caller, peer and max_id propagated", dialogs.request)
	}
}

func TestMessagesReadSavedHistoryRejectsInvalidScopeAndEmptyPeer(t *testing.T) {
	const userID int64 = 910031
	cases := []struct {
		name  string
		user  int64
		peer  *mtproto.InputPeer
		want  error
		calls int
	}{
		{name: "missing caller", peer: mtproto.MakeTLInputPeerChat(&mtproto.InputPeer{ChatId: 910032}).To_InputPeer(), want: mtproto.ErrAuthKeyUnregistered},
		{name: "invalid peer constructor", user: userID, peer: mtproto.MakeTLInputPeerEmpty(nil).To_InputPeer(), want: mtproto.ErrPeerIdInvalid},
		{name: "empty saved dialog", user: userID, peer: mtproto.MakeTLInputPeerChat(&mtproto.InputPeer{ChatId: 910033}).To_InputPeer(), want: mtproto.ErrPeerIdInvalid, calls: 1},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dialogs := &savedHistoryDialogClient{response: mtproto.BoolTrue}
			if tc.name == "empty saved dialog" {
				dialogs.err = mtproto.ErrPeerIdInvalid
			}
			c := New(context.Background(), &svc.ServiceContext{Dao: &bffdao.Dao{DialogClient: dialogs}})
			c.MD = &metadata.RpcMetadata{UserId: tc.user}
			_, err := c.MessagesReadSavedHistory(&mtproto.TLMessagesReadSavedHistory{
				ParentPeer: mtproto.MakeTLInputPeerSelf(nil).To_InputPeer(),
				Peer:       tc.peer,
				MaxId:      1,
			})
			if err != tc.want {
				t.Fatalf("MessagesReadSavedHistory() error = %v, want %v", err, tc.want)
			}
			if dialogs.calls != tc.calls {
				t.Fatalf("dialog RPC calls = %d, want %d", dialogs.calls, tc.calls)
			}
		})
	}
}

func TestMessagesReadSavedHistoryRejectsForeignParentAndNegativeMaxID(t *testing.T) {
	const userID int64 = 910041
	for _, tc := range []struct {
		name       string
		parentPeer *mtproto.InputPeer
		maxID      int32
		want       error
	}{
		{
			name:       "foreign parent",
			parentPeer: mtproto.MakeTLInputPeerChat(&mtproto.InputPeer{ChatId: 910042}).To_InputPeer(),
			maxID:      1,
			want:       mtproto.ErrPeerIdInvalid,
		},
		{
			name:       "negative max id",
			parentPeer: mtproto.MakeTLInputPeerSelf(nil).To_InputPeer(),
			maxID:      -1,
			want:       mtproto.ErrMessageIdInvalid,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dialogs := &savedHistoryDialogClient{response: mtproto.BoolTrue}
			c := New(context.Background(), &svc.ServiceContext{Dao: &bffdao.Dao{DialogClient: dialogs}})
			c.MD = &metadata.RpcMetadata{UserId: userID}
			_, err := c.MessagesReadSavedHistory(&mtproto.TLMessagesReadSavedHistory{
				ParentPeer: tc.parentPeer,
				Peer:       mtproto.MakeTLInputPeerChat(&mtproto.InputPeer{ChatId: 910043}).To_InputPeer(),
				MaxId:      tc.maxID,
			})
			if err != tc.want {
				t.Fatalf("MessagesReadSavedHistory() error = %v, want %v", err, tc.want)
			}
			if dialogs.calls != 0 {
				t.Fatalf("dialog RPC calls = %d, want 0", dialogs.calls)
			}
		})
	}
}

func TestMessagesReadSavedHistoryFailsClosedWhenDialogProviderUnavailable(t *testing.T) {
	const userID int64 = 910051
	request := &mtproto.TLMessagesReadSavedHistory{
		ParentPeer: mtproto.MakeTLInputPeerSelf(nil).To_InputPeer(),
		Peer:       mtproto.MakeTLInputPeerChat(&mtproto.InputPeer{ChatId: 910052}).To_InputPeer(),
		MaxId:      1,
	}

	c := New(context.Background(), &svc.ServiceContext{Dao: &bffdao.Dao{}})
	c.MD = &metadata.RpcMetadata{UserId: userID}
	if got, err := c.MessagesReadSavedHistory(request); got != nil || err != mtproto.ErrMethodNotImpl {
		t.Fatalf("missing dialog provider = (%v, %v), want METHOD_NOT_IMPL", got, err)
	}

	for _, response := range []*mtproto.Bool{nil, mtproto.BoolFalse} {
		dialogs := &savedHistoryDialogClient{response: response}
		c := New(context.Background(), &svc.ServiceContext{Dao: &bffdao.Dao{DialogClient: dialogs}})
		c.MD = &metadata.RpcMetadata{UserId: userID}
		if got, err := c.MessagesReadSavedHistory(request); got != nil || err != mtproto.ErrInternalServerError {
			t.Fatalf("dialog response %v = (%v, %v), want INTERNAL_SERVER_ERROR", response, got, err)
		}
		if dialogs.calls != 1 {
			t.Fatalf("dialog RPC calls = %d, want 1", dialogs.calls)
		}
	}
}
