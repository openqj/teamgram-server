package core

import (
	"context"
	"errors"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/messages/internal/dao"
	"github.com/teamgram/teamgram-server/app/bff/messages/internal/svc"
	messageclient "github.com/teamgram/teamgram-server/app/service/biz/message/client"
	messagepb "github.com/teamgram/teamgram-server/app/service/biz/message/message"
	userclient "github.com/teamgram/teamgram-server/app/service/biz/user/client"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
	"github.com/zeromicro/go-zero/core/logx"
)

type recentLocationsMessageClient struct {
	messageclient.MessageClient
	getHistory func(*messagepb.TLMessageGetHistoryMessages) (*messagepb.Vector_MessageBox, error)
	requests   []*messagepb.TLMessageGetHistoryMessages
}

func (c *recentLocationsMessageClient) MessageGetHistoryMessages(_ context.Context, in *messagepb.TLMessageGetHistoryMessages) (*messagepb.Vector_MessageBox, error) {
	c.requests = append(c.requests, in)
	return c.getHistory(in)
}

type recentLocationsUserClient struct {
	userclient.UserClient
	err error
}

func (c *recentLocationsUserClient) UserGetMutableUsers(context.Context, *userpb.TLUserGetMutableUsers) (*userpb.Vector_ImmutableUser, error) {
	return &userpb.Vector_ImmutableUser{}, c.err
}

func newRecentLocationsCore(messageClient messageclient.MessageClient) *MessagesCore {
	return newRecentLocationsCoreWithUserClient(messageClient, &recentLocationsUserClient{})
}

func newRecentLocationsCoreWithUserClient(messageClient messageclient.MessageClient, users userclient.UserClient) *MessagesCore {
	ctx := context.Background()
	return &MessagesCore{
		ctx:    ctx,
		Logger: logx.WithContext(ctx),
		svcCtx: &svc.ServiceContext{Dao: &dao.Dao{
			MessageClient: messageClient,
			UserClient:    users,
		}},
		MD: &metadata.RpcMetadata{UserId: 41},
	}
}

func recentLocationsBox(id int32, geo bool) *mtproto.MessageBox {
	media := mtproto.MakeTLMessageMediaEmpty(&mtproto.MessageMedia{}).To_MessageMedia()
	if geo {
		media = mtproto.MakeTLMessageMediaGeo(&mtproto.MessageMedia{}).To_MessageMedia()
	}
	msg := mtproto.MakeTLMessage(&mtproto.Message{
		Id:     id,
		PeerId: mtproto.MakePeerUser(42),
		Media:  media,
	}).To_Message()
	return mtproto.MakeTLMessageBox(&mtproto.MessageBox{
		MessageId: id,
		PeerType:  mtproto.PEER_USER,
		PeerId:    42,
		Message:   msg,
	}).To_MessageBox()
}

func TestMessagesGetRecentLocationsPaginatesPastNonGeoHistory(t *testing.T) {
	firstPage := make([]*mtproto.MessageBox, 0, recentLocationsHistoryPageSize)
	for id := int32(200); id > 100; id-- {
		firstPage = append(firstPage, recentLocationsBox(id, false))
	}
	client := &recentLocationsMessageClient{
		getHistory: func(in *messagepb.TLMessageGetHistoryMessages) (*messagepb.Vector_MessageBox, error) {
			switch in.GetOffsetId() {
			case 0:
				return &messagepb.Vector_MessageBox{Datas: firstPage}, nil
			case 101:
				return &messagepb.Vector_MessageBox{Datas: []*mtproto.MessageBox{
					recentLocationsBox(100, true),
					recentLocationsBox(99, false),
				}}, nil
			default:
				t.Fatalf("history offset = %d, want 0 or 101", in.GetOffsetId())
				return nil, nil
			}
		},
	}
	peer := mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: 42, AccessHash: 99}).To_InputPeer()

	got, err := newRecentLocationsCore(client).MessagesGetRecentLocations(&mtproto.TLMessagesGetRecentLocations{
		Peer:  peer,
		Limit: 1,
	})
	if err != nil {
		t.Fatalf("MessagesGetRecentLocations() error = %v", err)
	}
	if got == nil || len(got.GetMessages()) != 1 || got.GetMessages()[0].GetId() != 100 {
		t.Fatalf("MessagesGetRecentLocations() = %+v, want geo message 100", got)
	}
	if len(client.requests) != 2 || client.requests[0].GetLimit() != recentLocationsHistoryPageSize || client.requests[1].GetOffsetId() != 101 {
		t.Fatalf("history requests = %+v, want two 100-message pages offset at 101", client.requests)
	}
}

