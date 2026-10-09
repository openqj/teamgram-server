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
)

func TestUserPostgresPrivacySnapshotContext(t *testing.T) {
	pg := userPostgresTest(t, 1)
	ctx := context.Background()
	ownerID := time.Now().UnixNano()
	peerID, premiumID, botID, chatID := ownerID+1, ownerID+2, ownerID+3, ownerID+4
	insertUserPostgresFixtures(t, pg, ownerID, peerID, premiumID, botID)
	if _, err := pg.Pool.Exec(ctx, `UPDATE users SET premium=TRUE,premium_expire_date=$2 WHERE id=$1`, premiumID, time.Now().Add(time.Hour).Unix()); err != nil {
		t.Fatal(err)
	}
	if _, err := pg.Pool.Exec(ctx, `UPDATE users SET user_type=$2,is_bot=TRUE WHERE id=$1`, botID, user.UserTypeBot); err != nil {
		t.Fatal(err)
	}
	if _, err := pg.Pool.Exec(ctx, `INSERT INTO user_contacts(owner_user_id,contact_user_id,close_friend) VALUES($1,$2,TRUE)`, ownerID, peerID); err != nil {
		t.Fatal(err)
	}
	if _, err := pg.Pool.Exec(ctx, `INSERT INTO chats(id,creator_user_id,access_hash,random_id) VALUES($1,$2,1,$1)`, chatID, ownerID); err != nil {
		t.Fatal(err)
	}
	if _, err := pg.Pool.Exec(ctx, `INSERT INTO chat_participants(chat_id,user_id,state) VALUES($1,$2,$3)`, chatID, peerID, mtproto.ChatMemberStateNormal); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = pg.Pool.Exec(ctx, `DELETE FROM chat_participants WHERE chat_id=$1`, chatID)
		_, _ = pg.Pool.Exec(ctx, `DELETE FROM chats WHERE id=$1`, chatID)
	})
	c := New(ctx, &svc.ServiceContext{Dao: &userdao.Dao{Postgres: pg}})
	rules := map[int32][]*mtproto.PrivacyRule{
		mtproto.PHONE_NUMBER:  {mtproto.MakeTLPrivacyValueDisallowAll(nil).To_PrivacyRule(), mtproto.MakeTLPrivacyValueAllowPremium(nil).To_PrivacyRule()},
		mtproto.PROFILE_PHOTO: {mtproto.MakeTLPrivacyValueDisallowAll(nil).To_PrivacyRule(), mtproto.MakeTLPrivacyValueAllowBots(nil).To_PrivacyRule()},
		mtproto.ABOUT:         {mtproto.MakeTLPrivacyValueDisallowAll(nil).To_PrivacyRule(), mtproto.MakeTLPrivacyValueAllowCloseFriends(nil).To_PrivacyRule()},
		mtproto.BIRTHDAY:      {mtproto.MakeTLPrivacyValueAllowChatParticipants(&mtproto.PrivacyRule{Chats: []int64{chatID}}).To_PrivacyRule(), mtproto.MakeTLPrivacyValueDisallowAll(nil).To_PrivacyRule()},
	}
	for key, list := range rules {
		if _, err := c.UserSetPrivacy(&user.TLUserSetPrivacy{UserId: ownerID, KeyType: key, Rules: list}); err != nil {
			t.Fatal(err)
		}
	}
	viewers := []int64{peerID, premiumID, botID, ownerID + 99}
	assertSnapshot := func(snapshot *mtproto.ImmutableUser) {
		t.Helper()
		for _, id := range viewers {
			for key, want := range map[int32]bool{
				mtproto.PHONE_NUMBER: id == premiumID, mtproto.PROFILE_PHOTO: id == botID,
				mtproto.ABOUT: id == peerID, mtproto.BIRTHDAY: id == peerID,
			} {
				if got := snapshot.CheckPrivacy(int(key), id); got != want {
					t.Fatalf("snapshot key %d for %d = %v, want %v", key, id, got, want)
				}
			}
		}
		if snapshot.ToUser(premiumID).GetPhone() == nil || snapshot.ToUser(peerID).GetPhone() != nil {
			t.Fatal("ID-only user rendering ignored prepared premium decisions")
		}
	}
	one, err := c.UserGetImmutableUser(&user.TLUserGetImmutableUser{Id: ownerID, Privacy: true, Contacts: viewers})
	if err != nil {
		t.Fatal(err)
	}
	assertSnapshot(one)
	oneV2, err := c.UserGetImmutableUserV2(&user.TLUserGetImmutableUserV2{Id: ownerID, Privacy: true, To: viewers})
	if err != nil {
		t.Fatal(err)
	}
	assertSnapshot(oneV2)
	for _, ids := range [][]int64{{ownerID}, {ownerID, peerID}} {
		batch, err := c.UserGetMutableUsers(&user.TLUserGetMutableUsers{Id: ids, To: viewers})
		if err != nil {
			t.Fatal(err)
		}
		entry, ok := batch.GetImmutableUser(ownerID)
		if !ok {
			t.Fatal("legacy batch missed privacy owner")
		}
		assertSnapshot(entry)
		batchV2, err := c.UserGetMutableUsersV2(&user.TLUserGetMutableUsersV2{Id: ids, Privacy: true, To: viewers})
		if err != nil {
			t.Fatal(err)
		}
		entryV2, ok := batchV2.GetImmutableUser(ownerID)
		if !ok {
			t.Fatal("V2 batch missed privacy owner")
		}
		assertSnapshot(entryV2)
	}
	for key, list := range rules {
		raw, err := c.UserGetPrivacy(&user.TLUserGetPrivacy{UserId: ownerID, KeyType: key})
		if err != nil || len(raw.GetDatas()) != len(list) || raw.GetDatas()[0].GetPredicateName() != list[0].GetPredicateName() || raw.GetDatas()[1].GetPredicateName() != list[1].GetPredicateName() {
			t.Fatalf("snapshot read changed raw rules: (%v, %v)", raw, err)
		}
	}
	if _, err := pg.Pool.Exec(ctx, `UPDATE users SET premium_expire_date=1 WHERE id=$1`, premiumID); err != nil {
		t.Fatal(err)
	}
	if _, err := pg.Pool.Exec(ctx, `UPDATE user_contacts SET close_friend=FALSE WHERE owner_user_id=$1 AND contact_user_id=$2`, ownerID, peerID); err != nil {
		t.Fatal(err)
	}
	if _, err := pg.Pool.Exec(ctx, `UPDATE chat_participants SET state=$3 WHERE chat_id=$1 AND user_id=$2`, chatID, peerID, mtproto.ChatMemberStateLeft); err != nil {
		t.Fatal(err)
	}
	refreshed, err := c.UserGetImmutableUser(&user.TLUserGetImmutableUser{Id: ownerID, Privacy: true, Contacts: viewers})
	if err != nil {
		t.Fatal(err)
	}
	if refreshed.CheckPrivacy(mtproto.PHONE_NUMBER, premiumID) || refreshed.CheckPrivacy(mtproto.ABOUT, peerID) || refreshed.CheckPrivacy(mtproto.BIRTHDAY, peerID) {
		t.Fatal("refreshed snapshot retained expired permission")
	}
	if _, err := pg.Pool.Exec(ctx, `UPDATE user_privacies SET rules='null' WHERE user_id=$1 AND key_type=$2`, ownerID, mtproto.PHONE_NUMBER); err != nil {
		t.Fatal(err)
	}
	if got, err := c.UserGetImmutableUser(&user.TLUserGetImmutableUser{Id: ownerID, Privacy: true, Contacts: viewers}); got != nil || !errors.Is(err, mtproto.ErrPrivacyValueInvalid) {
		t.Fatalf("null stored rules = (%v, %v), want error", got, err)
	}
}

