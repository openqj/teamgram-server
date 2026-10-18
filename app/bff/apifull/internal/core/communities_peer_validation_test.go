package core

import (
	"errors"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/domain"
)

func TestCommunityPeerTypeRejectsIncompleteChannel(t *testing.T) {
	peer := mtproto.MakeTLInputPeerChannel(&mtproto.InputPeer{ChannelId: 81031001}).To_InputPeer()
	if _, _, _, err := communityPeerType(81031000, peer); !errors.Is(err, mtproto.ErrChannelInvalid) {
		t.Fatalf("incomplete channel peer = %v, want CHANNEL_INVALID", err)
	}
}

func TestCommunityPeerTypeRejectsIncompleteUser(t *testing.T) {
	peer := mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: 81031001}).To_InputPeer()
	if _, _, _, err := communityPeerType(81031000, peer); !errors.Is(err, mtproto.ErrPeerIdInvalid) {
		t.Fatalf("incomplete user peer = %v, want PEER_ID_INVALID", err)
	}
}

func TestCommunityPeerTypeAcceptsSelf(t *testing.T) {
	const callerID = int64(81031000)
	typ, id, accessHash, err := communityPeerType(callerID, mtproto.MakeTLInputPeerSelf(nil).To_InputPeer())
	if err != nil {
		t.Fatalf("self peer = (%v, %v, %v, %v), want success", typ, id, accessHash, err)
	}
	if typ != domain.CommunityPeerUser || id != callerID || accessHash != 0 {
		t.Fatalf("self peer = (%v, %v, %v), want user caller with zero hash", typ, id, accessHash)
	}
}

func TestCommunityPeerTypeRejectsStaleOrUnknownChannel(t *testing.T) {
	if !domain.Ready() {
		t.Skip("APIFULL_POSTGRES_DSN is not configured")
	}
	channelID := time.Now().UnixNano()
	ownerID := channelID + 1
	accessHash := channelID + 10
	if err := domain.SaveChannel(domain.Channel{
		ID: channelID, AccessHash: accessHash, Creator: ownerID, Title: "community-peer-validation", Megagroup: true,
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = domain.DeleteChannel(ownerID, channelID) })

	stale := mtproto.MakeTLInputPeerChannel(&mtproto.InputPeer{
		ChannelId: channelID, AccessHash: accessHash + 1,
	}).To_InputPeer()
	if _, _, _, err := communityPeerType(ownerID, stale); !errors.Is(err, mtproto.ErrChannelInvalid) {
		t.Fatalf("stale channel peer = %v, want CHANNEL_INVALID", err)
	}

	unknown := mtproto.MakeTLInputPeerChannel(&mtproto.InputPeer{
		ChannelId: channelID + 1, AccessHash: accessHash,
	}).To_InputPeer()
	if _, _, _, err := communityPeerType(ownerID, unknown); !errors.Is(err, mtproto.ErrChannelInvalid) {
		t.Fatalf("unknown channel peer = %v, want CHANNEL_INVALID", err)
	}
}
