package core

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/dialog"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/internal/dao"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/internal/svc"
)

func TestDialogGetSavedDialogsRejectsInvalidRequest(t *testing.T) {
	cases := []struct {
		name string
		in   *dialog.TLDialogGetSavedDialogs
		want error
	}{
		{name: "nil request", want: mtproto.ErrInputRequestInvalid},
		{name: "missing user", in: &dialog.TLDialogGetSavedDialogs{}, want: mtproto.ErrInputRequestInvalid},
		{name: "negative user", in: &dialog.TLDialogGetSavedDialogs{UserId: -1}, want: mtproto.ErrInputRequestInvalid},
		{name: "negative offset date", in: &dialog.TLDialogGetSavedDialogs{UserId: 1, OffsetDate: -1}, want: mtproto.ErrOffsetInvalid},
		{name: "negative offset id", in: &dialog.TLDialogGetSavedDialogs{UserId: 1, OffsetId: -1}, want: mtproto.ErrOffsetInvalid},
		{name: "negative limit", in: &dialog.TLDialogGetSavedDialogs{UserId: 1, Limit: -1}, want: mtproto.ErrLimitInvalid},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := (&DialogCore{}).DialogGetSavedDialogs(tc.in)
			if got != nil || err != tc.want {
				t.Fatalf("DialogGetSavedDialogs() = (%v, %v), want (nil, %v)", got, err, tc.want)
			}
		})
	}
}

func TestDialogGetSavedDialogsRequiresPostgresStore(t *testing.T) {
	request := &dialog.TLDialogGetSavedDialogs{UserId: 1}
	cores := map[string]*DialogCore{
		"nil core":            nil,
		"nil service context": {svcCtx: nil},
		"nil dao":             {svcCtx: &svc.ServiceContext{}},
		"nil postgres":        {svcCtx: &svc.ServiceContext{Dao: &dao.Dao{}}},
		"nil store":           {svcCtx: &svc.ServiceContext{Dao: &dao.Dao{Postgres: &dao.Postgres{}}}},
	}

	for name, c := range cores {
		t.Run(name, func(t *testing.T) {
			got, err := c.DialogGetSavedDialogs(request)
			if got != nil || err != mtproto.ErrMethodNotImpl {
				t.Fatalf("DialogGetSavedDialogs() = (%v, %v), want (nil, METHOD_NOT_IMPL)", got, err)
			}
		})
	}
}
