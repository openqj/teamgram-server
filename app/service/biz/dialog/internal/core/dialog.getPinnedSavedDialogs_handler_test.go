package core

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/dialog"
	postgres_dao "github.com/teamgram/teamgram-server/app/service/biz/dialog/internal/dal/dao/postgres_dao"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/internal/dao"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/internal/svc"
)

func TestDialogGetPinnedSavedDialogsRejectsInvalidRequest(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   *dialog.TLDialogGetPinnedSavedDialogs
	}{
		{name: "nil request"},
		{name: "missing user", in: &dialog.TLDialogGetPinnedSavedDialogs{}},
		{name: "negative user", in: &dialog.TLDialogGetPinnedSavedDialogs{UserId: -1}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := (&DialogCore{}).DialogGetPinnedSavedDialogs(tc.in)
			if got != nil || err != mtproto.ErrInputRequestInvalid {
				t.Fatalf("DialogGetPinnedSavedDialogs() = (%v, %v), want (nil, INPUT_REQUEST_INVALID)", got, err)
			}
		})
	}
}

func TestDialogGetPinnedSavedDialogsRequiresPostgresStore(t *testing.T) {
	request := &dialog.TLDialogGetPinnedSavedDialogs{UserId: 1}
	cores := map[string]*DialogCore{
		"nil core":            nil,
		"nil service context": {svcCtx: nil},
		"nil dao":             {svcCtx: &svc.ServiceContext{}},
		"nil postgres":        {svcCtx: &svc.ServiceContext{Dao: &dao.Dao{}}},
		"nil store":           {svcCtx: &svc.ServiceContext{Dao: &dao.Dao{Postgres: &dao.Postgres{}}}},
		"nil saved dialogs":   {svcCtx: &svc.ServiceContext{Dao: &dao.Dao{Postgres: &dao.Postgres{Store: &postgres_dao.Store{}}}}},
	}

	for name, c := range cores {
		t.Run(name, func(t *testing.T) {
			got, err := c.DialogGetPinnedSavedDialogs(request)
			if got != nil || err != mtproto.ErrMethodNotImpl {
				t.Fatalf("DialogGetPinnedSavedDialogs() = (%v, %v), want (nil, METHOD_NOT_IMPL)", got, err)
			}
		})
	}
}
