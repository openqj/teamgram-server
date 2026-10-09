package core

import (
	"context"
	"errors"
	"fmt"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/service/biz/user/internal/dal/dao/postgres_dao"
	userdao "github.com/teamgram/teamgram-server/app/service/biz/user/internal/dao"
	"github.com/teamgram/teamgram-server/app/service/biz/user/internal/svc"
	"github.com/teamgram/teamgram-server/app/service/biz/user/user"
	"github.com/teamgram/teamgram-server/pkg/storage/postgres"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func TestUserPostgresSecurityPropagatesDatabaseFailure(t *testing.T) {
	pool, err := pgxpool.New(context.Background(), "postgres://user_failure:user_failure@127.0.0.1:1/user_failure_test")
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	c := New(ctx, &svc.ServiceContext{Dao: &userdao.Dao{Postgres: &userdao.Postgres{
		Pool: pool, Store: postgres_dao.NewStore(pool),
	}}})
	c.MD = &metadata.RpcMetadata{UserId: 1}

	tests := []struct {
		name string
		call func() error
	}{
		{"block", func() error {
			_, err := c.UserBlockPeer(&user.TLUserBlockPeer{UserId: 1, PeerType: mtproto.PEER_USER, PeerId: 2})
			return err
		}},
		{"unblock", func() error {
			_, err := c.UserUnBlockPeer(&user.TLUserUnBlockPeer{UserId: 1, PeerType: mtproto.PEER_USER, PeerId: 2})
			return err
		}},
		{"blocked", func() error {
			_, err := c.UserBlockedByUser(&user.TLUserBlockedByUser{UserId: 1, PeerUserId: 2})
			return err
		}},
		{"is blocked", func() error {
			_, err := c.UserIsBlockedByUser(&user.TLUserIsBlockedByUser{UserId: 1, PeerUserId: 2})
			return err
		}},
		{"blocked list", func() error {
			_, err := c.UserCheckBlockUserList(&user.TLUserCheckBlockUserList{UserId: 1, Id: []int64{2, 3}})
			return err
		}},
		{"get privacy", func() error {
			_, err := c.UserGetPrivacy(&user.TLUserGetPrivacy{UserId: 1, KeyType: mtproto.PHONE_NUMBER})
			return err
		}},
		{"check privacy", func() error {
			_, err := c.UserCheckPrivacy(&user.TLUserCheckPrivacy{UserId: 1, KeyType: mtproto.PHONE_NUMBER, PeerId: 2})
			return err
		}},
		{"set privacy", func() error {
			_, err := c.UserSetPrivacy(&user.TLUserSetPrivacy{UserId: 1, KeyType: mtproto.PHONE_NUMBER, Rules: []*mtproto.PrivacyRule{mtproto.MakeTLPrivacyValueDisallowAll(nil).To_PrivacyRule()}})
			return err
		}},
		{"check bots", func() error { _, err := c.UserCheckBots(&user.TLUserCheckBots{Id: []int64{1}}); return err }},
		{"user data", func() error {
			_, err := c.UserGetUserDataListByIdList(&user.TLUserGetUserDataListByIdList{UserIdList: []int64{1, 2}})
			return err
		}},
		{"mutable users", func() error {
			_, err := c.UserGetMutableUsers(&user.TLUserGetMutableUsers{Id: []int64{1, 2}})
			return err
		}},
		{"mutable users v2", func() error {
			_, err := c.UserGetMutableUsersV2(&user.TLUserGetMutableUsersV2{Id: []int64{1, 2}})
			return err
		}},
		{"contact list", func() error { _, err := c.UserGetContactList(&user.TLUserGetContactList{UserId: 1}); return err }},
		{"contact", func() error { _, err := c.UserGetContact(&user.TLUserGetContact{UserId: 1, Id: 2}); return err }},
		{"last seen", func() error { _, err := c.UserGetLastSeen(&user.TLUserGetLastSeen{Id: 1}); return err }},
		{"last seens", func() error { _, err := c.UserGetLastSeens(&user.TLUserGetLastSeens{Id: []int64{1, 2}}); return err }},
		{"deactivate usernames", func() error {
			_, err := c.UserDeactivateAllChannelUsernames(&user.TLUserDeactivateAllChannelUsernames{ChannelId: 1})
			return err
		}},
		{"toggle username", func() error {
			_, err := c.UserToggleUsername(&user.TLUserToggleUsername{PeerType: mtproto.PEER_USER, PeerId: 1, Username: "failure", Active: mtproto.BoolFalse})
			return err
		}},
		{"reorder usernames", func() error {
			_, err := c.UserReorderUsernames(&user.TLUserReorderUsernames{PeerType: mtproto.PEER_USER, PeerId: 1, UsernameList: []string{"failure"}})
			return err
		}},
		{"profile name", func() error {
			_, err := c.UserUpdateFirstAndLastName(&user.TLUserUpdateFirstAndLastName{UserId: 1})
			return err
		}},
		{"profile about", func() error { _, err := c.UserUpdateAbout(&user.TLUserUpdateAbout{UserId: 1}); return err }},
		{"emoji status", func() error { _, err := c.UserUpdateEmojiStatus(&user.TLUserUpdateEmojiStatus{UserId: 1}); return err }},
		{"stories max id", func() error { _, err := c.UserSetStoriesMaxId(&user.TLUserSetStoriesMaxId{UserId: 1}); return err }},
		{"color", func() error { _, err := c.UserSetColor(&user.TLUserSetColor{UserId: 1}); return err }},
		{"birthday", func() error { _, err := c.UserUpdateBirthday(&user.TLUserUpdateBirthday{UserId: 1}); return err }},
		{"personal channel", func() error {
			_, err := c.UserUpdatePersonalChannel(&user.TLUserUpdatePersonalChannel{UserId: 1})
			return err
		}},
		{"premium", func() error {
			_, err := c.UserUpdatePremium(&user.TLUserUpdatePremium{UserId: 1, Premium: mtproto.BoolFalse})
			return err
		}},
		{"contact ids", func() error { _, err := c.UserGetContactIdList(&user.TLUserGetContactIdList{UserId: 1}); return err }},
		{"check contact", func() error { _, err := c.UserCheckContact(&user.TLUserCheckContact{UserId: 1, Id: 2}); return err }},
		{"user by id", func() error { _, err := c.UserGetUserDataById(&user.TLUserGetUserDataById{UserId: 1}); return err }},
		{"is bot", func() error { _, err := c.UserIsBot(&user.TLUserIsBot{Id: 1}); return err }},
		{"birthdays", func() error { _, err := c.UserGetBirthdays(&user.TLUserGetBirthdays{UserId: 1}); return err }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.call(); !errors.Is(err, context.Canceled) {
				t.Fatalf("database error = %v, want context.Canceled", err)
			}
		})
	}
}

