package core

import (
	"errors"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func TestSearchPostsQueryNormalizesHashtags(t *testing.T) {
	cases := []struct {
		name string
		in   *mtproto.TLChannelsSearchPosts
		want string
	}{
		{name: "plain hashtag", in: &mtproto.TLChannelsSearchPosts{Hashtag_STRING: "topic"}, want: "#topic"},
		{name: "prefixed hashtag", in: &mtproto.TLChannelsSearchPosts{Hashtag_STRING: "#topic"}, want: "#topic"},
		{name: "flag hashtag", in: &mtproto.TLChannelsSearchPosts{Hashtag_FLAGSTRING: mtproto.MakeFlagsString(" topic ")}, want: "#topic"},
		{name: "query wins", in: &mtproto.TLChannelsSearchPosts{Query: mtproto.MakeFlagsString(" announcement "), Hashtag_STRING: "topic"}, want: "announcement"},
		{name: "empty", in: &mtproto.TLChannelsSearchPosts{}, want: ""},
		{name: "nil request", want: ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := searchPostsQuery(tc.in); got != tc.want {
				t.Fatalf("searchPostsQuery() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestChannelsSearchPostsRejectsInvalidRequest(t *testing.T) {
	authenticated := &MessagesCore{MD: &metadata.RpcMetadata{UserId: 42}}
	channelPeer := mtproto.MakeTLInputPeerChannel(&mtproto.InputPeer{ChannelId: 7, AccessHash: 700}).To_InputPeer()
	for _, tc := range []struct {
		name string
		core *MessagesCore
		in   *mtproto.TLChannelsSearchPosts
		want error
	}{
		{name: "nil core", want: mtproto.ErrAuthKeyUnregistered},
		{name: "missing metadata", core: &MessagesCore{}, want: mtproto.ErrAuthKeyUnregistered},
		{name: "nil request", core: authenticated, want: mtproto.ErrPeerIdInvalid},
		{name: "missing offset peer", core: authenticated, in: &mtproto.TLChannelsSearchPosts{}, want: mtproto.ErrPeerIdInvalid},
		{name: "bot", core: &MessagesCore{MD: &metadata.RpcMetadata{UserId: 42, IsBot: true}}, in: &mtproto.TLChannelsSearchPosts{OffsetPeer: channelPeer, Query: mtproto.MakeFlagsString("topic")}, want: mtproto.ErrBotMethodInvalid},
		{name: "negative limit", core: authenticated, in: &mtproto.TLChannelsSearchPosts{OffsetPeer: channelPeer, Query: mtproto.MakeFlagsString("topic"), Limit: -1}, want: mtproto.ErrLimitInvalid},
		{name: "limit too large", core: authenticated, in: &mtproto.TLChannelsSearchPosts{OffsetPeer: channelPeer, Query: mtproto.MakeFlagsString("topic"), Limit: 101}, want: mtproto.ErrLimitInvalid},
		{name: "negative offset id", core: authenticated, in: &mtproto.TLChannelsSearchPosts{OffsetPeer: channelPeer, Query: mtproto.MakeFlagsString("topic"), OffsetId: -1}, want: mtproto.ErrOffsetInvalid},
		{name: "paid stars", core: authenticated, in: &mtproto.TLChannelsSearchPosts{OffsetPeer: channelPeer, Query: mtproto.MakeFlagsString("topic"), AllowPaidStars: mtproto.MakeFlagsInt64(1)}, want: mtproto.ErrMethodNotImpl},
		{name: "negative offset rate", core: authenticated, in: &mtproto.TLChannelsSearchPosts{OffsetPeer: channelPeer, Query: mtproto.MakeFlagsString("topic"), OffsetRate: -1}, want: mtproto.ErrOffsetInvalid},
		{name: "empty query", core: authenticated, in: &mtproto.TLChannelsSearchPosts{OffsetPeer: channelPeer}, want: mtproto.ErrSearchQueryEmpty},
		{name: "blank hashtag", core: authenticated, in: &mtproto.TLChannelsSearchPosts{OffsetPeer: channelPeer, Hashtag_STRING: "  "}, want: mtproto.ErrSearchQueryEmpty},
		{name: "zero paid stars is free", core: authenticated, in: &mtproto.TLChannelsSearchPosts{OffsetPeer: channelPeer, AllowPaidStars: wrapperspb.Int64(0)}, want: mtproto.ErrSearchQueryEmpty},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := tc.core.ChannelsSearchPosts(tc.in)
			if got != nil || !errors.Is(err, tc.want) {
				t.Fatalf("ChannelsSearchPosts() = (%v, %v), want (nil, %v)", got, err, tc.want)
			}
		})
	}
}