func TestUserPostgresPrivacySnapshotReverseRecipients(t *testing.T) {
	pg := userPostgresTest(t, 1)
	ctx := context.Background()
	ownerID := time.Now().UnixNano()
	premiumID, otherID := ownerID+1, ownerID+2
	insertUserPostgresFixtures(t, pg, ownerID, premiumID, otherID)
	if _, err := pg.Pool.Exec(ctx, `UPDATE users SET premium=TRUE WHERE id=$1`, premiumID); err != nil {
		t.Fatal(err)
	}
	if _, err := pg.Pool.Exec(ctx, `INSERT INTO user_contacts(owner_user_id,contact_user_id) VALUES($1,$2)`, premiumID, ownerID); err != nil {
		t.Fatal(err)
	}
	c := New(ctx, &svc.ServiceContext{Dao: &userdao.Dao{Postgres: pg}})
	if _, err := c.UserSetPrivacy(&user.TLUserSetPrivacy{UserId: ownerID, KeyType: mtproto.PHONE_NUMBER, Rules: []*mtproto.PrivacyRule{
		mtproto.MakeTLPrivacyValueDisallowAll(nil).To_PrivacyRule(),
		mtproto.MakeTLPrivacyValueAllowPremium(nil).To_PrivacyRule(),
	}}); err != nil {
		t.Fatal(err)
	}
	for _, ids := range [][]int64{{ownerID}, {ownerID, otherID}} {
		batch, err := c.UserGetMutableUsersV2(&user.TLUserGetMutableUsersV2{Id: ids, Privacy: true, HasTo: true})
		if err != nil {
			t.Fatal(err)
		}
		entry, exists := batch.GetImmutableUser(ownerID)
		if !exists || len(entry.GetReverseContacts()) != 1 || entry.ToUser(premiumID).GetPhone() == nil {
			t.Fatalf("reverse recipient %v = %v, want resolved Premium exception", ids, batch)
		}
	}
}

