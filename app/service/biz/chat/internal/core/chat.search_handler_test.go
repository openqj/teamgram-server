package core

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
	chatpb "github.com/teamgram/teamgram-server/app/service/biz/chat/chat"
)

func TestChatSearchValidatesPagingBeforeDAOAccess(t *testing.T) {
	tests := []struct {
		name string
		in   *chatpb.TLChatSearch
		want error
	}{
		{name: "nil request", want: mtproto.ErrInputRequestInvalid},
		{name: "missing caller", in: &chatpb.TLChatSearch{Q: "abc", Limit: 1}, want: mtproto.ErrUserIdInvalid},
		{name: "negative offset", in: &chatpb.TLChatSearch{SelfId: 7, Q: "abc", Offset: -1, Limit: 1}, want: mtproto.ErrInputRequestInvalid},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := (&ChatCore{}).ChatSearch(tt.in)
			if got != nil || err != tt.want {
				t.Fatalf("ChatSearch() = (%v, %v), want (nil, %v)", got, err, tt.want)
			}
		})
	}
}

func TestChatSearchReturnsEmptyForShortQueryWithoutDAOAccess(t *testing.T) {
	got, err := (&ChatCore{}).ChatSearch(&chatpb.TLChatSearch{SelfId: 7, Q: "ab", Limit: 1})
	if err != nil {
		t.Fatalf("ChatSearch() error = %v", err)
	}
	if got == nil || len(got.GetDatas()) != 0 {
		t.Fatalf("ChatSearch() = %v, want an empty result", got)
	}
}
