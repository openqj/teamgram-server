package core

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/persist"
)

func TestAIComposeMethodsFailClosedWithoutBackend(t *testing.T) {
	userID := time.Now().UnixNano()
	c := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: userID}}

	composed, err := c.MessagesComposeMessageWithAI(&mtproto.TLMessagesComposeMessageWithAI{})
	if composed != nil || !errors.Is(err, mtproto.ErrMethodNotImpl) {
		t.Fatalf("AI compose: result=%+v err=%v", composed, err)
	}
	key := fmt.Sprintf("set:%d:messages.composeMessageWithAI", userID)
	if raw, err := persist.Default.Get(key); err != nil || raw != "" {
		t.Fatalf("unavailable AI compose persisted request: value=%q err=%v", raw, err)
	}

	example, err := c.AicomposeGetToneExample(&mtproto.TLAicomposeGetToneExample{Num: 2})
	if example != nil || !errors.Is(err, mtproto.ErrMethodNotImpl) {
		t.Fatalf("AI tone example: result=%+v err=%v", example, err)
	}
}
