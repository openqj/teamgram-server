package core

import (
	"reflect"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/persist"
)

func sameRPCErrorCode(got, want error) bool {
	return got != nil && want != nil && got.Error() == want.Error()
}

func nilRPCResult(v any) bool {
	if v == nil {
		return true
	}
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Ptr, reflect.Slice:
		return rv.IsNil()
	default:
		return false
	}
}

func TestStoriesMethodsRequireAuthentication(t *testing.T) {
	c := &ApiFullCore{MD: &metadata.RpcMetadata{}}
	cases := []struct {
		name string
		call func() (any, error)
	}{
		{"send", func() (any, error) { return c.StoriesSendStory(nil) }},
		{"get all", func() (any, error) { return c.StoriesGetAllStories(nil) }},
		{"start live", func() (any, error) { return c.StoriesStartLive(nil) }},
		{"create album", func() (any, error) { return c.StoriesCreateAlbum(nil) }},
		{"report", func() (any, error) { return c.StoriesReport19D8EB45(nil) }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := tc.call()
			if !nilRPCResult(result) || !sameRPCErrorCode(err, mtproto.ErrAuthKeyUnregistered) {
				t.Fatalf("result=%v err=%v, want nil result and AUTH_KEY_UNREGISTERED", result, err)
			}
		})
	}
}

func TestStoryPageByIDUsesExclusiveSparseCursor(t *testing.T) {
	items := []*mtproto.StoryItem{
		mtproto.MakeTLStoryItem(&mtproto.StoryItem{Id: 1}).To_StoryItem(),
		mtproto.MakeTLStoryItem(&mtproto.StoryItem{Id: 4}).To_StoryItem(),
		mtproto.MakeTLStoryItem(&mtproto.StoryItem{Id: 9}).To_StoryItem(),
	}
	page := storyPageByID(items, 4, 100)
	if len(page) != 1 || page[0].GetId() != 9 {
		t.Fatalf("storyPageByID(offset=4) = %v, want [9]", page)
	}
	page = storyPageByID(items, 0, 2)
	if len(page) != 2 || page[0].GetId() != 1 || page[1].GetId() != 4 {
		t.Fatalf("storyPageByID(offset=0) = %v, want [1 4]", page)
	}
}

func TestStoryMaxIDUsesPersistedMaximum(t *testing.T) {
	stories := &userStoryStore{
		Order: []int32{9, 1},
		Items: map[int32]*mtproto.StoryItem{
			1: mtproto.MakeTLStoryItem(&mtproto.StoryItem{Id: 1}).To_StoryItem(),
			9: mtproto.MakeTLStoryItem(&mtproto.StoryItem{Id: 9}).To_StoryItem(),
		},
	}
	if got := storyMaxID(stories); got != 9 {
		t.Fatalf("storyMaxID() = %d, want 9", got)
	}
}

func TestStoriesReadPeerUserStoriesUsesTargetStore(t *testing.T) {
	const callerID int64 = 229906
	const targetID int64 = 777000
	callerKey := storyKey(callerID)
	targetKey := storyKey(targetID)
	t.Cleanup(func() {
		_ = persist.Default.Set(callerKey, "")
		_ = persist.Default.Set(targetKey, "")
	})
	if err := saveUserStories(targetID, &userStoryStore{
		Order: []int32{8},
		Items: map[int32]*mtproto.StoryItem{8: mtproto.MakeTLStoryItem(&mtproto.StoryItem{Id: 8}).To_StoryItem()},
	}); err != nil {
		t.Fatal(err)
	}
	c := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: callerID}}
	peer := mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: targetID, AccessHash: 1}).To_InputPeer()
	got, err := c.StoriesGetPeerStories(&mtproto.TLStoriesGetPeerStories{Peer: peer})
	if err != nil || got == nil || got.GetStories() == nil || got.GetStories().GetPeer().GetUserId() != targetID || len(got.GetStories().GetStories()) != 1 || got.GetStories().GetStories()[0].GetId() != 8 {
		t.Fatalf("peer stories=(%#v,%v)", got, err)
	}
	maxIDs, err := c.StoriesGetPeerMaxIDs78499170(&mtproto.TLStoriesGetPeerMaxIDs78499170{Id: []*mtproto.InputPeer{peer}})
	if err != nil || maxIDs == nil || len(maxIDs.GetDatas()) != 1 || maxIDs.GetDatas()[0].GetMaxId().GetValue() != 8 {
		t.Fatalf("peer max ids=(%#v,%v)", maxIDs, err)
	}
}

