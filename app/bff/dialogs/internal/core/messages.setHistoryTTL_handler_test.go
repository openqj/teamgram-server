package core

import (
	"context"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/dialogs/internal/dao"
	"github.com/teamgram/teamgram-server/app/bff/dialogs/internal/svc"
	syncclient "github.com/teamgram/teamgram-server/app/messenger/sync/client"
	dialogclient "github.com/teamgram/teamgram-server/app/service/biz/dialog/client"
	"github.com/zeromicro/go-zero/core/logx"
)

type setHistoryTTLDialogClientStub struct {
	dialogclient.DialogClient
}

type setHistoryTTLSyncClientStub struct {
	syncclient.SyncClient
}

func newSetHistoryTTLValidationCore() *DialogsCore {
	return &DialogsCore{
		Logger: logx.WithContext(context.Background()),
		MD:     &metadata.RpcMetadata{UserId: 42},
	}
}

func setHistoryTTLUserRequest() *mtproto.TLMessagesSetHistoryTTL {
	return &mtproto.TLMessagesSetHistoryTTL{
		Peer:   mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: 84, AccessHash: 840}).To_InputPeer(),
		Period: 86400,
	}
}

func setHistoryTTLChatRequest() *mtproto.TLMessagesSetHistoryTTL {
	return &mtproto.TLMessagesSetHistoryTTL{
		Peer:   mtproto.MakeTLInputPeerChat(&mtproto.InputPeer{ChatId: 84}).To_InputPeer(),
		Period: 86400,
	}
}

func TestMessagesSetHistoryTTLRejectsInvalidRequest(t *testing.T) {
	cases := []struct {
		name  string
		core  *DialogsCore
		input *mtproto.TLMessagesSetHistoryTTL
		want  error
	}{
		{name: "unauthenticated", core: &DialogsCore{}, input: setHistoryTTLUserRequest(), want: mtproto.ErrAuthKeyUnregistered},
		{name: "nil request", core: newSetHistoryTTLValidationCore(), want: mtproto.ErrPeerIdInvalid},
		{name: "missing peer", core: newSetHistoryTTLValidationCore(), input: &mtproto.TLMessagesSetHistoryTTL{}, want: mtproto.ErrPeerIdInvalid},
		{name: "negative period", core: newSetHistoryTTLValidationCore(), input: &mtproto.TLMessagesSetHistoryTTL{Peer: setHistoryTTLUserRequest().Peer, Period: -1}, want: mtproto.ErrTtlPeriodInvalid},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := tc.core.MessagesSetHistoryTTL(tc.input)
			if got != nil || err != tc.want {
				t.Fatalf("MessagesSetHistoryTTL() = (%v, %v), want (nil, %v)", got, err, tc.want)
			}
		})
	}
}

func TestMessagesSetHistoryTTLRequiresProviders(t *testing.T) {
	cases := []struct {
		name  string
		core  *DialogsCore
		input *mtproto.TLMessagesSetHistoryTTL
	}{
		{name: "nil service context", core: newSetHistoryTTLValidationCore(), input: setHistoryTTLUserRequest()},
		{name: "nil dao", core: &DialogsCore{MD: &metadata.RpcMetadata{UserId: 42}, svcCtx: &svc.ServiceContext{}}, input: setHistoryTTLUserRequest()},
		{name: "nil dialog client", core: &DialogsCore{MD: &metadata.RpcMetadata{UserId: 42}, svcCtx: &svc.ServiceContext{Dao: &dao.Dao{}}}, input: setHistoryTTLUserRequest()},
		{name: "nil chat client", core: &DialogsCore{MD: &metadata.RpcMetadata{UserId: 42}, svcCtx: &svc.ServiceContext{Dao: &dao.Dao{
			DialogClient: &setHistoryTTLDialogClientStub{},
			SyncClient:   &setHistoryTTLSyncClientStub{},
		}}}, input: setHistoryTTLChatRequest()},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := tc.core.MessagesSetHistoryTTL(tc.input)
			if got != nil || err != mtproto.ErrInternalServerError {
				t.Fatalf("MessagesSetHistoryTTL() = (%v, %v), want (nil, INTERNAL_SERVER_ERROR)", got, err)
			}
		})
	}
}
