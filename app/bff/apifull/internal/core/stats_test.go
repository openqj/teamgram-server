package core

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/apifull/channelview"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/domain"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/persist"
)

func TestStatsUnauthed(t *testing.T) {
	c := &ApiFullCore{}
	checks := []func() error{
		func() error { _, err := c.StatsGetBroadcastStats(nil); return err },
		func() error { _, err := c.StatsLoadAsyncGraph(nil); return err },
		func() error { _, err := c.StatsGetMegagroupStats(nil); return err },
		func() error { _, err := c.StatsGetMessagePublicForwards5F150144(nil); return err },
		func() error { _, err := c.StatsGetMessageStats(nil); return err },
		func() error { _, err := c.StatsGetStoryStats(nil); return err },
		func() error { _, err := c.StatsGetStoryPublicForwards(nil); return err },
		func() error { _, err := c.StatsGetPollStats(nil); return err },
		func() error { _, err := c.StatsGetMessagePublicForwards5630281B(nil); return err },
		func() error { _, err := c.StatsGetBroadcastRevenueStats(nil); return err },
		func() error { _, err := c.StatsGetBroadcastRevenueWithdrawalUrl(nil); return err },
		func() error { _, err := c.StatsGetBroadcastRevenueTransactions(nil); return err },
		func() error { _, err := c.ChannelsGetChannelRecommendations(nil); return err },
	}
	for i, check := range checks {
		if err := check(); !errors.Is(err, mtproto.ErrAuthKeyUnregistered) {
			t.Fatalf("handler %d: got %v", i, err)
		}
	}
}

func TestStatsRequireChannelTarget(t *testing.T) {
	c := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: 1}}
	if got, err := c.StatsGetBroadcastStats(nil); got != nil || !errors.Is(err, mtproto.ErrChannelInvalid) {
		t.Fatalf("broadcast stats = (%+v, %v), want CHANNEL_INVALID", got, err)
	}
	if got, err := c.StatsGetMegagroupStats(nil); got != nil || !errors.Is(err, mtproto.ErrChannelInvalid) {
		t.Fatalf("megagroup stats = (%+v, %v), want CHANNEL_INVALID", got, err)
	}
}

func TestStoryStatsFailsClosedWithoutStoryProvider(t *testing.T) {
	c := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: 1}}
	if got, err := c.StatsGetStoryStats(nil); got != nil || !errors.Is(err, mtproto.ErrMethodNotImpl) {
		t.Fatalf("story stats = (%+v, %v), want METHOD_NOT_IMPL", got, err)
	}
}

func TestPollStatsValidatesInputBeforeProvider(t *testing.T) {
	c := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: 1}}
	if got, err := c.StatsGetPollStats(nil); got != nil || !errors.Is(err, mtproto.ErrInputRequestInvalid) {
		t.Fatalf("nil poll stats = (%+v, %v), want INPUT_REQUEST_INVALID", got, err)
	}
	if got, err := c.StatsGetPollStats(&mtproto.TLStatsGetPollStats{}); got != nil || !errors.Is(err, mtproto.ErrPeerIdInvalid) {
		t.Fatalf("missing poll peer = (%+v, %v), want PEER_ID_INVALID", got, err)
	}
	peer := mtproto.MakeTLInputPeerSelf(&mtproto.InputPeer{}).To_InputPeer()
	if got, err := c.StatsGetPollStats(&mtproto.TLStatsGetPollStats{Peer: peer}); got != nil || !errors.Is(err, mtproto.ErrMessageIdInvalid) {
		t.Fatalf("missing poll message = (%+v, %v), want MESSAGE_ID_INVALID", got, err)
	}
}

