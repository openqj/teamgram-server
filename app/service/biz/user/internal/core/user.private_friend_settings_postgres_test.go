package core

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
	userdao "github.com/teamgram/teamgram-server/app/service/biz/user/internal/dao"
	"github.com/teamgram/teamgram-server/app/service/biz/user/internal/svc"
	"github.com/teamgram/teamgram-server/app/service/biz/user/user"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func TestUserPostgresPrivateFriendSettingsRoundTrip(t *testing.T) {
	pg := userPostgresTest(t, 2)
	ctx := context.Background()
	ownerID := time.Now().UnixNano()
	friendID, otherID := ownerID+1, ownerID+2
	insertUserPostgresFixtures(t, pg, ownerID, friendID, otherID)

	c := New(ctx, &svc.ServiceContext{Dao: &userdao.Dao{Postgres: pg}})
	for _, contactID := range []int64{friendID, otherID} {
		if result, err := c.UserAddContact(&user.TLUserAddContact{UserId: ownerID, Id: contactID}); err != nil {
			t.Fatalf("add contact %d = (%v, %v)", contactID, result, err)
		}
	}

	if result, err := c.UserEditCloseFriends(&user.TLUserEditCloseFriends{UserId: ownerID, Id: []int64{friendID}}); err != nil || !mtproto.FromBool(result) {
		t.Fatalf("set close friend = (%v, %v)", result, err)
	}
	assertCloseFriendState(t, ctx, pg, ownerID, friendID, true)
	assertCloseFriendState(t, ctx, pg, ownerID, otherID, false)

	privacyRules := []*mtproto.PrivacyRule{
		mtproto.MakeTLPrivacyValueDisallowAll(nil).To_PrivacyRule(),
		mtproto.MakeTLPrivacyValueAllowCloseFriends(nil).To_PrivacyRule(),
	}
	if result, err := c.UserSetPrivacy(&user.TLUserSetPrivacy{UserId: ownerID, KeyType: mtproto.ABOUT, Rules: privacyRules}); err != nil || !mtproto.FromBool(result) {
		t.Fatalf("set close-friend privacy = (%v, %v)", result, err)
	}
	if allowed, err := c.UserCheckPrivacy(&user.TLUserCheckPrivacy{UserId: ownerID, KeyType: mtproto.ABOUT, PeerId: friendID}); err != nil || !mtproto.FromBool(allowed) {
		t.Fatalf("close friend privacy = (%v, %v), want allowed", allowed, err)
	}
	if allowed, err := c.UserCheckPrivacy(&user.TLUserCheckPrivacy{UserId: ownerID, KeyType: mtproto.ABOUT, PeerId: otherID}); err != nil || mtproto.FromBool(allowed) {
		t.Fatalf("ordinary friend privacy = (%v, %v), want denied", allowed, err)
	}

	if result, err := c.UserEditCloseFriends(&user.TLUserEditCloseFriends{UserId: ownerID, Id: []int64{otherID}}); err != nil || !mtproto.FromBool(result) {
		t.Fatalf("replace close friends = (%v, %v)", result, err)
	}
	assertCloseFriendState(t, ctx, pg, ownerID, friendID, false)
	assertCloseFriendState(t, ctx, pg, ownerID, otherID, true)
	if allowed, err := c.UserCheckPrivacy(&user.TLUserCheckPrivacy{UserId: ownerID, KeyType: mtproto.ABOUT, PeerId: friendID}); err != nil || mtproto.FromBool(allowed) {
		t.Fatalf("replaced close friend privacy = (%v, %v), want denied", allowed, err)
	}
	if allowed, err := c.UserCheckPrivacy(&user.TLUserCheckPrivacy{UserId: ownerID, KeyType: mtproto.ABOUT, PeerId: otherID}); err != nil || !mtproto.FromBool(allowed) {
		t.Fatalf("new close friend privacy = (%v, %v), want allowed", allowed, err)
	}

	settings := mtproto.MakeTLPeerSettings(&mtproto.PeerSettings{
		ReportSpam:            true,
		AddContact:            true,
		BlockContact:          true,
		ShareContact:          true,
		NeedContactsException: true,
		ReportGeo:             true,
		Autoarchived:          true,
		GeoDistance:           wrapperspb.Int32(75),
	}).To_PeerSettings()
	if result, err := c.UserAddPeerSettings(&user.TLUserAddPeerSettings{UserId: ownerID, PeerType: mtproto.PEER_USER, PeerId: otherID, Settings: settings}); err != nil || !mtproto.FromBool(result) {
		t.Fatalf("add peer settings = (%v, %v)", result, err)
	}
	got, err := c.UserGetPeerSettings(&user.TLUserGetPeerSettings{UserId: ownerID, PeerType: mtproto.PEER_USER, PeerId: otherID})
	if err != nil || got == nil || !got.GetReportSpam() || !got.GetAddContact() || !got.GetBlockContact() || !got.GetShareContact() || !got.GetNeedContactsException() || !got.GetReportGeo() || !got.GetAutoarchived() || got.GetGeoDistance() == nil || got.GetGeoDistance().GetValue() != 75 {
		t.Fatalf("get peer settings = (%v, %v), want persisted flags", got, err)
	}

	if result, err := c.UserDeletePeerSettings(&user.TLUserDeletePeerSettings{UserId: ownerID, PeerType: mtproto.PEER_USER, PeerId: otherID}); err != nil || !mtproto.FromBool(result) {
		t.Fatalf("delete peer settings = (%v, %v)", result, err)
	}
	got, err = c.UserGetPeerSettings(&user.TLUserGetPeerSettings{UserId: ownerID, PeerType: mtproto.PEER_USER, PeerId: otherID})
	if err != nil || got == nil || got.GetReportSpam() || got.GetGeoDistance() != nil {
		t.Fatalf("peer settings after delete = (%v, %v), want defaults", got, err)
	}

	if result, err := c.UserEditCloseFriends(&user.TLUserEditCloseFriends{UserId: ownerID}); err != nil || !mtproto.FromBool(result) {
		t.Fatalf("clear close friends = (%v, %v)", result, err)
	}
	if allowed, err := c.UserCheckPrivacy(&user.TLUserCheckPrivacy{UserId: ownerID, KeyType: mtproto.ABOUT, PeerId: otherID}); err != nil || mtproto.FromBool(allowed) {
		t.Fatalf("cleared close friend privacy = (%v, %v), want denied", allowed, err)
	}

	if _, err := c.UserCheckPrivacy(&user.TLUserCheckPrivacy{UserId: ownerID, KeyType: mtproto.ABOUT, PeerId: ownerID + 99}); !errors.Is(err, mtproto.ErrUserIdInvalid) {
		t.Fatalf("missing privacy peer error = %v, want USER_ID_INVALID", err)
	}
}

func assertCloseFriendState(t *testing.T, ctx context.Context, pg *userdao.Postgres, ownerID, peerID int64, want bool) {
	t.Helper()
	var got bool
	if err := pg.Pool.QueryRow(ctx, `SELECT close_friend FROM user_contacts WHERE owner_user_id=$1 AND contact_user_id=$2`, ownerID, peerID).Scan(&got); err != nil {
		t.Fatalf("read close friend state for %d: %v", peerID, err)
	}
	if got != want {
		t.Fatalf("close friend state for %d = %v, want %v", peerID, got, want)
	}
}
