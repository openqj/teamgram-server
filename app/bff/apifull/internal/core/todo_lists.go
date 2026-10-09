// Copyright 2026 Teamgram Authors
//  All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//   http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//
// Author: teamgramio (teamgram.io@gmail.com)
//

package core

import (
	"encoding/json"
	"fmt"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/persist"
	"google.golang.org/protobuf/proto"
)

// RPCTodoListsServer: Layer 229 methods previously returned ERR_ENTERPRISE_IS_BLOCKED.

type todoListState struct {
	Items []*mtproto.TodoItem `json:"items"`
	Done  map[int32]bool      `json:"done"`
}

func todoStoreKey(userID int64, peer *mtproto.InputPeer, msgID int32) string {
	var peerUser, peerChat, peerChannel int64
	if peer != nil {
		peerUser = peer.GetUserId()
		peerChat = peer.GetChatId()
		peerChannel = peer.GetChannelId()
	}
	return fmt.Sprintf("todo:%d:%d:%d:%d:%d", userID, peerUser, peerChat, peerChannel, msgID)
}

func loadTodo(key string) (*todoListState, error) {
	raw, err := persist.Default.Get(key)
	if err != nil {
		return nil, err
	}
	st := &todoListState{Done: map[int32]bool{}}
	if raw == "" {
		return st, nil
	}
	if err := json.Unmarshal([]byte(raw), st); err != nil {
		return nil, err
	}
	if st.Done == nil {
		st.Done = map[int32]bool{}
	}
	return st, nil
}

func saveTodo(key string, st *todoListState) error {
	raw, err := json.Marshal(st)
	if err != nil {
		return err
	}
	return persist.Default.Set(key, string(raw))
}

func (s *todoListState) upsert(list []*mtproto.TodoItem) []*mtproto.TodoItem {
	if s.Done == nil {
		s.Done = map[int32]bool{}
	}
	index := make(map[int32]int, len(s.Items))
	for i, it := range s.Items {
		if it != nil {
			index[it.GetId()] = i
		}
	}
	added := make([]*mtproto.TodoItem, 0, len(list))
	for _, it := range list {
		if it == nil {
			continue
		}
		cp := proto.Clone(it).(*mtproto.TodoItem)
		if i, ok := index[cp.Id]; ok {
			s.Items[i] = cp
		} else {
			s.Items = append(s.Items, cp)
			index[cp.Id] = len(s.Items) - 1
			if _, exists := s.Done[cp.Id]; !exists {
				s.Done[cp.Id] = false
			}
		}
		added = append(added, cp)
	}
	return added
}

func (s *todoListState) toggle(completed, incompleted []int32) {
	if s.Done == nil {
		s.Done = map[int32]bool{}
	}
	known := make(map[int32]struct{}, len(s.Items))
	for _, it := range s.Items {
		if it != nil {
			known[it.GetId()] = struct{}{}
		}
	}
	for _, id := range incompleted {
		if _, ok := known[id]; ok {
			s.Done[id] = false
		}
	}
	for _, id := range completed {
		if _, ok := known[id]; ok {
			s.Done[id] = true
		}
	}
}

func todoUpdates(msgID int32, action *mtproto.MessageAction) *mtproto.Updates {
	msg := mtproto.MakeTLMessageService(&mtproto.Message{
		Id:     msgID,
		FromId: mtproto.MakeTLPeerUser(&mtproto.Peer{}).To_Peer(),
		PeerId: mtproto.MakeTLPeerUser(&mtproto.Peer{}).To_Peer(),
		Action: action,
	}).To_Message()
	upd := mtproto.MakeTLUpdateNewMessage(&mtproto.Update{
		Message_MESSAGE: msg,
	}).To_Update()
	return mtproto.MakeUpdatesByUpdates(upd)
}

func (c *ApiFullCore) MessagesToggleTodoCompleted(in *mtproto.TLMessagesToggleTodoCompleted) (*mtproto.Updates, error) {
	userID, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil {
		return mtproto.MakeEmptyUpdates(), nil
	}
	key := todoStoreKey(userID, in.GetPeer(), in.GetMsgId())
	completed := append([]int32(nil), in.GetCompleted()...)
	incompleted := append([]int32(nil), in.GetIncompleted()...)
	st, err := loadTodo(key)
	if err != nil {
		return nil, err
	}
	st.toggle(completed, incompleted)
	if err := saveTodo(key, st); err != nil {
		return nil, err
	}
	return todoUpdates(in.GetMsgId(), mtproto.MakeTLMessageActionTodoCompletions(&mtproto.MessageAction{
		Completed:   completed,
		Incompleted: incompleted,
	}).To_MessageAction()), nil
}

func (c *ApiFullCore) MessagesAppendTodoList(in *mtproto.TLMessagesAppendTodoList) (*mtproto.Updates, error) {
	userID, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil {
		return mtproto.MakeEmptyUpdates(), nil
	}
	key := todoStoreKey(userID, in.GetPeer(), in.GetMsgId())
	st, err := loadTodo(key)
	if err != nil {
		return nil, err
	}
	added := st.upsert(in.GetList())
	if err := saveTodo(key, st); err != nil {
		return nil, err
	}
	return todoUpdates(in.GetMsgId(), mtproto.MakeTLMessageActionTodoAppendTasks(&mtproto.MessageAction{
		List: added,
	}).To_MessageAction()), nil
}
