package core

import (
	"errors"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/domain"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/persist"
)

// Public-forward analytics needs a reverse index of forwarded messages and
// stories. APIFull does not own that authority, so these methods must not turn
// a validated source into a fabricated empty success response.
func TestStatsPublicForwardsFailClosedWithoutReverseIndex(t *testing.T) {
	const (
		owner   int64 = 99180241
		storyID int32 = 7
	)
	channelID := time.Now().UnixNano()
	accessHash := channelID + 11
	if err := domain.SaveChannel(domain.Channel{
		ID: channelID, AccessHash: accessHash, Creator: owner,
		Title: "public-forwards-provider-gap", Broadcast: true,
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = domain.DeleteChannel(owner, channelID) })
	row, err := domain.InsertChannelMessage(channelID, owner, 0, "source")
	if err != nil {
		t.Fatal(err)
	}

	input := mtproto.MakeTLInputChannel(&mtproto.InputChannel{
		ChannelId: channelID, AccessHash: accessHash,
	}).To_InputChannel()
	c := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: owner}}
	message, err := c.StatsGetMessagePublicForwards5F150144(&mtproto.TLStatsGetMessagePublicForwards5F150144{
		Channel: input, MsgId: row.MessageID, Limit: 10,
	})
	if message != nil || !errors.Is(err, mtproto.ErrMethodNotImpl) {
		t.Fatalf("stats.getMessagePublicForwards = (%v, %v), want nil and METHOD_NOT_IMPL", message, err)
	}
	legacy, err := c.StatsGetMessagePublicForwards5630281B(&mtproto.TLStatsGetMessagePublicForwards5630281B{
		Channel: input, MsgId: row.MessageID, Limit: 10,
	})
	if legacy != nil || !errors.Is(err, mtproto.ErrMethodNotImpl) {
		t.Fatalf("legacy stats.getMessagePublicForwards = (%v, %v), want nil and METHOD_NOT_IMPL", legacy, err)
	}

	key := storyKey(owner)
	t.Cleanup(func() { _ = persist.Default.Set(key, `{"next":0,"items":{},"order":[]}`) })
	item := mtproto.MakeTLStoryItem(&mtproto.StoryItem{
		Id: storyID, Media: mtproto.MakeTLMessageMediaEmpty(nil).To_MessageMedia(),
		Date: 1, Out: true,
	}).To_StoryItem()
	if err := saveUserStories(owner, &userStoryStore{
		Next: storyID, Order: []int32{storyID}, Items: map[int32]*mtproto.StoryItem{storyID: item},
	}); err != nil {
		t.Fatal(err)
	}
	story, err := c.StatsGetStoryPublicForwards(&mtproto.TLStatsGetStoryPublicForwards{
		Peer: mtproto.MakeTLInputPeerSelf(nil).To_InputPeer(), Id: storyID, Limit: 10,
	})
	if story != nil || !errors.Is(err, mtproto.ErrMethodNotImpl) {
		t.Fatalf("stats.getStoryPublicForwards = (%v, %v), want nil and METHOD_NOT_IMPL", story, err)
	}
}

func TestStatsPublicForwardsValidateBeforeProvider(t *testing.T) {
	c := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: 99180242}}
	channel := mtproto.MakeTLInputChannel(&mtproto.InputChannel{ChannelId: 42, AccessHash: 43}).To_InputChannel()
	if got, err := c.StatsGetMessagePublicForwards5F150144(&mtproto.TLStatsGetMessagePublicForwards5F150144{
		Channel: channel, MsgId: 1, Limit: 0,
	}); got != nil || !errors.Is(err, mtproto.ErrLimitInvalid) {
		t.Fatalf("message limit validation = (%v, %v), want LIMIT_INVALID", got, err)
	}
	if got, err := c.StatsGetMessagePublicForwards5630281B(&mtproto.TLStatsGetMessagePublicForwards5630281B{
		Channel: channel, MsgId: 1, Limit: 0,
	}); got != nil || !errors.Is(err, mtproto.ErrLimitInvalid) {
		t.Fatalf("legacy message limit validation = (%v, %v), want LIMIT_INVALID", got, err)
	}
	if got, err := c.StatsGetStoryPublicForwards(&mtproto.TLStatsGetStoryPublicForwards{
		Peer: mtproto.MakeTLInputPeerSelf(nil).To_InputPeer(), Id: 1, Limit: 0,
	}); got != nil || !errors.Is(err, mtproto.ErrLimitInvalid) {
		t.Fatalf("story limit validation = (%v, %v), want LIMIT_INVALID", got, err)
	}
	if got, err := c.StatsGetStoryPublicForwards(&mtproto.TLStatsGetStoryPublicForwards{
		Peer: mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: 99180243}).To_InputPeer(), Id: 1, Limit: 10,
	}); got != nil || !errors.Is(err, mtproto.ErrChatAdminRequired) {
		t.Fatalf("cross-user story validation = (%v, %v), want CHAT_ADMIN_REQUIRED", got, err)
	}
}

func TestStatsPublicForwardsAuthenticateBeforeValidation(t *testing.T) {
	c := &ApiFullCore{}
	if got, err := c.StatsGetMessagePublicForwards5F150144(nil); got != nil || !errors.Is(err, mtproto.ErrAuthKeyUnregistered) {
		t.Fatalf("message forwards auth = (%v, %v), want AUTH_KEY_UNREGISTERED", got, err)
	}
	if got, err := c.StatsGetMessagePublicForwards5630281B(nil); got != nil || !errors.Is(err, mtproto.ErrAuthKeyUnregistered) {
		t.Fatalf("legacy message forwards auth = (%v, %v), want AUTH_KEY_UNREGISTERED", got, err)
	}
	if got, err := c.StatsGetStoryPublicForwards(nil); got != nil || !errors.Is(err, mtproto.ErrAuthKeyUnregistered) {
		t.Fatalf("story forwards auth = (%v, %v), want AUTH_KEY_UNREGISTERED", got, err)
	}
}
