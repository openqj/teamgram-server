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
	"fmt"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/persist"
	messagepb "github.com/teamgram/teamgram-server/app/service/biz/message/message"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

// RPCGamesServer: Layer 229 methods previously returned ERR_ENTERPRISE_IS_BLOCKED.

const (
	gameScorePrefix       = "game:"
	inlineGameScorePrefix = "game:inline:"
)

func gameScoreScope(prefix string) string {
	if prefix == inlineGameScorePrefix {
		return "inline"
	}
	return "peer"
}

func loadGameScores(prefix, key string) ([]persist.GameScore, error) {
	return persist.LoadGameScores(gameScoreScope(prefix), key)
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
	updated, err := persist.StoreGameScore(gameScoreScope(prefix), key, userID, score, force)
	if err != nil {
		return err
	}
	if !updated {
		return mtproto.ErrBotScoreNotModified
	}
	return nil
}

func (c *ApiFullCore) listGameScores(prefix, key string) (*mtproto.Messages_HighScores, error) {
	board, err := loadGameScores(prefix, key)
	if err != nil {
		return nil, err
	}
	scores := make([]*mtproto.HighScore, 0, len(board))
	users := make([]*mtproto.User, 0, len(board))
	for i, p := range board {
		scores = append(scores, mtproto.MakeTLHighScore(&mtproto.HighScore{
			Pos:    int32(i + 1),
			UserId: p.UserID,
			Score:  p.Score,
		}).To_HighScore())
		user := mtproto.MakeTLUser(&mtproto.User{Id: p.UserID}).To_User()
		if c != nil && c.svcCtx != nil && c.svcCtx.Dao != nil && c.svcCtx.Dao.UserClient != nil {
			if profile, profileErr := c.svcCtx.Dao.UserGetImmutableUser(c.secretContext(), &userpb.TLUserGetImmutableUser{Id: p.UserID}); profileErr == nil && profile != nil && profile.GetUser() != nil && !profile.Deleted() {
				user = profile.ToUser(c.MD.GetUserId())
			}
		}
		users = append(users, user)
	}
	return mtproto.MakeTLMessagesHighScores(&mtproto.Messages_HighScores{
		Scores: scores,
		Users:  users,
	}).To_Messages_HighScores(), nil
}

func (c *ApiFullCore) validateGamePlayer(input *mtproto.InputUser, authID int64) (int64, error) {
	if input == nil || input.GetUserId() == 0 || input.GetPredicateName() == mtproto.Predicate_inputUserSelf {
		return authID, nil
	}
	if input.GetUserId() < 0 {
		return 0, mtproto.ErrUserIdInvalid
	}
	if input.GetAccessHash() == 0 {
		if c == nil || c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.UserClient == nil {
			if !persist.PostgresEnabled() {
				return input.GetUserId(), nil
			}
			return 0, mtproto.ErrUserIdInvalid
		}
		return 0, mtproto.ErrUserIdInvalid
	}
	if c == nil || c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.UserClient == nil {
		if persist.PostgresEnabled() {
			return 0, mtproto.ErrMethodNotImpl
		}
		return input.GetUserId(), nil
	}
	profile, err := c.svcCtx.Dao.UserGetImmutableUser(c.secretContext(), &userpb.TLUserGetImmutableUser{Id: input.GetUserId()})
	if err != nil {
		return 0, err
	}
	if profile == nil || profile.GetUser() == nil || profile.Deleted() || profile.GetUser().GetAccessHash() != input.GetAccessHash() {
		return 0, mtproto.ErrUserIdInvalid
	}
	return input.GetUserId(), nil
}

