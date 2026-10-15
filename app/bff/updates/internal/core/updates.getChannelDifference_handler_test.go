package core

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/updates/updates"
)

func TestChannelDifferenceResponseUsesEmptyConstructor(t *testing.T) {
	empty := channelDifferenceResponse(&updates.ChannelDifference{Final: true, Pts: 7})
	if empty.GetPredicateName() != mtproto.Predicate_updates_channelDifferenceEmpty || !empty.GetFinal() || empty.GetPts() != 7 {
		t.Fatalf("empty difference = %v", empty)
	}

	message := mtproto.MakeTLMessage(&mtproto.Message{Id: 8, Message: "recovered"}).To_Message()
	nonEmpty := channelDifferenceResponse(&updates.ChannelDifference{Final: true, Pts: 8, NewMessages: []*mtproto.Message{message}})
	if nonEmpty.GetPredicateName() != mtproto.Predicate_updates_channelDifference || len(nonEmpty.GetNewMessages()) != 1 {
		t.Fatalf("non-empty difference = %v", nonEmpty)
	}
}
