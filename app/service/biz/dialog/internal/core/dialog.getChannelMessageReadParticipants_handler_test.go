package core

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/dialog"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/internal/dao"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/internal/svc"
)

func TestDialogGetChannelMessageReadParticipantsRejectsInvalidInput(t *testing.T) {
	for name, in := range map[string]*dialog.TLDialogGetChannelMessageReadParticipants{
		"nil request":     nil,
		"missing user":    {ChannelId: 1, MsgId: 1},
		"missing channel": {UserId: 1, MsgId: 1},
		"missing message": {UserId: 1, ChannelId: 1},
	} {
		t.Run(name, func(t *testing.T) {
			if got, err := (&DialogCore{}).DialogGetChannelMessageReadParticipants(in); got != nil || err != mtproto.ErrInputRequestInvalid {
				t.Fatalf("result=(%+v, %v), want (nil, INPUT_REQUEST_INVALID)", got, err)
			}
		})
	}
}

func TestDialogGetChannelMessageReadParticipantsFailsClosedWithoutPostgresStore(t *testing.T) {
	request := &dialog.TLDialogGetChannelMessageReadParticipants{UserId: 1, ChannelId: 2, MsgId: 3}
	cores := map[string]*DialogCore{
		"nil core":            nil,
		"nil service context": {svcCtx: nil},
		"nil dao":             {svcCtx: &svc.ServiceContext{}},
		"nil postgres":        {svcCtx: &svc.ServiceContext{Dao: &dao.Dao{}}},
		"nil store":           {svcCtx: &svc.ServiceContext{Dao: &dao.Dao{Postgres: &dao.Postgres{}}}},
	}
	for name, c := range cores {
		t.Run(name, func(t *testing.T) {
			if got, err := c.DialogGetChannelMessageReadParticipants(request); got != nil || err != mtproto.ErrMethodNotImpl {
				t.Fatalf("result=(%+v, %v), want (nil, METHOD_NOT_IMPL)", got, err)
			}
		})
	}
}