func TestUserPostgresProfileMutationAndBirthdayPrivacy(t *testing.T) {
	pg := userPostgresTest(t, 4)
	ctx := context.Background()
	ownerID := time.Now().UnixNano()
	peerID := ownerID + 1
	insertUserPostgresFixtures(t, pg, ownerID, peerID)
	c := New(ctx, &svc.ServiceContext{Dao: &userdao.Dao{Postgres: pg}})
	if got, err := c.UserUpdateFirstAndLastName(&user.TLUserUpdateFirstAndLastName{UserId: peerID, FirstName: "Given", LastName: "Family"}); err != nil || !mtproto.FromBool(got) {
		t.Fatalf("update profile = (%v, %v)", got, err)
	}
	stored, err := pg.Store.Users.SelectByID(ctx, peerID)
	if err != nil || stored.FirstName != "Given" || stored.LastName != "Family" {
		t.Fatalf("stored profile = (%v, %v)", stored, err)
	}
	if got, err := c.UserUpdateAbout(&user.TLUserUpdateAbout{UserId: ownerID + 2, About: "Missing"}); got != nil || !errors.Is(err, mtproto.ErrUserIdInvalid) {
		t.Fatalf("update missing profile = (%v, %v)", got, err)
	}
	if _, err := pg.Pool.Exec(ctx, `INSERT INTO user_contacts(owner_user_id,contact_user_id) VALUES($1,$2)`, ownerID, peerID); err != nil {
		t.Fatal(err)
	}
	if _, err := c.UserUpdateBirthday(&user.TLUserUpdateBirthday{UserId: peerID, Birthday: mtproto.MakeTLBirthday(&mtproto.Birthday{Day: 2, Month: 3}).To_Birthday()}); err != nil {
		t.Fatal(err)
	}
	if got, err := c.UserGetBirthdays(&user.TLUserGetBirthdays{UserId: ownerID}); err != nil || len(got.GetDatas()) != 1 || got.GetDatas()[0].GetContactId() != peerID {
		t.Fatalf("visible birthdays = (%v, %v)", got, err)
	}
	if _, err := c.UserSetPrivacy(&user.TLUserSetPrivacy{UserId: peerID, KeyType: mtproto.BIRTHDAY, Rules: []*mtproto.PrivacyRule{mtproto.MakeTLPrivacyValueDisallowAll(nil).To_PrivacyRule()}}); err != nil {
		t.Fatal(err)
	}
	if got, err := c.UserGetBirthdays(&user.TLUserGetBirthdays{UserId: ownerID}); err != nil || len(got.GetDatas()) != 0 {
		t.Fatalf("private birthdays = (%v, %v), want empty", got, err)
	}
	immutable, err := c.UserGetImmutableUser(&user.TLUserGetImmutableUser{Id: peerID, Privacy: true, Contacts: []int64{ownerID}})
	if err != nil || immutable.CheckPrivacy(mtproto.BIRTHDAY, ownerID) {
		t.Fatalf("snapshot birthday privacy = (%v, %v), want denied", immutable, err)
	}
	if _, err := pg.Pool.Exec(ctx, `UPDATE users SET premium=TRUE WHERE id=$1`, ownerID); err != nil {
		t.Fatal(err)
	}
	if _, err := c.UserSetPrivacy(&user.TLUserSetPrivacy{UserId: peerID, KeyType: mtproto.BIRTHDAY, Rules: []*mtproto.PrivacyRule{
		mtproto.MakeTLPrivacyValueDisallowAll(nil).To_PrivacyRule(),
		mtproto.MakeTLPrivacyValueAllowPremium(nil).To_PrivacyRule(),
	}}); err != nil {
		t.Fatal(err)
	}
	if got, err := c.UserGetBirthdays(&user.TLUserGetBirthdays{UserId: ownerID}); err != nil || len(got.GetDatas()) != 1 {
		t.Fatalf("premium birthday exception = (%v, %v), want visible", got, err)
	}
	if got, err := c.UserGetFullUser(&user.TLUserGetFullUser{SelfUserId: ownerID, Id: peerID}); err != nil || got.GetFullUser().GetBirthday() == nil {
		t.Fatalf("full user premium birthday = (%v, %v), want visible", got, err)
	}
	if _, err := c.UserSetPrivacy(&user.TLUserSetPrivacy{UserId: peerID, KeyType: mtproto.BIRTHDAY, Rules: []*mtproto.PrivacyRule{
		mtproto.MakeTLPrivacyValueAllowChatParticipants(&mtproto.PrivacyRule{Chats: []int64{ownerID + 99}}).To_PrivacyRule(),
		mtproto.MakeTLPrivacyValueDisallowAll(nil).To_PrivacyRule(),
	}}); err != nil {
		t.Fatal(err)
	}
	if got, err := c.UserGetBirthdays(&user.TLUserGetBirthdays{UserId: ownerID}); err != nil || len(got.GetDatas()) != 0 {
		t.Fatalf("nonparticipant birthday = (%v, %v), want denied", got, err)
	}
	if got, err := c.UserGetFullUser(&user.TLUserGetFullUser{SelfUserId: ownerID, Id: peerID}); err != nil || got.GetFullUser().GetBirthday() != nil {
		t.Fatalf("full user nonparticipant birthday = (%v, %v), want denied", got, err)
	}
}

