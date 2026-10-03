package core

import (
	"errors"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/persist"
)

type reportReactionStore struct {
	writes int
}

func (s *reportReactionStore) Get(string) (string, error) { return "", nil }

func (s *reportReactionStore) Set(string, string) error {
	s.writes++
	return nil
}

func TestReactionWrite(t *testing.T) {
	c := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: 1}}
	if _, err := c.MessagesSendReaction(&mtproto.TLMessagesSendReaction{}); !errors.Is(err, mtproto.ErrPeerIdInvalid) {
		t.Fatalf("empty reaction peer error = %v, want PEER_ID_INVALID", err)
	}
}

func TestDefaultReactionRoundtrip(t *testing.T) {
	c := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: 1}}
	rx := mtproto.MakeTLReactionEmoji(&mtproto.Reaction{Emoticon: "👍"}).To_Reaction()
	if _, err := c.MessagesSetDefaultReaction(&mtproto.TLMessagesSetDefaultReaction{Reaction_REACTION: rx}); err != nil {
		t.Fatal(err)
	}
	got, err := c.MessagesGetTopReactions(&mtproto.TLMessagesGetTopReactions{})
	if err != nil {
		t.Fatal(err)
	}
	list := got.GetReactions()
	if len(list) == 0 || list[0].GetEmoticon() != "👍" {
		t.Fatalf("emoticon: %#v", list)
	}
}

func TestGroupMessageReactionsAreVisibleToOtherMembers(t *testing.T) {
	const (
		aliceID   int64 = 81031
		bobID     int64 = 81032
		groupID   int64 = 81033
		messageID int32 = 81034
	)
	peer := &mtproto.InputPeer{PredicateName: mtproto.Predicate_inputPeerChat, ChatId: groupID}
	keys := []string{
		reactListKey(aliceID),
		reactListKey(bobID),
		sharedReactListKey(aliceID, peer),
	}
	for _, key := range keys {
		if err := persist.Default.Set(key, ""); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		for _, key := range keys {
			_ = persist.Default.Set(key, "")
		}
	})

	alice := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: aliceID}}
	bob := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: bobID}}
	reaction := mtproto.MakeTLReactionEmoji(&mtproto.Reaction{Emoticon: "👍"}).To_Reaction()
	if _, err := bob.MessagesSendReaction(&mtproto.TLMessagesSendReaction{
		Peer:                        peer,
		MsgId:                       messageID,
		Reaction_FLAGVECTORREACTION: []*mtproto.Reaction{reaction},
	}); err != nil {
		t.Fatalf("Bob sends reaction: %v", err)
	}

	list, err := alice.MessagesGetMessageReactionsList(&mtproto.TLMessagesGetMessageReactionsList{
		Peer:  peer,
		Id:    messageID,
		Limit: 20,
	})
	if err != nil {
		t.Fatalf("Alice lists reactions: %v", err)
	}
	if list.GetCount() != 1 || len(list.GetReactions()) != 1 {
		t.Fatalf("Alice reaction list = %#v, want Bob's single reaction", list)
	}
	item := list.GetReactions()[0]
	if item.GetPeerId().GetUserId() != bobID || item.GetReaction() != "👍" || item.GetMy() {
		t.Fatalf("Alice sees reaction = %#v, want Bob's non-local thumbs-up", item)
	}

	updates, err := alice.MessagesGetMessagesReactions(&mtproto.TLMessagesGetMessagesReactions{
		Peer: peer,
		Id:   []int32{messageID},
	})
	if err != nil {
		t.Fatalf("Alice gets reaction update: %v", err)
	}
	if len(updates.GetUpdates()) != 1 || len(updates.GetUpdates()[0].GetReactions_MESSAGEREACTIONS().GetResults()) != 1 {
		t.Fatalf("Alice reaction update = %#v, want one shared reaction", updates)
	}

	if _, err := bob.MessagesSendReaction(&mtproto.TLMessagesSendReaction{
		Peer:  peer,
		MsgId: messageID,
	}); err != nil {
		t.Fatalf("Bob clears reaction: %v", err)
	}
	cleared, err := alice.MessagesGetMessageReactionsList(&mtproto.TLMessagesGetMessageReactionsList{
		Peer:  peer,
		Id:    messageID,
		Limit: 20,
	})
	if err != nil {
		t.Fatalf("Alice lists cleared reactions: %v", err)
	}
	if cleared.GetCount() != 0 || len(cleared.GetReactions()) != 0 {
		t.Fatalf("Alice sees cleared reactions = %#v, want none", cleared)
	}
}

func TestMessagesReportReactionFailsWithoutPersisting(t *testing.T) {
	store := &reportReactionStore{}
	previous := persist.Default
	persist.Use(store)
	t.Cleanup(func() { persist.Use(previous) })

	c := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: 81009}}
	reply, err := c.MessagesReportReaction(&mtproto.TLMessagesReportReaction{})
	if reply != nil || !errors.Is(err, mtproto.ErrMethodNotImpl) {
		t.Fatalf("report reaction: %#v %v, want nil result and METHOD_NOT_IMPL", reply, err)
	}
	if store.writes != 0 {
		t.Fatalf("report reaction wrote %d times, want no persistence", store.writes)
	}
}
