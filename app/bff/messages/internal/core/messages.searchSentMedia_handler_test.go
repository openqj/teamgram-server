package core

import (
	"context"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/messages/internal/dao"
	"github.com/teamgram/teamgram-server/app/bff/messages/internal/svc"
	messageclient "github.com/teamgram/teamgram-server/app/service/biz/message/client"
	messagepb "github.com/teamgram/teamgram-server/app/service/biz/message/message"
	userclient "github.com/teamgram/teamgram-server/app/service/biz/user/client"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

type searchSentMediaClient struct {
	messageclient.MessageClient
	request *messagepb.TLMessageSearchByMediaType
	result  *mtproto.MessageBoxList
}

func (c *searchSentMediaClient) MessageSearchByMediaType(_ context.Context, in *messagepb.TLMessageSearchByMediaType) (*mtproto.MessageBoxList, error) {
	c.request = in
	if c.result != nil {
		return c.result, nil
	}
	return mtproto.MakeTLMessageBoxList(&mtproto.MessageBoxList{}).To_MessageBoxList(), nil
}

type searchSentMediaResultUserClient struct {
	userclient.UserClient
}

func (*searchSentMediaResultUserClient) UserGetMutableUsers(context.Context, *userpb.TLUserGetMutableUsers) (*userpb.Vector_ImmutableUser, error) {
	return &userpb.Vector_ImmutableUser{Datas: []*mtproto.ImmutableUser{
		mtproto.MakeTLImmutableUser(&mtproto.ImmutableUser{User: &mtproto.UserData{Id: 41}}).To_ImmutableUser(),
		mtproto.MakeTLImmutableUser(&mtproto.ImmutableUser{User: &mtproto.UserData{Id: 42}}).To_ImmutableUser(),
	}}, nil
}

func searchSentMediaRequest() *mtproto.TLMessagesSearchSentMedia {
	return &mtproto.TLMessagesSearchSentMedia{
		Filter: mtproto.MakeTLInputMessagesFilterPhotoVideo(&mtproto.MessagesFilter{}).To_MessagesFilter(),
		Limit:  100,
	}
}

func TestMessagesSearchSentMediaRejectsInvalidRequests(t *testing.T) {
	authenticated := &MessagesCore{MD: &metadata.RpcMetadata{UserId: 41}}
	tests := []struct {
		name string
		core *MessagesCore
		in   *mtproto.TLMessagesSearchSentMedia
		want error
	}{
		{name: "nil core", want: mtproto.ErrAuthKeyUnregistered},
		{name: "missing metadata", core: &MessagesCore{}, want: mtproto.ErrAuthKeyUnregistered},
		{name: "nil request", core: authenticated, want: mtproto.ErrInputRequestInvalid},
		{name: "missing filter", core: authenticated, in: &mtproto.TLMessagesSearchSentMedia{Limit: 1}, want: mtproto.ErrInputFilterInvalid},
		{
			name: "unknown filter",
			core: authenticated,
			in: &mtproto.TLMessagesSearchSentMedia{
				Filter: &mtproto.MessagesFilter{PredicateName: "inputMessagesFilterUnknown"},
				Limit:  1,
			},
			want: mtproto.ErrInputFilterInvalid,
		},
		{name: "negative limit", core: authenticated, in: func() *mtproto.TLMessagesSearchSentMedia { in := searchSentMediaRequest(); in.Limit = -1; return in }(), want: mtproto.ErrLimitInvalid},
		{name: "limit over maximum", core: authenticated, in: func() *mtproto.TLMessagesSearchSentMedia { in := searchSentMediaRequest(); in.Limit = 101; return in }(), want: mtproto.ErrLimitInvalid},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.core.MessagesSearchSentMedia(tt.in)
			if got != nil || err != tt.want {
				t.Fatalf("MessagesSearchSentMedia() = (%v, %v), want (nil, %v)", got, err, tt.want)
			}
		})
	}
}