func userPostgresTest(t *testing.T, maxConns int32) *userdao.Postgres {
	t.Helper()
	dsn := os.Getenv("TEAMGRAM_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("TEAMGRAM_POSTGRES_DSN must point to a PostgreSQL 18 test database")
	}
	pg, err := userdao.NewPostgres(postgres.Config{DSN: dsn, MaxConns: maxConns})
	if err != nil {
		t.Fatalf("open PostgreSQL 18 test database: %v", err)
	}
	t.Cleanup(pg.Close)
	return pg
}

func insertUserPostgresFixtures(t *testing.T, pg *userdao.Postgres, ids ...int64) {
	t.Helper()
	ctx := context.Background()
	t.Cleanup(func() { _, _ = pg.Pool.Exec(ctx, `DELETE FROM users WHERE id=ANY($1::bigint[])`, ids) })
	for _, id := range ids {
		if _, err := pg.Pool.Exec(ctx, `INSERT INTO users(id,phone) VALUES($1,$2)`, id, fmt.Sprintf("pg-security-%d", id)); err != nil {
			t.Fatal(err)
		}
	}
}

func TestUserPostgresBlockPrivacyAndMutableUsersRoundTrip(t *testing.T) {
	pg := userPostgresTest(t, 4)
	ctx := context.Background()
	ownerID := time.Now().UnixNano()
	peerID, otherID := ownerID+1, ownerID+2
	insertUserPostgresFixtures(t, pg, ownerID, peerID, otherID)
	c := New(ctx, &svc.ServiceContext{Dao: &userdao.Dao{Postgres: pg}})

	if got, err := c.UserBlockPeer(&user.TLUserBlockPeer{UserId: ownerID, PeerType: mtproto.PEER_USER, PeerId: peerID}); err != nil || !mtproto.FromBool(got) {
		t.Fatalf("block = (%v, %v)", got, err)
	}
	if got, err := c.UserBlockedByUser(&user.TLUserBlockedByUser{UserId: ownerID, PeerUserId: peerID}); err != nil || !mtproto.FromBool(got) {
		t.Fatalf("blocked = (%v, %v)", got, err)
	}
	if got, err := c.UserBlockPeer(&user.TLUserBlockPeer{UserId: ownerID, PeerType: mtproto.PEER_CHANNEL, PeerId: otherID}); got != nil || !errors.Is(err, mtproto.ErrPeerIdInvalid) {
		t.Fatalf("invalid block peer = (%v, %v)", got, err)
	}
	if got, err := c.UserCheckBlockUserList(&user.TLUserCheckBlockUserList{UserId: ownerID, Id: []int64{otherID, peerID, peerID}}); err != nil || !reflect.DeepEqual(got.GetDatas(), []int64{peerID, peerID}) {
		t.Fatalf("ordered blocked list = (%v, %v)", got, err)
	}
	if got, err := c.UserUnBlockPeer(&user.TLUserUnBlockPeer{UserId: ownerID, PeerType: mtproto.PEER_USER, PeerId: peerID}); err != nil || !mtproto.FromBool(got) {
		t.Fatalf("unblock = (%v, %v)", got, err)
	}
	if got, err := c.UserBlockedByUser(&user.TLUserBlockedByUser{UserId: ownerID, PeerUserId: peerID}); err != nil || mtproto.FromBool(got) {
		t.Fatalf("blocked after unblock = (%v, %v)", got, err)
	}
	if got, err := c.UserBlockPeer(&user.TLUserBlockPeer{UserId: ownerID + 99, PeerType: mtproto.PEER_USER, PeerId: peerID}); got != nil || !errors.Is(err, mtproto.ErrUserIdInvalid) {
		t.Fatalf("missing block owner = (%v, %v), want USER_ID_INVALID", got, err)
	}
	if _, err := pg.Pool.Exec(ctx, `UPDATE users SET deleted=TRUE WHERE id=$1`, ownerID); err != nil {
		t.Fatal(err)
	}
	if got, err := c.UserBlockPeer(&user.TLUserBlockPeer{UserId: ownerID, PeerType: mtproto.PEER_USER, PeerId: peerID}); got != nil || !errors.Is(err, mtproto.ErrUserIdInvalid) {
		t.Fatalf("deleted block owner = (%v, %v), want USER_ID_INVALID", got, err)
	}
	if _, err := pg.Pool.Exec(ctx, `UPDATE users SET deleted=FALSE WHERE id=$1`, ownerID); err != nil {
		t.Fatal(err)
	}

	rules := []*mtproto.PrivacyRule{mtproto.MakeTLPrivacyValueDisallowAll(nil).To_PrivacyRule()}
	if got, err := c.UserSetPrivacy(&user.TLUserSetPrivacy{UserId: ownerID, KeyType: mtproto.PHONE_NUMBER, Rules: rules}); err != nil || !mtproto.FromBool(got) {
		t.Fatalf("set privacy = (%v, %v)", got, err)
	}
	if got, err := c.UserGetPrivacy(&user.TLUserGetPrivacy{UserId: ownerID, KeyType: mtproto.PHONE_NUMBER}); err != nil || len(got.GetDatas()) != 1 || got.GetDatas()[0].GetPredicateName() != mtproto.Predicate_privacyValueDisallowAll {
		t.Fatalf("get privacy = (%v, %v)", got, err)
	}
	if _, err := pg.Pool.Exec(ctx, `UPDATE user_privacies SET rules='{' WHERE user_id=$1 AND key_type=$2`, ownerID, mtproto.PHONE_NUMBER); err != nil {
		t.Fatal(err)
	}
	if got, err := c.UserGetPrivacy(&user.TLUserGetPrivacy{UserId: ownerID, KeyType: mtproto.PHONE_NUMBER}); got != nil || err == nil {
		t.Fatalf("corrupt stored privacy = (%v, %v), want error", got, err)
	}
	if got, err := c.UserGetMutableUsers(&user.TLUserGetMutableUsers{Id: []int64{ownerID, peerID}}); got != nil || err == nil {
		t.Fatalf("mutable users with corrupt privacy = (%v, %v), want error", got, err)
	}
	if _, err := c.UserSetPrivacy(&user.TLUserSetPrivacy{UserId: ownerID, KeyType: mtproto.PHONE_NUMBER, Rules: rules}); err != nil {
		t.Fatal(err)
	}
	if _, err := pg.Pool.Exec(ctx, `INSERT INTO user_contacts(owner_user_id,contact_user_id) VALUES($1,$2)`, peerID, ownerID); err != nil {
		t.Fatal(err)
	}
	if got, err := c.UserGetMutableUsersV2(&user.TLUserGetMutableUsersV2{Id: []int64{ownerID, peerID}}); err != nil {
		t.Fatal(err)
	} else {
		for _, entry := range got.GetUsers() {
			if len(entry.GetReverseContacts()) != 0 {
				t.Fatalf("has_to=false returned reverse contacts: %v", entry.GetReverseContacts())
			}
		}
	}
	got, err := c.UserGetMutableUsersV2(&user.TLUserGetMutableUsersV2{Id: []int64{ownerID, peerID}, HasTo: true, To: []int64{peerID}})
	if err != nil || len(got.GetUsers()) != 2 || len(got.GetUsers()[0].GetReverseContacts()) != 1 || got.GetUsers()[0].GetReverseContacts()[0].GetUserId() != peerID {
		t.Fatalf("has_to=true returned (%v, %v), want requested reverse contact", got, err)
	}
}

