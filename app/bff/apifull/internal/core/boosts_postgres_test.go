package core

import (
	"os"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/domain"
)

func requireCoreBoostDB(t *testing.T) {
	t.Helper()
	if os.Getenv("APIFULL_POSTGRES_DSN") == "" {
		t.Skip("APIFULL_POSTGRES_DSN is not configured")
	}
}

func TestBoostMethodsUsePostgresInventory(t *testing.T) {
	requireCoreBoostDB(t)
	uid := time.Now().UnixNano()
	channelID := uid + 10
	storyID := uid + 11
	if err := domain.SaveChannel(domain.Channel{ID: channelID, AccessHash: channelID, Creator: uid, Title: "boost target"}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = domain.DeleteChannel(uid, channelID) })
	c := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: uid}}
	peer := &mtproto.InputPeer{ChannelId: channelID}
	my, err := c.PremiumApplyBoost(&mtproto.TLPremiumApplyBoost{Slots: []int32{1}, Peer: peer})
	if err != nil || my == nil || len(my.GetMyBoosts()) != 1 {
		t.Fatalf("apply = %#v, %v", my, err)
	}
	list, err := c.PremiumGetBoostsList(&mtproto.TLPremiumGetBoostsList{Peer: peer, Limit: 10})
	if err != nil || list == nil || list.GetCount() != 1 || len(list.GetBoosts()) != 1 {
		t.Fatalf("list = %#v, %v", list, err)
	}
	status, err := c.PremiumGetBoostsStatus(&mtproto.TLPremiumGetBoostsStatus{Peer: peer})
	if err != nil || status == nil || status.GetBoosts() != 1 || !status.GetMyBoost() {
		t.Fatalf("status = %#v, %v", status, err)
	}
	if _, err = c.ChannelsSetBoostsToUnblockRestrictions(&mtproto.TLChannelsSetBoostsToUnblockRestrictions{
		Channel: &mtproto.InputChannel{ChannelId: channelID}, Boosts: 1,
	}); err != nil {
		t.Fatalf("set restrictions: %v", err)
	}
	storyPeer := &mtproto.InputPeer{UserId: storyID}
	if result, err := c.StoriesApplyBoost(&mtproto.TLStoriesApplyBoost{Peer: storyPeer}); err != nil || result != mtproto.BoolTrue {
		t.Fatalf("stories apply = %v, %v", result, err)
	}
	my, err = c.PremiumGetMyBoosts(&mtproto.TLPremiumGetMyBoosts{})
	if err != nil || my == nil || len(my.GetMyBoosts()) != 2 {
		t.Fatalf("my boosts = %#v, %v", my, err)
	}
	userBoosts, err := c.PremiumGetUserBoosts(&mtproto.TLPremiumGetUserBoosts{Peer: peer,
		UserId: mtproto.MakeTLInputUserSelf(nil).To_InputUser()})
	if err != nil || userBoosts == nil || userBoosts.GetCount() != 1 {
		t.Fatalf("user boosts = %#v, %v", userBoosts, err)
	}
	storyStatus, err := c.StoriesGetBoostsStatus(&mtproto.TLStoriesGetBoostsStatus{Peer: storyPeer})
	if err != nil || storyStatus == nil || storyStatus.GetBoosts() != 1 || !storyStatus.GetMyBoost() {
		t.Fatalf("story status = %#v, %v", storyStatus, err)
	}
	boosters, err := c.StoriesGetBoostersList(&mtproto.TLStoriesGetBoostersList{Peer: storyPeer, Limit: 10})
	if err != nil || boosters == nil || boosters.GetCount() != 1 || len(boosters.GetBoosters()) != 1 {
		t.Fatalf("boosters = %#v, %v", boosters, err)
	}
	canApply, err := c.StoriesCanApplyBoost(&mtproto.TLStoriesCanApplyBoost{Peer: storyPeer})
	if err != nil || canApply == nil {
		t.Fatalf("can apply = %#v, %v", canApply, err)
	}
}