// validateGameMessage binds a score update to the actual game message. The
// isolated in-memory fixture has no message reader, so it keeps the legacy
// score-only behavior; production PostgreSQL traffic fails closed until the
// authoritative message service can confirm the game and sending bot.
func (c *ApiFullCore) validateGameMessage(authID int64, peerInput *mtproto.InputPeer, messageID int32) error {
	if peerInput == nil || messageID <= 0 {
		return mtproto.ErrGameBotInvalid
	}
	peer := mtproto.FromInputPeer2(authID, peerInput)
	if peer == nil || !peer.IsUserOrChatOrChannel() || peer.PeerId <= 0 {
		return mtproto.ErrPeerIdInvalid
	}
	d := c.apifullDao()
	if d == nil || d.PollMessageReader == nil {
		if persist.PostgresEnabled() {
			return mtproto.ErrMethodNotImpl
		}
		return nil
	}
	box, err := d.MessageGetUserMessage(c.secretContext(), &messagepb.TLMessageGetUserMessage{UserId: authID, Id: messageID})
	if err != nil {
		return err
	}
	if box == nil || box.GetMessage() == nil || box.GetMessage().GetMedia() == nil || box.GetMessage().GetMedia().GetGame() == nil ||
		box.GetSenderUserId() != authID || box.GetMessageId() != messageID {
		return mtproto.ErrGameBotInvalid
	}
	peerType, peerID := peer.PeerType, peer.PeerId
	if peer.IsSelfUser(authID) {
		peerType, peerID = mtproto.PEER_USER, authID
	}
	if box.GetPeerType() != peerType || box.GetPeerId() != peerID {
		return mtproto.ErrGameBotInvalid
	}
	return nil
}

func validateInlineGameID(id *mtproto.InputBotInlineMessageID) error {
	if id == nil || id.GetDcId() <= 0 || id.GetAccessHash() == 0 {
		return mtproto.ErrGameBotInvalid
	}
	if id.GetId_INT64() <= 0 && id.GetId_INT32() <= 0 {
		return mtproto.ErrGameBotInvalid
	}
	return nil
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
	if in == nil || in.GetPeer() == nil || in.GetId() <= 0 {
		return nil, mtproto.ErrGameBotInvalid
	}
	if err = c.validateGameMessage(userID, in.GetPeer(), in.GetId()); err != nil {
		return nil, err
	}
	playerID, playerErr := c.validateGamePlayer(in.GetUserId(), userID)
	if playerErr != nil {
		return nil, playerErr
	}
	if err = putGameScore(gameScorePrefix, gamePeerKey(in.GetPeer(), in.GetId()), playerID, in.GetScore(), in.GetForce()); err != nil {
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
		return nil, mtproto.ErrGameBotInvalid
	}
	if err = validateInlineGameID(in.GetId()); err != nil {
		return nil, err
	}
	playerID, playerErr := c.validateGamePlayer(in.GetUserId(), userID)
	if playerErr != nil {
		return nil, playerErr
	}
	if err = putGameScore(inlineGameScorePrefix, inlineGameKey(in.GetId()), playerID, in.GetScore(), in.GetForce()); err != nil {
		return nil, err
	}
	return mtproto.BoolTrue, nil
}

func (c *ApiFullCore) MessagesGetGameHighScores(in *mtproto.TLMessagesGetGameHighScores) (*mtproto.Messages_HighScores, error) {
	userID, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil || in.GetPeer() == nil || in.GetId() <= 0 {
		return nil, mtproto.ErrGameBotInvalid
	}
	if err = c.validateGameMessage(userID, in.GetPeer(), in.GetId()); err != nil {
		return nil, err
	}
	if _, err = c.validateGamePlayer(in.GetUserId(), userID); err != nil {
		return nil, err
	}
	return c.listGameScores(gameScorePrefix, gamePeerKey(in.GetPeer(), in.GetId()))
}

func (c *ApiFullCore) MessagesGetInlineGameHighScores(in *mtproto.TLMessagesGetInlineGameHighScores) (*mtproto.Messages_HighScores, error) {
	userID, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil || validateInlineGameID(in.GetId()) != nil {
		return nil, mtproto.ErrGameBotInvalid
	}
	if _, err = c.validateGamePlayer(in.GetUserId(), userID); err != nil {
		return nil, err
	}
	return c.listGameScores(inlineGameScorePrefix, inlineGameKey(in.GetId()))
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
