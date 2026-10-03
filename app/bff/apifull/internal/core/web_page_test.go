package core

import (
	"errors"
	"sync"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/persist"
)

type webPageTestStore struct {
	mu sync.Mutex
	m  map[string]string
}

func (s *webPageTestStore) Get(key string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.m[key], nil
}

func (s *webPageTestStore) Set(key, value string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.m == nil {
		s.m = make(map[string]string)
	}
	s.m[key] = value
	return nil
}

func TestWebPageURLNormalization(t *testing.T) {
	canonical, parsed, err := normalizeWebPageURL(" HTTPS://Example.COM:443/a/../b#fragment ")
	if err != nil {
		t.Fatal(err)
	}
	if canonical != "https://example.com/b" {
		t.Fatalf("canonical URL = %q, want https://example.com/b", canonical)
	}
	if parsed.Hostname() != "example.com" || parsed.Path != "/b" {
		t.Fatalf("parsed URL = %#v", parsed)
	}
	for _, raw := range []string{
		"",
		"mailto:user@example.com",
		"https://user:password@example.com/secret",
		"https://example.com:99999/private",
		"http://127.0.0.1/private",
		"https://example.local/page",
	} {
		if _, _, err := normalizeWebPageURL(raw); !errors.Is(err, mtproto.ErrUrlInvalid) {
			t.Errorf("normalizeWebPageURL(%q) error = %v, want URL_INVALID", raw, err)
		}
	}
}

func TestWebPagePreviewPersistsAndReadsCanonicalRecord(t *testing.T) {
	oldStore := persist.Default
	persist.Use(&webPageTestStore{})
	t.Cleanup(func() { persist.Use(oldStore) })

	urlText := "https://EXAMPLE.com:443/a/../b#fragment"
	alice := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: 101}}
	preview, err := alice.MessagesGetWebPagePreview570D6F6F(&mtproto.TLMessagesGetWebPagePreview570D6F6F{
		Message: "see " + urlText + ".",
	})
	if err != nil {
		t.Fatal(err)
	}
	if preview == nil || preview.GetMedia() == nil || preview.GetMedia().GetWebpage() == nil {
		t.Fatalf("preview = %v, want webpage media", preview)
	}
	page := preview.GetMedia().GetWebpage()
	if page.GetUrl_STRING() != "https://example.com/b" || page.GetDisplayUrl() != "example.com/b" || page.GetSiteName().GetValue() != "example.com" {
		t.Fatalf("page metadata = %v", page)
	}
	if page.GetId() == 0 || page.GetHash() == 0 {
		t.Fatalf("page identity = id %d hash %d, want stable non-zero values", page.GetId(), page.GetHash())
	}

	bob := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: 202}}
	readback, err := bob.MessagesGetWebPage32CA8F91(&mtproto.TLMessagesGetWebPage32CA8F91{Url: "https://example.com/b"})
	if err != nil {
		t.Fatal(err)
	}
	if readback.GetUrl_STRING() != page.GetUrl_STRING() || readback.GetId() != page.GetId() || readback.GetHash() != page.GetHash() {
		t.Fatalf("readback = %v, initial = %v", readback, page)
	}

	notModified, err := bob.MessagesGetWebPage8D9692A3(&mtproto.TLMessagesGetWebPage8D9692A3{
		Url:  "HTTPS://example.com:443/b#ignored",
		Hash: page.GetHash(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if notModified.GetWebpage().GetPredicateName() != mtproto.Predicate_webPageNotModified {
		t.Fatalf("hash response predicate = %q, want %q", notModified.GetWebpage().GetPredicateName(), mtproto.Predicate_webPageNotModified)
	}

	raw, err := persist.Default.Get(webPageKey("https://example.com/b"))
	if err != nil || raw == "" {
		t.Fatalf("persisted record = %q, err = %v", raw, err)
	}
}

func TestWebPagePreviewEntityAndEmptyBranches(t *testing.T) {
	oldStore := persist.Default
	persist.Use(&webPageTestStore{})
	t.Cleanup(func() { persist.Use(oldStore) })
	c := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: 303}}

	entityPreview, err := c.MessagesGetWebPagePreview8B68B0CC(&mtproto.TLMessagesGetWebPagePreview8B68B0CC{
		Message:  "Teamgram",
		Entities: []*mtproto.MessageEntity{{PredicateName: mtproto.Predicate_messageEntityTextUrl, Url: "https://example.org/docs"}},
	})
	if err != nil || entityPreview.GetWebpage() == nil || entityPreview.GetWebpage().GetUrl_STRING() != "https://example.org/docs" {
		t.Fatalf("entity preview = (%v, %v)", entityPreview, err)
	}

	empty, err := c.MessagesGetWebPagePreview570D6F6F(&mtproto.TLMessagesGetWebPagePreview570D6F6F{Message: "no link here"})
	if err != nil || empty.GetMedia().GetPredicateName() != mtproto.Predicate_messageMediaEmpty {
		t.Fatalf("empty preview = (%v, %v)", empty, err)
	}

	if _, err := c.MessagesGetWebPage32CA8F91(&mtproto.TLMessagesGetWebPage32CA8F91{Url: "not a URL"}); !errors.Is(err, mtproto.ErrUrlInvalid) {
		t.Fatalf("invalid direct URL error = %v, want URL_INVALID", err)
	}
}
