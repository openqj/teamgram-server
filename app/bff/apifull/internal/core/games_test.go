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
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/persist"
)

func TestGameHighScoreRoundtrip(t *testing.T) {
	c := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: 1}}
	peer := &mtproto.InputPeer{UserId: 1}
	player := &mtproto.InputUser{UserId: 1}
	inlineID := &mtproto.InputBotInlineMessageID{DcId: 2, Id_INT64: 9}
	for _, key := range []string{
		gameScorePrefix + gamePeerKey(peer, 11),
		inlineGameScorePrefix + inlineGameKey(inlineID),
	} {
		if err := persist.Default.Set(key, ""); err != nil {
			t.Fatal(err)
		}
	}

	if _, err := c.MessagesSetGameScore(&mtproto.TLMessagesSetGameScore{
		Peer:   peer,
		Id:     11,
		UserId: player,
		Score:  42,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := c.MessagesSetGameScore(&mtproto.TLMessagesSetGameScore{
		Peer:   peer,
		Id:     11,
		UserId: player,
		Score:  42,
	}); err != mtproto.ErrBotScoreNotModified {
		t.Fatalf("unchanged score error = %v", err)
	}
	got, err := c.MessagesGetGameHighScores(&mtproto.TLMessagesGetGameHighScores{
		Peer:   peer,
		Id:     11,
		UserId: player,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || len(got.Scores) != 1 || got.Scores[0].GetScore() != 42 || got.Scores[0].GetUserId() != 1 {
		t.Fatalf("peer scores = %+v", got)
	}

	if _, err = c.MessagesSetInlineGameScore(&mtproto.TLMessagesSetInlineGameScore{
		Id:     inlineID,
		UserId: player,
		Score:  7,
	}); err != nil {
		t.Fatal(err)
	}
	got, err = c.MessagesGetInlineGameHighScores(&mtproto.TLMessagesGetInlineGameHighScores{
		Id:     inlineID,
		UserId: player,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || len(got.Scores) != 1 || got.Scores[0].GetScore() != 7 || got.Scores[0].GetUserId() != 1 {
		t.Fatalf("inline scores = %+v", got)
	}
}

func TestGameShortNameRoundtrip(t *testing.T) {
	c := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: 1}}
	if _, err := c.MessagesSendGame(&mtproto.InputGame{ShortName: "prod-game"}); err != nil {
		t.Fatal(err)
	}
	got, err := c.MessagesGetGame(nil)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || got.GetShortName() != "prod-game" {
		t.Fatalf("short name: %+v", got)
	}
}

func TestGameHighScoreRequiresAuth(t *testing.T) {
	c := &ApiFullCore{}
	if _, err := c.MessagesSetGameScore(&mtproto.TLMessagesSetGameScore{Score: 1}); err != mtproto.ErrAuthKeyUnregistered {
		t.Fatalf("set: %v", err)
	}
	if _, err := c.MessagesGetGameHighScores(&mtproto.TLMessagesGetGameHighScores{}); err != mtproto.ErrAuthKeyUnregistered {
		t.Fatalf("get: %v", err)
	}
}
