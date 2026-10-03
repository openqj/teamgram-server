package core

import (
	"context"
	"errors"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/contacts/internal/dao"
	"github.com/teamgram/teamgram-server/app/bff/contacts/internal/svc"
	syncclient "github.com/teamgram/teamgram-server/app/messenger/sync/client"
	"github.com/teamgram/teamgram-server/app/messenger/sync/sync"
	messageclient "github.com/teamgram/teamgram-server/app/service/biz/message/client"
	messagepb "github.com/teamgram/teamgram-server/app/service/biz/message/message"
	userclient "github.com/teamgram/teamgram-server/app/service/biz/user/client"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
	"github.com/zeromicro/go-zero/core/logx"
)

type deleteContactsMessageClientStub struct {
	messageclient.MessageClient
	box     *mtproto.MessageBox
	err     error
	request *messagepb.TLMessageGetUserMessage
}

func (s *deleteContactsMessageClientStub) MessageGetUserMessage(_ context.Context, in *messagepb.TLMessageGetUserMessage) (*mtproto.MessageBox, error) {
	s.request = in
	return s.box, s.err
}

type deleteContactsUserClientStub struct {
	userclient.UserClient
	users           *userpb.Vector_ImmutableUser
	getMutableCalls int
	deleteRequests  []*userpb.TLUserDeleteContact
}

func (s *deleteContactsUserClientStub) UserGetMutableUsers(_ context.Context, _ *userpb.TLUserGetMutableUsers) (*userpb.Vector_ImmutableUser, error) {
	s.getMutableCalls++
	return s.users, nil
}

func (s *deleteContactsUserClientStub) UserDeleteContact(_ context.Context, in *userpb.TLUserDeleteContact) (*mtproto.Bool, error) {
	s.deleteRequests = append(s.deleteRequests, in)
	return mtproto.BoolTrue, nil
}

type deleteContactsSyncClientStub struct {
	syncclient.SyncClient
	request *sync.TLSyncUpdatesNotMe
}

func (s *deleteContactsSyncClientStub) SyncUpdatesNotMe(_ context.Context, in *sync.TLSyncUpdatesNotMe) (*mtproto.Void, error) {
	s.request = in
	return nil, nil
}

func TestContactsDeleteContactsResolvesInputUserFromAccessibleGroupMessage(t *testing.T) {
	const (
		selfID    = int64(42)
		userID    = int64(84)
		chatID    = int64(17)
		messageID = int32(123)
	)
	messageClient := &deleteContactsMessageClientStub{box: mtproto.MakeTLMessageBox(&mtproto.MessageBox{
		UserId:       selfID,
		MessageId:    messageID,
		SenderUserId: userID,
		PeerType:     mtproto.PEER_CHAT,
		PeerId:       chatID,
		Message:      &mtproto.Message{Id: messageID},
	}).To_MessageBox()}
	userClient := &deleteContactsUserClientStub{users: &userpb.Vector_ImmutableUser{Datas: []*mtproto.ImmutableUser{
		mtproto.MakeTLImmutableUser(&mtproto.ImmutableUser{User: &mtproto.UserData{Id: selfID, FirstName: "Owner"}}).To_ImmutableUser(),
		mtproto.MakeTLImmutableUser(&mtproto.ImmutableUser{User: &mtproto.UserData{Id: userID, FirstName: "Peer"}}).To_ImmutableUser(),
	}}}
	syncClient := &deleteContactsSyncClientStub{}
	core := newDeleteContactsTestCore(selfID, userClient, messageClient, syncClient)
	inputUser := mtproto.MakeTLInputUserFromMessage(&mtproto.InputUser{
		UserId: userID,
		Peer:   mtproto.MakeTLInputPeerChat(&mtproto.InputPeer{ChatId: chatID}).To_InputPeer(),
		MsgId:  messageID,
	}).To_InputUser()

	got, err := core.ContactsDeleteContacts(&mtproto.TLContactsDeleteContacts{Id: []*mtproto.InputUser{inputUser}})
	if err != nil || got == nil {
		t.Fatalf("ContactsDeleteContacts() = (%+v, %v), want Updates", got, err)
	}
	if messageClient.request == nil || messageClient.request.UserId != selfID || messageClient.request.Id != messageID {
		t.Fatalf("message lookup = %+v, want user=%d message=%d", messageClient.request, selfID, messageID)
	}
	if userClient.getMutableCalls != 1 || len(userClient.deleteRequests) != 1 || userClient.deleteRequests[0].UserId != selfID || userClient.deleteRequests[0].Id != userID {
		t.Fatalf("contact deletion = (lookup calls %d, requests %+v), want one deletion from %d to %d", userClient.getMutableCalls, userClient.deleteRequests, selfID, userID)
	}
	if syncClient.request == nil || syncClient.request.UserId != selfID {
		t.Fatalf("sync request = %+v, want update for user %d", syncClient.request, selfID)
	}
}

func TestContactsDeleteContactsRejectsUnprovenInputUserFromMessage(t *testing.T) {
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
			name: "message belongs to another peer",
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
			messageClient := &deleteContactsMessageClientStub{box: tt.box, err: tt.err}
			userClient := &deleteContactsUserClientStub{}
			syncClient := &deleteContactsSyncClientStub{}
			core := newDeleteContactsTestCore(selfID, userClient, messageClient, syncClient)
			inputUser := mtproto.MakeTLInputUserFromMessage(&mtproto.InputUser{
				UserId: userID,
				Peer:   mtproto.MakeTLInputPeerChat(&mtproto.InputPeer{ChatId: chatID}).To_InputPeer(),
				MsgId:  messageID,
			}).To_InputUser()

			got, err := core.ContactsDeleteContacts(&mtproto.TLContactsDeleteContacts{Id: []*mtproto.InputUser{inputUser}})
			if !errors.Is(err, mtproto.ErrContactIdInvalid) || got != nil {
				t.Fatalf("ContactsDeleteContacts() = (%+v, %v), want CONTACT_ID_INVALID", got, err)
			}
			if userClient.getMutableCalls != 0 || len(userClient.deleteRequests) != 0 || syncClient.request != nil {
				t.Fatalf("unproven message caused side effects: lookups=%d deletions=%+v sync=%+v", userClient.getMutableCalls, userClient.deleteRequests, syncClient.request)
			}
		})
	}
}

func newDeleteContactsTestCore(selfID int64, userClient *deleteContactsUserClientStub, messageClient *deleteContactsMessageClientStub, syncClient *deleteContactsSyncClientStub) *ContactsCore {
	ctx := context.Background()
	return &ContactsCore{
		ctx: ctx,
		svcCtx: &svc.ServiceContext{Dao: &dao.Dao{
			UserClient:    userClient,
			MessageClient: messageClient,
			SyncClient:    syncClient,
		}},
		Logger: logx.WithContext(ctx),
		MD:     &metadata.RpcMetadata{UserId: selfID},
	}
}