func TestUserPostgresPrivacyRuleEvaluation(t *testing.T) {
	pg := userPostgresTest(t, 1)
	ctx := context.Background()
	ownerID := time.Now().UnixNano()
	peerID, botID, premiumID := ownerID+1, ownerID+2, ownerID+3
	insertUserPostgresFixtures(t, pg, ownerID, peerID, botID, premiumID)
	if _, err := pg.Pool.Exec(ctx, `UPDATE users SET user_type=$2,is_bot=TRUE WHERE id=$1`, botID, user.UserTypeBot); err != nil {
		t.Fatal(err)
	}
	if _, err := pg.Pool.Exec(ctx, `UPDATE users SET premium=TRUE,premium_expire_date=$2 WHERE id=$1`, premiumID, time.Now().Add(time.Hour).Unix()); err != nil {
		t.Fatal(err)
	}
	c := New(ctx, &svc.ServiceContext{Dao: &userdao.Dao{Postgres: pg}})
	setRules := func(rules ...*mtproto.PrivacyRule) {
		t.Helper()
		if _, err := c.UserSetPrivacy(&user.TLUserSetPrivacy{UserId: ownerID, KeyType: mtproto.PHONE_NUMBER, Rules: rules}); err != nil {
			t.Fatal(err)
		}
	}
	check := func(id int64, want bool) {
		t.Helper()
		got, err := c.UserCheckPrivacy(&user.TLUserCheckPrivacy{UserId: ownerID, KeyType: mtproto.PHONE_NUMBER, PeerId: id})
		if err != nil || got == nil || mtproto.FromBool(got) != want {
			t.Fatalf("privacy for %d = (%v, %v), want %v", id, got, err, want)
		}
	}
	allowAll := mtproto.MakeTLPrivacyValueAllowAll(nil).To_PrivacyRule()
	denyAll := mtproto.MakeTLPrivacyValueDisallowAll(nil).To_PrivacyRule()
	check(peerID, false)
	check(ownerID, true)
	setRules(allowAll)
	check(peerID, true)
	setRules(denyAll)
	check(peerID, false)
	if _, err := pg.Pool.Exec(ctx, `INSERT INTO user_contacts(owner_user_id,contact_user_id,close_friend) VALUES($1,$2,TRUE)`, ownerID, peerID); err != nil {
		t.Fatal(err)
	}
	setRules(mtproto.MakeTLPrivacyValueAllowContacts(nil).To_PrivacyRule())
	check(peerID, true)
	check(premiumID, false)
	setRules(denyAll, mtproto.MakeTLPrivacyValueAllowCloseFriends(nil).To_PrivacyRule())
	check(peerID, true)
	if _, err := pg.Pool.Exec(ctx, `UPDATE user_contacts SET is_deleted=TRUE WHERE owner_user_id=$1 AND contact_user_id=$2`, ownerID, peerID); err != nil {
		t.Fatal(err)
	}
	check(peerID, false)
	setRules(denyAll, mtproto.MakeTLPrivacyValueAllowPremium(nil).To_PrivacyRule())
	check(premiumID, true)
	if _, err := pg.Pool.Exec(ctx, `UPDATE users SET premium_expire_date=$2 WHERE id=$1`, premiumID, time.Now().Add(-time.Hour).Unix()); err != nil {
		t.Fatal(err)
	}
	check(premiumID, false)
	if _, err := pg.Pool.Exec(ctx, `UPDATE users SET premium_expire_date=0 WHERE id=$1`, premiumID); err != nil {
		t.Fatal(err)
	}
	check(premiumID, true)
	setRules(denyAll, mtproto.MakeTLPrivacyValueAllowBots(nil).To_PrivacyRule())
	check(botID, true)
	check(peerID, false)
	setRules(allowAll, mtproto.MakeTLPrivacyValueDisallowBots(nil).To_PrivacyRule())
	check(botID, false)
	check(peerID, true)
	for _, in := range []*user.TLUserCheckPrivacy{
		{UserId: ownerID, KeyType: 0, PeerId: peerID},
		{UserId: ownerID, KeyType: mtproto.PHONE_NUMBER, PeerId: ownerID + 4},
		{UserId: ownerID + 4, KeyType: mtproto.PHONE_NUMBER, PeerId: peerID},
	} {
		if got, err := c.UserCheckPrivacy(in); got != nil || err == nil {
			t.Fatalf("invalid privacy request = (%v, %v), want error", got, err)
		}
	}
	if _, err := pg.Pool.Exec(ctx, `UPDATE user_privacies SET rules='[null]' WHERE user_id=$1 AND key_type=$2`, ownerID, mtproto.PHONE_NUMBER); err != nil {
		t.Fatal(err)
	}
	if got, err := c.UserCheckPrivacy(&user.TLUserCheckPrivacy{UserId: ownerID, KeyType: mtproto.PHONE_NUMBER, PeerId: peerID}); got != nil || !errors.Is(err, mtproto.ErrPrivacyValueInvalid) {
		t.Fatalf("invalid stored rule = (%v, %v), want privacy value error", got, err)
	}
}

