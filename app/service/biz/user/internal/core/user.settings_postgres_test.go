package core

import (
	"context"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/service/biz/user/internal/dal/dataobject"
	userdao "github.com/teamgram/teamgram-server/app/service/biz/user/internal/dao"
	"github.com/teamgram/teamgram-server/app/service/biz/user/internal/svc"
	"github.com/teamgram/teamgram-server/app/service/biz/user/user"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func TestUserPostgresSettingsAndImporterRoundTrip(t *testing.T) {
	pg := userPostgresTest(t, 2)
	ctx := context.Background()
	userID := time.Now().UnixNano()
	phone := "pg-importer-" + time.Now().Format("150405.000000000")
	if _, err := pg.Pool.Exec(ctx, `INSERT INTO users (id, phone) VALUES ($1, $2)`, userID, phone); err != nil {
		t.Fatalf("seed user = %v", err)
	}
	t.Cleanup(func() {
		_, _ = pg.Pool.Exec(ctx, `DELETE FROM users WHERE id=$1`, userID)
		_, _ = pg.Pool.Exec(ctx, `DELETE FROM user_settings WHERE user_id=$1`, userID)
		_, _ = pg.Pool.Exec(ctx, `DELETE FROM default_history_ttl WHERE user_id=$1`, userID)
		_, _ = pg.Pool.Exec(ctx, `DELETE FROM user_global_privacy_settings WHERE user_id=$1`, userID)
		_, _ = pg.Pool.Exec(ctx, `DELETE FROM unregistered_contacts WHERE phone=$1`, phone)
	})

	c := New(ctx, &svc.ServiceContext{Dao: &userdao.Dao{Postgres: pg}})
	c.MD = nil

	if got, err := c.UserSetContentSettings(&user.TLUserSetContentSettings{UserId: userID, SensitiveEnabled: true}); err != nil || !mtproto.FromBool(got) {
		t.Fatalf("set content settings = (%v, %v)", got, err)
	}
	if got, err := c.UserGetContentSettings(&user.TLUserGetContentSettings{UserId: userID}); err != nil || got == nil || !got.GetSensitiveEnabled() {
		t.Fatalf("get content settings = (%v, %v), want enabled", got, err)
	}

	if got, err := c.UserSetDefaultHistoryTTL(&user.TLUserSetDefaultHistoryTTL{UserId: userID, Ttl: 604800}); err != nil || !mtproto.FromBool(got) {
		t.Fatalf("set default history ttl = (%v, %v)", got, err)
	}
	if got, err := c.UserGetDefaultHistoryTTL(&user.TLUserGetDefaultHistoryTTL{UserId: userID}); err != nil || got == nil || got.GetPeriod() != 604800 {
		t.Fatalf("get default history ttl = (%v, %v), want 604800", got, err)
	}

	settings := mtproto.MakeTLGlobalPrivacySettings(&mtproto.GlobalPrivacySettings{
		ArchiveAndMuteNewNoncontactPeers_FLAGBOOLEAN: true,
		KeepArchivedUnmuted:                          true,
		HideReadMarks:                                true,
		DisplayGiftsButton:                           true,
		NoncontactPeersPaidStars:                     wrapperspb.Int64(17),
	}).To_GlobalPrivacySettings()
	if got, err := c.UserSetGlobalPrivacySettings(&user.TLUserSetGlobalPrivacySettings{UserId: userID, Settings: settings}); err != nil || !mtproto.FromBool(got) {
		t.Fatalf("set global privacy = (%v, %v)", got, err)
	}
	gotPrivacy, err := c.UserGetGlobalPrivacySettings(&user.TLUserGetGlobalPrivacySettings{UserId: userID})
	if err != nil || gotPrivacy == nil || !gotPrivacy.GetArchiveAndMuteNewNoncontactPeers_FLAGBOOLEAN() ||
		!gotPrivacy.GetKeepArchivedUnmuted() || !gotPrivacy.GetHideReadMarks() || !gotPrivacy.GetDisplayGiftsButton() ||
		gotPrivacy.GetNoncontactPeersPaidStars() == nil || gotPrivacy.GetNoncontactPeersPaidStars().GetValue() != 17 {
		t.Fatalf("get global privacy = (%v, %v), want persisted fields", gotPrivacy, err)
	}

	if _, _, err := pg.Store.Unregistered.InsertOrUpdate(ctx, &dataobject.UnregisteredContactsDO{
		Phone: phone, ImporterUserId: userID, ImportFirstName: "Importer", ImportLastName: "User",
	}); err != nil {
		t.Fatalf("insert unregistered contact = %v", err)
	}
	c.MD = &metadata.RpcMetadata{UserId: userID}
	contacts, err := c.UserGetImportersByPhone(&user.TLUserGetImportersByPhone{Phone: phone})
	if err != nil || contacts == nil || len(contacts.GetDatas()) != 1 || contacts.GetDatas()[0].GetClientId() != userID {
		t.Fatalf("get importers = (%v, %v), want importer %d", contacts, err, userID)
	}
	if got, err := c.UserDeleteImportersByPhone(&user.TLUserDeleteImportersByPhone{Phone: phone}); err != nil || !mtproto.FromBool(got) {
		t.Fatalf("delete importer = (%v, %v)", got, err)
	}
	contacts, err = c.UserGetImportersByPhone(&user.TLUserGetImportersByPhone{Phone: phone})
	if err != nil || contacts == nil || len(contacts.GetDatas()) != 0 {
		t.Fatalf("get importers after delete = (%v, %v), want empty", contacts, err)
	}
}
