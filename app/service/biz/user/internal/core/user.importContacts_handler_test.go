package core

import (
	"context"
	"errors"
	"testing"

	"github.com/teamgram/marmota/pkg/stores/sqlx"
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/user/internal/dal/dao/mysql_dao"
	userdao "github.com/teamgram/teamgram-server/app/service/biz/user/internal/dao"
	"github.com/teamgram/teamgram-server/app/service/biz/user/internal/svc"
	"github.com/teamgram/teamgram-server/app/service/biz/user/user"
	"github.com/zeromicro/go-zero/core/logx"
)

func TestUserImportContactsEmptyVectorIsNoOp(t *testing.T) {
	got, err := (&UserCore{}).UserImportContacts(&user.TLUserImportContacts{UserId: 1})
	if err != nil || got == nil || len(got.GetImported()) != 0 || len(got.GetUsers()) != 0 {
		t.Fatalf("UserImportContacts(empty) = (%+v, %v), want empty success", got, err)
	}
}

func TestUserImportContactsRejectsMalformedInput(t *testing.T) {
	valid := mtproto.MakeTLInputPhoneContact(&mtproto.InputContact{ClientId: 1, Phone: "+12025550102"}).To_InputContact()
	tests := []struct {
		name string
		in   *user.TLUserImportContacts
	}{
		{name: "nil request"},
		{name: "missing owner", in: &user.TLUserImportContacts{Contacts: []*mtproto.InputContact{valid}}},
		{name: "nil contact", in: &user.TLUserImportContacts{UserId: 1, Contacts: []*mtproto.InputContact{nil}}},
		{name: "duplicate phone", in: &user.TLUserImportContacts{UserId: 1, Contacts: []*mtproto.InputContact{
			valid,
			mtproto.MakeTLInputPhoneContact(&mtproto.InputContact{ClientId: 2, Phone: "+12025550102"}).To_InputContact(),
		}}},
		{name: "duplicate client id", in: &user.TLUserImportContacts{UserId: 1, Contacts: []*mtproto.InputContact{
			valid,
			mtproto.MakeTLInputPhoneContact(&mtproto.InputContact{ClientId: 1, Phone: "+12025550103"}).To_InputContact(),
		}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := (&UserCore{}).UserImportContacts(tt.in)
			if got != nil || !errors.Is(err, mtproto.ErrInputRequestInvalid) {
				t.Fatalf("UserImportContacts() = (%+v, %v), want INPUT_REQUEST_INVALID", got, err)
			}
		})
	}
}

func TestPrepareImportContactsNormalizesE164ForUserLookup(t *testing.T) {
	contact := mtproto.MakeTLInputPhoneContact(&mtproto.InputContact{
		ClientId: 1,
		Phone:    "+12025550102",
	}).To_InputContact()

	items, itemByPhone, phones, err := prepareImportContacts([]*mtproto.InputContact{contact})
	if err != nil {
		t.Fatalf("prepareImportContacts() error = %v", err)
	}
	if len(phones) != 1 || phones[0] != "12025550102" {
		t.Fatalf("lookup phones = %v, want fixture digits [12025550102]", phones)
	}
	if items[0].C.GetPhone() != "12025550102" {
		t.Fatalf("contact phone = %q, want canonical fixture digits", items[0].C.GetPhone())
	}
	if itemByPhone["12025550102"] != items[0] {
		t.Fatalf("normalized lookup key does not point to the imported contact")
	}
}

func TestPrepareImportContactsRejectsDuplicateAfterNormalization(t *testing.T) {
	contacts := []*mtproto.InputContact{
		mtproto.MakeTLInputPhoneContact(&mtproto.InputContact{ClientId: 1, Phone: "+12025550102"}).To_InputContact(),
		mtproto.MakeTLInputPhoneContact(&mtproto.InputContact{ClientId: 2, Phone: "+1 202 555 0102"}).To_InputContact(),
	}
	if _, _, _, err := prepareImportContacts(contacts); !errors.Is(err, mtproto.ErrInputRequestInvalid) {
		t.Fatalf("prepareImportContacts() error = %v, want INPUT_REQUEST_INVALID", err)
	}
}

func TestPrepareImportContactsRejectsInvalidPhone(t *testing.T) {
	contact := mtproto.MakeTLInputPhoneContact(&mtproto.InputContact{ClientId: 1, Phone: "+123"}).To_InputContact()
	if _, _, _, err := prepareImportContacts([]*mtproto.InputContact{contact}); !errors.Is(err, mtproto.ErrPhoneNumberInvalid) {
		t.Fatalf("prepareImportContacts() error = %v, want PHONE_NUMBER_INVALID", err)
	}
}

func TestPrepareImportContactsNormalizesSpecialPhone(t *testing.T) {
	contact := mtproto.MakeTLInputPhoneContact(&mtproto.InputContact{ClientId: 1, Phone: "+42400"}).To_InputContact()
	items, _, phones, err := prepareImportContacts([]*mtproto.InputContact{contact})
	if err != nil {
		t.Fatalf("prepareImportContacts() error = %v", err)
	}
	if phones[0] != "42400" || items[0].C.GetPhone() != "42400" {
		t.Fatalf("special phone = (%q, %q), want canonical 42400", phones[0], items[0].C.GetPhone())
	}
}

func TestMakePopularInvitesPreservesInputOrderAndCounts(t *testing.T) {
	items := []*contactItem{
		{Unregistered: true, C: mtproto.MakeTLInputPhoneContact(&mtproto.InputContact{ClientId: 71, Phone: "+101"}).To_InputContact()},
		{Unregistered: false, C: mtproto.MakeTLInputPhoneContact(&mtproto.InputContact{ClientId: 72, Phone: "+102"}).To_InputContact()},
		{Unregistered: true, C: mtproto.MakeTLInputPhoneContact(&mtproto.InputContact{ClientId: 73, Phone: "+103"}).To_InputContact()},
	}

	got := makePopularInvites(items, map[string]int32{"+101": 4, "+103": 2})
	if len(got) != 2 {
		t.Fatalf("popular invites = %#v, want two unregistered contacts", got)
	}
	if got[0].GetClientId() != 71 || got[0].GetImporters() != 4 || got[1].GetClientId() != 73 || got[1].GetImporters() != 2 {
		t.Fatalf("popular invites = %#v, want [(client_id=71, importers=4), (client_id=73, importers=2)]", got)
	}
}

func TestUserImportContactsPopularInvitesPropagatesCountQueryError(t *testing.T) {
	ctx := context.Background()
	db, err := sqlx.Open(&sqlx.Config{
		DSN: "popular:popular@tcp(127.0.0.1:1)/teamgram_popular_error_test?timeout=50ms&readTimeout=50ms&writeTimeout=50ms",
	})
	if err != nil {
		t.Fatalf("open lazy test database connection: %v", err)
	}
	core := &UserCore{
		ctx: ctx,
		svcCtx: &svc.ServiceContext{
			Dao: &userdao.Dao{Mysql: &userdao.Mysql{
				UnregisteredContactsDAO: mysql_dao.NewUnregisteredContactsDAO(db),
			}},
		},
		Logger: logx.WithContext(ctx),
	}
	items := []*contactItem{{
		Unregistered: true,
		C:            mtproto.MakeTLInputPhoneContact(&mtproto.InputContact{ClientId: 71, Phone: "+101"}).To_InputContact(),
	}}
	got, err := core.getPopularInvites(items)
	if err == nil || got != nil {
		t.Fatalf("getPopularInvites() = (%v, %v), want nil result and count query error", got, err)
	}
}
