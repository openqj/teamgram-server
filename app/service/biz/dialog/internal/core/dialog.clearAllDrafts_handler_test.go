package core

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/dialog"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/internal/dao"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/internal/svc"
)

func TestDialogClearAllDraftsRejectsInvalidRequest(t *testing.T) {
	cases := []struct {
		name string
		in   *dialog.TLDialogClearAllDrafts
	}{
		{name: "nil request"},
		{name: "missing user", in: &dialog.TLDialogClearAllDrafts{}},
		{name: "negative user", in: &dialog.TLDialogClearAllDrafts{UserId: -1}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := (&DialogCore{}).DialogClearAllDrafts(tc.in)
			if got != nil || err != mtproto.ErrInputRequestInvalid {
				t.Fatalf("DialogClearAllDrafts() = (%v, %v), want (nil, INPUT_REQUEST_INVALID)", got, err)
			}
		})
	}
}

func TestDialogClearAllDraftsRequiresPostgresStore(t *testing.T) {
	request := &dialog.TLDialogClearAllDrafts{UserId: 1}
	cores := map[string]*DialogCore{
		"nil core":            nil,
		"nil service context": {svcCtx: nil},
		"nil dao":             {svcCtx: &svc.ServiceContext{}},
		"nil postgres":        {svcCtx: &svc.ServiceContext{Dao: &dao.Dao{}}},
		"nil store":           {svcCtx: &svc.ServiceContext{Dao: &dao.Dao{Postgres: &dao.Postgres{}}}},
	}

	for name, c := range cores {
		t.Run(name, func(t *testing.T) {
			got, err := c.DialogClearAllDrafts(request)
			if got != nil || err != mtproto.ErrMethodNotImpl {
				t.Fatalf("DialogClearAllDrafts() = (%v, %v), want (nil, METHOD_NOT_IMPL)", got, err)
			}
		})
	}
}