func TestUserPostgresSetPrivacyValidatesBeforeWrite(t *testing.T) {
	pg := userPostgresTest(t, 1)
	ctx := context.Background()
	ownerID := time.Now().UnixNano()
	insertUserPostgresFixtures(t, pg, ownerID)
	c := New(ctx, &svc.ServiceContext{Dao: &userdao.Dao{Postgres: pg}})
	storedRules := []*mtproto.PrivacyRule{
		mtproto.MakeTLPrivacyValueDisallowAll(nil).To_PrivacyRule(),
		mtproto.MakeTLPrivacyValueAllowPremium(nil).To_PrivacyRule(),
	}
	if _, err := c.UserSetPrivacy(&user.TLUserSetPrivacy{UserId: ownerID, KeyType: mtproto.PHONE_NUMBER, Rules: storedRules}); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name  string
		rules []*mtproto.PrivacyRule
	}{
		{name: "empty"},
		{name: "nil", rules: []*mtproto.PrivacyRule{nil}},
		{name: "unknown", rules: []*mtproto.PrivacyRule{{PredicateName: "unknown"}}},
		{name: "multiple base", rules: []*mtproto.PrivacyRule{storedRules[0], mtproto.MakeTLPrivacyValueAllowAll(nil).To_PrivacyRule()}},
		{name: "invalid user", rules: []*mtproto.PrivacyRule{mtproto.MakeTLPrivacyValueAllowUsers(&mtproto.PrivacyRule{Users: []int64{0}}).To_PrivacyRule()}},
		{name: "invalid chat", rules: []*mtproto.PrivacyRule{mtproto.MakeTLPrivacyValueAllowChatParticipants(&mtproto.PrivacyRule{Chats: []int64{-1}}).To_PrivacyRule()}},
		{name: "unexpected list", rules: []*mtproto.PrivacyRule{mtproto.MakeTLPrivacyValueAllowAll(&mtproto.PrivacyRule{Users: []int64{ownerID}}).To_PrivacyRule()}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := c.UserSetPrivacy(&user.TLUserSetPrivacy{UserId: ownerID, KeyType: mtproto.PHONE_NUMBER, Rules: tc.rules})
			if got != nil || !errors.Is(err, mtproto.ErrPrivacyValueInvalid) {
				t.Fatalf("set invalid rules = (%v, %v), want PRIVACY_VALUE_INVALID", got, err)
			}
			rules, err := c.UserGetPrivacy(&user.TLUserGetPrivacy{UserId: ownerID, KeyType: mtproto.PHONE_NUMBER})
			if err != nil || len(rules.GetDatas()) != 2 || rules.GetDatas()[0].GetPredicateName() != storedRules[0].GetPredicateName() || rules.GetDatas()[1].GetPredicateName() != storedRules[1].GetPredicateName() {
				t.Fatalf("rules after invalid write = (%v, %v), want original order", rules, err)
			}
		})
	}
	if got, err := c.UserSetPrivacy(&user.TLUserSetPrivacy{UserId: ownerID + 1, KeyType: mtproto.PHONE_NUMBER, Rules: storedRules}); got != nil || !errors.Is(err, mtproto.ErrUserIdInvalid) {
		t.Fatalf("missing privacy owner = (%v, %v), want USER_ID_INVALID", got, err)
	}
	if _, err := pg.Pool.Exec(ctx, `UPDATE users SET deleted=TRUE WHERE id=$1`, ownerID); err != nil {
		t.Fatal(err)
	}
	if got, err := c.UserSetPrivacy(&user.TLUserSetPrivacy{UserId: ownerID, KeyType: mtproto.PHONE_NUMBER, Rules: storedRules}); got != nil || !errors.Is(err, mtproto.ErrUserIdInvalid) {
		t.Fatalf("deleted privacy owner = (%v, %v), want USER_ID_INVALID", got, err)
	}
}