func TestPollStatsReadsPersistedBallots(t *testing.T) {
	usePollTestStore(t)
	const (
		userID int64 = 98111
		chatID int64 = 98112
		msgID  int32 = 31
		pollID int64 = 98113
	)
	poll := pollTestPoll(pollID, true, false, false)
	reader := &pollMessageReaderStub{boxes: map[int64]*mtproto.MessageBox{
		userID: pollTestBox(chatID, msgID, poll),
	}}
	core := pollTestCore(userID, reader, &pollChatClientStub{chat: pollTestChat(chatID, userID)})
	peer := pollTestInputPeer(chatID)
	if _, err := core.MessagesSendVote(&mtproto.TLMessagesSendVote{
		Peer: peer, MsgId: msgID, Options: [][]byte{[]byte("one")},
	}); err != nil {
		t.Fatalf("persist poll vote: %v", err)
	}

	stats, err := core.StatsGetPollStats(&mtproto.TLStatsGetPollStats{Peer: peer, MsgId: msgID})
	if err != nil || stats == nil || stats.GetVotesGraph() == nil || stats.GetVotesGraph().GetJson() == nil {
		t.Fatalf("poll stats = (%+v, %v), want typed graph", stats, err)
	}
	if got := stats.GetVotesGraph().GetJson().GetData(); got != `{"count":1}` {
		t.Fatalf("poll stats graph = %q, want one persisted vote", got)
	}
	if err := stats.Encode(mtproto.NewEncodeBuf(256), 229); err != nil {
		t.Fatalf("encode poll stats: %v", err)
	}
}

func TestStatsRejectsInvalidHashAndNonAdmin(t *testing.T) {
	channelID := time.Now().UnixNano()
	const owner int64 = 98101
	const member int64 = 98102
	if err := domain.SaveChannel(domain.Channel{
		ID: channelID, AccessHash: channelID + 7, Creator: owner, Title: "stats-auth-test", Broadcast: true,
	}); err != nil {
		t.Fatal(err)
	}
	if err := domain.JoinChannel(channelID, member); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = domain.DeleteChannel(owner, channelID) })

	badHash := mtproto.MakeTLInputChannel(&mtproto.InputChannel{ChannelId: channelID, AccessHash: channelID + 8}).To_InputChannel()
	ownerCore := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: owner}}
	if _, err := ownerCore.StatsGetBroadcastStats(&mtproto.TLStatsGetBroadcastStats{Channel: badHash}); !errors.Is(err, mtproto.ErrChannelInvalid) {
		t.Fatalf("broadcast stats bad hash = %v", err)
	}
	if _, err := ownerCore.StatsGetMegagroupStats(&mtproto.TLStatsGetMegagroupStats{Channel: badHash}); !errors.Is(err, mtproto.ErrChannelInvalid) {
		t.Fatalf("megagroup stats bad hash = %v", err)
	}

	input := mtproto.MakeTLInputChannel(&mtproto.InputChannel{ChannelId: channelID, AccessHash: channelID + 7}).To_InputChannel()
	if got, err := ownerCore.StatsGetBroadcastStats(&mtproto.TLStatsGetBroadcastStats{Channel: input}); err != nil || got == nil {
		t.Fatalf("broadcast stats owner = result=%+v err=%v", got, err)
	}
	if got, err := ownerCore.StatsGetMegagroupStats(&mtproto.TLStatsGetMegagroupStats{Channel: input}); err != nil || got == nil {
		t.Fatalf("megagroup stats owner = result=%+v err=%v", got, err)
	}
	memberCore := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: member}}
	if _, err := memberCore.StatsGetBroadcastStats(&mtproto.TLStatsGetBroadcastStats{Channel: input}); !errors.Is(err, mtproto.ErrChatAdminRequired) {
		t.Fatalf("broadcast stats ordinary member = %v", err)
	}
	if _, err := memberCore.StatsGetMegagroupStats(&mtproto.TLStatsGetMegagroupStats{Channel: input}); !errors.Is(err, mtproto.ErrChatAdminRequired) {
		t.Fatalf("megagroup stats ordinary member = %v", err)
	}
}