func TestStoriesReadPeerMaxIDsReturnsEmptyRecentStoryForUnknownUser(t *testing.T) {
	const callerID int64 = 229907
	const targetID int64 = 777001
	t.Cleanup(func() { _ = persist.Default.Set(storyKey(targetID), "") })
	c := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: callerID}}
	peer := mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: targetID, AccessHash: 1}).To_InputPeer()
	maxIDs, err := c.StoriesGetPeerMaxIDs78499170(&mtproto.TLStoriesGetPeerMaxIDs78499170{Id: []*mtproto.InputPeer{peer}})
	if err != nil || maxIDs == nil || len(maxIDs.GetDatas()) != 1 || maxIDs.GetDatas()[0].GetMaxId() != nil {
		t.Fatalf("unknown peer max ids=(%#v,%v)", maxIDs, err)
	}
}

func TestStoriesSendReactionRequiresReaction(t *testing.T) {
	uid := int64(229903)
	t.Cleanup(func() { _ = persist.Default.Set(storyKey(uid), "") })
	item := mtproto.MakeTLStoryItem(&mtproto.StoryItem{Id: 1}).To_StoryItem()
	if err := saveUserStories(uid, &userStoryStore{Order: []int32{1}, Items: map[int32]*mtproto.StoryItem{1: item}}); err != nil {
		t.Fatal(err)
	}
	c := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: uid}}
	if got, err := c.StoriesSendReaction(&mtproto.TLStoriesSendReaction{Peer: mtproto.MakeTLInputPeerSelf(nil).To_InputPeer(), StoryId: 1}); got != nil || !sameRPCErrorCode(err, mtproto.ErrReactionEmpty) {
		t.Fatalf("sendReaction(nil) = %v, %v; want nil result and REACTION_EMPTY", got, err)
	}
	peer := mtproto.MakeTLInputPeerSelf(nil).To_InputPeer()
	if _, err := c.StoriesSendReaction(&mtproto.TLStoriesSendReaction{
		Peer: peer, StoryId: 1,
		Reaction: mtproto.MakeTLReactionEmoji(&mtproto.Reaction{Emoticon: "👍"}).To_Reaction(),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := c.StoriesSendReaction(&mtproto.TLStoriesSendReaction{
		Peer: peer, StoryId: 1,
		Reaction: mtproto.MakeTLReactionEmpty(nil).To_Reaction(),
	}); err != nil {
		t.Fatal(err)
	}
	list, err := c.StoriesGetStoryReactionsList(&mtproto.TLStoriesGetStoryReactionsList{Peer: peer, Id: 1})
	if err != nil || list == nil || list.GetCount() != 0 {
		t.Fatalf("reactionEmpty should remove reaction: list=%#v err=%v", list, err)
	}
}

func TestStoriesReactionListFiltersAndPaginates(t *testing.T) {
	uid := int64(229904)
	t.Cleanup(func() { _ = persist.Default.Set(storyKey(uid), "") })
	item := mtproto.MakeTLStoryItem(&mtproto.StoryItem{Id: 1}).To_StoryItem()
	if err := saveUserStories(uid, &userStoryStore{
		Order:     []int32{1},
		Items:     map[int32]*mtproto.StoryItem{1: item},
		Reactions: map[int32][]storyReactionRec{1: {{UserId: 11, Emoticon: "👍"}, {UserId: 12, Emoticon: "❤️"}}},
	}); err != nil {
		t.Fatal(err)
	}
	c := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: uid}}
	list, err := c.StoriesGetStoryReactionsList(&mtproto.TLStoriesGetStoryReactionsList{
		Peer:     mtproto.MakeTLInputPeerSelf(nil).To_InputPeer(),
		Id:       1,
		Reaction: mtproto.MakeTLReactionEmoji(&mtproto.Reaction{Emoticon: "👍"}).To_Reaction(),
		Limit:    1,
	})
	if err != nil || list == nil || list.GetCount() != 1 || len(list.GetReactions()) != 1 || list.GetNextOffset() != nil {
		t.Fatalf("filtered reaction list = %#v, %v", list, err)
	}
	list, err = c.StoriesGetStoryReactionsList(&mtproto.TLStoriesGetStoryReactionsList{
		Peer: mtproto.MakeTLInputPeerSelf(nil).To_InputPeer(), Id: 1, Limit: 1,
	})
	if err != nil || list == nil || list.GetCount() != 2 || len(list.GetReactions()) != 1 || list.GetNextOffset().GetValue() != "1" {
		t.Fatalf("paginated reaction list = %#v, %v", list, err)
	}
}