func TestUserPostgresPrivacyChatParticipants(t *testing.T) {
	pg := userPostgresTest(t, 1)
	ctx := context.Background()
	ownerID := time.Now().UnixNano()
	peerID := ownerID + 1
	chatID, channelID, creatorChannelID := ownerID+2, ownerID+3, ownerID+4
	insertUserPostgresFixtures(t, pg, ownerID, peerID)
	t.Cleanup(func() {
		_, _ = pg.Pool.Exec(ctx, `DELETE FROM chat_participants WHERE chat_id=$1`, chatID)
		_, _ = pg.Pool.Exec(ctx, `DELETE FROM chats WHERE id=$1`, chatID)
		_, _ = pg.Pool.Exec(ctx, `DELETE FROM apifull_channel_member WHERE channel_id=ANY($1::bigint[])`, []int64{channelID, creatorChannelID, chatID})
		_, _ = pg.Pool.Exec(ctx, `DELETE FROM apifull_channel WHERE id=ANY($1::bigint[])`, []int64{channelID, creatorChannelID, chatID})
	})
	if _, err := pg.Pool.Exec(ctx, `INSERT INTO chats(id,creator_user_id,access_hash,random_id) VALUES($1,$2,1,$1)`, chatID, ownerID); err != nil {
		t.Fatal(err)
	}
	if _, err := pg.Pool.Exec(ctx, `INSERT INTO chat_participants(chat_id,user_id,state) VALUES($1,$2,$3)`, chatID, peerID, mtproto.ChatMemberStateNormal); err != nil {
		t.Fatal(err)
	}
	if _, err := pg.Pool.Exec(ctx, `INSERT INTO apifull_channel(id,creator_user_id,access_hash,title,created_at) VALUES($1,$2,1,'Privacy member',1),($3,$4,1,'Privacy creator',1)`, channelID, ownerID, creatorChannelID, peerID); err != nil {
		t.Fatal(err)
	}
	if _, err := pg.Pool.Exec(ctx, `INSERT INTO apifull_channel_member(channel_id,user_id,joined_at) VALUES($1,$2,1)`, channelID, peerID); err != nil {
		t.Fatal(err)
	}
	c := New(ctx, &svc.ServiceContext{Dao: &userdao.Dao{Postgres: pg}})
	check := func(ids []int64, allowRule, want bool) {
		t.Helper()
		rules := []*mtproto.PrivacyRule{
			mtproto.MakeTLPrivacyValueAllowChatParticipants(&mtproto.PrivacyRule{Chats: ids}).To_PrivacyRule(),
			mtproto.MakeTLPrivacyValueDisallowAll(nil).To_PrivacyRule(),
		}
		if !allowRule {
			rules[0] = mtproto.MakeTLPrivacyValueDisallowChatParticipants(&mtproto.PrivacyRule{Chats: ids}).To_PrivacyRule()
			rules[1] = mtproto.MakeTLPrivacyValueAllowAll(nil).To_PrivacyRule()
		}
		if _, err := c.UserSetPrivacy(&user.TLUserSetPrivacy{UserId: ownerID, KeyType: mtproto.PHONE_NUMBER, Rules: rules}); err != nil {
			t.Fatal(err)
		}
		got, err := c.UserCheckPrivacy(&user.TLUserCheckPrivacy{UserId: ownerID, KeyType: mtproto.PHONE_NUMBER, PeerId: peerID})
		if err != nil || got == nil || mtproto.FromBool(got) != want {
			t.Fatalf("chat privacy %v = (%v, %v), want %v", ids, got, err, want)
		}
	}
	check([]int64{chatID}, true, true)
	check([]int64{chatID}, false, false)
	if _, err := pg.Pool.Exec(ctx, `UPDATE chat_participants SET state=$3 WHERE chat_id=$1 AND user_id=$2`, chatID, peerID, mtproto.ChatMemberStateLeft); err != nil {
		t.Fatal(err)
	}
	check([]int64{chatID}, true, false)
	if _, err := pg.Pool.Exec(ctx, `UPDATE chat_participants SET state=$3 WHERE chat_id=$1 AND user_id=$2`, chatID, peerID, mtproto.ChatMemberStateNormal); err != nil {
		t.Fatal(err)
	}
	if _, err := pg.Pool.Exec(ctx, `UPDATE chats SET deactivated=TRUE WHERE id=$1`, chatID); err != nil {
		t.Fatal(err)
	}
	check([]int64{chatID}, true, false)
	check([]int64{creatorChannelID}, true, true)
	check([]int64{channelID}, true, true)
	if _, err := pg.Pool.Exec(ctx, `UPDATE apifull_channel_member SET banned_rights='{"send_messages":true}' WHERE channel_id=$1 AND user_id=$2`, channelID, peerID); err != nil {
		t.Fatal(err)
	}
	check([]int64{channelID}, true, true)
	if _, err := pg.Pool.Exec(ctx, `UPDATE apifull_channel_member SET banned_rights='{"view_messages":true,"until_date":1}' WHERE channel_id=$1 AND user_id=$2`, channelID, peerID); err != nil {
		t.Fatal(err)
	}
	check([]int64{channelID}, true, false)
	check([]int64{channelID}, false, true)
	if _, err := pg.Pool.Exec(ctx, `DELETE FROM apifull_channel_member WHERE channel_id=$1 AND user_id=$2`, channelID, peerID); err != nil {
		t.Fatal(err)
	}
	check([]int64{channelID}, true, false)
	check([]int64{chatID, creatorChannelID}, true, true)
	if _, err := pg.Pool.Exec(ctx, `UPDATE chats SET deactivated=FALSE WHERE id=$1`, chatID); err != nil {
		t.Fatal(err)
	}
	if _, err := pg.Pool.Exec(ctx, `INSERT INTO apifull_channel(id,creator_user_id,access_hash,title,created_at) VALUES($1,$2,1,'Same numeric id',1)`, chatID, ownerID); err != nil {
		t.Fatal(err)
	}
	check([]int64{chatID}, true, false)
	if _, err := pg.Pool.Exec(ctx, `INSERT INTO apifull_channel_member(channel_id,user_id,joined_at,banned_rights) VALUES($1,$2,1,'{')`, channelID, peerID); err != nil {
		t.Fatal(err)
	}
	if _, err := c.UserSetPrivacy(&user.TLUserSetPrivacy{UserId: ownerID, KeyType: mtproto.PHONE_NUMBER, Rules: []*mtproto.PrivacyRule{
		mtproto.MakeTLPrivacyValueAllowChatParticipants(&mtproto.PrivacyRule{Chats: []int64{channelID}}).To_PrivacyRule(),
		mtproto.MakeTLPrivacyValueAllowAll(nil).To_PrivacyRule(),
	}}); err != nil {
		t.Fatal(err)
	}
	if got, err := c.UserCheckPrivacy(&user.TLUserCheckPrivacy{UserId: ownerID, KeyType: mtproto.PHONE_NUMBER, PeerId: peerID}); got != nil || err == nil {
		t.Fatalf("corrupt membership = (%v, %v), want error", got, err)
	}
	if _, err := pg.Pool.Exec(ctx, `UPDATE apifull_channel_member SET banned_rights='null' WHERE channel_id=$1 AND user_id=$2`, channelID, peerID); err != nil {
		t.Fatal(err)
	}
	if got, err := c.UserCheckPrivacy(&user.TLUserCheckPrivacy{UserId: ownerID, KeyType: mtproto.PHONE_NUMBER, PeerId: peerID}); got != nil || err == nil {
		t.Fatalf("null membership ban = (%v, %v), want error", got, err)
	}
}

