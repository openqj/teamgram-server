package core

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/persist"
)

func TestStatsGetPollStatsReadsAuthorizedDurableBallots(t *testing.T) {
	const userID int64 = 991701
	const chatID int64 = 991702
	const msgID int32 = 37
	const pollID int64 = 991703
	dsn := os.Getenv("APIFULL_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("APIFULL_POSTGRES_DSN must point to an isolated PostgreSQL 18 database")
	}
	if err := persist.OpenPostgresReadOnly(dsn); err != nil {
		t.Fatal(err)
	}
	reader := &pollMessageReaderStub{boxes: map[int64]*mtproto.MessageBox{
		userID: pollTestBox(chatID, msgID, pollTestPoll(pollID, true, false, false)),
	}}
	core := pollTestCore(userID, reader, &pollChatClientStub{chat: pollTestChat(chatID, userID)})
	ballots, err := json.Marshal([]pollBallotJSON{
		{UserId: userID, Options: [][]byte{[]byte("one")}, Date: 100},
		{UserId: userID + 1, Options: [][]byte{[]byte("two")}, Date: 101},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err = persist.Default.Set(pollBallotKey(pollID), string(ballots)); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = persist.Default.Set(pollBallotKey(pollID), "") })
	result, err := core.StatsGetPollStats(&mtproto.TLStatsGetPollStats{
		Peer:  pollTestInputPeer(chatID),
		MsgId: msgID,
	})
	if err != nil || result == nil || result.GetVotesGraph() == nil || result.GetVotesGraph().GetJson() == nil {
		t.Fatalf("stats.getPollStats = (%#v, %v)", result, err)
	}
	if result.GetVotesGraph().GetJson().GetData() != `{"count":2}` {
		t.Fatalf("votes graph = %q, want count 2", result.GetVotesGraph().GetJson().GetData())
	}
	if reader.calls != 1 {
		t.Fatalf("message reader calls = %d, want 1", reader.calls)
	}
}
