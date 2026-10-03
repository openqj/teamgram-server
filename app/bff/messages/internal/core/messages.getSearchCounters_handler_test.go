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
	"github.com/zeromicro/go-zero/core/logx"
)

type searchCountersMessageClient struct {
	messageclient.MessageClient
	mediaTypes []int32
}

func (c *searchCountersMessageClient) MessageGetSearchCounter(_ context.Context, in *messagepb.TLMessageGetSearchCounter) (*mtproto.Int32, error) {
	c.mediaTypes = append(c.mediaTypes, in.GetMediaType())
	return &mtproto.Int32{V: 3}, nil
}

func searchCounterFilter(predicate string) *mtproto.MessagesFilter {
	return &mtproto.MessagesFilter{PredicateName: predicate}
}

func TestMessagesGetSearchCountersMapsSupportedMediaFilters(t *testing.T) {
	cases := []struct {
		name      string
		predicate string
		mediaType int32
	}{
		{name: "photos", predicate: mtproto.Predicate_inputMessagesFilterPhotos, mediaType: mtproto.MEDIA_PHOTOS_ONLY},
		{name: "video", predicate: mtproto.Predicate_inputMessagesFilterVideo, mediaType: mtproto.MEDIA_VIDEOS_ONLY},
		{name: "voice", predicate: mtproto.Predicate_inputMessagesFilterVoice, mediaType: mtproto.MEDIA_VOICE_FILE},
		{name: "round video", predicate: mtproto.Predicate_inputMessagesFilterRoundVideo, mediaType: mtproto.MEDIA_ROUND_FILE},
		{name: "phone calls", predicate: mtproto.Predicate_inputMessagesFilterPhoneCalls, mediaType: mtproto.MEDIA_PHONE_CALL},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client := &searchCountersMessageClient{}
			core := &MessagesCore{
				ctx:    context.Background(),
				svcCtx: &svc.ServiceContext{Dao: &dao.Dao{MessageClient: client}},
				Logger: logx.WithContext(context.Background()),
				MD:     &metadata.RpcMetadata{UserId: 41},
			}
			got, err := core.MessagesGetSearchCounters(&mtproto.TLMessagesGetSearchCounters{
				Peer:    mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: 42, AccessHash: 99}).To_InputPeer(),
				Filters: []*mtproto.MessagesFilter{searchCounterFilter(tc.predicate)},
			})
			if err != nil {
				t.Fatalf("MessagesGetSearchCounters() error = %v", err)
			}
			if got == nil || len(got.GetDatas()) != 1 || got.GetDatas()[0].GetCount() != 3 {
				t.Fatalf("counter result = %v, want one count of 3", got)
			}
			if len(client.mediaTypes) != 1 || client.mediaTypes[0] != tc.mediaType {
				t.Fatalf("media types = %v, want [%d]", client.mediaTypes, tc.mediaType)
			}
		})
	}
}

func TestMessagesGetSearchCountersRejectsUnauthenticatedAndMissingProvider(t *testing.T) {
	core := &MessagesCore{ctx: context.Background(), svcCtx: &svc.ServiceContext{Dao: &dao.Dao{}}}
	core.MD = &metadata.RpcMetadata{}
	request := &mtproto.TLMessagesGetSearchCounters{
		Peer: mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: 42, AccessHash: 99}).To_InputPeer(),
	}
	if _, err := core.MessagesGetSearchCounters(request); err != mtproto.ErrAuthKeyUnregistered {
		t.Fatalf("unauthenticated error = %v, want %v", err, mtproto.ErrAuthKeyUnregistered)
	}
	core.MD.UserId = 41
	if _, err := core.MessagesGetSearchCounters(request); err != mtproto.ErrInternalServerError {
		t.Fatalf("missing provider error = %v, want %v", err, mtproto.ErrInternalServerError)
	}
}
