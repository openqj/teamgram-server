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
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/dialog"
	messageclient "github.com/teamgram/teamgram-server/app/service/biz/message/client"
	messagepb "github.com/teamgram/teamgram-server/app/service/biz/message/message"
	"github.com/zeromicro/go-zero/core/logx"
)

type savedDialogsDialogClientStub struct {
	dialogclient.DialogClient
	response *dialog.SavedDialogList
}

func (c *savedDialogsDialogClientStub) DialogGetSavedDialogs(context.Context, *dialog.TLDialogGetSavedDialogs) (*dialog.SavedDialogList, error) {
	return c.response, nil
}

type savedDialogsMessageClientStub struct {
	messageclient.MessageClient
	err error
}

func (c *savedDialogsMessageClientStub) MessageGetUserMessageList(context.Context, *messagepb.TLMessageGetUserMessageList) (*messagepb.Vector_MessageBox, error) {
	return nil, c.err
}

func TestMessagesGetSavedDialogsPropagatesMessageLookupError(t *testing.T) {
	lookupErr := errors.New("saved top message lookup failed")
	ctx := context.Background()
	core := &DialogsCore{
		ctx:    ctx,
		Logger: logx.WithContext(ctx),
		svcCtx: &svc.ServiceContext{Dao: &dao.Dao{
			DialogClient: &savedDialogsDialogClientStub{response: &dialog.SavedDialogList{
				Count: 1,
				Dialogs: []*mtproto.SavedDialog{{
					Peer:       mtproto.MakePeerUser(42),
					TopMessage: 10,
				}},
			}},
			MessageClient: &savedDialogsMessageClientStub{err: lookupErr},
		}},
		MD: &metadata.RpcMetadata{UserId: 41},
	}

	got, err := core.MessagesGetSavedDialogs(&mtproto.TLMessagesGetSavedDialogs{
		OffsetPeer: mtproto.MakeTLInputPeerEmpty(nil).To_InputPeer(),
		Limit:      20,
	})
	if got != nil || err != lookupErr {
		t.Fatalf("MessagesGetSavedDialogs() = (%+v, %v), want (nil, original message lookup error)", got, err)
	}
}
