package core

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/message/message"
)

func TestMessageSearchRejectsEmptyQueryBeforeDAOAccess(t *testing.T) {
	core := &MessageCore{}

	got, err := core.MessageSearch(&message.TLMessageSearch{
		UserId:   41,
		PeerType: mtproto.PEER_CHAT,
		PeerId:   77,
	})
	if got != nil || err != mtproto.ErrSearchQueryEmpty {
		t.Fatalf("MessageSearch() = (%v, %v), want (nil, SEARCH_QUERY_EMPTY)", got, err)
	}
}
