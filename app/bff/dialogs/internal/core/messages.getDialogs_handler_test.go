package core

import (
	"context"
	"errors"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/dialogs/internal/dao"
	"github.com/teamgram/teamgram-server/app/bff/dialogs/internal/svc"
	chatpb "github.com/teamgram/teamgram-server/app/service/biz/chat/chat"
	chatclient "github.com/teamgram/teamgram-server/app/service/biz/chat/client"
	dialogclient "github.com/teamgram/teamgram-server/app/service/biz/dialog/client"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/dialog"
	messageclient "github.com/teamgram/teamgram-server/app/service/biz/message/client"
	messagepb "github.com/teamgram/teamgram-server/app/service/biz/message/message"
	userclient "github.com/teamgram/teamgram-server/app/service/biz/user/client"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
	"github.com/zeromicro/go-zero/core/logx"
)

type getDialogsDialogClientStub struct {
	dialogclient.DialogClient
	response *dialog.Vector_DialogExt
	err      error
}

func (s *getDialogsDialogClientStub) DialogGetDialogs(context.Context, *dialog.TLDialogGetDialogs) (*dialog.Vector_DialogExt, error) {
	return s.response, s.err
}

type getDialogsMessageClientStub struct {
	messageclient.MessageClient
	listErr error
}

func (s *getDialogsMessageClientStub) MessageGetUserMessageList(context.Context, *messagepb.TLMessageGetUserMessageList) (*messagepb.Vector_MessageBox, error) {
	return &messagepb.Vector_MessageBox{}, s.listErr
}

type getDialogsUserClientStub struct {
	userclient.UserClient
	settingsErr error
	usersErr    error
}

func (s *getDialogsUserClientStub) UserGetAllNotifySettings(context.Context, *userpb.TLUserGetAllNotifySettings) (*userpb.Vector_PeerPeerNotifySettings, error) {
	return &userpb.Vector_PeerPeerNotifySettings{}, s.settingsErr
}

func (s *getDialogsUserClientStub) UserGetMutableUsersV2(context.Context, *userpb.TLUserGetMutableUsersV2) (*mtproto.MutableUsers, error) {
	return &mtproto.MutableUsers{}, s.usersErr
}

type getDialogsChatClientStub struct {
	chatclient.ChatClient
	err error
}

func (s *getDialogsChatClientStub) ChatGetChatListByIdList(context.Context, *chatpb.TLChatGetChatListByIdList) (*chatpb.Vector_MutableChat, error) {
	return &chatpb.Vector_MutableChat{}, s.err
}

func newGetDialogsTestCore(dialogs dialogclient.DialogClient, messages messageclient.MessageClient, users userclient.UserClient, chats chatclient.ChatClient) *DialogsCore {
	ctx := context.Background()
	return &DialogsCore{
		ctx: ctx,
		svcCtx: &svc.ServiceContext{Dao: &dao.Dao{
			DialogClient:  dialogs,
			MessageClient: messages,
			UserClient:    users,
			ChatClient:    chats,
		}},
		Logger: logx.WithContext(ctx),
		MD:     &metadata.RpcMetadata{UserId: 41},
	}
}

func getDialogsFixture(peer *mtproto.Peer) *dialog.Vector_DialogExt {
	return &dialog.Vector_DialogExt{Datas: []*dialog.DialogExt{{
		Order: 100,
		Dialog: mtproto.MakeTLDialog(&mtproto.Dialog{
			Peer:       peer,
			TopMessage: 10,
		}).To_Dialog(),
	}}}
}

func TestMessagesGetDialogsPropagatesLoadErrors(t *testing.T) {
	wantErr := errors.New("dependency unavailable")
	tests := []struct {
		name        string
		peer        *mtproto.Peer
		dialogErr   error
		settingsErr error
		messageErr  error
		usersErr    error
		chatsErr    error
	}{
		{name: "dialog load", dialogErr: wantErr},
		{name: "notify settings load", settingsErr: wantErr},
		{name: "top message load", peer: mtproto.MakePeerChat(77), messageErr: wantErr},
		{name: "user hydration", peer: mtproto.MakePeerUser(84), usersErr: wantErr},
		{name: "chat hydration", peer: mtproto.MakePeerChat(77), chatsErr: wantErr},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dialogs := &getDialogsDialogClientStub{response: &dialog.Vector_DialogExt{}, err: tc.dialogErr}
			if tc.peer != nil {
				dialogs.response = getDialogsFixture(tc.peer)
			}
			messages := &getDialogsMessageClientStub{listErr: tc.messageErr}
			users := &getDialogsUserClientStub{settingsErr: tc.settingsErr, usersErr: tc.usersErr}
			chats := &getDialogsChatClientStub{err: tc.chatsErr}
			core := newGetDialogsTestCore(dialogs, messages, users, chats)

			got, err := core.MessagesGetDialogs(&mtproto.TLMessagesGetDialogs{
				OffsetPeer: mtproto.MakeTLInputPeerEmpty(nil).To_InputPeer(),
				Limit:      20,
			})
			if got != nil || !errors.Is(err, wantErr) {
				t.Fatalf("MessagesGetDialogs() = (%v, %v), want nil result and propagated load error", got, err)
			}
		})
	}
}
