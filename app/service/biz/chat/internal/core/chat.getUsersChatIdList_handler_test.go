package core

import (
	"context"
	"errors"
	"testing"

	"github.com/teamgram/teamgram-server/app/service/biz/chat/internal/dal/dataobject"
)

func TestCollectUsersChatIdListPropagatesDAOError(t *testing.T) {
	wantErr := errors.New("membership query failed")
	_, err := collectUsersChatIdList(context.Background(), []int64{1}, func(context.Context, []int64, func(int, int, *dataobject.ChatParticipantsDO)) ([]dataobject.ChatParticipantsDO, error) {
		return nil, wantErr
	})
	if err != wantErr {
		t.Fatalf("collectUsersChatIdList() error = %v, want %v", err, wantErr)
	}
}

func TestCollectUsersChatIdListGroupsRowsByUser(t *testing.T) {
	rows := []dataobject.ChatParticipantsDO{
		{UserId: 1, ChatId: 10},
		{UserId: 2, ChatId: 20},
		{UserId: 1, ChatId: 11},
	}
	result, err := collectUsersChatIdList(context.Background(), []int64{1, 2}, func(context.Context, []int64, func(int, int, *dataobject.ChatParticipantsDO)) ([]dataobject.ChatParticipantsDO, error) {
		return rows, nil
	})
	if err != nil {
		t.Fatalf("collectUsersChatIdList() error = %v", err)
	}
	if len(result.GetDatas()) != 2 {
		t.Fatalf("expected 2 users, got %d", len(result.GetDatas()))
	}
	if got := result.GetDatas()[0]; got.UserId != 1 || len(got.ChatIdList) != 2 || got.ChatIdList[0] != 10 || got.ChatIdList[1] != 11 {
		t.Fatalf("unexpected first user chat ids: %+v", got)
	}
	if got := result.GetDatas()[1]; got.UserId != 2 || len(got.ChatIdList) != 1 || got.ChatIdList[0] != 20 {
		t.Fatalf("unexpected second user chat ids: %+v", got)
	}
}
