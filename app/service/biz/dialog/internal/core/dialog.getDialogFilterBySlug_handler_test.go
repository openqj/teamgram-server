package core

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/dialog"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/internal/dao"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/internal/svc"
)

func TestDialogGetDialogFilterBySlugRejectsInvalidRequest(t *testing.T) {
	cases := []struct {
		name string
		in   *dialog.TLDialogGetDialogFilterBySlug
		want error
	}{
		{name: "nil request", want: mtproto.ErrInputRequestInvalid},
		{name: "missing user", in: &dialog.TLDialogGetDialogFilterBySlug{Slug: "shared"}, want: mtproto.ErrInputRequestInvalid},
		{name: "empty slug", in: &dialog.TLDialogGetDialogFilterBySlug{UserId: 1}, want: mtproto.ErrInviteSlugEmpty},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := (&DialogCore{}).DialogGetDialogFilterBySlug(tc.in)
			if got != nil || err != tc.want {
				t.Fatalf("DialogGetDialogFilterBySlug() = (%v, %v), want (nil, %v)", got, err, tc.want)
			}
		})
	}
}

func TestDialogGetDialogFilterBySlugRequiresPostgresStore(t *testing.T) {
	request := &dialog.TLDialogGetDialogFilterBySlug{UserId: 1, Slug: "shared"}
	cores := map[string]*DialogCore{
		"nil core":            nil,
		"nil service context": {svcCtx: nil},
		"nil dao":             {svcCtx: &svc.ServiceContext{}},
		"nil postgres":        {svcCtx: &svc.ServiceContext{Dao: &dao.Dao{}}},
		"nil store":           {svcCtx: &svc.ServiceContext{Dao: &dao.Dao{Postgres: &dao.Postgres{}}}},
	}

	for name, c := range cores {
		t.Run(name, func(t *testing.T) {
			got, err := c.DialogGetDialogFilterBySlug(request)
			if got != nil || err != mtproto.ErrMethodNotImpl {
				t.Fatalf("DialogGetDialogFilterBySlug() = (%v, %v), want (nil, METHOD_NOT_IMPL)", got, err)
			}
		})
	}
}
