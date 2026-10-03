package core

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/persist"
	"google.golang.org/grpc/status"
)

func sameRPCErrorCode(got, want error) bool {
	return got != nil && want != nil && status.Code(got) == status.Code(want) && status.Convert(got).Message() == status.Convert(want).Message()
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
			if result != nil || !sameRPCErrorCode(err, mtproto.ErrAuthKeyUnregistered) {
				t.Fatalf("result=%v err=%v, want nil result and AUTH_KEY_UNREGISTERED", result, err)
			}
		})
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
	}{
		{"can send count", func() (any, error) { return c.StoriesCanSendStory30EB63F0(nil) }},
		{"send", func() (any, error) { return c.StoriesSendStory(nil) }},
		{"edit", func() (any, error) { return c.StoriesEditStory(nil) }},
		{"delete", func() (any, error) { return c.StoriesDeleteStories(nil) }},
		{"get all", func() (any, error) { return c.StoriesGetAllStories(nil) }},
		{"get by id", func() (any, error) { return c.StoriesGetStoriesByID(nil) }},
		{"increment views", func() (any, error) { return c.StoriesIncrementStoryViews(nil) }},
		{"export link", func() (any, error) { return c.StoriesExportStoryLink(nil) }},
		{"send reaction", func() (any, error) { return c.StoriesSendReaction(nil) }},
		{"peer stories", func() (any, error) { return c.StoriesGetPeerStories(nil) }},
		{"peer max ids", func() (any, error) { return c.StoriesGetPeerMaxIDs78499170(nil) }},
		{"toggle hidden", func() (any, error) { return c.StoriesTogglePeerStoriesHidden(nil) }},
		{"search posts", func() (any, error) { return c.StoriesSearchPosts(nil) }},
		{"create album", func() (any, error) { return c.StoriesCreateAlbum(nil) }},
		{"get albums", func() (any, error) { return c.StoriesGetAlbums(nil) }},
		{"start live", func() (any, error) { return c.StoriesStartLive(nil) }},
		{"can send bool", func() (any, error) { return c.StoriesCanSendStoryC7DFDFDD(nil) }},
		{"user max ids", func() (any, error) { return c.UsersGetStoriesMaxIDs(nil) }},
		{"contact hidden", func() (any, error) { return c.ContactsToggleStoriesHidden(nil) }},
		{"get user stories", func() (any, error) { return c.StoriesGetUserStories(nil) }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := tc.call()
			if result != nil || !sameRPCErrorCode(err, mtproto.ErrMethodNotImpl) {
				t.Fatalf("result=%v err=%v, want nil result and METHOD_NOT_IMPL", result, err)
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
