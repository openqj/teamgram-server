package core

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/dialog"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/internal/dal/dao/postgres_dao"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/internal/dao"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/internal/svc"
)

func TestDialogGetDialogsByIdListRejectsInvalidRequestAndProvider(t *testing.T) {
	valid := &dialog.TLDialogGetDialogsByIdList{UserId: 41, IdList: []int64{41}}
	tests := []struct {
		name string
		core *DialogCore
		in   *dialog.TLDialogGetDialogsByIdList
		want error
	}{
		{name: "nil request", in: nil, want: mtproto.ErrInputRequestInvalid},
		{name: "missing user", in: &dialog.TLDialogGetDialogsByIdList{IdList: []int64{41}}, want: mtproto.ErrInputRequestInvalid},
		{name: "zero peer id", in: &dialog.TLDialogGetDialogsByIdList{UserId: 41, IdList: []int64{0}}, want: mtproto.ErrPeerIdInvalid},
		{name: "nil core", core: nil, in: valid, want: mtproto.ErrMethodNotImpl},
		{name: "nil service context", core: &DialogCore{}, in: valid, want: mtproto.ErrMethodNotImpl},
		{name: "nil dao", core: &DialogCore{svcCtx: &svc.ServiceContext{}}, in: valid, want: mtproto.ErrMethodNotImpl},
		{name: "nil postgres", core: &DialogCore{svcCtx: &svc.ServiceContext{Dao: &dao.Dao{}}}, in: valid, want: mtproto.ErrMethodNotImpl},
		{name: "nil store", core: &DialogCore{svcCtx: &svc.ServiceContext{Dao: &dao.Dao{Postgres: &dao.Postgres{}}}}, in: valid, want: mtproto.ErrMethodNotImpl},
		{name: "nil dialogs provider", core: &DialogCore{svcCtx: &svc.ServiceContext{Dao: &dao.Dao{Postgres: &dao.Postgres{Store: &postgres_dao.Store{}}}}}, in: valid, want: mtproto.ErrMethodNotImpl},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.core.DialogGetDialogsByIdList(tt.in)
			if got != nil || err != tt.want {
				t.Fatalf("DialogGetDialogsByIdList() = (%v, %v), want (nil, %v)", got, err, tt.want)
			}
		})
	}
}
