package core

import (
	"context"
	"math"
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

type searchByMediaTypeClient struct {
	messageclient.MessageClient
	got    *messagepb.TLMessageSearchByMediaType
	result *mtproto.MessageBoxList
}

func (c *searchByMediaTypeClient) MessageSearchByMediaType(_ context.Context, in *messagepb.TLMessageSearchByMediaType) (*mtproto.MessageBoxList, error) {
	c.got = in
	return c.result, nil
}

type searchFilterUserClient struct {
	userclient.UserClient
}

func (*searchFilterUserClient) UserGetMutableUsers(context.Context, *userpb.TLUserGetMutableUsers) (*userpb.Vector_ImmutableUser, error) {
	return &userpb.Vector_ImmutableUser{}, nil
}

func TestMessagesSearchMapsVoiceMediaFiltersToCanonicalMessageQuery(t *testing.T) {
	tests := []struct {
		name      string
		filter    *mtproto.MessagesFilter
		mediaType int32
	}{
		{
			name:      "voice",
			filter:    mtproto.MakeTLInputMessagesFilterVoice(&mtproto.MessagesFilter{}).To_MessagesFilter(),
			mediaType: mtproto.MEDIA_VOICE_FILE,
		},
		{
			name:      "round video",
			filter:    mtproto.MakeTLInputMessagesFilterRoundVideo(&mtproto.MessagesFilter{}).To_MessagesFilter(),
			mediaType: mtproto.MEDIA_ROUND_FILE,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &searchByMediaTypeClient{
				result: mtproto.MakeTLMessageBoxList(&mtproto.MessageBoxList{}).To_MessageBoxList(),
			}
			core := &MessagesCore{
				ctx: context.Background(),
				svcCtx: &svc.ServiceContext{Dao: &dao.Dao{
					MessageClient: client,
					UserClient:    &searchFilterUserClient{},
				}},
				MD: &metadata.RpcMetadata{UserId: 41},
			}
			peer := mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: 42, AccessHash: 99}).To_InputPeer()

			got, err := core.MessagesSearch(&mtproto.TLMessagesSearch{
				Peer:   peer,
				Filter: tt.filter,
				Limit:  80,
			})
			if err != nil {
				t.Fatalf("MessagesSearch() error = %v", err)
			}
			if got == nil || len(got.GetMessages()) != 0 {
				t.Fatalf("MessagesSearch() = %+v, want empty result from canonical message query", got)
			}
			if client.got == nil {
				t.Fatal("MessageSearchByMediaType was not called")
			}
			if client.got.UserId != 41 || client.got.PeerType != mtproto.PEER_USER || client.got.PeerId != 42 {
				t.Fatalf("query peer = (%d, %d, %d), want (41, %d, 42)", client.got.UserId, client.got.PeerType, client.got.PeerId, mtproto.PEER_USER)
			}
			if client.got.MediaType != tt.mediaType || client.got.Offset != math.MaxInt32 || client.got.Limit != 50 {
				t.Fatalf("query media/pagination = (%d, %d, %d), want (%d, %d, 50)", client.got.MediaType, client.got.Offset, client.got.Limit, tt.mediaType, math.MaxInt32)
			}
		})
	}
}