func TestUserSetBotInfoPostgresSingleConnectionAndRollback(t *testing.T) {
	pg := userPostgresTest(t, 1)
	ctx := context.Background()
	botID := time.Now().UnixNano()
	insertUserPostgresFixtures(t, pg, botID)
	if _, err := pg.Pool.Exec(ctx, `UPDATE users SET user_type=$1,is_bot=TRUE WHERE id=$2`, user.UserTypeBot, botID); err != nil {
		t.Fatal(err)
	}
	if _, err := pg.Pool.Exec(ctx, `INSERT INTO bots(bot_id,token) VALUES($1,$2)`, botID, fmt.Sprintf("pg-profile-%d", botID)); err != nil {
		t.Fatal(err)
	}
	callCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	c := New(callCtx, &svc.ServiceContext{Dao: &userdao.Dao{Postgres: pg}})
	c.MD = &metadata.RpcMetadata{UserId: botID}
	if got, err := c.UserSetBotInfo(&user.BotRegistrySetBotInfoRequest{BotId: botID, Name: wrapperspb.String("Profile"), About: wrapperspb.String("About"), Description: wrapperspb.String("Description")}); err != nil || !mtproto.FromBool(got) {
		t.Fatalf("set bot info with one connection = (%v, %v)", got, err)
	}
	var name, about, description string
	if err := pg.Pool.QueryRow(ctx, `SELECT u.first_name,u.about,b.description FROM users u JOIN bots b ON b.bot_id=u.id WHERE u.id=$1`, botID).Scan(&name, &about, &description); err != nil || name != "Profile" || about != "About" || description != "Description" {
		t.Fatalf("stored bot profile = (%q,%q,%q), %v", name, about, description, err)
	}

	// The temporary constraint makes the second write fail without changing the shared schema.
	if _, err := pg.Pool.Exec(ctx, `CREATE TEMP TABLE bots (LIKE public.bots INCLUDING ALL)`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = pg.Pool.Exec(ctx, `DROP TABLE pg_temp.bots`) })
	if _, err := pg.Pool.Exec(ctx, `INSERT INTO pg_temp.bots SELECT * FROM public.bots WHERE bot_id=$1`, botID); err != nil {
		t.Fatal(err)
	}
	if _, err := pg.Pool.Exec(ctx, `ALTER TABLE pg_temp.bots ADD CHECK(description<>'reject')`); err != nil {
		t.Fatal(err)
	}
	if got, err := c.UserSetBotInfo(&user.BotRegistrySetBotInfoRequest{BotId: botID, Name: wrapperspb.String("Changed"), Description: wrapperspb.String("reject")}); got != nil || err == nil {
		t.Fatalf("rejected bot profile = (%v, %v), want error", got, err)
	}
	if err := pg.Pool.QueryRow(ctx, `SELECT first_name FROM users WHERE id=$1`, botID).Scan(&name); err != nil || name != "Profile" {
		t.Fatalf("name after bot update failure = (%q, %v), want Profile", name, err)
	}
}

