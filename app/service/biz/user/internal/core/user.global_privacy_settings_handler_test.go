package core

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	mysqldriver "github.com/go-sql-driver/mysql"
	"github.com/teamgram/marmota/pkg/stores/sqlx"
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/user/internal/dal/dao/mysql_dao"
	userdao "github.com/teamgram/teamgram-server/app/service/biz/user/internal/dao"
	"github.com/teamgram/teamgram-server/app/service/biz/user/internal/svc"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
	"github.com/zeromicro/go-zero/core/logx"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func globalPrivacyAuditDB(t *testing.T) *sqlx.DB {
	t.Helper()
	dsn := os.Getenv("APIFULL_MYSQL_DSN")
	if dsn == "" {
		t.Skip("APIFULL_MYSQL_DSN must point to the isolated teamgram_audit database")
	}
	config, err := mysqldriver.ParseDSN(dsn)
	if err != nil {
		t.Fatalf("parse APIFULL_MYSQL_DSN: %v", err)
	}
	if config.DBName != "teamgram_audit" || config.Addr != "127.0.0.1:13306" {
		t.Fatalf("refusing non-audit MySQL target %q at %q", config.DBName, config.Addr)
	}
	return sqlx.NewMySQL(&sqlx.Config{DSN: dsn})
}

func newGlobalPrivacyStorageCore(ctx context.Context, db *sqlx.DB) *UserCore {
	return &UserCore{
		ctx: ctx,
		svcCtx: &svc.ServiceContext{Dao: &userdao.Dao{Mysql: &userdao.Mysql{
			UserGlobalPrivacySettingsDAO: mysql_dao.NewUserGlobalPrivacySettingsDAO(db),
		}}},
		Logger: logx.WithContext(ctx),
	}
}

func userGlobalPrivacySettings() *mtproto.GlobalPrivacySettings {
	return mtproto.MakeTLGlobalPrivacySettings(&mtproto.GlobalPrivacySettings{
		ArchiveAndMuteNewNoncontactPeers_FLAGBOOLEAN: true,
		KeepArchivedUnmuted:                          true,
		KeepArchivedFolders:                          false,
		HideReadMarks:                                true,
		NewNoncontactPeersRequirePremium:             true,
		DisplayGiftsButton:                           true,
		NoncontactPeersPaidStars:                     wrapperspb.Int64(17),
		DisallowedGifts: &mtproto.DisallowedGiftsSettings{
			DisallowUnlimitedStargifts:    true,
			DisallowLimitedStargifts:      false,
			DisallowUniqueStargifts:       true,
			DisallowPremiumGifts:          true,
			DisallowStargiftsFromChannels: false,
		},
	}).To_GlobalPrivacySettings()
}

func TestUserGlobalPrivacySettingsRoundTripInAuditDB(t *testing.T) {
	db := globalPrivacyAuditDB(t)
	ctx := context.Background()
	userID := time.Now().UnixNano()
	core := newGlobalPrivacyStorageCore(ctx, db)
	t.Cleanup(func() {
		_, _ = db.Exec(ctx, "delete from user_global_privacy_settings where user_id = ?", userID)
	})

	settings := userGlobalPrivacySettings()
	saved, err := core.UserSetGlobalPrivacySettings(&userpb.TLUserSetGlobalPrivacySettings{
		UserId:   userID,
		Settings: settings,
	})
	if err != nil || saved == nil || saved.GetPredicateName() != mtproto.Predicate_boolTrue {
		t.Fatalf("UserSetGlobalPrivacySettings() = (%v, %v), want BoolTrue", saved, err)
	}

	got, err := core.UserGetGlobalPrivacySettings(&userpb.TLUserGetGlobalPrivacySettings{UserId: userID})
	if err != nil {
		t.Fatalf("UserGetGlobalPrivacySettings() error = %v", err)
	}
	if got == nil || !got.GetArchiveAndMuteNewNoncontactPeers_FLAGBOOLEAN() || !got.GetKeepArchivedUnmuted() ||
		got.GetKeepArchivedFolders() || !got.GetHideReadMarks() || !got.GetNewNoncontactPeersRequirePremium() ||
		!got.GetDisplayGiftsButton() || got.GetNoncontactPeersPaidStars() == nil ||
		got.GetNoncontactPeersPaidStars().GetValue() != 17 || got.GetDisallowedGifts() == nil ||
		!got.GetDisallowedGifts().GetDisallowUnlimitedStargifts() ||
		!got.GetDisallowedGifts().GetDisallowUniqueStargifts() ||
		!got.GetDisallowedGifts().GetDisallowPremiumGifts() ||
		got.GetDisallowedGifts().GetDisallowLimitedStargifts() ||
		got.GetDisallowedGifts().GetDisallowStargiftsFromChannels() {
		t.Fatalf("read-back settings = %+v, want persisted flags", got)
	}

	cleared := mtproto.MakeTLGlobalPrivacySettings(&mtproto.GlobalPrivacySettings{}).To_GlobalPrivacySettings()
	if saved, err = core.UserSetGlobalPrivacySettings(&userpb.TLUserSetGlobalPrivacySettings{UserId: userID, Settings: cleared}); err != nil || saved == nil {
		t.Fatalf("UserSetGlobalPrivacySettings(clear optional fields) = (%v, %v)", saved, err)
	}
	got, err = core.UserGetGlobalPrivacySettings(&userpb.TLUserGetGlobalPrivacySettings{UserId: userID})
	if err != nil {
		t.Fatalf("UserGetGlobalPrivacySettings(clear optional fields) error = %v", err)
	}
	if got.GetNoncontactPeersPaidStars() != nil || got.GetDisallowedGifts() != nil || got.GetDisplayGiftsButton() {
		t.Fatalf("cleared optional settings = %+v, want nil gifts/stars and false display flag", got)
	}
}