func TestUserPostgresNoPaidMessageDefaultAndExceptions(t *testing.T) {
	pg := userPostgresTest(t, 1)
	ctx := context.Background()
	ownerID := time.Now().UnixNano()
	peerID := ownerID + 1
	insertUserPostgresFixtures(t, pg, ownerID, peerID)
	c := New(ctx, &svc.ServiceContext{Dao: &userdao.Dao{Postgres: pg}})
	assertExemption := func(want bool) {
		t.Helper()
		got, err := c.UserCheckPrivacy(&user.TLUserCheckPrivacy{UserId: ownerID, KeyType: mtproto.NO_PAID_MESSAGES, PeerId: peerID})
		if err != nil || got == nil || mtproto.FromBool(got) != want {
			stored, _ := pg.Store.Privacies.SelectPrivacy(ctx, ownerID, mtproto.NO_PAID_MESSAGES)
			t.Fatalf("paid exemption = (%v, %v), want %v, stored %v", got, err, want, stored)
		}
		snapshot, err := c.UserGetImmutableUser(&user.TLUserGetImmutableUser{Id: ownerID, Privacy: true, Contacts: []int64{peerID}})
		if err != nil || snapshot.CheckPrivacy(mtproto.NO_PAID_MESSAGES, peerID) != want {
			t.Fatalf("paid exemption snapshot = (%v, %v), want %v", snapshot, err, want)
		}
	}
	assertExemption(false)
	if _, err := c.UserSetPrivacy(&user.TLUserSetPrivacy{UserId: ownerID, KeyType: mtproto.NO_PAID_MESSAGES, Rules: []*mtproto.PrivacyRule{
		mtproto.MakeTLPrivacyValueAllowUsers(&mtproto.PrivacyRule{Users: []int64{peerID}}).To_PrivacyRule(),
		mtproto.MakeTLPrivacyValueDisallowAll(nil).To_PrivacyRule(),
	}}); err != nil {
		t.Fatal(err)
	}
	assertExemption(true)
	if _, err := c.UserSetPrivacy(&user.TLUserSetPrivacy{UserId: ownerID, KeyType: mtproto.NO_PAID_MESSAGES, Rules: []*mtproto.PrivacyRule{
		mtproto.MakeTLPrivacyValueDisallowAll(nil).To_PrivacyRule(),
	}}); err != nil {
		t.Fatal(err)
	}
	assertExemption(false)
	if _, err := pg.Pool.Exec(ctx, `UPDATE users SET deleted=TRUE WHERE id=$1`, peerID); err != nil {
		t.Fatal(err)
	}
	if got, err := c.UserCheckPrivacy(&user.TLUserCheckPrivacy{UserId: ownerID, KeyType: mtproto.NO_PAID_MESSAGES, PeerId: peerID}); got != nil || !errors.Is(err, mtproto.ErrUserIdInvalid) {
		t.Fatalf("deleted paid-exemption viewer = (%v, %v), want USER_ID_INVALID", got, err)
	}
}

func TestUserPostgresPrivacySnapshotPropagatesMembershipFailure(t *testing.T) {
	pg := userPostgresTest(t, 1)
	ctx := context.Background()
	ownerID := time.Now().UnixNano()
	peerID := ownerID + 1
	insertUserPostgresFixtures(t, pg, ownerID, peerID)
	c := New(ctx, &svc.ServiceContext{Dao: &userdao.Dao{Postgres: pg}})
	if _, err := c.UserSetPrivacy(&user.TLUserSetPrivacy{UserId: ownerID, KeyType: mtproto.ABOUT, Rules: []*mtproto.PrivacyRule{
		mtproto.MakeTLPrivacyValueDisallowChatParticipants(&mtproto.PrivacyRule{Chats: []int64{ownerID + 99}}).To_PrivacyRule(),
		mtproto.MakeTLPrivacyValueAllowAll(nil).To_PrivacyRule(),
	}}); err != nil {
		t.Fatal(err)
	}
	if _, err := pg.Pool.Exec(ctx, `CREATE TEMP TABLE chat_participants(unrelated bigint)`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = pg.Pool.Exec(ctx, `DROP TABLE pg_temp.chat_participants`) })
	for _, tc := range []struct {
		name string
		call func() (any, error)
	}{
		{"single", func() (any, error) {
			got, err := c.UserGetImmutableUser(&user.TLUserGetImmutableUser{Id: ownerID, Privacy: true, Contacts: []int64{peerID}})
			return got, err
		}},
		{"single V2", func() (any, error) {
			got, err := c.UserGetImmutableUserV2(&user.TLUserGetImmutableUserV2{Id: ownerID, Privacy: true, To: []int64{peerID}})
			return got, err
		}},
		{"batch", func() (any, error) {
			got, err := c.UserGetMutableUsers(&user.TLUserGetMutableUsers{Id: []int64{ownerID, peerID}, To: []int64{peerID}})
			return got, err
		}},
		{"batch V2", func() (any, error) {
			got, err := c.UserGetMutableUsersV2(&user.TLUserGetMutableUsersV2{Id: []int64{ownerID, peerID}, Privacy: true, To: []int64{peerID}})
			return got, err
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := tc.call()
			if err == nil {
				t.Fatalf("snapshot membership failure = (%v, %v), want query error", got, err)
			}
		})
	}
}
