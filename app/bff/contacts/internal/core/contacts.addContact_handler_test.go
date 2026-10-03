package core

import (
	"context"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/contacts/internal/dao"
	"github.com/teamgram/teamgram-server/app/bff/contacts/internal/svc"
	messageclient "github.com/teamgram/teamgram-server/app/service/biz/message/client"
	messagepb "github.com/teamgram/teamgram-server/app/service/biz/message/message"
	userclient "github.com/teamgram/teamgram-server/app/service/biz/user/client"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
	"github.com/zeromicro/go-zero/core/logx"
)

type addContactMessageClientStub struct {
	messageclient.MessageClient
	box     *mtproto.MessageBox
	err     error
	request *messagepb.TLMessageGetUserMessage
}

func (s *addContactMessageClientStub) MessageGetUserMessage(_ context.Context, in *messagepb.TLMessageGetUserMessage) (*mtproto.MessageBox, error) {
	s.request = in
	return s.box, s.err
}

type addContactUserClientStub struct {
	userclient.UserClient
	users      *mtproto.MutableUsers
	addRequest *userpb.TLUserAddContact
	addCalls   int
}

func (s *addContactUserClientStub) UserGetMutableUsersV2(_ context.Context, _ *userpb.TLUserGetMutableUsersV2) (*mtproto.MutableUsers, error) {
	return s.users, nil
}

func (s *addContactUserClientStub) UserAddContact(_ context.Context, in *userpb.TLUserAddContact) (*mtproto.Bool, error) {
	s.addCalls++
	s.addRequest = in
	return mtproto.BoolFalse, nil
}

func TestContactsAddContactResolvesUserFromAccessibleGroupMessage(t *testing.T) {
	const (
		selfID    = int64(42)
		userID    = int64(84)
		chatID    = int64(17)
		messageID = int32(123)
	)
	messageClient := &addContactMessageClientStub{box: mtproto.MakeTLMessageBox(&mtproto.MessageBox{
		UserId:       selfID,
		MessageId:    messageID,
		SenderUserId: userID,
		PeerType:     mtproto.PEER_CHAT,
		PeerId:       chatID,
		Message:      &mtproto.Message{Id: messageID},
	}).To_MessageBox()}
	userClient := &addContactUserClientStub{users: &mtproto.MutableUsers{Users: []*mtproto.ImmutableUser{
		mtproto.MakeTLImmutableUser(&mtproto.ImmutableUser{User: &mtproto.UserData{Id: selfID, FirstName: "Owner"}}).To_ImmutableUser(),
		mtproto.MakeTLImmutableUser(&mtproto.ImmutableUser{User: &mtproto.UserData{Id: userID, FirstName: "Peer"}}).To_ImmutableUser(),
	}}}
	core := newAddContactTestCore(selfID, userClient, messageClient)
	inputUser := mtproto.MakeTLInputUserFromMessage(&mtproto.InputUser{
		UserId: userID,
		Peer:   mtproto.MakeTLInputPeerChat(&mtproto.InputPeer{ChatId: chatID}).To_InputPeer(),
		MsgId:  messageID,
	}).To_InputUser()

	got, err := core.ContactsAddContact(&mtproto.TLContactsAddContact{
		Id:        inputUser,
		FirstName: "Added",
	})
	if err != nil || got == nil {
		t.Fatalf("ContactsAddContact() = (%+v, %v), want Updates", got, err)
	}
	if messageClient.request == nil || messageClient.request.UserId != selfID || messageClient.request.Id != messageID {
		t.Fatalf("message lookup = %+v, want user=%d message=%d", messageClient.request, selfID, messageID)
	}
	if userClient.addCalls != 1 || userClient.addRequest == nil || userClient.addRequest.UserId != selfID || userClient.addRequest.Id != userID {
		t.Fatalf("contact write = (%d, %+v), want one write from %d to %d", userClient.addCalls, userClient.addRequest, selfID, userID)
	}
}

func TestContactsAddContactRejectsUnprovenInputUserFromMessage(t *testing.T) {
	const (
		selfID    = int64(42)
		userID    = int64(84)
		chatID    = int64(17)
		messageID = int32(123)
	)
	tests := []struct {
		name string
		box  *mtproto.MessageBox
		err  error
	}{
		{
			name: "message from another peer",
			box: mtproto.MakeTLMessageBox(&mtproto.MessageBox{
				UserId: selfID, MessageId: messageID, SenderUserId: userID,
				PeerType: mtproto.PEER_CHAT, PeerId: chatID + 1, Message: &mtproto.Message{Id: messageID},
			}).To_MessageBox(),
		},
		{
			name: "user is not the group message author",
			box: mtproto.MakeTLMessageBox(&mtproto.MessageBox{
				UserId: selfID, MessageId: messageID, SenderUserId: userID + 1,
				PeerType: mtproto.PEER_CHAT, PeerId: chatID, Message: &mtproto.Message{Id: messageID},
			}).To_MessageBox(),
		},
		{
			name: "message lookup failed",
			err:  mtproto.ErrMessageIdInvalid,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			messageClient := &addContactMessageClientStub{box: tt.box, err: tt.err}
			userClient := &addContactUserClientStub{}
			core := newAddContactTestCore(selfID, userClient, messageClient)
			inputUser := mtproto.MakeTLInputUserFromMessage(&mtproto.InputUser{
				UserId: userID,
				Peer:   mtproto.MakeTLInputPeerChat(&mtproto.InputPeer{ChatId: chatID}).To_InputPeer(),
				MsgId:  messageID,
			}).To_InputUser()

			got, err := core.ContactsAddContact(&mtproto.TLContactsAddContact{Id: inputUser, FirstName: "Added"})
			if err != mtproto.ErrContactIdInvalid || got != nil {
				t.Fatalf("ContactsAddContact() = (%+v, %v), want CONTACT_ID_INVALID", got, err)
			}
			if userClient.addCalls != 0 {
				t.Fatalf("invalid message context caused %d contact writes", userClient.addCalls)
			}
		})
	}
}

func newAddContactTestCore(selfID int64, userClient *addContactUserClientStub, messageClient *addContactMessageClientStub) *ContactsCore {
	ctx := context.Background()
	core := &ContactsCore{
		ctx: ctx,
		svcCtx: &svc.ServiceContext{Dao: &dao.Dao{
			UserClient:    userClient,
			MessageClient: messageClient,
		}},
		Logger: logx.WithContext(ctx),
		MD:     &metadata.RpcMetadata{UserId: selfID},
	}
	return core
}
