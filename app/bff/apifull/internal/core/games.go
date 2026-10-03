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
	"sort"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/persist"
)

// RPCGamesServer: Layer 229 methods previously returned ERR_ENTERPRISE_IS_BLOCKED.

const (
	gameScorePrefix       = "game:"
	inlineGameScorePrefix = "game:inline:"
)

func loadGameScores(key string) (map[int64]int32, error) {
	raw, err := persist.Default.Get(key)
	if err != nil {
		return nil, err
	}
	users := map[int64]int32{}
	if raw == "" {
		return users, nil
	}
	if err = json.Unmarshal([]byte(raw), &users); err != nil {
		return nil, err
	}
	return users, nil
}

func saveGameScores(key string, users map[int64]int32) error {
	b, err := json.Marshal(users)
	if err != nil {
		return err
	}
	return persist.Default.Set(key, string(b))
}

func gamePeerKey(peer *mtproto.InputPeer, msgID int32) string {
	if peer == nil {
		return fmt.Sprintf("0:0:0:%d", msgID)
	}
	return fmt.Sprintf("%d:%d:%d:%d", peer.GetUserId(), peer.GetChatId(), peer.GetChannelId(), msgID)
}

func inlineGameKey(id *mtproto.InputBotInlineMessageID) string {
	if id == nil {
		return "0:0:0:0:0"
	}
	return fmt.Sprintf("%d:%d:%d:%d:%d", id.GetDcId(), id.GetId_INT64(), id.GetAccessHash(), id.GetOwnerId(), id.GetId_INT32())
}

func gamePlayerID(in *mtproto.InputUser, auth int64) int64 {
	if in != nil && in.GetUserId() != 0 {
		return in.GetUserId()
	}
	return auth
}

func putGameScore(prefix, key string, userID int64, score int32, force bool) error {
	if score < 0 {
		return mtproto.ErrScoreInvalid
	}
	storeKey := prefix + key
	users, err := loadGameScores(storeKey)
	if err != nil {
		return err
	}
	if prev, ok := users[userID]; ok && !force && score <= prev {
		return mtproto.ErrBotScoreNotModified
	}
	users[userID] = score
	return saveGameScores(storeKey, users)
}

func listGameScores(prefix, key string) (*mtproto.Messages_HighScores, error) {
	board, err := loadGameScores(prefix + key)
	if err != nil {
		return nil, err
	}
	type pair struct {
		userID int64
		score  int32
	}
	var pairs []pair
	for userID, score := range board {
		pairs = append(pairs, pair{userID: userID, score: score})
	}
	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i].score != pairs[j].score {
			return pairs[i].score > pairs[j].score
		}
		return pairs[i].userID < pairs[j].userID
	})
	scores := make([]*mtproto.HighScore, 0, len(pairs))
	users := make([]*mtproto.User, 0, len(pairs))
	for i, p := range pairs {
		scores = append(scores, mtproto.MakeTLHighScore(&mtproto.HighScore{
			Pos:    int32(i + 1),
			UserId: p.userID,
			Score:  p.score,
		}).To_HighScore())
		users = append(users, mtproto.MakeTLUser(&mtproto.User{Id: p.userID}).To_User())
	}
	return mtproto.MakeTLMessagesHighScores(&mtproto.Messages_HighScores{
		Scores: scores,
		Users:  users,
	}).To_Messages_HighScores(), nil
}

func emptyUpdates() *mtproto.Updates {
	return mtproto.MakeTLUpdates(&mtproto.Updates{
		Updates: []*mtproto.Update{},
		Users:   []*mtproto.User{},
		Chats:   []*mtproto.Chat{},
	}).To_Updates()
}

func (c *ApiFullCore) MessagesSetGameScore(in *mtproto.TLMessagesSetGameScore) (*mtproto.Updates, error) {
	userID, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil {
		return nil, mtproto.ErrScoreInvalid
	}
	if err = putGameScore(gameScorePrefix, gamePeerKey(in.GetPeer(), in.GetId()), gamePlayerID(in.GetUserId(), userID), in.GetScore(), in.GetForce()); err != nil {
		return nil, err
	}
	return emptyUpdates(), nil
}

func (c *ApiFullCore) MessagesSetInlineGameScore(in *mtproto.TLMessagesSetInlineGameScore) (*mtproto.Bool, error) {
	userID, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil {
		return nil, mtproto.ErrScoreInvalid
	}
	if err = putGameScore(inlineGameScorePrefix, inlineGameKey(in.GetId()), gamePlayerID(in.GetUserId(), userID), in.GetScore(), in.GetForce()); err != nil {
		return nil, err
	}
	return mtproto.BoolTrue, nil
}

func (c *ApiFullCore) MessagesGetGameHighScores(in *mtproto.TLMessagesGetGameHighScores) (*mtproto.Messages_HighScores, error) {
	if _, err := c.requireUserId(); err != nil {
		return nil, err
	}
	if in == nil {
		return listGameScores(gameScorePrefix, gamePeerKey(nil, 0))
	}
	return listGameScores(gameScorePrefix, gamePeerKey(in.GetPeer(), in.GetId()))
}

func (c *ApiFullCore) MessagesGetInlineGameHighScores(in *mtproto.TLMessagesGetInlineGameHighScores) (*mtproto.Messages_HighScores, error) {
	if _, err := c.requireUserId(); err != nil {
		return nil, err
	}
	if in == nil {
		return listGameScores(inlineGameScorePrefix, inlineGameKey(nil))
	}
	return listGameScores(inlineGameScorePrefix, inlineGameKey(in.GetId()))
}

func (c *ApiFullCore) MessagesGetEmojiGameInfo(in *mtproto.TLMessagesGetEmojiGameInfo) (*mtproto.Messages_EmojiGameInfo, error) {
	_ = in
	userID, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	shortName, err := persist.Default.Get(gameUserKey(userID))
	if err != nil {
		return nil, err
	}
	if shortName == "" {
		return mtproto.MakeTLMessagesEmojiGameUnavailable(&mtproto.Messages_EmojiGameInfo{}).To_Messages_EmojiGameInfo(), nil
	}
	return mtproto.MakeTLMessagesEmojiGameDiceInfo(&mtproto.Messages_EmojiGameInfo{
		GameHash: shortName,
		Params:   []int32{},
	}).To_Messages_EmojiGameInfo(), nil
}

func gameUserKey(userID int64) string {
	return fmt.Sprintf("game:%d", userID)
}

// MessagesSendGame stores the request short_name for the authenticated user.
func (c *ApiFullCore) MessagesSendGame(in *mtproto.InputGame) (*mtproto.Bool, error) {
	userID, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	shortName := ""
	if in != nil {
		shortName = in.GetShortName()
	}
	if err = persist.Default.Set(gameUserKey(userID), shortName); err != nil {
		return nil, err
	}
	return mtproto.BoolTrue, nil
}

// MessagesGetGame returns the stored Game, including its short_name.
func (c *ApiFullCore) MessagesGetGame(in *mtproto.InputGame) (*mtproto.Game, error) {
	userID, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	_ = in
	shortName, err := persist.Default.Get(gameUserKey(userID))
	if err != nil {
		return nil, err
	}
	return mtproto.MakeTLGame(&mtproto.Game{ShortName: shortName}).To_Game(), nil
}
