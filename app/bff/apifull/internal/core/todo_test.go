package core

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/persist"
)

func TestTodoAppendToggleRoundtrip(t *testing.T) {
	peer := &mtproto.InputPeer{UserId: 7}
	key := todoStoreKey(1, peer, 42)
	if err := persist.Default.Set(key, ""); err != nil {
		t.Fatal(err)
	}

	anon := &ApiFullCore{}
	if _, err := anon.MessagesAppendTodoList(&mtproto.TLMessagesAppendTodoList{
		Peer:  peer,
		MsgId: 42,
		List:  []*mtproto.TodoItem{{Id: 1}},
	}); err == nil {
		t.Fatal("append without auth")
	}
	if _, err := anon.MessagesToggleTodoCompleted(&mtproto.TLMessagesToggleTodoCompleted{
		Peer:      peer,
		MsgId:     42,
		Completed: []int32{1},
	}); err == nil {
		t.Fatal("toggle without auth")
	}

	c := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: 1}}
	appended, err := c.MessagesAppendTodoList(&mtproto.TLMessagesAppendTodoList{
		Peer:  peer,
		MsgId: 42,
		List: []*mtproto.TodoItem{
			{Id: 1},
			{Id: 2},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if appended == nil || len(appended.GetUpdates()) != 1 {
		t.Fatalf("append updates: %#v", appended)
	}
	gotList := appended.GetUpdates()[0].GetMessage_MESSAGE().GetAction().GetList()
	if len(gotList) != 2 || gotList[0].GetId() != 1 || gotList[1].GetId() != 2 {
		t.Fatalf("append list: %#v", gotList)
	}

	toggled, err := c.MessagesToggleTodoCompleted(&mtproto.TLMessagesToggleTodoCompleted{
		Peer:        peer,
		MsgId:       42,
		Completed:   []int32{1},
		Incompleted: []int32{2},
	})
	if err != nil {
		t.Fatal(err)
	}
	action := toggled.GetUpdates()[0].GetMessage_MESSAGE().GetAction()
	if len(action.GetCompleted()) != 1 || action.GetCompleted()[0] != 1 {
		t.Fatalf("toggle completed: %#v", action.GetCompleted())
	}

	st, err := loadTodo(key)
	if err != nil {
		t.Fatal(err)
	}
	if len(st.Items) != 2 || !st.Done[1] || st.Done[2] {
		t.Fatalf("store after toggle: %#v", st)
	}

	if _, err = c.MessagesToggleTodoCompleted(&mtproto.TLMessagesToggleTodoCompleted{
		Peer:        peer,
		MsgId:       42,
		Incompleted: []int32{1},
	}); err != nil {
		t.Fatal(err)
	}
	st, err = loadTodo(key)
	if err != nil {
		t.Fatal(err)
	}
	if st.Done[1] {
		t.Fatal("item 1 still completed")
	}

	if _, err = c.MessagesAppendTodoList(&mtproto.TLMessagesAppendTodoList{
		Peer:  peer,
		MsgId: 42,
		List:  []*mtproto.TodoItem{{Id: 3}},
	}); err != nil {
		t.Fatal(err)
	}
	st, err = loadTodo(key)
	if err != nil {
		t.Fatal(err)
	}
	if len(st.Items) != 3 || st.Items[2].GetId() != 3 || st.Done[1] || st.Done[3] {
		t.Fatalf("store after second append: items=%d done=%v", len(st.Items), st.Done)
	}
}
