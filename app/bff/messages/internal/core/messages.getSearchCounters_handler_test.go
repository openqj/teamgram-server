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
	"google.golang.org/protobuf/types/known/wrapperspb"
)

type searchCountersMessageClient struct {
	messageclient.MessageClient
	mediaTypes []int32
	counter    *mtproto.Int32
	nilResult  bool
	err        error
}

func (c *searchCountersMessageClient) MessageGetSearchCounter(_ context.Context, in *messagepb.TLMessageGetSearchCounter) (*mtproto.Int32, error) {
	c.mediaTypes = append(c.mediaTypes, in.GetMediaType())
	if c.err != nil {
		return nil, c.err
	}
	if c.nilResult {
		return nil, nil
	}
	if c.counter != nil {
		return c.counter, nil
	}
	return &mtproto.Int32{V: 3}, nil
}

type searchCountersUserClient struct {
	userclient.UserClient
	err error
}

func (c *searchCountersUserClient) UserGetMutableUsers(_ context.Context, in *userpb.TLUserGetMutableUsers) (*userpb.Vector_ImmutableUser, error) {
	if c.err != nil {
		return nil, c.err
	}
	return &userpb.Vector_ImmutableUser{Datas: []*mtproto.ImmutableUser{
		mtproto.MakeTLImmutableUser(&mtproto.ImmutableUser{User: &mtproto.UserData{
			Id:         in.GetId()[0],
			AccessHash: 99,
		}}).To_ImmutableUser(),
	}}, nil
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
		{name: "photo video", predicate: mtproto.Predicate_inputMessagesFilterPhotoVideo, mediaType: mtproto.MEDIA_PHOTOVIDEO},
		{name: "document", predicate: mtproto.Predicate_inputMessagesFilterDocument, mediaType: mtproto.MEDIA_FILE},
		{name: "url", predicate: mtproto.Predicate_inputMessagesFilterUrl, mediaType: mtproto.MEDIA_URL},
		{name: "gif", predicate: mtproto.Predicate_inputMessagesFilterGif, mediaType: mtproto.MEDIA_GIF},
		{name: "music", predicate: mtproto.Predicate_inputMessagesFilterMusic, mediaType: mtproto.MEDIA_MUSIC},
		{name: "voice", predicate: mtproto.Predicate_inputMessagesFilterVoice, mediaType: mtproto.MEDIA_VOICE_FILE},
		{name: "round voice", predicate: mtproto.Predicate_inputMessagesFilterRoundVoice, mediaType: mtproto.MEDIA_AUDIO},
		{name: "round video", predicate: mtproto.Predicate_inputMessagesFilterRoundVideo, mediaType: mtproto.MEDIA_ROUND_FILE},
		{name: "chat photos", predicate: mtproto.Predicate_inputMessagesFilterChatPhotos, mediaType: mtproto.MEDIA_CHAT_PHOTO},
		{name: "phone calls", predicate: mtproto.Predicate_inputMessagesFilterPhoneCalls, mediaType: mtproto.MEDIA_PHONE_CALL},
		{name: "pinned", predicate: mtproto.Predicate_inputMessagesFilterPinned, mediaType: mtproto.MEDIA_PINNED},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client := &searchCountersMessageClient{}
			core := &MessagesCore{
				ctx:    context.Background(),
				svcCtx: &svc.ServiceContext{Dao: &dao.Dao{MessageClient: client, UserClient: &searchCountersUserClient{}}},
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

func TestMessagesGetSearchCountersValidatesPeerAndRequest(t *testing.T) {
	client := &searchCountersMessageClient{}
	core := &MessagesCore{
		ctx:    context.Background(),
		svcCtx: &svc.ServiceContext{Dao: &dao.Dao{MessageClient: client, UserClient: &searchCountersUserClient{}}},
		Logger: logx.WithContext(context.Background()),
		MD:     &metadata.RpcMetadata{UserId: 41},
	}
	valid := func() *mtproto.TLMessagesGetSearchCounters {
		return &mtproto.TLMessagesGetSearchCounters{
			Peer:    mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: 42, AccessHash: 99}).To_InputPeer(),
			Filters: []*mtproto.MessagesFilter{searchCounterFilter(mtproto.Predicate_inputMessagesFilterPhotos)},
		}
	}
	tests := []struct {
		name string
		in   *mtproto.TLMessagesGetSearchCounters
		want error
	}{
		{name: "nil request", want: mtproto.ErrInputRequestInvalid},
		{name: "missing peer", in: &mtproto.TLMessagesGetSearchCounters{}, want: mtproto.ErrPeerIdInvalid},
		{name: "missing user access hash", in: func() *mtproto.TLMessagesGetSearchCounters {
			in := valid()
			in.Peer = mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: 42}).To_InputPeer()
			return in
		}(), want: mtproto.ErrPeerIdInvalid},
		{name: "zero top message", in: func() *mtproto.TLMessagesGetSearchCounters {
			in := valid()
			in.TopMsgId = wrapperspb.Int32(0)
			return in
		}(), want: mtproto.ErrMessageIdInvalid},
		{name: "top message scope", in: func() *mtproto.TLMessagesGetSearchCounters {
			in := valid()
			in.TopMsgId = mtproto.MakeFlagsInt32(17)
			return in
		}(), want: mtproto.ErrMethodNotImpl},
		{name: "unknown filter", in: func() *mtproto.TLMessagesGetSearchCounters {
			in := valid()
			in.Filters = []*mtproto.MessagesFilter{searchCounterFilter("inputMessagesFilterUnknown")}
			return in
		}(), want: mtproto.ErrInputFilterInvalid},
		{name: "known unsupported filter", in: func() *mtproto.TLMessagesGetSearchCounters {
			in := valid()
			in.Filters = []*mtproto.MessagesFilter{searchCounterFilter(mtproto.Predicate_inputMessagesFilterGeo)}
			return in
		}(), want: mtproto.ErrMethodNotImpl},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := core.MessagesGetSearchCounters(tc.in)
			if got != nil || err != tc.want {
				t.Fatalf("result = (%v, %v), want nil and %v", got, err, tc.want)
			}
		})
	}
}

func TestMessagesGetSearchCountersRejectsInvalidProviderResults(t *testing.T) {
	wantErr := errors.New("counter unavailable")
	base := func(client *searchCountersMessageClient) *MessagesCore {
		return &MessagesCore{
			ctx:    context.Background(),
			svcCtx: &svc.ServiceContext{Dao: &dao.Dao{MessageClient: client, UserClient: &searchCountersUserClient{}}},
			Logger: logx.WithContext(context.Background()),
			MD:     &metadata.RpcMetadata{UserId: 41},
		}
	}
	request := &mtproto.TLMessagesGetSearchCounters{
		Peer:    mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: 42, AccessHash: 99}).To_InputPeer(),
		Filters: []*mtproto.MessagesFilter{searchCounterFilter(mtproto.Predicate_inputMessagesFilterPhotos)},
	}
	tests := []struct {
		name   string
		client *searchCountersMessageClient
		want   error
	}{
		{name: "provider error", client: &searchCountersMessageClient{err: wantErr}, want: wantErr},
		{name: "nil result", client: &searchCountersMessageClient{nilResult: true}, want: mtproto.ErrInternalServerError},
		{name: "negative result", client: &searchCountersMessageClient{counter: &mtproto.Int32{V: -1}}, want: mtproto.ErrInternalServerError},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := base(tc.client).MessagesGetSearchCounters(request)
			if got != nil || !errors.Is(err, tc.want) {
				t.Fatalf("result = (%v, %v), want nil and %v", got, err, tc.want)
			}
		})
	}
}