func TestStoriesArchiveOmitsPinnedToTop(t *testing.T) {
	uid := int64(229902)
	t.Cleanup(func() { _ = persist.Default.Set(storyKey(uid), "") })
	item := mtproto.MakeTLStoryItem(&mtproto.StoryItem{Id: 4, Pinned: true}).To_StoryItem()
	if err := saveUserStories(uid, &userStoryStore{Order: []int32{4}, Items: map[int32]*mtproto.StoryItem{4: item}}); err != nil {
		t.Fatal(err)
	}
	c := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: uid}}
	archive, err := c.StoriesGetStoriesArchive(&mtproto.TLStoriesGetStoriesArchive{
		Peer: mtproto.MakeTLInputPeerSelf(nil).To_InputPeer(), Limit: 100,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(archive.GetPinnedToTop()) != 0 {
		t.Fatalf("archive pinned_to_top = %v, want empty", archive.GetPinnedToTop())
	}
}

func TestStoriesMethodsFailClosedWithoutProvider(t *testing.T) {
	uid := int64(229901)
	c := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: uid}}
	t.Cleanup(func() { _ = persist.Default.Set(storyKey(uid), "") })
	if err := persist.Default.Set(storyKey(uid), "sentinel"); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name string
		call func() (any, error)
		want error
	}{
		{"can send count", func() (any, error) { return c.StoriesCanSendStory30EB63F0(nil) }, mtproto.ErrMethodNotImpl},
		{"send", func() (any, error) { return c.StoriesSendStory(nil) }, mtproto.ErrMethodNotImpl},
		{"edit", func() (any, error) { return c.StoriesEditStory(nil) }, mtproto.ErrMethodNotImpl},
		{"delete", func() (any, error) { return c.StoriesDeleteStories(nil) }, mtproto.ErrMethodNotImpl},
		{"get all", func() (any, error) { return c.StoriesGetAllStories(nil) }, mtproto.ErrMethodNotImpl},
		{"get by id", func() (any, error) { return c.StoriesGetStoriesByID(nil) }, mtproto.ErrInputRequestInvalid},
		{"increment views", func() (any, error) { return c.StoriesIncrementStoryViews(nil) }, mtproto.ErrStoryIdEmpty},
		{"export link", func() (any, error) { return c.StoriesExportStoryLink(nil) }, mtproto.ErrMethodNotImpl},
		{"send reaction", func() (any, error) { return c.StoriesSendReaction(nil) }, mtproto.ErrMethodNotImpl},
		{"peer stories", func() (any, error) { return c.StoriesGetPeerStories(nil) }, mtproto.ErrMethodNotImpl},
		{"peer max ids", func() (any, error) { return c.StoriesGetPeerMaxIDs78499170(nil) }, mtproto.ErrMethodNotImpl},
		{"toggle hidden", func() (any, error) { return c.StoriesTogglePeerStoriesHidden(nil) }, mtproto.ErrInputRequestInvalid},
		{"search posts", func() (any, error) { return c.StoriesSearchPosts(nil) }, mtproto.ErrMethodNotImpl},
		{"create album", func() (any, error) { return c.StoriesCreateAlbum(nil) }, mtproto.ErrInputRequestInvalid},
		{"get albums", func() (any, error) { return c.StoriesGetAlbums(nil) }, mtproto.ErrInputRequestInvalid},
		{"start live", func() (any, error) { return c.StoriesStartLive(nil) }, mtproto.ErrMethodNotImpl},
		{"can send bool", func() (any, error) { return c.StoriesCanSendStoryC7DFDFDD(nil) }, mtproto.ErrMethodNotImpl},
		{"user max ids", func() (any, error) { return c.UsersGetStoriesMaxIDs(nil) }, mtproto.ErrMethodNotImpl},
		{"contact hidden", func() (any, error) { return c.ContactsToggleStoriesHidden(nil) }, mtproto.ErrMethodNotImpl},
		{"get user stories", func() (any, error) { return c.StoriesGetUserStories(nil) }, mtproto.ErrMethodNotImpl},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := tc.call()
			if !nilRPCResult(result) || !sameRPCErrorCode(err, tc.want) {
				t.Fatalf("result=%v err=%v, want nil result and %v", result, err, tc.want)
			}
		})
	}
	raw, err := persist.Default.Get(storyKey(uid))
	if err != nil {
		t.Fatal(err)
	}
	if raw != "sentinel" {
		t.Fatalf("unsupported story methods mutated local state: %q", raw)
	}
}

func TestStoriesStartLiveValidRequestFailsClosedWithoutProvider(t *testing.T) {
	const uid int64 = 229905
	key := storyKey(uid)
	old := persist.Default
	if err := saveUserStories(uid, &userStoryStore{Order: []int32{}, Items: map[int32]*mtproto.StoryItem{}}); err != nil {
		t.Fatal(err)
	}
	before, err := persist.Default.Get(key)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		persist.Use(old)
		_ = persist.Default.Set(key, "")
	})

	c := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: uid}}
	got, err := c.StoriesStartLive(&mtproto.TLStoriesStartLive{
		Peer: mtproto.MakeTLInputPeerSelf(nil).To_InputPeer(),
	})
	if got != nil || !sameRPCErrorCode(err, mtproto.ErrMethodNotImpl) {
		t.Fatalf("valid stories.startLive = (%v, %v), want nil result and METHOD_NOT_IMPL", got, err)
	}
	if value, readErr := persist.Default.Get(key); readErr != nil || value != before {
		t.Fatalf("stories.startLive changed local state: before=%q after=%q err=%v", before, value, readErr)
	}
}