func newGlobalPrivacyDAOErrorCore(t *testing.T) *UserCore {
	t.Helper()
	ctx := context.Background()
	db, err := sqlx.Open(&sqlx.Config{
		DSN: "global_privacy:global_privacy@tcp(127.0.0.1:1)/teamgram_global_privacy_error_test?timeout=50ms&readTimeout=50ms&writeTimeout=50ms",
	})
	if err != nil {
		t.Fatalf("open lazy test database connection: %v", err)
	}
	return newGlobalPrivacyStorageCore(ctx, db)
}

func TestUserGetGlobalPrivacySettingsPropagatesStorageError(t *testing.T) {
	core := newGlobalPrivacyDAOErrorCore(t)
	got, err := core.UserGetGlobalPrivacySettings(&userpb.TLUserGetGlobalPrivacySettings{UserId: 42})
	if got != nil || err == nil {
		t.Fatalf("UserGetGlobalPrivacySettings() = (%v, %v), want nil result and storage error", got, err)
	}
}

func TestUserSetGlobalPrivacySettingsPropagatesStorageError(t *testing.T) {
	core := newGlobalPrivacyDAOErrorCore(t)
	got, err := core.UserSetGlobalPrivacySettings(&userpb.TLUserSetGlobalPrivacySettings{
		UserId:   42,
		Settings: userGlobalPrivacySettings(),
	})
	if got != nil || err == nil {
		t.Fatalf("UserSetGlobalPrivacySettings() = (%v, %v), want nil result and storage error", got, err)
	}
}

func TestUserGlobalPrivacySettingsRejectsNilInputs(t *testing.T) {
	core := &UserCore{}
	if got, err := core.UserGetGlobalPrivacySettings(nil); got != nil || err == nil {
		t.Fatalf("UserGetGlobalPrivacySettings(nil) = (%v, %v), want request error", got, err)
	}
	if got, err := core.UserSetGlobalPrivacySettings(nil); got != nil || err == nil {
		t.Fatalf("UserSetGlobalPrivacySettings(nil) = (%v, %v), want request error", got, err)
	}
	if got, err := core.UserSetGlobalPrivacySettings(&userpb.TLUserSetGlobalPrivacySettings{UserId: 42}); got != nil || err == nil {
		t.Fatalf("UserSetGlobalPrivacySettings(settings=nil) = (%v, %v), want settings error", got, err)
	}
}

func TestGlobalPrivacyOptionalFieldsPreserveNilSemantics(t *testing.T) {
	stored, err := encodeGlobalPrivacyDisallowedGifts(nil)
	if err != nil || stored.Valid {
		t.Fatalf("encode nil disallowed gifts = (%v, %v), want invalid SQL value", stored, err)
	}
	decoded, err := decodeGlobalPrivacyDisallowedGifts(stored)
	if err != nil || decoded != nil {
		t.Fatalf("decode nil disallowed gifts = (%v, %v), want nil", decoded, err)
	}

	want := &mtproto.DisallowedGiftsSettings{
		DisallowUniqueStargifts: true,
	}
	stored, err = encodeGlobalPrivacyDisallowedGifts(want)
	if err != nil || !stored.Valid || stored.String == "" {
		t.Fatalf("encode disallowed gifts = (%v, %v), want JSON SQL value", stored, err)
	}
	decoded, err = decodeGlobalPrivacyDisallowedGifts(stored)
	if err != nil || decoded == nil || !decoded.GetDisallowUniqueStargifts() || decoded.GetDisallowLimitedStargifts() {
		t.Fatalf("decode disallowed gifts = (%v, %v), want unique=true only", decoded, err)
	}

	if _, err = decodeGlobalPrivacyDisallowedGifts(sql.NullString{String: "{", Valid: true}); err == nil {
		t.Fatal("malformed disallowed gifts JSON unexpectedly decoded")
	}
}