func TestMessagesSearchSentMediaFailsClosedForUnsupportedProvider(t *testing.T) {
	core := &MessagesCore{MD: &metadata.RpcMetadata{UserId: 41}}

	got, err := core.MessagesSearchSentMedia(searchSentMediaRequest())
	if got != nil || err != mtproto.ErrMethodNotImpl {
		t.Fatalf("MessagesSearchSentMedia() = (%v, %v), want (nil, METHOD_NOT_IMPL)", got, err)
	}
}

func TestMessagesSearchSentMediaUsesGlobalSentMediaQuery(t *testing.T) {
	client := &searchSentMediaClient{}
	core := &MessagesCore{
		ctx:    context.Background(),
		svcCtx: &svc.ServiceContext{Dao: &dao.Dao{MessageClient: client, UserClient: &searchFilterUserClient{}}},
		MD:     &metadata.RpcMetadata{UserId: 41},
	}

	got, err := core.MessagesSearchSentMedia(searchSentMediaRequest())
	if err != nil {
		t.Fatalf("MessagesSearchSentMedia() error = %v", err)
	}
	if got == nil || len(got.GetMessages()) != 0 {
		t.Fatalf("MessagesSearchSentMedia() = %+v, want empty typed result", got)
	}
	if client.request == nil || client.request.GetUserId() != 41 || client.request.GetPeerType() != mtproto.PEER_UNKNOWN || client.request.GetPeerId() != 0 || client.request.GetMediaType() != mtproto.MEDIA_PHOTOVIDEO {
		t.Fatalf("global sent-media request = %+v", client.request)
	}
}

func TestMessagesSearchSentMediaReturnsTypedMessagesAndHydratedUsers(t *testing.T) {
	client := &searchSentMediaClient{result: mtproto.MakeTLMessageBoxList(&mtproto.MessageBoxList{BoxList: []*mtproto.MessageBox{
		mtproto.MakeTLMessageBox(&mtproto.MessageBox{
			MessageId: 7,
			PeerType:  mtproto.PEER_USER,
			PeerId:    42,
			Message: mtproto.MakeTLMessage(&mtproto.Message{
				Id:     7,
				FromId: mtproto.MakePeerUser(41),
				PeerId: mtproto.MakePeerUser(42),
				Media:  mtproto.MakeTLMessageMediaPhoto(&mtproto.MessageMedia{}).To_MessageMedia(),
			}).To_Message(),
		}).To_MessageBox(),
	}}).To_MessageBoxList()}
	core := &MessagesCore{
		ctx: context.Background(),
		svcCtx: &svc.ServiceContext{Dao: &dao.Dao{
			MessageClient: client,
			UserClient:    &searchSentMediaResultUserClient{},
		}},
		MD: &metadata.RpcMetadata{UserId: 41},
	}

	got, err := core.MessagesSearchSentMedia(searchSentMediaRequest())
	if err != nil {
		t.Fatalf("MessagesSearchSentMedia() error = %v", err)
	}
	if got == nil || len(got.GetMessages()) != 1 || got.GetMessages()[0].GetId() != 7 {
		t.Fatalf("MessagesSearchSentMedia() = %+v, want one typed message id=7", got)
	}
	if len(got.GetUsers()) != 2 {
		t.Fatalf("MessagesSearchSentMedia() users = %d, want 2 hydrated users", len(got.GetUsers()))
	}
}

func TestMessagesSearchSentMediaRejectsBots(t *testing.T) {
	core := &MessagesCore{MD: &metadata.RpcMetadata{UserId: 41, IsBot: true}}

	got, err := core.MessagesSearchSentMedia(searchSentMediaRequest())
	if got != nil || err != mtproto.ErrBotMethodInvalid {
		t.Fatalf("MessagesSearchSentMedia() = (%v, %v), want (nil, BOT_METHOD_INVALID)", got, err)
	}
}
