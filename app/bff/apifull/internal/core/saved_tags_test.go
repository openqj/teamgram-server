package core

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/persist"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func TestSavedTagRoundtrip(t *testing.T) {
	rx := mtproto.MakeTLReactionEmoji(&mtproto.Reaction{Emoticon: "👍"}).To_Reaction()
	anon := &ApiFullCore{}
	if _, err := anon.MessagesUpdateSavedReactionTag(&mtproto.TLMessagesUpdateSavedReactionTag{
		Reaction: rx,
		Title:    wrapperspb.String("nice"),
	}); err == nil {
		t.Fatal("update without auth")
	}
	if _, err := anon.MessagesGetSavedReactionTags(&mtproto.TLMessagesGetSavedReactionTags{}); err == nil {
		t.Fatal("get without auth")
	}

	if err := persist.Default.Set("tags:1", ""); err != nil {
		t.Fatal(err)
	}
	c := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: 1}}
	if _, err := c.MessagesUpdateSavedReactionTag(&mtproto.TLMessagesUpdateSavedReactionTag{
		Reaction: rx,
		Title:    wrapperspb.String("nice"),
	}); err != nil {
		t.Fatal(err)
	}
	raw, err := persist.Default.Get("tags:1")
	if err != nil || raw == "" {
		t.Fatalf("stored json: %q %v", raw, err)
	}
	got, err := c.MessagesGetSavedReactionTags(&mtproto.TLMessagesGetSavedReactionTags{})
	if err != nil {
		t.Fatal(err)
	}
	tags := got.GetTags()
	if len(tags) != 1 || tags[0].GetTitle().GetValue() != "nice" || tags[0].GetReaction().GetEmoticon() != "👍" {
		t.Fatalf("tags: %#v", tags)
	}
}
