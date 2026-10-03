package core

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/domain"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/persist"
)

func TestChannelReactionsRequireMembershipAndKeepSharedState(t *testing.T) {
	seed := time.Now().UnixNano()
	channelID := seed
	aliceID := int64(1_700_000_000 + seed%100_000_000)
	bobID := aliceID + 1
	carolID := aliceID + 2
	const messageID int32 = 1

	if err := domain.SaveChannel(domain.Channel{
		ID:         channelID,
		AccessHash: channelID,
		Creator:    aliceID,
		Title:      "reaction-authorization-test",
		Megagroup:  true,
	}); err != nil {
		t.Fatal(err)
	}
	if err := domain.JoinChannel(channelID, bobID); err != nil {
		t.Fatal(err)
	}
	peer := mtproto.MakeTLInputPeerChannel(&mtproto.InputPeer{
		ChannelId:  channelID,
		AccessHash: channelID,
	}).To_InputPeer()
	keys := []string{
		sharedReactListKey(aliceID, peer),
		reactListKey(aliceID),
		reactListKey(bobID),
		reactListKey(carolID),
		reactKey(aliceID, "send"),
		reactKey(bobID, "send"),
		reactKey(carolID, "send"),
		reactKey(carolID, "delAll"),
		reactKey(carolID, "delOne"),
		fmt.Sprintf("react:%d", aliceID),
		fmt.Sprintf("react:%d", bobID),
	}
	clear := func() {
		for _, key := range keys {
			_ = persist.Default.Set(key, "")
		}
	}
	clear()
	t.Cleanup(func() {
		clear()
		if err := domain.DeleteChannel(aliceID, channelID); err != nil {
			t.Errorf("delete test channel: %v", err)
		}
	})

	alice := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: aliceID}}
	bob := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: bobID}}
	carol := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: carolID}}
	reaction := func(emoticon string) *mtproto.Reaction {
		return mtproto.MakeTLReactionEmoji(&mtproto.Reaction{Emoticon: emoticon}).To_Reaction()
	}
	send := func(c *ApiFullCore, emoticon string) error {
		_, err := c.MessagesSendReaction(&mtproto.TLMessagesSendReaction{
			Peer:                        peer,
			MsgId:                       messageID,
			Reaction_FLAGVECTORREACTION: []*mtproto.Reaction{reaction(emoticon)},
		})
		return err
	}
	counts := func() map[string]int32 {
		updates, err := alice.MessagesGetMessagesReactions(&mtproto.TLMessagesGetMessagesReactions{
			Peer: peer,
			Id:   []int32{messageID},
		})
		if err != nil {
			t.Fatalf("read shared reactions: %v", err)
		}
		if len(updates.GetUpdates()) != 1 {
			t.Fatalf("reaction updates = %#v, want one message update", updates)
		}
		result := make(map[string]int32)
		for _, count := range updates.GetUpdates()[0].GetReactions_MESSAGEREACTIONS().GetResults() {
			emoticon := count.GetReaction()
			if emoticon == "" {
				emoticon = count.GetReaction_REACTION().GetEmoticon()
			}
			result[emoticon] = count.GetCount()
		}
		return result
	}

	if err := send(alice, "👍"); err != nil {
		t.Fatalf("Alice sends reaction: %v", err)
	}
	if err := send(bob, "👍"); err != nil {
		t.Fatalf("Bob sends reaction: %v", err)
	}
	if err := send(bob, "👍"); err != nil {
		t.Fatalf("Bob repeats reaction: %v", err)
	}
	if got := counts(); len(got) != 1 || got["👍"] != 2 {
		t.Fatalf("duplicate reaction counts = %#v, want one thumbs-up from each user", got)
	}

	if err := send(bob, "🔥"); err != nil {
		t.Fatalf("Bob switches reaction: %v", err)
	}
	if got := counts(); len(got) != 2 || got["👍"] != 1 || got["🔥"] != 1 {
		t.Fatalf("switched reaction counts = %#v, want Alice thumbs-up and Bob fire", got)
	}

	list, err := alice.MessagesGetMessageReactionsList(&mtproto.TLMessagesGetMessageReactionsList{
		Peer:  peer,
		Id:    messageID,
		Limit: 20,
	})
	if err != nil {
		t.Fatalf("list shared reactions: %v", err)
	}
	gotByUser := map[int64]string{}
	for _, item := range list.GetReactions() {
		gotByUser[item.GetPeerId().GetUserId()] = item.GetReaction()
	}
	if list.GetCount() != 2 || gotByUser[aliceID] != "👍" || gotByUser[bobID] != "🔥" {
		t.Fatalf("reaction list = %#v, want Alice thumbs-up and Bob fire", list)
	}

	if _, err = alice.MessagesSetDefaultReaction(&mtproto.TLMessagesSetDefaultReaction{Reaction_REACTION: reaction("👍")}); err != nil {
		t.Fatalf("set Alice default reaction: %v", err)
	}
	if _, err = bob.MessagesSetDefaultReaction(&mtproto.TLMessagesSetDefaultReaction{Reaction_REACTION: reaction("🔥")}); err != nil {
		t.Fatalf("set Bob default reaction: %v", err)
	}
	aliceTop, err := alice.MessagesGetTopReactions(&mtproto.TLMessagesGetTopReactions{})
	if err != nil || len(aliceTop.GetReactions()) != 1 || aliceTop.GetReactions()[0].GetEmoticon() != "👍" {
		t.Fatalf("Alice top reactions = %#v, err=%v", aliceTop, err)
	}
	bobTop, err := bob.MessagesGetTopReactions(&mtproto.TLMessagesGetTopReactions{})
	if err != nil || len(bobTop.GetReactions()) != 1 || bobTop.GetReactions()[0].GetEmoticon() != "🔥" {
		t.Fatalf("Bob top reactions = %#v, err=%v", bobTop, err)
	}

	if _, err = alice.MessagesSendReaction(&mtproto.TLMessagesSendReaction{Peer: &mtproto.InputPeer{}}); !errors.Is(err, mtproto.ErrPeerIdInvalid) {
		t.Fatalf("invalid peer send error = %v, want PEER_ID_INVALID", err)
	}
	badHash := mtproto.MakeTLInputPeerChannel(&mtproto.InputPeer{
		ChannelId:  channelID,
		AccessHash: channelID + 1,
	}).To_InputPeer()
	if _, err = bob.MessagesGetMessagesReactions(&mtproto.TLMessagesGetMessagesReactions{Peer: badHash, Id: []int32{messageID}}); !errors.Is(err, mtproto.ErrChannelInvalid) {
		t.Fatalf("bad-hash read error = %v, want CHANNEL_INVALID", err)
	}

	participant := mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: bobID}).To_InputPeer()
	for _, check := range []struct {
		name   string
		action func() error
	}{
		{"send", func() error {
			_, err := carol.MessagesSendReaction(&mtproto.TLMessagesSendReaction{Peer: peer, MsgId: messageID, Reaction_FLAGVECTORREACTION: []*mtproto.Reaction{reaction("👎")}})
			return err
		}},
		{"get", func() error {
			_, err := carol.MessagesGetMessagesReactions(&mtproto.TLMessagesGetMessagesReactions{Peer: peer, Id: []int32{messageID}})
			return err
		}},
		{"list", func() error {
			_, err := carol.MessagesGetMessageReactionsList(&mtproto.TLMessagesGetMessageReactionsList{Peer: peer, Id: messageID, Limit: 20})
			return err
		}},
		{"delete all", func() error {
			_, err := carol.MessagesDeleteParticipantReactions(&mtproto.TLMessagesDeleteParticipantReactions{Peer: peer, Participant: participant})
			return err
		}},
		{"delete one", func() error {
			_, err := carol.MessagesDeleteParticipantReaction(&mtproto.TLMessagesDeleteParticipantReaction{Peer: peer, MsgId: messageID, Participant: participant})
			return err
		}},
	} {
		if err := check.action(); !errors.Is(err, mtproto.ErrUserNotParticipant) {
			t.Fatalf("outsider %s error = %v, want USER_NOT_PARTICIPANT", check.name, err)
		}
	}
	if got := counts(); len(got) != 2 || got["👍"] != 1 || got["🔥"] != 1 {
		t.Fatalf("outsider changed shared reactions: %#v", got)
	}
}
