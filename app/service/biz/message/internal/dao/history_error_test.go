package dao

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/teamgram/marmota/pkg/stores/sqlx"
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/message/internal/dal/dao/mysql_dao"
)

func TestGetOffsetIdBackwardHistoryMessagesPropagatesDAOError(t *testing.T) {
	daos := newHistoryErrorDAO(t)

	got, err := daos.GetOffsetIdBackwardHistoryMessages(
		context.Background(),
		41,
		mtproto.MakePeerUtil(mtproto.PEER_USER, 42),
		100,
		0,
		0,
		20,
		0,
	)
	if got != nil || err == nil {
		t.Fatalf("GetOffsetIdBackwardHistoryMessages() = (%v, %v), want (nil, query error)", got, err)
	}
}

func TestHistoryMethodsPropagateDAOErrors(t *testing.T) {
	daos := newHistoryErrorDAO(t)
	peer := mtproto.MakePeerUtil(mtproto.PEER_USER, 42)
	cases := []struct {
		name string
		call func() ([]*mtproto.MessageBox, error)
	}{
		{
			name: "forward id",
			call: func() ([]*mtproto.MessageBox, error) {
				return daos.GetOffsetIdForwardHistoryMessages(context.Background(), 41, peer, 100, 0, 0, 20, 0)
			},
		},
		{
			name: "backward date",
			call: func() ([]*mtproto.MessageBox, error) {
				return daos.GetOffsetDateBackwardHistoryMessages(context.Background(), 41, peer, 100, 0, 0, 20, 0)
			},
		},
		{
			name: "forward date",
			call: func() ([]*mtproto.MessageBox, error) {
				return daos.GetOffsetDateForwardHistoryMessages(context.Background(), 41, peer, 100, 0, 0, 20, 0)
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := tc.call()
			if got != nil || err == nil {
				t.Fatalf("history method = (%v, %v), want (nil, query error)", got, err)
			}
		})
	}
}

func TestHistoryMethodsRejectChannelWithoutGenericStorage(t *testing.T) {
	daos := &Dao{}
	peer := mtproto.MakePeerUtil(mtproto.PEER_CHANNEL, 42)
	cases := []struct {
		name string
		call func() ([]*mtproto.MessageBox, error)
	}{
		{
			name: "backward id",
			call: func() ([]*mtproto.MessageBox, error) {
				return daos.GetOffsetIdBackwardHistoryMessages(context.Background(), 41, peer, 100, 0, 0, 20, 0)
			},
		},
		{
			name: "forward id",
			call: func() ([]*mtproto.MessageBox, error) {
				return daos.GetOffsetIdForwardHistoryMessages(context.Background(), 41, peer, 100, 0, 0, 20, 0)
			},
		},
		{
			name: "backward date",
			call: func() ([]*mtproto.MessageBox, error) {
				return daos.GetOffsetDateBackwardHistoryMessages(context.Background(), 41, peer, 100, 0, 0, 20, 0)
			},
		},
		{
			name: "forward date",
			call: func() ([]*mtproto.MessageBox, error) {
				return daos.GetOffsetDateForwardHistoryMessages(context.Background(), 41, peer, 100, 0, 0, 20, 0)
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := tc.call()
			if got != nil || err != mtproto.ErrEnterpriseIsBlocked {
				t.Fatalf("history method = (%v, %v), want (nil, ERR_ENTERPRISE_IS_BLOCKED)", got, err)
			}
		})
	}
}

func TestSavedHistoryMethodsPropagateDAOErrors(t *testing.T) {
	daos := newHistoryErrorDAO(t)
	peer := mtproto.MakePeerUtil(mtproto.PEER_USER, 42)
	cases := []struct {
		name string
		call func() ([]*mtproto.MessageBox, error)
	}{
		{
			name: "backward id",
			call: func() ([]*mtproto.MessageBox, error) {
				return daos.GetOffsetIdBackwardSavedHistoryMessages(context.Background(), 41, peer, 100, 0, 0, 20, 0)
			},
		},
		{
			name: "forward id",
			call: func() ([]*mtproto.MessageBox, error) {
				return daos.GetOffsetIdForwardSavedHistoryMessages(context.Background(), 41, peer, 100, 0, 0, 20, 0)
			},
		},
		{
			name: "backward date",
			call: func() ([]*mtproto.MessageBox, error) {
				return daos.GetOffsetDateBackwardSavedHistoryMessages(context.Background(), 41, peer, 100, 0, 0, 20, 0)
			},
		},
		{
			name: "forward date",
			call: func() ([]*mtproto.MessageBox, error) {
				return daos.GetOffsetDateForwardSavedHistoryMessages(context.Background(), 41, peer, 100, 0, 0, 20, 0)
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := tc.call()
			if got != nil || err == nil {
				t.Fatalf("saved history method = (%v, %v), want (nil, query error)", got, err)
			}
		})
	}
}

func TestSavedHistoryMethodsRejectUnsupportedPeers(t *testing.T) {
	dao := &Dao{}
	peer := mtproto.MakePeerUtil(mtproto.PEER_CHANNEL, 42)
	got, err := dao.GetOffsetIdBackwardSavedHistoryMessages(context.Background(), 41, peer, 100, 0, 0, 20, 0)
	if got != nil || err != mtproto.ErrEnterpriseIsBlocked {
		t.Fatalf("saved history channel = (%v, %v), want (nil, ERR_ENTERPRISE_IS_BLOCKED)", got, err)
	}
	got, err = dao.GetOffsetIdBackwardSavedHistoryMessages(context.Background(), 41, nil, 100, 0, 0, 20, 0)
	if got != nil || err != mtproto.ErrPeerIdInvalid {
		t.Fatalf("saved history nil peer = (%v, %v), want (nil, PEER_ID_INVALID)", got, err)
	}
}

func newHistoryErrorDAO(t *testing.T) *Dao {
	t.Helper()
	db, err := sqlx.Open(&sqlx.Config{DSN: "root@unix(" + filepath.Join(t.TempDir(), "missing.sock") + ")/message_history_error_test?timeout=100ms"})
	if err != nil {
		t.Fatalf("open lazy test connection: %v", err)
	}
	return &Dao{Mysql: &Mysql{
		DB:          db,
		MessagesDAO: mysql_dao.NewMessagesDAO(db, 1),
	}}
}
