package core

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func TestHasNonZeroDateFilterTreatsEmptyWrappersAsUnset(t *testing.T) {
	peer := mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: 7, AccessHash: 11}).To_InputPeer()
	cases := []struct {
		name string
		in   *mtproto.TLMessagesDeleteHistory
		want bool
	}{
		{name: "nil wrappers", in: &mtproto.TLMessagesDeleteHistory{Peer: peer}, want: false},
		{name: "empty wrappers", in: &mtproto.TLMessagesDeleteHistory{Peer: peer, MinDate: wrapperspb.Int32(0), MaxDate: wrapperspb.Int32(0)}, want: false},
		{name: "min date", in: &mtproto.TLMessagesDeleteHistory{Peer: peer, MinDate: wrapperspb.Int32(1)}, want: true},
		{name: "max date", in: &mtproto.TLMessagesDeleteHistory{Peer: peer, MaxDate: wrapperspb.Int32(1)}, want: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := hasNonZeroDateFilter(tc.in); got != tc.want {
				t.Fatalf("hasNonZeroDateFilter() = %v, want %v", got, tc.want)
			}
		})
	}
}
