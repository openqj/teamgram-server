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