func TestStatsUsePersistedChannelMemberCount(t *testing.T) {
	channelID := time.Now().UnixNano()
	owner := channelID + 101
	member := channelID + 102
	accessHash := channelID + 103
	if err := domain.SaveChannel(domain.Channel{
		ID: channelID, AccessHash: accessHash, Creator: owner, Title: "stats-member-count", Broadcast: true,
	}); err != nil {
		t.Fatal(err)
	}
	if err := domain.JoinChannel(channelID, member); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = domain.DeleteChannel(owner, channelID) })

	legacyKeys := []string{
		fmt.Sprintf("b1:%d:url", owner),
		fmt.Sprintf("set:%d:premium.applyBoost", owner),
	}
	for _, key := range legacyKeys {
		if err := persist.Default.Set(key, "legacy"); err != nil {
			t.Fatal(err)
		}
		key := key
		t.Cleanup(func() { _, _ = persist.CompareAndDelete(key, "legacy") })
	}

	input := mtproto.MakeTLInputChannel(&mtproto.InputChannel{
		ChannelId: channelID, AccessHash: accessHash,
	}).To_InputChannel()
	c := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: owner}}
	broadcast, err := c.StatsGetBroadcastStats(&mtproto.TLStatsGetBroadcastStats{Channel: input})
	if err != nil || broadcast.GetFollowers().GetCurrent() != 2 {
		t.Fatalf("broadcast followers = %+v, err=%v, want 2 persisted members", broadcast.GetFollowers(), err)
	}
	megagroup, err := c.StatsGetMegagroupStats(&mtproto.TLStatsGetMegagroupStats{Channel: input})
	if err != nil || megagroup.GetMembers().GetCurrent() != 2 {
		t.Fatalf("megagroup members = %+v, err=%v, want 2 persisted members", megagroup.GetMembers(), err)
	}
}

func TestStatsGetMessageStatsUsesPersistedChannelViews(t *testing.T) {
	channelID := time.Now().UnixNano()
	const owner int64 = 98201
	const member int64 = 98202
	accessHash := channelID + 9
	if err := domain.SaveChannel(domain.Channel{
		ID: channelID, AccessHash: accessHash, Creator: owner, Title: "message-stats-test", Broadcast: true,
	}); err != nil {
		t.Fatal(err)
	}
	if err := domain.JoinChannel(channelID, member); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = domain.DeleteChannel(owner, channelID) })
	if _, err := channelview.Post(owner, channelID, "counted", 0); err != nil {
		t.Fatal(err)
	}

	input := mtproto.MakeTLInputChannel(&mtproto.InputChannel{
		ChannelId: channelID, AccessHash: accessHash,
	}).To_InputChannel()
	memberCore := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: member}}
	if _, err := memberCore.ChannelsReadHistory(&mtproto.TLChannelsReadHistory{Channel: input, MaxId: 1}); err != nil {
		t.Fatalf("record member read: %v", err)
	}

	ownerCore := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: owner}}
	stats, err := ownerCore.StatsGetMessageStats(&mtproto.TLStatsGetMessageStats{Channel: input, MsgId: 1})
	if err != nil || stats == nil || stats.GetViewsGraph() == nil || stats.GetViewsGraph().GetJson() == nil || stats.GetViewsGraph().GetJson().GetData() != `{"count":1}` {
		t.Fatalf("message stats: result=%+v err=%v", stats, err)
	}
	if err = stats.Encode(mtproto.NewEncodeBuf(1024), 229); err != nil {
		t.Fatalf("encode message stats: %v", err)
	}

	badHash := mtproto.MakeTLInputChannel(&mtproto.InputChannel{
		ChannelId: channelID, AccessHash: accessHash + 1,
	}).To_InputChannel()
	if _, err = ownerCore.StatsGetMessageStats(&mtproto.TLStatsGetMessageStats{Channel: badHash, MsgId: 1}); !errors.Is(err, mtproto.ErrChannelInvalid) {
		t.Fatalf("bad hash: %v", err)
	}
	if _, err = ownerCore.StatsGetMessageStats(&mtproto.TLStatsGetMessageStats{Channel: input, MsgId: 2}); !errors.Is(err, mtproto.ErrMessageIdInvalid) {
		t.Fatalf("unknown message: %v", err)
	}
	if _, err = memberCore.StatsGetMessageStats(&mtproto.TLStatsGetMessageStats{Channel: input, MsgId: 1}); !errors.Is(err, mtproto.ErrChatAdminRequired) {
		t.Fatalf("ordinary member: %v", err)
	}
}
