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
	"errors"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
)

func TestInlineBotRoundtrip(t *testing.T) {
	zero := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: 0}}
	if _, err := zero.MessagesSavePreparedInlineMessage(&mtproto.TLMessagesSavePreparedInlineMessage{
		Result: &mtproto.InputBotInlineResult{Id: "nope"},
	}); err != mtproto.ErrAuthKeyUnregistered {
		t.Fatalf("save uid 0: %v", err)
	}
	if _, err := zero.MessagesGetPreparedInlineMessage(&mtproto.TLMessagesGetPreparedInlineMessage{Id: "nope"}); err != mtproto.ErrAuthKeyUnregistered {
		t.Fatalf("get uid 0: %v", err)
	}
	if _, err := zero.MessagesSetBotCallbackAnswer(&mtproto.TLMessagesSetBotCallbackAnswer{}); err != mtproto.ErrAuthKeyUnregistered {
		t.Fatalf("set callback uid 0: %v", err)
	}
	if _, err := zero.MessagesGetBotCallbackAnswer(&mtproto.TLMessagesGetBotCallbackAnswer{}); err != mtproto.ErrAuthKeyUnregistered {
		t.Fatalf("get callback uid 0: %v", err)
	}

	c := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: 81007}}
	id := "res-81007"
	saved, err := c.MessagesSavePreparedInlineMessage(&mtproto.TLMessagesSavePreparedInlineMessage{
		Result: &mtproto.InputBotInlineResult{Id: id},
	})
	if err != nil {
		t.Fatal(err)
	}
	if saved == nil || saved.GetId() != id {
		t.Fatalf("saved id: %+v", saved)
	}
	got, err := c.MessagesGetPreparedInlineMessage(&mtproto.TLMessagesGetPreparedInlineMessage{Id: id})
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || got.GetResult() == nil || got.GetResult().GetId() != id {
		t.Fatalf("prepared: %+v", got)
	}

	if got, err := c.MessagesSetBotCallbackAnswer(&mtproto.TLMessagesSetBotCallbackAnswer{QueryId: 81007}); got != nil || !errors.Is(err, mtproto.ErrMethodNotImpl) {
		t.Fatalf("set callback: got=%v err=%v, want METHOD_NOT_IMPL", got, err)
	}
	if got, err := c.MessagesGetBotCallbackAnswer(&mtproto.TLMessagesGetBotCallbackAnswer{Peer: mtproto.MakeTLInputPeerSelf(nil).To_InputPeer(), MsgId: 1}); got != nil || !errors.Is(err, mtproto.ErrMethodNotImpl) {
		t.Fatalf("get callback: got=%v err=%v, want METHOD_NOT_IMPL", got, err)
	}
	if got, err := c.MessagesSetBotGuestChatResult(&mtproto.TLMessagesSetBotGuestChatResult{QueryId: 81007}); got != nil || !errors.Is(err, mtproto.ErrMethodNotImpl) {
		t.Fatalf("guest result: got=%v err=%v, want METHOD_NOT_IMPL", got, err)
	}
}