func TestMessagesGetRecentLocationsFailsClosedOnHistoryReadFailure(t *testing.T) {
	historyErr := errors.New("history read failed")
	client := &recentLocationsMessageClient{
		getHistory: func(*messagepb.TLMessageGetHistoryMessages) (*messagepb.Vector_MessageBox, error) {
			return nil, historyErr
		},
	}
	peer := mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: 42, AccessHash: 99}).To_InputPeer()

	got, err := newRecentLocationsCore(client).MessagesGetRecentLocations(&mtproto.TLMessagesGetRecentLocations{Peer: peer, Limit: 10})
	if got != nil || !errors.Is(err, historyErr) {
		t.Fatalf("MessagesGetRecentLocations() = (%+v, %v), want (nil, history read error)", got, err)
	}
}

func TestMessagesGetRecentLocationsDoesNotReturnPartialResultsAfterLaterReadFailure(t *testing.T) {
	historyErr := errors.New("later history page failed")
	firstPage := make([]*mtproto.MessageBox, 0, recentLocationsHistoryPageSize)
	for id := int32(300); id > 200; id-- {
		firstPage = append(firstPage, recentLocationsBox(id, id == 300))
	}
	client := &recentLocationsMessageClient{
		getHistory: func(in *messagepb.TLMessageGetHistoryMessages) (*messagepb.Vector_MessageBox, error) {
			if in.GetOffsetId() == 0 {
				return &messagepb.Vector_MessageBox{Datas: firstPage}, nil
			}
			return nil, historyErr
		},
	}
	peer := mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: 42, AccessHash: 99}).To_InputPeer()

	got, err := newRecentLocationsCore(client).MessagesGetRecentLocations(&mtproto.TLMessagesGetRecentLocations{Peer: peer, Limit: 2})
	if got != nil || !errors.Is(err, historyErr) {
		t.Fatalf("MessagesGetRecentLocations() = (%+v, %v), want nil result after later-page error", got, err)
	}
}

func TestMessagesGetRecentLocationsFailsClosedOnNilHistoryPage(t *testing.T) {
	client := &recentLocationsMessageClient{
		getHistory: func(*messagepb.TLMessageGetHistoryMessages) (*messagepb.Vector_MessageBox, error) {
			return nil, nil
		},
	}
	peer := mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: 42, AccessHash: 99}).To_InputPeer()

	got, err := newRecentLocationsCore(client).MessagesGetRecentLocations(&mtproto.TLMessagesGetRecentLocations{Peer: peer, Limit: 10})
	if got != nil || err != mtproto.ErrInternalServerError {
		t.Fatalf("MessagesGetRecentLocations() = (%+v, %v), want (nil, INTERNAL_SERVER_ERROR)", got, err)
	}
}

func TestMessagesGetRecentLocationsPropagatesUserHydrationError(t *testing.T) {
	hydrationErr := errors.New("user hydration failed")
	client := &recentLocationsMessageClient{
		getHistory: func(*messagepb.TLMessageGetHistoryMessages) (*messagepb.Vector_MessageBox, error) {
			return &messagepb.Vector_MessageBox{Datas: []*mtproto.MessageBox{recentLocationsBox(100, true)}}, nil
		},
	}
	core := newRecentLocationsCoreWithUserClient(client, &recentLocationsUserClient{err: hydrationErr})
	peer := mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: 42, AccessHash: 99}).To_InputPeer()

	got, err := core.MessagesGetRecentLocations(&mtproto.TLMessagesGetRecentLocations{Peer: peer, Limit: 1})
	if got != nil || !errors.Is(err, hydrationErr) {
		t.Fatalf("MessagesGetRecentLocations() = (%+v, %v), want (nil, user hydration error)", got, err)
	}
}

func TestMessagesGetRecentLocationsChannelDoesNotReturnUnverifiedEmptySuccess(t *testing.T) {
	client := &recentLocationsMessageClient{
		getHistory: func(*messagepb.TLMessageGetHistoryMessages) (*messagepb.Vector_MessageBox, error) {
			t.Fatal("channel history must not use the generic message service")
			return nil, nil
		},
	}
	peer := mtproto.MakeTLInputPeerChannel(&mtproto.InputPeer{ChannelId: 10, AccessHash: 11}).To_InputPeer()

	got, err := newRecentLocationsCore(client).MessagesGetRecentLocations(&mtproto.TLMessagesGetRecentLocations{Peer: peer, Limit: 1})
	if got != nil || err == nil {
		t.Fatalf("MessagesGetRecentLocations(channel) = (%+v, %v), want nil result and propagated channel history error", got, err)
	}
	if len(client.requests) != 0 {
		t.Fatalf("generic message history requests = %d, want none for channel", len(client.requests))
	}
}
