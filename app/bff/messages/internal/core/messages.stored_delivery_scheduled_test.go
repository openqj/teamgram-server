package core

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/apifull/schedstore"
	"github.com/zeromicro/go-zero/core/logx"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func TestStoredScheduledDeliveryValidatesDateAndPeer(t *testing.T) {
	userID := time.Now().UnixNano()
	targetID := userID + 1
	ctx := context.Background()
	c := &MessagesCore{
		ctx:    ctx,
		MD:     &metadata.RpcMetadata{UserId: userID},
		Logger: logx.WithContext(ctx),
	}
	t.Cleanup(func() {
		messages, err := schedstore.List(userID)
		if err != nil {
			t.Errorf("list scheduled cleanup: %v", err)
			return
		}
		ids := make([]int32, 0, len(messages))
		for _, message := range messages {
			ids = append(ids, message.GetId())
		}
		if _, err = schedstore.Delete(userID, ids); err != nil {
			t.Errorf("delete scheduled cleanup: %v", err)
		}
	})

	future := wrapperspb.Int32(int32(time.Now().Add(time.Hour).Unix()))
	peer := mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: targetID}).To_InputPeer()
	updates, err := c.MessagesSendMessage(&mtproto.TLMessagesSendMessage{
		Peer:         peer,
		Message:      "scheduled text",
		ScheduleDate: future,
	})
	if err != nil || updates == nil || len(updates.GetUpdates()) != 1 ||
		updates.GetUpdates()[0].GetPredicateName() != mtproto.Predicate_updateNewScheduledMessage {
		t.Fatalf("schedule message: result=%+v err=%v", updates, err)
	}
	stored, err := schedstore.List(userID)
	if err != nil || len(stored) != 1 || stored[0].GetMessage() != "scheduled text" || stored[0].GetPeerId().GetUserId() != targetID || stored[0].GetDate() != future.GetValue() {
		t.Fatalf("stored schedule: messages=%+v err=%v", stored, err)
	}

	if _, err = c.MessagesSendMessage(&mtproto.TLMessagesSendMessage{
		Peer:         peer,
		Message:      "past date",
		ScheduleDate: wrapperspb.Int32(int32(time.Now().Add(-time.Minute).Unix())),
	}); !errors.Is(err, mtproto.ErrScheduleDateInvalid) {
		t.Fatalf("past schedule: got %v, want SCHEDULE_DATE_INVALID", err)
	}
	if _, err = c.MessagesSendMessage(&mtproto.TLMessagesSendMessage{
		Peer:         mtproto.MakeTLInputPeerEmpty(nil).To_InputPeer(),
		Message:      "invalid peer",
		ScheduleDate: future,
	}); !errors.Is(err, mtproto.ErrPeerIdInvalid) {
		t.Fatalf("invalid schedule peer: got %v, want PEER_ID_INVALID", err)
	}
	stored, err = schedstore.List(userID)
	if err != nil || len(stored) != 1 {
		t.Fatalf("rejected schedules changed queue: messages=%+v err=%v", stored, err)
	}
}
