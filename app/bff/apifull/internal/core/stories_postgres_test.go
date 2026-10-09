package core

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/persist"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func TestStoriesPostgresSelfCrud(t *testing.T) {
	uid := int64(99180229)
	key := storyKey(uid)
	t.Cleanup(func() {
		_ = persist.Default.Set(key, `{"next":0,"items":{},"order":[]}`)
	})
	item := mtproto.MakeTLStoryItem(&mtproto.StoryItem{
		Id:      1,
		Caption: wrapperspb.String("postgres story"),
		Media:   mtproto.MakeTLMessageMediaEmpty(nil).To_MessageMedia(),
		Date:    1,
		Out:     true,
	}).To_StoryItem()
	if err := saveUserStories(uid, &userStoryStore{
		Next:  1,
		Order: []int32{1},
		Items: map[int32]*mtproto.StoryItem{1: item},
	}); err != nil {
		t.Fatal(err)
	}
	c := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: uid}}
	self := mtproto.MakeTLInputPeerSelf(nil).To_InputPeer()
	archive, err := c.StoriesGetStoriesArchive(&mtproto.TLStoriesGetStoriesArchive{Peer: self, Limit: 100})
	if err != nil || archive == nil || len(archive.GetStories()) != 1 || archive.GetStories()[0].GetCaption().GetValue() != "postgres story" {
		t.Fatalf("archive = %v, %v", archive, err)
	}
	id := archive.GetStories()[0].GetId()
	if _, err = c.StoriesIncrementStoryViews(&mtproto.TLStoriesIncrementStoryViews{Peer: self, Id: []int32{id}}); err != nil {
		t.Fatal(err)
	}
	if _, err = c.StoriesSendReaction(&mtproto.TLStoriesSendReaction{Peer: self, StoryId: id, Reaction: mtproto.MakeTLReactionEmoji(&mtproto.Reaction{Emoticon: "👍"}).To_Reaction()}); err != nil {
		t.Fatal(err)
	}
	if _, err = c.StoriesDeleteStories(&mtproto.TLStoriesDeleteStories{Peer: self, Id: []int32{id}}); err != nil {
		t.Fatal(err)
	}
	archive, err = c.StoriesGetStoriesArchive(&mtproto.TLStoriesGetStoriesArchive{Peer: self, Limit: 100})
	if err != nil || archive == nil || len(archive.GetStories()) != 0 {
		t.Fatalf("archive after delete = %v, %v", archive, err)
	}
}

func TestStoriesPostgresSelfOwnedMethods(t *testing.T) {
	uid := int64(99180230)
	key := storyKey(uid)
	t.Cleanup(func() { _ = persist.Default.Set(key, `{"next":0,"items":{},"order":[]}`) })
	item := mtproto.MakeTLStoryItem(&mtproto.StoryItem{
		Id: 7, Caption: wrapperspb.String("postgres searchable story"),
		Media: mtproto.MakeTLMessageMediaEmpty(nil).To_MessageMedia(), Date: 1, Out: true,
	}).To_StoryItem()
	if err := saveUserStories(uid, &userStoryStore{Next: 7, Order: []int32{7}, Items: map[int32]*mtproto.StoryItem{7: item}}); err != nil {
		t.Fatal(err)
	}
	c := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: uid}}
	self := mtproto.MakeTLInputPeerSelf(nil).To_InputPeer()
	if got, err := c.StoriesCanSendStory30EB63F0(&mtproto.TLStoriesCanSendStory30EB63F0{Peer: self}); err != nil || got == nil || got.GetCountRemains() != 99 {
		t.Fatalf("can send count = %#v, %v", got, err)
	}
	if got, err := c.StoriesEditStory(&mtproto.TLStoriesEditStory{Peer: self, Id: 7, Caption: wrapperspb.String("edited searchable story")}); err != nil || got == nil {
		t.Fatalf("edit story = %#v, %v", got, err)
	}
	if got, err := c.StoriesExportStoryLink(&mtproto.TLStoriesExportStoryLink{Peer: self, Id: 7}); err != nil || got == nil || got.GetLink() == "" {
		t.Fatalf("export story link = %#v, %v", got, err)
	}
	if got, err := c.StoriesActivateStealthMode(&mtproto.TLStoriesActivateStealthMode{Past: true, Future: true}); err != nil || got == nil {
		t.Fatalf("activate stealth = %#v, %v", got, err)
	}
	if got, err := c.StoriesSendReaction(&mtproto.TLStoriesSendReaction{Peer: self, StoryId: 7, Reaction: mtproto.MakeTLReactionEmoji(&mtproto.Reaction{Emoticon: "👍"}).To_Reaction()}); err != nil || got == nil {
		t.Fatalf("send reaction = %#v, %v", got, err)
	}
	stats, err := c.StatsGetStoryStats(&mtproto.TLStatsGetStoryStats{Peer: self, Id: 7})
	if err != nil || stats == nil || stats.GetViewsGraph() == nil || stats.GetViewsGraph().GetJson() == nil || stats.GetViewsGraph().GetJson().GetData() != `{"count":0}` || stats.GetReactionsByEmotionGraph() == nil || stats.GetReactionsByEmotionGraph().GetJson().GetData() != `{"count":1}` {
		t.Fatalf("story stats = %#v, %v", stats, err)
	}
	if got, err := c.StoriesGetPeerStories(&mtproto.TLStoriesGetPeerStories{Peer: self}); err != nil || got == nil || got.GetStories() == nil || len(got.GetStories().GetStories()) != 1 {
		t.Fatalf("peer stories = %#v, %v", got, err)
	}
	if got, err := c.StoriesGetPeerMaxIDs78499170(&mtproto.TLStoriesGetPeerMaxIDs78499170{Id: []*mtproto.InputPeer{self}}); err != nil || got == nil || len(got.GetDatas()) != 1 || got.GetDatas()[0].GetMaxId().GetValue() != 7 {
		t.Fatalf("peer max ids = %#v, %v", got, err)
	}
	if got, err := c.StoriesSearchPosts(&mtproto.TLStoriesSearchPosts{Peer: self, Hashtag: wrapperspb.String("searchable"), Limit: 10}); err != nil || got == nil || got.GetCount() != 1 || len(got.GetStories()) != 1 {
		t.Fatalf("search posts = %#v, %v", got, err)
	}
	if got, err := c.StoriesGetAllStories(&mtproto.TLStoriesGetAllStories{}); err != nil || got == nil || got.GetCount() != 1 {
		t.Fatalf("all stories = %#v, %v", got, err)
	}
}
