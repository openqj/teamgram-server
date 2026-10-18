package store

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func TestAppendConvertedCallbackUsesClickerChatForBotPrivatePeer(t *testing.T) {
	const botID int64 = 900000000001
	encoded := make([]encodedUpdate, 0, 1)
	store := &Store{}
	store.appendConvertedForBot(context.Background(), &encoded, botID, &mtproto.Update{
		PredicateName:  mtproto.Predicate_updateBotCallbackQuery,
		QueryId_INT64:  99,
		UserId:         7,
		Peer_PEER:      mtproto.MakePeerUser(botID),
		MsgId_INT32:    636,
		Data_FLAGBYTES: []byte("ping"),
	}, []*mtproto.User{
		{Id: 7, FirstName: wrapperspb.String("Clicker")},
		{Id: botID, Bot: true},
	}, nil)

	if len(encoded) != 1 {
		t.Fatalf("encoded updates = %d, want 1", len(encoded))
	}
	var update struct {
		CallbackQuery struct {
			Message struct {
				Chat struct {
					ID int64 `json:"id"`
				} `json:"chat"`
				MessageID int `json:"message_id"`
			} `json:"message"`
		} `json:"callback_query"`
	}
	if err := json.Unmarshal(encoded[0].data, &update); err != nil {
		t.Fatal(err)
	}
	if got := update.CallbackQuery.Message.Chat.ID; got != 7 {
		t.Fatalf("callback chat id = %d, want clicker id 7", got)
	}
	if got := update.CallbackQuery.Message.MessageID; got != 636 {
		t.Fatalf("callback message id = %d, want 636", got)
	}
}
