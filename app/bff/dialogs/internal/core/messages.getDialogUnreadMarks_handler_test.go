package core

import (
	"context"
	"errors"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/dialogs/internal/dao"
	"github.com/teamgram/teamgram-server/app/bff/dialogs/internal/svc"
	dialogclient "github.com/teamgram/teamgram-server/app/service/biz/dialog/client"
	dialogpb "github.com/teamgram/teamgram-server/app/service/biz/dialog/dialog"
	"github.com/zeromicro/go-zero/core/logx"
)

const messagesGetDialogUnreadMarksCallerID int64 = 42

type messagesGetDialogUnreadMarksClientStub struct {
	dialogclient.DialogClient
	response *dialogpb.Vector_DialogPeer
	err      error
	calls    int
	request  *dialogpb.TLDialogGetDialogUnreadMarkList
}

func (s *messagesGetDialogUnreadMarksClientStub) DialogGetDialogUnreadMarkList(_ context.Context, in *dialogpb.TLDialogGetDialogUnreadMarkList) (*dialogpb.Vector_DialogPeer, error) {
	s.calls++
	s.request = in
	return s.response, s.err
}

func newMessagesGetDialogUnreadMarksTestCore(client dialogclient.DialogClient) *DialogsCore {
	ctx := context.Background()
	return &DialogsCore{
		ctx: ctx,
		svcCtx: &svc.ServiceContext{Dao: &dao.Dao{
			DialogClient: client,
		}},
		Logger: logx.WithContext(ctx),
		MD:     &metadata.RpcMetadata{UserId: messagesGetDialogUnreadMarksCallerID},
	}
}

func messagesGetDialogUnreadMarksPeer(peer *mtproto.Peer) *mtproto.DialogPeer {
	return mtproto.MakeTLDialogPeer(&mtproto.DialogPeer{Peer: peer}).To_DialogPeer()
}

func TestMessagesGetDialogUnreadMarksReturnsProviderPeers(t *testing.T) {
	client := &messagesGetDialogUnreadMarksClientStub{response: &dialogpb.Vector_DialogPeer{Datas: []*mtproto.DialogPeer{
		messagesGetDialogUnreadMarksPeer(mtproto.MakePeerUser(84)),
		messagesGetDialogUnreadMarksPeer(mtproto.MakePeerChat(77)),
	}}}
	core := newMessagesGetDialogUnreadMarksTestCore(client)

	got, err := core.MessagesGetDialogUnreadMarks(&mtproto.TLMessagesGetDialogUnreadMarks{})
	if err != nil {
		t.Fatalf("MessagesGetDialogUnreadMarks() error = %v", err)
	}
	if got == nil || len(got.GetDatas()) != 2 || got.GetDatas()[0].GetPeer().GetUserId() != 84 || got.GetDatas()[1].GetPeer().GetChatId() != 77 {
		t.Fatalf("dialog peers = %+v, want user 84 and chat 77", got)
	}
	if client.calls != 1 || client.request == nil || client.request.GetUserId() != messagesGetDialogUnreadMarksCallerID {
		t.Fatalf("provider calls/request = %d/%+v, want one caller-scoped request", client.calls, client.request)
	}
}

func TestMessagesGetDialogUnreadMarksRejectsUnauthenticatedOrMissingProvider(t *testing.T) {
	for _, tc := range []struct {
		name string
		core *DialogsCore
		want error
	}{
		{name: "nil core", core: nil, want: mtproto.ErrAuthKeyUnregistered},
		{name: "missing metadata", core: &DialogsCore{}, want: mtproto.ErrAuthKeyUnregistered},
		{name: "missing service context", core: &DialogsCore{MD: &metadata.RpcMetadata{UserId: messagesGetDialogUnreadMarksCallerID}}, want: mtproto.ErrInternalServerError},
		{name: "missing dialog provider", core: newMessagesGetDialogUnreadMarksTestCore(nil), want: mtproto.ErrInternalServerError},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := tc.core.MessagesGetDialogUnreadMarks(nil)
			if got != nil || !errors.Is(err, tc.want) {
				t.Fatalf("MessagesGetDialogUnreadMarks() = (%v, %v), want nil/%v", got, err, tc.want)
			}
		})
	}
}

func TestMessagesGetDialogUnreadMarksRejectsNilOrMalformedProviderResponse(t *testing.T) {
	validRequest := &mtproto.TLMessagesGetDialogUnreadMarks{}
	for _, tc := range []struct {
		name     string
		response *dialogpb.Vector_DialogPeer
	}{
		{name: "nil response"},
		{name: "nil dialog peer", response: &dialogpb.Vector_DialogPeer{Datas: []*mtproto.DialogPeer{nil}}},
		{name: "nil peer", response: &dialogpb.Vector_DialogPeer{Datas: []*mtproto.DialogPeer{{}}}},
		{name: "unknown peer", response: &dialogpb.Vector_DialogPeer{Datas: []*mtproto.DialogPeer{{Peer: &mtproto.Peer{PredicateName: "peerUnknown"}}}}},
		{name: "zero user id", response: &dialogpb.Vector_DialogPeer{Datas: []*mtproto.DialogPeer{messagesGetDialogUnreadMarksPeer(&mtproto.Peer{PredicateName: mtproto.Predicate_peerUser})}}},
		{name: "zero chat id", response: &dialogpb.Vector_DialogPeer{Datas: []*mtproto.DialogPeer{messagesGetDialogUnreadMarksPeer(&mtproto.Peer{PredicateName: mtproto.Predicate_peerChat})}}},
		{name: "zero channel id", response: &dialogpb.Vector_DialogPeer{Datas: []*mtproto.DialogPeer{messagesGetDialogUnreadMarksPeer(&mtproto.Peer{PredicateName: mtproto.Predicate_peerChannel})}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := &messagesGetDialogUnreadMarksClientStub{response: tc.response}
			core := newMessagesGetDialogUnreadMarksTestCore(client)

			got, err := core.MessagesGetDialogUnreadMarks(validRequest)
			if got != nil || !errors.Is(err, mtproto.ErrInternalServerError) {
				t.Fatalf("MessagesGetDialogUnreadMarks() = (%v, %v), want nil/INTERNAL_SERVER_ERROR", got, err)
			}
		})
	}
}

func TestMessagesGetDialogUnreadMarksPropagatesProviderError(t *testing.T) {
	providerErr := errors.New("dialog unread marks unavailable")
	client := &messagesGetDialogUnreadMarksClientStub{err: providerErr}
	core := newMessagesGetDialogUnreadMarksTestCore(client)

	got, err := core.MessagesGetDialogUnreadMarks(nil)
	if got != nil || !errors.Is(err, providerErr) {
		t.Fatalf("MessagesGetDialogUnreadMarks() = (%v, %v), want nil/provider error", got, err)
	}
}