func TestUserExportBotTokenPostgresOwnershipAndRotation(t *testing.T) {
	pg := userPostgresTest(t, 1)
	ctx := context.Background()
	ownerID := time.Now().UnixNano()
	botID := ownerID + 1
	otherID := ownerID + 2
	insertUserPostgresFixtures(t, pg, ownerID, botID, otherID)
	if _, err := pg.Pool.Exec(ctx, `UPDATE users SET user_type=$1,is_bot=TRUE,access_hash=$2 WHERE id=$3`, user.UserTypeBot, botID, botID); err != nil {
		t.Fatal(err)
	}
	token := fmt.Sprintf("pg-export-%d:initial", botID)
	if _, err := pg.Pool.Exec(ctx, `INSERT INTO bots(bot_id,creator_user_id,token) VALUES($1,$2,$3)`, botID, ownerID, token); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = pg.Pool.Exec(ctx, `DELETE FROM bots WHERE bot_id=$1`, botID) })

	c := New(ctx, &svc.ServiceContext{Dao: &userdao.Dao{Postgres: pg}})
	c.MD = &metadata.RpcMetadata{UserId: ownerID}
	if got, err := c.UserExportBotToken(&user.TLUserExportBotToken{BotId: botID}); err != nil || got == nil || got.GetV() != token {
		t.Fatalf("export token = (%v, %v), want original token", got, err)
	}
	if got, err := c.UserExportBotToken(&user.TLUserExportBotToken{BotId: botID, AccessHash: botID + 1}); got != nil || !errors.Is(err, mtproto.ErrUserIdInvalid) {
		t.Fatalf("wrong access hash = (%v, %v), want USER_ID_INVALID", got, err)
	}
	rotatedResult, err := c.UserExportBotToken(&user.TLUserExportBotToken{BotId: botID, Revoke: true})
	if err != nil || rotatedResult == nil || rotatedResult.GetV() == token {
		t.Fatalf("rotated token = (%v, %v), want a new token", rotatedResult, err)
	}
	rotated := rotatedResult.GetV()
	var stored string
	if err := pg.Pool.QueryRow(ctx, `SELECT token FROM bots WHERE bot_id=$1`, botID).Scan(&stored); err != nil || stored != rotated {
		t.Fatalf("stored rotated token = (%q, %v), want %q", stored, err, rotated)
	}
	c.MD = &metadata.RpcMetadata{UserId: otherID}
	if got, err := c.UserExportBotToken(&user.TLUserExportBotToken{BotId: botID}); got != nil || !errors.Is(err, mtproto.ErrForbiddenUserBotInvalid) {
		t.Fatalf("unauthorized export = (%v, %v), want FORBIDDEN_USER_BOT_INVALID", got, err)
	}
}

func TestUserDeactivateChannelUsernamesPostgresRoundTrip(t *testing.T) {
	pg := userPostgresTest(t, 2)
	ctx := context.Background()
	peerID := time.Now().UnixNano()
	if _, err := pg.Pool.Exec(ctx, `INSERT INTO apifull_channel(id,creator_user_id,access_hash,title,created_at) VALUES($1,$1,1,'Deactivate fixture',1)`, peerID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = pg.Pool.Exec(ctx, `DELETE FROM apifull_channel WHERE id=$1`, peerID) })
	names := []string{fmt.Sprintf("pg-basic-%d", peerID), fmt.Sprintf("pg-collectible-%d", peerID), fmt.Sprintf("pg-user-%d", peerID)}
	if _, err := pg.Pool.Exec(ctx, `INSERT INTO username(username,peer_type,peer_id,editable,active) VALUES($1,$2,$3,TRUE,TRUE),($4,$2,$3,FALSE,TRUE),($5,$6,$3,FALSE,TRUE)`, names[0], mtproto.PEER_CHANNEL, peerID, names[1], names[2], mtproto.PEER_USER); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = pg.Pool.Exec(ctx, `DELETE FROM username WHERE username=ANY($1::text[])`, names) })
	c := New(ctx, &svc.ServiceContext{Dao: &userdao.Dao{Postgres: pg}})
	c.MD = &metadata.RpcMetadata{UserId: peerID}
	if got, err := c.UserDeactivateAllChannelUsernames(&user.TLUserDeactivateAllChannelUsernames{ChannelId: peerID}); err != nil || !mtproto.FromBool(got) {
		t.Fatalf("deactivate channel usernames = (%v, %v)", got, err)
	}
	for i, name := range names {
		entry, err := pg.Store.Username.SelectByUsername(ctx, name)
		if err != nil || entry == nil || entry.Deleted || entry.Active != (i != 1) {
			t.Fatalf("username %q after deactivation = (%v, %v)", name, entry, err)
		}
	}
}
