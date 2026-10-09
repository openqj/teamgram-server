package domain

import (
	"testing"
	"time"
)

func TestCommunityPersistenceRoundTrip(t *testing.T) {
	requireMigrationDB(t)
	communityID := time.Now().UnixNano()
	ownerID := communityID + 1
	initialPeerID := communityID + 2
	pendingPeerID := communityID + 3
	cleanup := func() {
		for _, query := range []string{
			`DELETE FROM apifull_community_dialog_state WHERE community_id=?`,
			`DELETE FROM apifull_community_peer WHERE community_id=?`,
			`DELETE FROM apifull_community WHERE community_id=?`,
			`DELETE FROM apifull_channel WHERE id=?`,
		} {
			if _, err := db.Exec(query, communityID); err != nil {
				t.Errorf("cleanup community fixture: %v", err)
			}
		}
	}
	t.Cleanup(cleanup)

	channel := Channel{ID: communityID, AccessHash: communityID + 10, Creator: ownerID, Title: "community-test", CreatedAt: time.Now().Unix()}
	if err := CreateCommunityWithPeer(channel, false, CommunityPeerUser, initialPeerID, initialPeerID+10); err != nil {
		t.Fatalf("create community: %v", err)
	}
	loaded, ok, err := LoadCommunity(communityID)
	if err != nil || !ok || loaded.OwnerID != ownerID || loaded.Title != channel.Title {
		t.Fatalf("loaded community = %+v ok=%v err=%v", loaded, ok, err)
	}
	peers, err := ListCommunityPeers(communityID, false)
	if err != nil || len(peers) != 1 || peers[0].PeerID != initialPeerID || !peers[0].Approved {
		t.Fatalf("initial peers = %+v err=%v", peers, err)
	}

	visible := false
	if err = ToggleCommunityPeer(ownerID, communityID, CommunityPeerUser, pendingPeerID, pendingPeerID+10, &visible, false, false, time.Now().Unix()); err != nil {
		t.Fatalf("create pending peer: %v", err)
	}
	pending, err := ListCommunityPeers(communityID, true)
	if err != nil || len(pending) != 1 || pending[0].PeerID != pendingPeerID || pending[0].Visible == nil || *pending[0].Visible {
		t.Fatalf("pending peers = %+v err=%v", pending, err)
	}
	if err = ApproveCommunityPeer(ownerID, communityID, CommunityPeerUser, pendingPeerID, false); err != nil {
		t.Fatalf("approve peer: %v", err)
	}
	if err = BanCommunityPeer(ownerID, communityID, CommunityPeerUser, pendingPeerID, false); err != nil {
		t.Fatalf("ban peer: %v", err)
	}
	if peers, err = ListCommunityPeers(communityID, false); err != nil || len(peers) != 1 || peers[0].PeerID != initialPeerID {
		t.Fatalf("banned peer remained visible: %+v err=%v", peers, err)
	}

	if err = SetCommunityCollapsed(ownerID, communityID, true); err != nil {
		t.Fatalf("collapse community: %v", err)
	}
	if collapsed, err := CommunityCollapsed(ownerID, communityID); err != nil || !collapsed {
		t.Fatalf("collapsed state = %v err=%v", collapsed, err)
	}
	communities, err := ListCommunitiesForUser(ownerID)
	if err != nil || len(communities) != 1 || !communities[0].Collapsed {
		t.Fatalf("listed communities = %+v err=%v", communities, err)
	}
}
