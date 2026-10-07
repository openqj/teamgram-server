package core

import (
	"errors"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func TestSavedReactionTagsRoundTripAndHash(t *testing.T) {
	const userID int64 = 981204
	if err := saveSavedTags(userID, nil); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = saveSavedTags(userID, nil) })
	c := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: userID}}
	reaction := mtproto.MakeTLReactionEmoji(&mtproto.Reaction{Emoticon: "👍"}).To_Reaction()
	if ok, err := c.MessagesUpdateSavedReactionTag(&mtproto.TLMessagesUpdateSavedReactionTag{
		Reaction: reaction,
		Title:    wrapperspb.String("work"),
	}); err != nil || !mtproto.FromBool(ok) {
		t.Fatalf("save reaction tag = (%v, %v)", ok, err)
	}

	tags, err := c.MessagesGetSavedReactionTags(nil)
	if err != nil || tags == nil || len(tags.GetTags()) != 1 || tags.GetTags()[0].GetTitle().GetValue() != "work" {
		t.Fatalf("get reaction tags = (%v, %v)", tags, err)
	}
	notModified, err := c.MessagesGetSavedReactionTags(&mtproto.TLMessagesGetSavedReactionTags{Hash: tags.GetHash()})
	if err != nil || notModified.GetPredicateName() != "messages_savedReactionTagsNotModified" {
		t.Fatalf("not-modified tags = (%v, %v)", notModified, err)
	}
	if ok, err := c.MessagesUpdateSavedReactionTag(&mtproto.TLMessagesUpdateSavedReactionTag{Reaction: reaction}); err != nil || !mtproto.FromBool(ok) {
		t.Fatalf("delete reaction tag = (%v, %v)", ok, err)
	}
	cleared, err := c.MessagesGetSavedReactionTags(nil)
	if err != nil || len(cleared.GetTags()) != 0 {
		t.Fatalf("cleared reaction tags = (%v, %v)", cleared, err)
	}
}

func TestSavedReactionTagsValidateAuthenticationAndReaction(t *testing.T) {
	unauthenticated := &ApiFullCore{MD: &metadata.RpcMetadata{}}
	if result, err := unauthenticated.MessagesGetSavedReactionTags(nil); result != nil || !errors.Is(err, mtproto.ErrAuthKeyUnregistered) {
		t.Fatalf("unauthenticated tags = (%v, %v)", result, err)
	}
	c := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: 981205}}
	if result, err := c.MessagesUpdateSavedReactionTag(&mtproto.TLMessagesUpdateSavedReactionTag{}); result != nil || !errors.Is(err, mtproto.ErrReactionInvalid) {
		t.Fatalf("invalid reaction = (%v, %v)", result, err)
	}
}

func TestDefaultTagReactionsUseSavedTags(t *testing.T) {
	const userID int64 = 981206
	if err := saveSavedTags(userID, nil); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = saveSavedTags(userID, nil) })
	c := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: userID}}
	defaults, err := c.MessagesGetDefaultTagReactions(nil)
	if err != nil || len(defaults.GetReactions()) == 0 {
		t.Fatalf("default reactions without tags = (%v, %v), want built-in defaults", defaults, err)
	}
	if _, err = c.MessagesUpdateSavedReactionTag(&mtproto.TLMessagesUpdateSavedReactionTag{
		Reaction: mtproto.MakeTLReactionEmoji(&mtproto.Reaction{Emoticon: "👍"}).To_Reaction(),
		Title:    wrapperspb.String("work"),
	}); err != nil {
		t.Fatal(err)
	}
	defaults, err = c.MessagesGetDefaultTagReactions(nil)
	if err != nil || len(defaults.GetReactions()) != 1 || defaults.GetReactions()[0].GetEmoticon() != "👍" {
		t.Fatalf("default reactions with tags = (%v, %v), want saved tag", defaults, err)
	}
}
