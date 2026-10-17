package core

import (
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/dialog"
	postgres_dao "github.com/teamgram/teamgram-server/app/service/biz/dialog/internal/dal/dao/postgres_dao"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/internal/dao"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/internal/svc"
)

func TestDialogSetHistoryTTLRejectsInvalidRequest(t *testing.T) {
	cases := []struct {
		name string
		in   *dialog.TLDialogSetHistoryTTL
		want error
	}{
		{name: "nil request", want: mtproto.ErrInputRequestInvalid},
		{name: "missing user", in: &dialog.TLDialogSetHistoryTTL{PeerType: mtproto.PEER_USER, PeerId: 2}, want: mtproto.ErrInputRequestInvalid},
		{name: "negative user", in: &dialog.TLDialogSetHistoryTTL{UserId: -1, PeerType: mtproto.PEER_USER, PeerId: 2}, want: mtproto.ErrInputRequestInvalid},
		{name: "missing peer", in: &dialog.TLDialogSetHistoryTTL{UserId: 1, PeerType: mtproto.PEER_USER}, want: mtproto.ErrInputRequestInvalid},
		{name: "negative peer", in: &dialog.TLDialogSetHistoryTTL{UserId: 1, PeerType: mtproto.PEER_USER, PeerId: -1}, want: mtproto.ErrInputRequestInvalid},
		{name: "unknown peer type", in: &dialog.TLDialogSetHistoryTTL{UserId: 1, PeerType: 99, PeerId: 2}, want: mtproto.ErrPeerIdInvalid},
		{name: "negative TTL", in: &dialog.TLDialogSetHistoryTTL{UserId: 1, PeerType: mtproto.PEER_USER, PeerId: 2, TtlPeriod: -1}, want: mtproto.ErrTtlPeriodInvalid},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := (&DialogCore{}).DialogSetHistoryTTL(tc.in)
			if got != nil || err != tc.want {
				t.Fatalf("DialogSetHistoryTTL() = (%v, %v), want (nil, %v)", got, err, tc.want)
			}
		})
	}
}

func TestDialogSetHistoryTTLRequiresPostgresStore(t *testing.T) {
	request := &dialog.TLDialogSetHistoryTTL{UserId: 1, PeerType: mtproto.PEER_USER, PeerId: 2}
	cores := map[string]*DialogCore{
		"nil core":            nil,
		"nil service context": {svcCtx: nil},
		"nil dao":             {svcCtx: &svc.ServiceContext{}},
		"nil postgres":        {svcCtx: &svc.ServiceContext{Dao: &dao.Dao{}}},
		"nil pool":            {svcCtx: &svc.ServiceContext{Dao: &dao.Dao{Postgres: &dao.Postgres{Store: &postgres_dao.Store{}}}}},
		"nil store":           {svcCtx: &svc.ServiceContext{Dao: &dao.Dao{Postgres: &dao.Postgres{Pool: &pgxpool.Pool{}}}}},
		"nil dialogs":         {svcCtx: &svc.ServiceContext{Dao: &dao.Dao{Postgres: &dao.Postgres{Pool: &pgxpool.Pool{}, Store: &postgres_dao.Store{}}}}},
	}

	for name, c := range cores {
		t.Run(name, func(t *testing.T) {
			got, err := c.DialogSetHistoryTTL(request)
			if got != nil || err != mtproto.ErrInternalServerError {
				t.Fatalf("DialogSetHistoryTTL() = (%v, %v), want (nil, INTERNAL_SERVER_ERROR)", got, err)
			}
		})
	}
}
