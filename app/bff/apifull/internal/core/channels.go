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
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"math"
	"time"
	"unicode/utf8"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/apifull/channelview"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/domain"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/persist"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
	"github.com/teamgram/teamgram-server/app/service/dfs/dfs"
)

// RPCChannelsServer: Layer 229 methods previously returned ERR_ENTERPRISE_IS_BLOCKED.

func (c *ApiFullCore) persistChan(method string, in any) error {
	raw, err := json.Marshal(in)
	if err != nil {
		return err
	}
	return persist.Default.Set(fmt.Sprintf("chan:%d:%s", c.MD.UserId, method), string(raw))
}

type chanStored struct {
	Title string `json:"title"`
	About string `json:"about"`
}

func chanDataKey(userId, id int64) string {
	return fmt.Sprintf("chan:%d:%d", userId, id)
}

func chanIndexKey(userId int64) string {
	return fmt.Sprintf("chan:%d:index", userId)
}

func chanChat(id int64, title string) *mtproto.Chat {
	return mtproto.MakeTLChannel(&mtproto.Chat{
		Id:                   id,
		Title:                title,
		AccessHash_FLAGINT64: mtproto.MakeFlagsInt64(id),
		Photo:                mtproto.MakeTLChatPhotoEmpty(nil).To_ChatPhoto(),
	}).To_Chat()
}

func (c *ApiFullCore) saveChan(userId, id int64, title, about string) error {
	raw, err := json.Marshal(chanStored{Title: title, About: about})
	if err != nil {
		return err
	}
	if err = persist.Default.Set(chanDataKey(userId, id), string(raw)); err != nil {
		return err
	}
	return c.appendChanIndex(userId, id)
}

func (c *ApiFullCore) appendChanIndex(userId, id int64) error {
	key := chanIndexKey(userId)
	raw, err := persist.Default.Get(key)
	if err != nil {
		return err
	}
	var ids []int64
	if raw != "" {
		if err = json.Unmarshal([]byte(raw), &ids); err != nil {
			return err
		}
	}
	ids = append(ids, id)
	buf, err := json.Marshal(ids)
	if err != nil {
		return err
	}
	return persist.Default.Set(key, string(buf))
}

func loadChan(userId, id int64) (chanStored, bool, error) {
	raw, err := persist.Default.Get(chanDataKey(userId, id))
	if err != nil || raw == "" {
		return chanStored{}, false, err
	}
	var rec chanStored
	if err = json.Unmarshal([]byte(raw), &rec); err != nil {
		return chanStored{}, false, err
	}
	return rec, true, nil
}

func (c *ApiFullCore) listUserChans(userId int64) ([]*mtproto.Chat, error) {
	if domain.Ready() {
		// The canonical table is the source of truth once configured. Query it
		// directly so recommendations do not depend on a stale KV index.
		stored, err := domain.ListByCreator(userId)
		if err != nil {
			return nil, err
		}
		chats := make([]*mtproto.Chat, 0, len(stored))
		for _, channel := range stored {
			chats = append(chats, channelview.Chat(channel, channel.Creator == userId))
		}
		return chats, nil
	}
	raw, err := persist.Default.Get(chanIndexKey(userId))
	if err != nil || raw == "" {
		return []*mtproto.Chat{}, err
	}
	var ids []int64
	if err = json.Unmarshal([]byte(raw), &ids); err != nil {
		return nil, err
	}
	chats := make([]*mtproto.Chat, 0, len(ids))
	for _, id := range ids {
		rec, ok, err := loadChan(userId, id)
		if err != nil {
			return nil, err
		}
		if ok {
			chat, err := c.channelChat(userId, id, rec.Title)
			if err != nil {
				return nil, err
			}
			chats = append(chats, chat)
		}
	}
	return chats, nil
}

func (c *ApiFullCore) channelChat(userId, id int64, fallbackTitle string) (*mtproto.Chat, error) {
	if domain.Ready() {
		ch, ok, err := domain.LoadChannel(id)
		if err != nil {
			return nil, err
		}
		if ok {
			return channelview.Chat(ch, ch.Creator == userId), nil
		}
	}
	return chanChat(id, fallbackTitle), nil
}

func (c *ApiFullCore) MessagesGetPersonalChannelHistory(in *mtproto.TLMessagesGetPersonalChannelHistory) (*mtproto.Messages_Messages, error) {
	viewerID, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil || in.GetUserId() == nil {
		return nil, mtproto.ErrUserIdInvalid
	}
	peer := mtproto.FromInputUser(viewerID, in.GetUserId())
	if (peer.PeerType != mtproto.PEER_SELF && peer.PeerType != mtproto.PEER_USER) || peer.PeerId == 0 {
		return nil, mtproto.ErrUserIdInvalid
	}
	d := c.apifullDao()
	if d == nil || d.UserClient == nil {
		return nil, mtproto.ErrInternalServerError
	}
	ctx := c.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	target, err := d.UserClient.UserGetImmutableUserV2(ctx, &userpb.TLUserGetImmutableUserV2{Id: peer.PeerId})
	if err != nil {
		return nil, err
	}
	if target == nil || target.GetUser() == nil || target.GetUser().GetDeleted() {
		return nil, mtproto.ErrUserIdInvalid
	}
	channelID := target.GetUser().GetPersonalChannelId()
	if channelID == 0 {
		return mtproto.MakeTLMessagesMessages(&mtproto.Messages_Messages{
			Messages: []*mtproto.Message{}, Topics: []*mtproto.ForumTopic{}, Chats: []*mtproto.Chat{}, Users: []*mtproto.User{},
		}).To_Messages_Messages(), nil
	}
	limit := in.GetLimit()
	return channelview.PersonalHistory(viewerID, peer.PeerId, channelID, in.GetMinId(), in.GetMaxId(), limit)
}

func (c *ApiFullCore) ChannelsReadHistory(in *mtproto.TLChannelsReadHistory) (*mtproto.Bool, error) {
	userId, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	var inputChannel *mtproto.InputChannel
	var maxID int32
	if in != nil {
		inputChannel = in.GetChannel()
		maxID = in.GetMaxId()
	}
	ch, err := c.resolveMemberChannel(userId, inputChannel, inputChannelID(inputChannel))
	if err != nil {
		return nil, err
	}
	if err = c.requireChannelMember(userId, ch.ID); err != nil {
		return nil, err
	}
	if _, err = channelview.ReadHistory(userId, ch.ID, maxID); err != nil {
		return nil, err
	}
	return mtproto.BoolTrue, nil
}

func (c *ApiFullCore) ChannelsDeleteMessages(in *mtproto.TLChannelsDeleteMessages) (*mtproto.Messages_AffectedMessages, error) {
	userId, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	var inputChannel *mtproto.InputChannel
	var channelID int64
	var ids []int32
	if in != nil {
		inputChannel = in.GetChannel()
		channelID = inputChannelID(inputChannel)
		ids = in.GetId()
	}
	if _, err = c.resolveMemberChannel(userId, inputChannel, channelID); err != nil {
		return nil, err
	}
	return channelview.DeleteMessages(userId, channelID, ids)
}

func (c *ApiFullCore) ChannelsGetMessages(in *mtproto.TLChannelsGetMessages) (*mtproto.Messages_Messages, error) {
	userId, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	var inputChannel *mtproto.InputChannel
	var ids []int32
	if in != nil {
		inputChannel = in.GetChannel()
		ids = append(ids, in.GetId_VECTORINT32()...)
		for _, item := range in.GetId_VECTORINPUTMESSAGE() {
			if item == nil {
				continue
			}
			ids = append(ids, item.GetId())
		}
	}
	ch, err := c.resolveMemberChannel(userId, inputChannel, inputChannelID(inputChannel))
	if err != nil {
		return nil, err
	}
	if err = c.requireChannelMember(userId, ch.ID); err != nil {
		return nil, err
	}
	return channelview.MessagesBox(userId, ch.ID, ids)
}

func (c *ApiFullCore) ChannelsGetParticipants(in *mtproto.TLChannelsGetParticipants) (*mtproto.Channels_ChannelParticipants, error) {
	userId, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	var channelID int64
	var filter *mtproto.ChannelParticipantsFilter
	var offset, limit int32
	var inputChannel *mtproto.InputChannel
	if in != nil {
		inputChannel = in.GetChannel()
		if inputChannel != nil {
			channelID = inputChannel.GetChannelId()
		}
		filter = in.GetFilter()
		offset = in.GetOffset()
		limit = in.GetLimit()
	}
	ch, err := c.resolveMemberChannel(userId, inputChannel, channelID)
	if err != nil {
		return nil, err
	}
	if err = c.requireChannelMember(userId, channelID); err != nil {
		// Telegram hides the membership roster for channels that have
		// participants hidden. A non-member may ask for the list, but must
		// receive an empty result rather than learn whether a user belongs to
		// the channel. Other channels still require membership as before.
		if !ch.ParticipantsHidden || !errors.Is(err, mtproto.ErrUserNotParticipant) {
			return nil, err
		}
	}
	members, err := domain.ListChannelMembers(channelID)
	if err != nil {
		return nil, c.mapChannelMemberError(err)
	}
	participants := channelview.Participants(userId, ch, filter, offset, limit, members)
	if users, err := c.hydrateChannelUsers(userId, participantUserIDs(participants)...); err != nil {
		return nil, err
	} else if users != nil {
		participants.Users = users
	}
	return participants, nil
}

func (c *ApiFullCore) ChannelsGetParticipant(in *mtproto.TLChannelsGetParticipant) (*mtproto.Channels_ChannelParticipant, error) {
	userId, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	var channelID int64
	var peer *mtproto.InputPeer
	var inputChannel *mtproto.InputChannel
	if in != nil {
		inputChannel = in.GetChannel()
		if inputChannel != nil {
			channelID = inputChannel.GetChannelId()
		}
		peer = in.GetParticipant()
	}
	ch, err := c.resolveMemberChannel(userId, inputChannel, channelID)
	if err != nil {
		return nil, err
	}
	if err = c.requireChannelMember(userId, channelID); err != nil {
		return nil, err
	}
	peerID, err := channelMemberPeerID(userId, peer)
	if err != nil {
		return nil, err
	}
	member, found, err := domain.LoadChannelMember(channelID, peerID)
	if err != nil {
		return nil, c.mapChannelMemberError(err)
	}
	box, err := channelview.Participant(userId, ch, peer, member, found)
	if err != nil {
		return nil, err
	}
	if users, err := c.hydrateChannelUsers(userId, peerID); err != nil {
		return nil, err
	} else if users != nil {
		box.Users = users
	}
	return box, nil
}

func participantUserIDs(result *mtproto.Channels_ChannelParticipants) []int64 {
	if result == nil {
		return nil
	}
	ids := make([]int64, 0, len(result.GetParticipants()))
	for _, participant := range result.GetParticipants() {
		if participant != nil && participant.GetUserId() != 0 {
			ids = append(ids, participant.GetUserId())
		}
	}
	return ids
}

func (c *ApiFullCore) ChannelsGetChannels(in *mtproto.TLChannelsGetChannels) (*mtproto.Messages_Chats, error) {
	userId, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	chats := []*mtproto.Chat{}
	if in != nil {
		for _, ic := range in.GetId() {
			if ic == nil || ic.GetChannelId() == 0 {
				return nil, mtproto.ErrChannelInvalid
			}
			channel, err := c.resolveMemberChannel(userId, ic, ic.GetChannelId())
			if err != nil {
				return nil, err
			}
			chats = append(chats, channelview.Chat(channel, channel.Creator == userId))
		}
	}
	return mtproto.MakeTLMessagesChats(&mtproto.Messages_Chats{Chats: chats}).To_Messages_Chats(), nil
}

func (c *ApiFullCore) ChannelsGetFullChannel(in *mtproto.TLChannelsGetFullChannel) (*mtproto.Messages_ChatFull, error) {
	userId, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	var inputChannel *mtproto.InputChannel
	if in != nil {
		inputChannel = in.GetChannel()
	}
	ch, err := c.resolveMemberChannel(userId, inputChannel, inputChannelID(inputChannel))
	if err != nil {
		return nil, err
	}
	if domain.Ready() {
		community, isCommunity, communityErr := domain.LoadCommunity(ch.ID)
		if communityErr != nil {
			return nil, mtproto.ErrInternalServerError
		}
		if isCommunity {
			canView, viewErr := domain.CommunityCanView(ch.ID, userId)
			if viewErr != nil {
				return nil, mtproto.ErrInternalServerError
			}
			if !canView {
				return nil, mtproto.ErrUserNotParticipant
			}
			return c.communityFull(userId, community, ch)
		}
	}
	if err = c.requireChannelMember(userId, ch.ID); err != nil {
		return nil, err
	}
	box, err := channelview.Full(userId, ch)
	if err != nil {
		c.Logger.Errorf("channels.getFullChannel - error: %v", err)
		return nil, err
	}
	channelFull := box.GetFullChat().To_ChannelFull().GetData2()
	stickerSetID, foundStickerSet, err := domain.LoadChannelStickerSet(ch.ID)
	if err != nil {
		return nil, mtproto.ErrInternalServerError
	}
	if foundStickerSet {
		set, loadErr := persist.GetStickerSet(stickerRequestContext(c), userId, stickerSetID, "")
		if loadErr != nil {
			return nil, stickerProviderError(c, loadErr)
		}
		if set == nil {
			return nil, mtproto.ErrInternalServerError
		}
		channelFull.Stickerset = stickerSetFromRecord(set, 0).GetSet()
	}
	emojiSetID, foundEmojiSet, err := domain.LoadChannelEmojiStickerSet(ch.ID)
	if err != nil {
		return nil, mtproto.ErrInternalServerError
	}
	if foundEmojiSet {
		set, loadErr := persist.GetStickerSet(stickerRequestContext(c), userId, emojiSetID, "")
		if loadErr != nil {
			return nil, stickerProviderError(c, loadErr)
		}
		if set == nil || !set.Emojis {
			return nil, mtproto.ErrInternalServerError
		}
		channelFull.Emojiset = stickerSetFromRecord(set, 0).GetSet()
	}
	return box, nil
}

// resolveChannel reads the canonical PostgreSQL channel row when configured. The
// KV-only path is retained solely for deployments that have no canonical
// store configured; it must not shadow a missing row in production.
func (c *ApiFullCore) resolveChannel(userID, id int64) (domain.Channel, error) {
	if id == 0 {
		return domain.Channel{}, mtproto.ErrChannelInvalid
	}
	ch, ok, err := domain.LoadChannel(id)
	if err != nil {
		c.Logger.Errorf("channels.resolveChannel - PostgreSQL read failed")
		return domain.Channel{}, mtproto.ErrInternalServerError
	}
	if ok {
		return ch, nil
	}
	if domain.Ready() {
		return domain.Channel{}, mtproto.ErrChannelInvalid
	}
	rec, found, err := loadChan(userID, id)
	if err != nil {
		return domain.Channel{}, err
	}
	if !found {
		return domain.Channel{}, mtproto.ErrChannelInvalid
	}
	return domain.Channel{
		ID:         id,
		AccessHash: id,
		Creator:    userID,
		Title:      rec.Title,
		About:      rec.About,
		Broadcast:  true,
	}, nil
}

func (c *ApiFullCore) resolveMemberChannel(userID int64, input *mtproto.InputChannel, id int64) (domain.Channel, error) {
	if input == nil || id == 0 {
		return domain.Channel{}, mtproto.ErrChannelInvalid
	}
	ch, err := c.resolveChannel(userID, id)
	if err != nil {
		return domain.Channel{}, err
	}
	if input.GetAccessHash() != ch.AccessHash {
		return domain.Channel{}, mtproto.ErrChannelInvalid
	}
	return ch, nil
}

func (c *ApiFullCore) joinChannel(userID int64, input *mtproto.InputChannel) error {
	ch, err := c.resolveMemberChannel(userID, input, inputChannelID(input))
	if err != nil {
		return err
	}
	if ch.Username == "" {
		d := c.apifullDao()
		if d == nil || d.UserClient == nil {
			return mtproto.ErrChannelPrivate
		}
		ctx := c.ctx
		if ctx == nil {
			ctx = context.Background()
		}
		username, err := d.UserClient.UserGetChannelUsername(ctx, &userpb.TLUserGetChannelUsername{ChannelId: ch.ID})
		if errors.Is(err, mtproto.ErrUsernameNotOccupied) {
			return mtproto.ErrChannelPrivate
		}
		if err != nil {
			return err
		}
		if username == nil || username.GetUsername() == "" || !username.GetActive() || username.GetPeer() == nil ||
			username.GetPeer().GetPredicateName() != mtproto.Predicate_peerChannel || username.GetPeer().GetChannelId() != ch.ID {
			return mtproto.ErrChannelPrivate
		}
	}
	if err = domain.JoinChannel(ch.ID, userID); err != nil {
		return c.mapChannelMemberError(err)
	}
	return nil
}

func (c *ApiFullCore) inviteChannelUsers(userID int64, input *mtproto.InputChannel, users []*mtproto.InputUser) error {
	ch, err := c.resolveMemberChannel(userID, input, inputChannelID(input))
	if err != nil {
		return err
	}
	if len(users) == 0 {
		return mtproto.ErrUsersTooFew
	}
	// Check the actor's channel permission before resolving invitee entities.
	// An unauthorized member must receive CHAT_ADMIN_REQUIRED even when the
	// optional User provider is unavailable.
	canInvite, err := domain.ChannelCanInvite(ch, userID, time.Now().Unix())
	if err != nil {
		return c.mapChannelMemberError(err)
	}
	if !canInvite {
		return mtproto.ErrChatAdminRequired
	}
	userIDs, err := c.resolveChannelInviteeIDs(userID, users)
	if err != nil {
		return err
	}
	if err = domain.InviteChannelMembers(ch.ID, userID, userIDs); err != nil {
		return c.mapChannelMemberError(err)
	}
	return nil
}

func (c *ApiFullCore) resolveChannelInviteeIDs(caller int64, users []*mtproto.InputUser) ([]int64, error) {
	d := c.apifullDao()
	if d == nil || d.UserClient == nil {
		return nil, mtproto.ErrInternalServerError
	}
	ctx := c.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	ids := make([]int64, 0, len(users))
	seen := make(map[int64]struct{}, len(users))
	for _, input := range users {
		if input == nil {
			return nil, mtproto.ErrUserIdInvalid
		}
		peer := mtproto.FromInputUser(caller, input)
		if peer == nil || (peer.PeerType != mtproto.PEER_SELF && peer.PeerType != mtproto.PEER_USER) || peer.PeerId <= 0 {
			return nil, mtproto.ErrUserIdInvalid
		}
		if _, ok := seen[peer.PeerId]; ok {
			continue
		}
		seen[peer.PeerId] = struct{}{}
		target, err := d.UserClient.UserGetImmutableUserV2(ctx, &userpb.TLUserGetImmutableUserV2{Id: peer.PeerId})
		if err != nil {
			return nil, err
		}
		if target == nil || target.GetUser() == nil || target.GetUser().GetId() != peer.PeerId {
			return nil, mtproto.ErrUserIdInvalid
		}
		if target.GetUser().GetDeleted() {
			return nil, mtproto.ErrInputUserDeactivated
		}
		ids = append(ids, peer.PeerId)
	}
	return ids, nil
}

func (c *ApiFullCore) requireChannelMember(userID, channelID int64) error {
	member, err := domain.ChannelIsMember(channelID, userID)
	if err != nil {
		return c.mapChannelMemberError(err)
	}
	if !member {
		return mtproto.ErrUserNotParticipant
	}
	return nil
}

func (c *ApiFullCore) mapChannelMemberError(err error) error {
	switch {
	case errors.Is(err, domain.ErrChannelMissing):
		return mtproto.ErrChannelInvalid
	case errors.Is(err, domain.ErrNotCreator):
		return mtproto.ErrChatAdminRequired
	case errors.Is(err, domain.ErrChannelCreator):
		return mtproto.ErrUserCreator
	case errors.Is(err, domain.ErrNotChannelMember):
		return mtproto.ErrUserNotParticipant
	case errors.Is(err, domain.ErrChannelMemberExists):
		return mtproto.ErrUserAlreadyParticipant
	case errors.Is(err, domain.ErrInvalidChannelMember):
		return mtproto.ErrUserIdInvalid
	case errors.Is(err, domain.ErrNoChannelMembers):
		return mtproto.ErrUsersTooFew
	default:
		return mtproto.ErrInternalServerError
	}
}

func channelMemberPeerID(caller int64, peer *mtproto.InputPeer) (int64, error) {
	if peer == nil {
		return 0, mtproto.ErrUserIdInvalid
	}
	switch peer.GetPredicateName() {
	case mtproto.Predicate_inputPeerSelf:
		return caller, nil
	case mtproto.Predicate_inputPeerUser:
		if peer.GetUserId() == 0 {
			return 0, mtproto.ErrUserIdInvalid
		}
		return peer.GetUserId(), nil
	default:
		return 0, mtproto.ErrUserIdInvalid
	}
}

func channelMemberInputUser(peer *mtproto.InputPeer) (*mtproto.InputUser, error) {
	if peer == nil {
		return nil, mtproto.ErrUserIdInvalid
	}
	switch peer.GetPredicateName() {
	case mtproto.Predicate_inputPeerSelf:
		return mtproto.MakeTLInputUserSelf(&mtproto.InputUser{}).To_InputUser(), nil
	case mtproto.Predicate_inputPeerUser:
		if peer.GetUserId() <= 0 {
			return nil, mtproto.ErrUserIdInvalid
		}
		return mtproto.MakeTLInputUser(&mtproto.InputUser{UserId: peer.GetUserId(), AccessHash: peer.GetAccessHash()}).To_InputUser(), nil
	default:
		return nil, mtproto.ErrUserIdInvalid
	}
}

func (c *ApiFullCore) ChannelsCreateChannel(in *mtproto.TLChannelsCreateChannel) (*mtproto.Updates, error) {
	userId, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	title, about := "", ""
	broadcast := true
	megagroup := false
	if in != nil {
		title = in.GetTitle()
		about = in.GetAbout()
		megagroup = in.GetMegagroup()
		broadcast = !megagroup
	}
	id := time.Now().UnixNano()
	if id <= 0 {
		id = 1
	}
	if err = c.saveChan(userId, id, title, about); err != nil {
		return nil, err
	}
	if err = domain.SaveChannel(domain.Channel{
		ID:         id,
		AccessHash: id,
		Creator:    userId,
		Title:      title,
		About:      about,
		Broadcast:  broadcast,
		Megagroup:  megagroup,
	}); err != nil {
		return nil, err
	}
	return mtproto.MakeTLUpdates(&mtproto.Updates{
		Chats: []*mtproto.Chat{channelview.Chat(domain.Channel{
			ID:         id,
			AccessHash: id,
			Creator:    userId,
			Title:      title,
			About:      about,
			Broadcast:  broadcast,
			Megagroup:  megagroup,
			CreatedAt:  time.Now().Unix(),
		}, true)},
	}).To_Updates(), nil
}

func channelAdminRights(in *mtproto.ChatAdminRights) *domain.ChannelAdminRights {
	if in == nil {
		return nil
	}
	return &domain.ChannelAdminRights{
		ChangeInfo:           in.GetChangeInfo(),
		PostMessages:         in.GetPostMessages(),
		EditMessages:         in.GetEditMessages(),
		DeleteMessages:       in.GetDeleteMessages(),
		BanUsers:             in.GetBanUsers(),
		InviteUsers:          in.GetInviteUsers(),
		PinMessages:          in.GetPinMessages(),
		AddAdmins:            in.GetAddAdmins(),
		Anonymous:            in.GetAnonymous(),
		ManageCall:           in.GetManageCall(),
		Other:                in.GetOther(),
		ManageTopics:         in.GetManageTopics(),
		PostStories:          in.GetPostStories(),
		EditStories:          in.GetEditStories(),
		DeleteStories:        in.GetDeleteStories(),
		ManageDirectMessages: in.GetManageDirectMessages(),
		ManageRanks:          in.GetManageRanks(),
		ManageLinkedPeers:    in.GetManageLinkedPeers(),
	}
}

func channelBannedRights(in *mtproto.ChatBannedRights) *domain.ChannelBannedRights {
	if in == nil || in.NoBanRights() {
		return nil
	}
	rights := &domain.ChannelBannedRights{
		ViewMessages:    in.GetViewMessages(),
		SendMessages:    in.GetSendMessages(),
		SendMedia:       in.GetSendMedia(),
		SendStickers:    in.GetSendStickers(),
		SendGifs:        in.GetSendGifs(),
		SendGames:       in.GetSendGames(),
		SendInline:      in.GetSendInline(),
		EmbedLinks:      in.GetEmbedLinks(),
		SendPolls:       in.GetSendPolls(),
		ChangeInfo:      in.GetChangeInfo(),
		InviteUsers:     in.GetInviteUsers(),
		PinMessages:     in.GetPinMessages(),
		ManageTopics:    in.GetManageTopics(),
		SendPhotos:      in.GetSendPhotos(),
		SendVideos:      in.GetSendVideos(),
		SendRoundvideos: in.GetSendRoundvideos(),
		SendAudios:      in.GetSendAudios(),
		SendVoices:      in.GetSendVoices(),
		SendDocs:        in.GetSendDocs(),
		SendPlain:       in.GetSendPlain(),
		EditRank:        in.GetEditRank(),
		SendReactions:   in.GetSendReactions(),
		UntilDate:       in.GetUntilDate(),
	}
	if !rights.Active(time.Now().Unix()) {
		return nil
	}
	return rights
}

// hydrateChannelUsers uses the canonical user service when this BFF has one.
// A configured client must return every requested entity or the response
// fails closed instead of emitting ID-only users.
func (c *ApiFullCore) hydrateChannelUsers(userID int64, ids ...int64) ([]*mtproto.User, error) {
	d := c.apifullDao()
	if d == nil || d.UserClient == nil {
		return nil, nil
	}
	unique := make([]int64, 0, len(ids)+1)
	seen := make(map[int64]struct{}, len(ids)+1)
	for _, id := range append(ids, userID) {
		if id <= 0 {
			return nil, mtproto.ErrUserIdInvalid
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		unique = append(unique, id)
	}
	ctx := c.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	mutable, err := d.UserClient.UserGetMutableUsersV2(ctx, &userpb.TLUserGetMutableUsersV2{
		Id: unique, Privacy: true, HasTo: true, To: []int64{userID},
	})
	if err != nil {
		return nil, err
	}
	if mutable == nil {
		return nil, mtproto.ErrInternalServerError
	}
	users := mutable.GetUserListByIdList(userID, ids...)
	if len(users) != len(ids) {
		return nil, mtproto.ErrUserIdInvalid
	}
	return users, nil
}

func (c *ApiFullCore) ChannelsEditAdmin(in *mtproto.TLChannelsEditAdmin) (*mtproto.Updates, error) {
	userID, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil || in.GetChannel() == nil || in.GetUserId() == nil || in.GetAdminRights() == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	channelID := inputChannelID(in.GetChannel())
	channel, err := c.resolveMemberChannel(userID, in.GetChannel(), channelID)
	if err != nil {
		return nil, err
	}
	targets, err := c.resolveChannelInviteeIDs(userID, []*mtproto.InputUser{in.GetUserId()})
	if err != nil {
		return nil, err
	}
	if len(targets) != 1 {
		return nil, mtproto.ErrUserIdInvalid
	}
	rank := in.GetRank_STRING()
	if in.GetRank_FLAGSTRING() != nil {
		rank = in.GetRank_FLAGSTRING().GetValue()
	}
	if len([]rune(rank)) > 64 {
		return nil, mtproto.ErrInputRequestInvalid
	}
	oldMember, found, err := domain.LoadChannelMember(channelID, targets[0])
	if err != nil {
		return nil, c.mapChannelMemberError(err)
	}
	if !found {
		return nil, mtproto.ErrUserNotParticipant
	}
	if err = domain.EditChannelAdmin(channelID, userID, targets[0], channelAdminRights(in.GetAdminRights()), rank); err != nil {
		return nil, c.mapChannelMemberError(err)
	}
	newMember, found, err := domain.LoadChannelMember(channelID, targets[0])
	if err != nil {
		return nil, c.mapChannelMemberError(err)
	}
	if !found {
		return nil, mtproto.ErrUserNotParticipant
	}
	update := mtproto.MakeTLUpdateChannelParticipant(&mtproto.Update{
		ChannelId:                              channelID,
		Date_INT32:                             int32(time.Now().Unix()),
		ActorId:                                userID,
		UserId:                                 targets[0],
		PrevParticipant_FLAGCHANNELPARTICIPANT: channelview.MemberParticipant(userID, channel, oldMember),
		NewParticipant_FLAGCHANNELPARTICIPANT:  channelview.MemberParticipant(userID, channel, newMember),
	}).To_Update()
	users := []*mtproto.User{mtproto.MakeTLUser(&mtproto.User{Id: targets[0], Self: targets[0] == userID}).To_User()}
	return mtproto.MakeUpdatesByUpdatesUsersChats(users, []*mtproto.Chat{channelview.Chat(channel, channel.Creator == userID)}, update), nil
}

func (c *ApiFullCore) ChannelsEditTitle(in *mtproto.TLChannelsEditTitle) (*mtproto.Updates, error) {
	userID, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil || in.GetChannel() == nil || in.GetChannel().GetChannelId() == 0 {
		return nil, mtproto.ErrChannelInvalid
	}
	id := in.GetChannel().GetChannelId()
	if _, err = c.resolveMemberChannel(userID, in.GetChannel(), id); err != nil {
		return nil, err
	}
	if err = domain.UpdateChannelTitle(userID, id, in.GetTitle()); err != nil {
		switch {
		case errors.Is(err, domain.ErrChannelMissing):
			return nil, mtproto.ErrChannelInvalid
		case errors.Is(err, domain.ErrNotCreator):
			return nil, mtproto.ErrChatAdminRequired
		default:
			return nil, err
		}
	}
	ch, ok, err := domain.LoadChannel(id)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, mtproto.ErrChannelInvalid
	}
	return mtproto.MakeUpdatesByUpdatesChats([]*mtproto.Chat{channelview.Chat(ch, ch.Creator == userID)}), nil
}

func (c *ApiFullCore) ChannelsEditPhoto(in *mtproto.TLChannelsEditPhoto) (*mtproto.Updates, error) {
	userID, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil || in.GetChannel() == nil || in.GetPhoto() == nil {
		return nil, mtproto.ErrInputConstructorInvalid
	}
	channel, err := c.resolveMemberChannel(userID, in.GetChannel(), inputChannelID(in.GetChannel()))
	if err != nil {
		return nil, err
	}
	photo := in.GetPhoto()
	var photoID int64
	var photoDCID int32
	var hasVideo bool
	switch photo.GetPredicateName() {
	case mtproto.Predicate_inputChatPhotoEmpty:
		// Clearing a photo is an owner-only mutation, enforced by the domain.
	case mtproto.Predicate_inputChatUploadedPhoto:
		if photo.GetFile() == nil && photo.GetVideo() == nil {
			return nil, mtproto.ErrMediaInvalid
		}
		d := c.apifullDao()
		if d == nil || d.DfsClient == nil {
			return nil, mtproto.ErrInternalServerError
		}
		creator := c.MD.GetPermAuthKeyId()
		if creator == 0 {
			creator = userID
		}
		uploaded, uploadErr := d.DfsClient.DfsUploadProfilePhotoFileV2(c.ctx, &dfs.TLDfsUploadProfilePhotoFileV2{
			Creator: creator, File: photo.GetFile(), Video: photo.GetVideo(),
			VideoStartTs: photo.GetVideoStartTs(), VideoEmojiMarkup: photo.GetVideoEmojiMarkup(),
		})
		if uploadErr != nil {
			return nil, uploadErr
		}
		if uploaded == nil || uploaded.GetId() <= 0 || uploaded.GetDcId() <= 0 || len(uploaded.GetSizes()) == 0 {
			return nil, mtproto.ErrMediaInvalid
		}
		photoID, photoDCID = uploaded.GetId(), uploaded.GetDcId()
		hasVideo = len(uploaded.GetVideoSizes()) > 0
	case mtproto.Predicate_inputChatPhoto:
		inputPhoto := photo.GetId()
		if inputPhoto == nil || inputPhoto.GetId() <= 0 || inputPhoto.GetAccessHash() == 0 {
			return nil, mtproto.ErrPhotoInvalid
		}
		photoID, photoDCID = inputPhoto.GetId(), 1
	default:
		return nil, mtproto.ErrInputConstructorInvalid
	}
	if err = domain.UpdateChannelPhoto(userID, channel.ID, photoID, photoDCID, hasVideo); err != nil {
		switch {
		case errors.Is(err, domain.ErrChannelMissing):
			return nil, mtproto.ErrChannelInvalid
		case errors.Is(err, domain.ErrNotCreator):
			return nil, mtproto.ErrChatAdminRequired
		default:
			return nil, err
		}
	}
	updated, ok, err := domain.LoadChannel(channel.ID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, mtproto.ErrChannelInvalid
	}
	return mtproto.MakeUpdatesByUpdatesChats([]*mtproto.Chat{channelview.Chat(updated, updated.Creator == userID)}), nil
}

func (c *ApiFullCore) ChannelsJoinChannel7F6A1E22(in *mtproto.TLChannelsJoinChannel7F6A1E22) (*mtproto.Messages_ChatInviteJoinResult, error) {
	userID, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	var input *mtproto.InputChannel
	if in != nil {
		input = in.GetChannel()
	}
	if err = c.joinChannel(userID, input); err != nil {
		return nil, err
	}
	return mtproto.MakeTLMessagesChatInviteJoinResultOk(&mtproto.Messages_ChatInviteJoinResult{
		Updates: mtproto.MakeEmptyUpdates(),
	}).To_Messages_ChatInviteJoinResult(), nil
}

func (c *ApiFullCore) ChannelsLeaveChannel(in *mtproto.TLChannelsLeaveChannel) (*mtproto.Updates, error) {
	userID, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	var input *mtproto.InputChannel
	if in != nil {
		input = in.GetChannel()
	}
	ch, err := c.resolveMemberChannel(userID, input, inputChannelID(input))
	if err != nil {
		return nil, err
	}
	if err = domain.LeaveChannel(ch.ID, userID); err != nil {
		return nil, c.mapChannelMemberError(err)
	}
	return mtproto.MakeEmptyUpdates(), nil
}

func (c *ApiFullCore) ChannelsInviteToChannelC9E33D54(in *mtproto.TLChannelsInviteToChannelC9E33D54) (*mtproto.Messages_InvitedUsers, error) {
	userID, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	var channel *mtproto.InputChannel
	var users []*mtproto.InputUser
	if in != nil {
		channel = in.GetChannel()
		users = in.GetUsers()
	}
	if err = c.inviteChannelUsers(userID, channel, users); err != nil {
		return nil, err
	}
	return mtproto.MakeTLMessagesInvitedUsers(&mtproto.Messages_InvitedUsers{
		Updates:         mtproto.MakeEmptyUpdates(),
		MissingInvitees: []*mtproto.MissingInvitee{},
	}).To_Messages_InvitedUsers(), nil
}

func (c *ApiFullCore) ChannelsDeleteChannel(in *mtproto.TLChannelsDeleteChannel) (*mtproto.Updates, error) {
	userID, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	// Keep the historical nil probe fail-closed. A real request with a
	// channel is handled by the durable audit path below.
	if in == nil {
		return nil, mtproto.ErrMethodNotImpl
	}
	if in.GetChannel() == nil || inputChannelID(in.GetChannel()) <= 0 {
		return nil, mtproto.ErrChannelInvalid
	}
	channel, err := c.resolveMemberChannel(userID, in.GetChannel(), inputChannelID(in.GetChannel()))
	if err != nil {
		return nil, err
	}
	if err = domain.DeleteChannel(userID, channel.ID); err != nil {
		return nil, c.mapChannelMemberError(err)
	}
	// Channels created by this APIFull shard also have a legacy KV record. Keep
	// that record from making a successfully deleted channel visible again while
	// retaining the existing Store interface used by Redis and MySQL backends.
	if err = persist.Default.Set(chanDataKey(channel.Creator, channel.ID), ""); err != nil {
		return nil, err
	}
	return mtproto.MakeEmptyUpdates(), nil
}

func (c *ApiFullCore) ChannelsExportMessageLink(in *mtproto.TLChannelsExportMessageLink) (*mtproto.ExportedMessageLink, error) {
	userID, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil || in.GetChannel() == nil || inputChannelID(in.GetChannel()) <= 0 || in.GetId() <= 0 {
		return nil, mtproto.ErrInputRequestInvalid
	}
	if in.GetGrouped() || in.GetThread() {
		return nil, mtproto.ErrMethodNotImpl
	}
	channel, err := c.resolveMemberChannel(userID, in.GetChannel(), inputChannelID(in.GetChannel()))
	if err != nil {
		return nil, err
	}
	if err = c.requireChannelMember(userID, channel.ID); err != nil {
		return nil, err
	}
	if channel.Username == "" {
		return nil, mtproto.ErrChannelPrivate
	}
	if _, err = channelview.MessagesBox(userID, channel.ID, []int32{in.GetId()}); err != nil {
		return nil, err
	}
	link := fmt.Sprintf("https://t.me/%s/%d", channel.Username, in.GetId())
	return mtproto.MakeTLExportedMessageLink(&mtproto.ExportedMessageLink{
		Link: link,
		Html: html.EscapeString(link),
	}).To_ExportedMessageLink(), nil
}

func (c *ApiFullCore) ChannelsToggleSignatures(in *mtproto.TLChannelsToggleSignatures) (*mtproto.Updates, error) {
	if _, err := c.requireUserId(); err != nil {
		return nil, err
	}
	if in == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	signatures, profiles := in.GetSignaturesEnabled(), in.GetProfilesEnabled()
	return c.updateChannelSettings(in.GetChannel(), domain.ChannelSettings{
		Signatures:        &signatures,
		SignatureProfiles: &profiles,
	})
}

func (c *ApiFullCore) ChannelsGetAdminedPublicChannels(in *mtproto.TLChannelsGetAdminedPublicChannels) (*mtproto.Messages_Chats, error) {
	_ = in
	userId, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	channels, err := domain.ListByCreator(userId)
	if err != nil {
		return nil, err
	}
	chats := make([]*mtproto.Chat, 0, len(channels))
	for _, channel := range channels {
		if channel.Username == "" {
			continue
		}
		chats = append(chats, channelview.Chat(channel, true))
	}
	return mtproto.MakeTLMessagesChats(&mtproto.Messages_Chats{Chats: chats}).To_Messages_Chats(), nil
}

func (c *ApiFullCore) ChannelsEditBanned(in *mtproto.TLChannelsEditBanned) (*mtproto.Updates, error) {
	userID, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil || in.GetChannel() == nil || in.GetParticipant() == nil || in.GetBannedRights() == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	channelID := inputChannelID(in.GetChannel())
	channel, err := c.resolveMemberChannel(userID, in.GetChannel(), channelID)
	if err != nil {
		return nil, err
	}
	target, err := channelMemberInputUser(in.GetParticipant())
	if err != nil {
		return nil, err
	}
	targetIDs, err := c.resolveChannelInviteeIDs(userID, []*mtproto.InputUser{target})
	if err != nil {
		return nil, err
	}
	if len(targetIDs) != 1 {
		return nil, mtproto.ErrUserIdInvalid
	}
	targetID := targetIDs[0]
	oldMember, oldFound, err := domain.LoadChannelMember(channelID, targetID)
	if err != nil {
		return nil, c.mapChannelMemberError(err)
	}
	rights := channelBannedRights(in.GetBannedRights())
	if err = domain.EditChannelBanned(channelID, userID, targetID, rights, time.Now().Unix()); err != nil {
		return nil, c.mapChannelMemberError(err)
	}
	newMember, newFound, err := domain.LoadChannelMember(channelID, targetID)
	if err != nil {
		return nil, c.mapChannelMemberError(err)
	}
	prev := channelview.LeftParticipant(targetID)
	if oldFound {
		prev = channelview.MemberParticipant(userID, channel, oldMember)
	}
	next := channelview.LeftParticipant(targetID)
	if newFound {
		next = channelview.MemberParticipant(userID, channel, newMember)
	}
	update := mtproto.MakeTLUpdateChannelParticipant(&mtproto.Update{
		ChannelId:                              channelID,
		Date_INT32:                             int32(time.Now().Unix()),
		ActorId:                                userID,
		UserId:                                 targetID,
		PrevParticipant_FLAGCHANNELPARTICIPANT: prev,
		NewParticipant_FLAGCHANNELPARTICIPANT:  next,
	}).To_Update()
	users := []*mtproto.User{mtproto.MakeTLUser(&mtproto.User{Id: targetID, Self: targetID == userID}).To_User()}
	return mtproto.MakeUpdatesByUpdatesUsersChats(users, []*mtproto.Chat{channelview.Chat(channel, channel.Creator == userID)}, update), nil
}

func (c *ApiFullCore) ChannelsGetAdminLog(in *mtproto.TLChannelsGetAdminLog) (*mtproto.Channels_AdminLogResults, error) {
	userID, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil || in.GetChannel() == nil || inputChannelID(in.GetChannel()) <= 0 {
		return nil, mtproto.ErrChannelInvalid
	}
	channel, err := c.resolveMemberChannel(userID, in.GetChannel(), inputChannelID(in.GetChannel()))
	if err != nil {
		return nil, err
	}
	canView, err := domain.CanViewChannelAdminLog(channel.ID, userID)
	if err != nil {
		return nil, c.mapChannelMemberError(err)
	}
	if !canView {
		return nil, mtproto.ErrChatAdminRequired
	}
	limit := in.GetLimit()
	if limit <= 0 {
		limit = 100
	}
	if limit > 100 {
		limit = 100
	}
	var actorIDs []int64
	seenActors := make(map[int64]struct{}, len(in.GetAdmins()))
	for _, admin := range in.GetAdmins() {
		if admin == nil {
			return nil, mtproto.ErrUserIdInvalid
		}
		peer := mtproto.FromInputUser(userID, admin)
		if peer == nil || peer.PeerId <= 0 || (peer.PeerType != mtproto.PEER_SELF && peer.PeerType != mtproto.PEER_USER) {
			return nil, mtproto.ErrUserIdInvalid
		}
		if _, ok := seenActors[peer.PeerId]; !ok {
			seenActors[peer.PeerId] = struct{}{}
			actorIDs = append(actorIDs, peer.PeerId)
		}
	}
	logs, err := domain.ListChannelAdminLogs(channel.ID, in.GetMinId(), in.GetMaxId(), limit, actorIDs)
	if err != nil {
		return nil, mtproto.ErrInternalServerError
	}
	events := make([]*mtproto.ChannelAdminLogEvent, 0, len(logs))
	userIDs := make([]int64, 0, len(logs)*2)
	seenUsers := make(map[int64]struct{}, len(logs)*2)
	for _, record := range logs {
		if !channelAdminLogFilterAllows(in.GetEventsFilter(), record) {
			continue
		}
		action := channelAdminLogAction(record, channel, userID)
		if action == nil {
			continue
		}
		events = append(events, mtproto.MakeTLChannelAdminLogEvent(&mtproto.ChannelAdminLogEvent{
			Id: record.ID, Date: int32(record.Date), UserId: record.ActorID, Action: action,
		}).To_ChannelAdminLogEvent())
		for _, id := range []int64{record.ActorID, record.TargetID} {
			if id > 0 {
				if _, ok := seenUsers[id]; !ok {
					seenUsers[id] = struct{}{}
					userIDs = append(userIDs, id)
				}
			}
		}
	}
	users := make([]*mtproto.User, 0, len(userIDs))
	if hydrated, hydrateErr := c.hydrateChannelUsers(userID, userIDs...); hydrateErr != nil {
		return nil, hydrateErr
	} else if hydrated != nil {
		users = hydrated
	} else {
		for _, id := range userIDs {
			users = append(users, mtproto.MakeTLUser(&mtproto.User{Id: id, Self: id == userID}).To_User())
		}
	}
	return mtproto.MakeTLChannelsAdminLogResults(&mtproto.Channels_AdminLogResults{
		Events: events,
		Chats:  []*mtproto.Chat{channelview.Chat(channel, channel.Creator == userID)},
		Users:  users,
	}).To_Channels_AdminLogResults(), nil
}

func channelAdminLogAction(record domain.ChannelAdminLog, channel domain.Channel, viewerID int64) *mtproto.ChannelAdminLogEventAction {
	snapshot := func(member *domain.ChannelMember) *mtproto.ChannelParticipant {
		if member == nil {
			return channelview.LeftParticipant(record.TargetID)
		}
		return channelview.MemberParticipant(viewerID, channel, *member)
	}
	switch record.Action {
	case domain.ChannelAdminLogParticipantJoin:
		return mtproto.MakeTLChannelAdminLogEventActionParticipantJoin(nil).To_ChannelAdminLogEventAction()
	case domain.ChannelAdminLogParticipantLeave:
		return mtproto.MakeTLChannelAdminLogEventActionParticipantLeave(nil).To_ChannelAdminLogEventAction()
	case domain.ChannelAdminLogParticipantInvite:
		return mtproto.MakeTLChannelAdminLogEventActionParticipantInvite(&mtproto.ChannelAdminLogEventAction{
			Participant_CHANNELPARTICIPANT: snapshot(record.New),
		}).To_ChannelAdminLogEventAction()
	case domain.ChannelAdminLogParticipantToggleBan:
		return mtproto.MakeTLChannelAdminLogEventActionParticipantToggleBan(&mtproto.ChannelAdminLogEventAction{
			PrevParticipant: snapshot(record.Prev), NewParticipant: snapshot(record.New),
		}).To_ChannelAdminLogEventAction()
	case domain.ChannelAdminLogParticipantToggleAdmin:
		return mtproto.MakeTLChannelAdminLogEventActionParticipantToggleAdmin(&mtproto.ChannelAdminLogEventAction{
			PrevParticipant: snapshot(record.Prev), NewParticipant: snapshot(record.New),
		}).To_ChannelAdminLogEventAction()
	default:
		return nil
	}
}

func channelAdminLogFilterAllows(filter *mtproto.ChannelAdminLogEventsFilter, record domain.ChannelAdminLog) bool {
	if filter == nil {
		return true
	}
	any := filter.GetJoin() || filter.GetLeave() || filter.GetInvite() || filter.GetBan() || filter.GetUnban() ||
		filter.GetKick() || filter.GetUnkick() || filter.GetPromote() || filter.GetDemote() || filter.GetEditRank()
	if !any {
		return false
	}
	switch record.Action {
	case domain.ChannelAdminLogParticipantJoin:
		return filter.GetJoin()
	case domain.ChannelAdminLogParticipantLeave:
		return filter.GetLeave()
	case domain.ChannelAdminLogParticipantInvite:
		return filter.GetInvite()
	case domain.ChannelAdminLogParticipantToggleAdmin:
		previousAdmin := record.Prev != nil && record.Prev.AdminRights != nil && !record.Prev.AdminRights.Empty()
		nextAdmin := record.New != nil && record.New.AdminRights != nil && !record.New.AdminRights.Empty()
		if previousAdmin && nextAdmin && record.Prev.Rank != record.New.Rank {
			return filter.GetEditRank()
		}
		if nextAdmin && !previousAdmin {
			return filter.GetPromote()
		}
		return filter.GetDemote()
	case domain.ChannelAdminLogParticipantToggleBan:
		previousBanned := record.Prev != nil && record.Prev.BannedRights != nil && record.Prev.BannedRights.Active(record.Date)
		nextBanned := record.New != nil && record.New.BannedRights != nil && record.New.BannedRights.Active(record.Date)
		previousKick := previousBanned && record.Prev.BannedRights.Kicks(record.Date)
		nextKick := nextBanned && record.New.BannedRights.Kicks(record.Date)
		if nextKick && !previousKick {
			return filter.GetKick()
		}
		if nextBanned && !nextKick {
			return filter.GetBan()
		}
		if previousKick {
			return filter.GetUnkick()
		}
		if previousBanned {
			return filter.GetUnban()
		}
		return false
	default:
		return false
	}
}

func (c *ApiFullCore) ChannelsSetStickers(in *mtproto.TLChannelsSetStickers) (*mtproto.Bool, error) {
	userID, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	// Preserve the historical nil probe used by the compatibility suite while
	// handling real requests through the canonical PostgreSQL providers.
	if in == nil {
		return nil, mtproto.ErrMethodNotImpl
	}
	if in.GetChannel() == nil || in.GetStickerset() == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	channel, err := c.resolveMemberChannel(userID, in.GetChannel(), inputChannelID(in.GetChannel()))
	if err != nil {
		return nil, err
	}
	setID := int64(0)
	set := in.GetStickerset()
	switch set.GetPredicateName() {
	case mtproto.Predicate_inputStickerSetEmpty:
		// Empty clears the channel's sticker binding.
	case mtproto.Predicate_inputStickerSetID:
		if set.GetId() <= 0 {
			return nil, mtproto.ErrStickersetInvalid
		}
		setID = set.GetId()
	case mtproto.Predicate_inputStickerSetShortName:
		if set.GetShortName() == "" {
			return nil, mtproto.ErrStickersetInvalid
		}
	default:
		return nil, mtproto.ErrStickersetInvalid
	}
	if setID != 0 || set.GetPredicateName() == mtproto.Predicate_inputStickerSetShortName {
		loaded, loadErr := persist.GetStickerSet(stickerRequestContext(c), userID, setID, set.GetShortName())
		if loadErr != nil {
			return nil, stickerProviderError(c, loadErr)
		}
		if accessErr := validateStickerSetAccess(set, loaded); accessErr != nil {
			return nil, accessErr
		}
		setID = loaded.ID
	}
	if err = domain.SetChannelStickerSet(userID, channel.ID, setID); err != nil {
		return nil, c.mapChannelMemberError(err)
	}
	return mtproto.BoolTrue, nil
}

func (c *ApiFullCore) ChannelsReadMessageContents(in *mtproto.TLChannelsReadMessageContents) (*mtproto.Bool, error) {
	userID, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil || in.GetChannel() == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	if len(in.GetId()) == 0 {
		return nil, mtproto.ErrMessageIdInvalid
	}
	seen := make(map[int32]struct{}, len(in.GetId()))
	for _, id := range in.GetId() {
		if id <= 0 {
			return nil, mtproto.ErrMessageIdInvalid
		}
		if _, ok := seen[id]; ok {
			return nil, mtproto.ErrMessageIdInvalid
		}
		seen[id] = struct{}{}
	}
	if !domain.Ready() {
		return nil, mtproto.ErrMethodNotImpl
	}
	channel, err := c.resolveMemberChannel(userID, in.GetChannel(), inputChannelID(in.GetChannel()))
	if err != nil {
		return nil, err
	}
	if err = c.requireChannelMember(userID, channel.ID); err != nil {
		return nil, err
	}
	if err = domain.MarkChannelMessageContentsRead(userID, channel.ID, in.GetChannel().GetAccessHash(), in.GetId()); err != nil {
		switch {
		case errors.Is(err, domain.ErrChannelMissing):
			return nil, mtproto.ErrChannelInvalid
		case errors.Is(err, domain.ErrInvalidChannelAccessHash):
			return nil, mtproto.ErrChannelInvalid
		case errors.Is(err, domain.ErrNotChannelMember):
			return nil, mtproto.ErrUserNotParticipant
		case errors.Is(err, domain.ErrInvalidMessageID), errors.Is(err, domain.ErrDuplicateMessageID), errors.Is(err, domain.ErrMessageMissing):
			return nil, mtproto.ErrMessageIdInvalid
		default:
			return nil, mtproto.ErrInternalServerError
		}
	}
	return mtproto.BoolTrue, nil
}

func (c *ApiFullCore) ChannelsDeleteHistory9BAA9647(in *mtproto.TLChannelsDeleteHistory9BAA9647) (*mtproto.Updates, error) {
	userId, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	var inputChannel *mtproto.InputChannel
	var channelID int64
	var maxID int32
	var forEveryone bool
	if in != nil {
		inputChannel = in.GetChannel()
		channelID = inputChannelID(inputChannel)
		maxID = in.GetMaxId()
		forEveryone = in.GetForEveryone()
	}
	if _, err = c.resolveMemberChannel(userId, inputChannel, channelID); err != nil {
		return nil, err
	}
	if err = c.requireChannelMember(userId, channelID); err != nil {
		return nil, err
	}
	return channelview.DeleteHistory(userId, channelID, maxID, forEveryone)
}

func (c *ApiFullCore) ChannelsTogglePreHistoryHidden(in *mtproto.TLChannelsTogglePreHistoryHidden) (*mtproto.Updates, error) {
	if in == nil || in.GetEnabled() == nil {
		if _, err := c.requireUserId(); err != nil {
			return nil, err
		}
		return nil, mtproto.ErrChannelInvalid
	}
	enabled := mtproto.FromBool(in.GetEnabled())
	return c.updateChannelSettings(in.GetChannel(), domain.ChannelSettings{HiddenPrehistory: &enabled})
}

func (c *ApiFullCore) ChannelsGetGroupsForDiscussion(in *mtproto.TLChannelsGetGroupsForDiscussion) (*mtproto.Messages_Chats, error) {
	_ = in
	userID, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if !domain.Ready() {
		return nil, mtproto.ErrMethodNotImpl
	}
	groups, err := domain.ListDiscussionGroups(userID)
	if err != nil {
		return nil, mtproto.ErrInternalServerError
	}
	chats := make([]*mtproto.Chat, 0, len(groups))
	for _, group := range groups {
		chats = append(chats, channelview.Chat(group, group.Creator == userID))
	}
	return mtproto.MakeTLMessagesChats(&mtproto.Messages_Chats{Chats: chats}).To_Messages_Chats(), nil
}

func (c *ApiFullCore) ChannelsSetDiscussionGroup(in *mtproto.TLChannelsSetDiscussionGroup) (*mtproto.Bool, error) {
	userID, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil || in.GetBroadcast() == nil || in.GetGroup() == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	broadcast, err := c.resolveMemberChannel(userID, in.GetBroadcast(), inputChannelID(in.GetBroadcast()))
	if err != nil {
		return nil, err
	}
	if !broadcast.Broadcast {
		return nil, mtproto.ErrChannelInvalid
	}
	groupID := int64(0)
	if in.GetGroup().GetPredicateName() != mtproto.Predicate_inputChannelEmpty {
		group, resolveErr := c.resolveMemberChannel(userID, in.GetGroup(), inputChannelID(in.GetGroup()))
		if resolveErr != nil {
			return nil, resolveErr
		}
		if !group.Megagroup {
			return nil, mtproto.ErrChannelInvalid
		}
		groupID = group.ID
	}
	if err = domain.SetDiscussionGroup(userID, broadcast.ID, groupID); err != nil {
		switch {
		case errors.Is(err, domain.ErrDiscussionChannelInvalid), errors.Is(err, domain.ErrChannelMissing):
			return nil, mtproto.ErrChannelInvalid
		case errors.Is(err, domain.ErrDiscussionAlreadyLinked):
			return nil, mtproto.ErrChatDiscussionUnallowed
		case errors.Is(err, domain.ErrDiscussionNotAllowed):
			return nil, mtproto.ErrChatAdminRequired
		default:
			return nil, mtproto.ErrInternalServerError
		}
	}
	return mtproto.BoolTrue, nil
}

func (c *ApiFullCore) ChannelsEditLocation(in *mtproto.TLChannelsEditLocation) (*mtproto.Bool, error) {
	userID, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil || in.GetChannel() == nil || in.GetGeoPoint() == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	channelID := inputChannelID(in.GetChannel())
	if _, err = c.resolveMemberChannel(userID, in.GetChannel(), channelID); err != nil {
		return nil, err
	}
	if utf8.RuneCountInString(in.GetAddress()) > 255 {
		return nil, mtproto.ErrInputRequestInvalid
	}
	var lat, long *float64
	switch in.GetGeoPoint().GetPredicateName() {
	case mtproto.Predicate_inputGeoPointEmpty:
		// A location-empty input clears both coordinates and address.
	case mtproto.Predicate_inputGeoPoint:
		latValue, longValue := in.GetGeoPoint().GetLat(), in.GetGeoPoint().GetLong()
		if math.IsNaN(latValue) || math.IsInf(latValue, 0) || math.IsNaN(longValue) || math.IsInf(longValue, 0) ||
			latValue < -90 || latValue > 90 || longValue < -180 || longValue > 180 {
			return nil, mtproto.ErrInputRequestInvalid
		}
		lat, long = &latValue, &longValue
	default:
		return nil, mtproto.ErrInputRequestInvalid
	}
	address := in.GetAddress()
	if lat == nil {
		address = ""
	}
	if err = domain.UpdateChannelLocation(userID, channelID, lat, long, address); err != nil {
		switch {
		case errors.Is(err, domain.ErrChannelMissing):
			return nil, mtproto.ErrChannelInvalid
		case errors.Is(err, domain.ErrNotCreator):
			return nil, mtproto.ErrChatAdminRequired
		case errors.Is(err, domain.ErrInvalidLocation):
			return nil, mtproto.ErrInputRequestInvalid
		default:
			return nil, mtproto.ErrInternalServerError
		}
	}
	return mtproto.BoolTrue, nil
}

func (c *ApiFullCore) ChannelsToggleSlowMode(in *mtproto.TLChannelsToggleSlowMode) (*mtproto.Updates, error) {
	if in == nil {
		if _, err := c.requireUserId(); err != nil {
			return nil, err
		}
		return nil, mtproto.ErrChannelInvalid
	}
	seconds := in.GetSeconds()
	return c.updateChannelSettings(in.GetChannel(), domain.ChannelSettings{SlowmodeSeconds: &seconds})
}

func (c *ApiFullCore) ChannelsGetInactiveChannels(in *mtproto.TLChannelsGetInactiveChannels) (*mtproto.Messages_InactiveChats, error) {
	_ = in
	userID, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if !domain.Ready() {
		return nil, mtproto.ErrMethodNotImpl
	}
	// Telegram exposes this method as a cleanup/discovery list.  Keep the
	// cutoff explicit and deterministic: channels with no message activity in
	// the last 30 days are returned, including channels that have never posted.
	cutoff := time.Now().Add(-30 * 24 * time.Hour).Unix()
	inactive, err := domain.ListInactiveChannels(userID, cutoff)
	if err != nil {
		return nil, mtproto.ErrInternalServerError
	}
	chats := make([]*mtproto.Chat, 0, len(inactive))
	dates := make([]int32, 0, len(inactive))
	for _, item := range inactive {
		chats = append(chats, channelview.Chat(item.Channel, item.Channel.Creator == userID))
		lastActive := item.LastActive
		if lastActive > math.MaxInt32 {
			lastActive = math.MaxInt32
		}
		if lastActive < math.MinInt32 {
			lastActive = math.MinInt32
		}
		dates = append(dates, int32(lastActive))
	}
	return mtproto.MakeTLMessagesInactiveChats(&mtproto.Messages_InactiveChats{
		Dates: dates,
		Chats: chats,
		Users: []*mtproto.User{},
	}).To_Messages_InactiveChats(), nil
}

func (c *ApiFullCore) ChannelsDeleteParticipantHistory(in *mtproto.TLChannelsDeleteParticipantHistory) (*mtproto.Messages_AffectedHistory, error) {
	userID, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil || in.GetChannel() == nil {
		return nil, mtproto.ErrChannelInvalid
	}
	if in.GetParticipant() == nil {
		return nil, mtproto.ErrUserIdInvalid
	}
	channel, err := c.resolveMemberChannel(userID, in.GetChannel(), inputChannelID(in.GetChannel()))
	if err != nil {
		return nil, err
	}
	participantID, err := channelMemberPeerID(userID, in.GetParticipant())
	if err != nil {
		return nil, err
	}
	pts, ptsCount, err := domain.DeleteChannelParticipantHistory(channel.ID, userID, participantID)
	if err != nil {
		return nil, c.mapChannelMemberError(err)
	}
	return mtproto.MakeTLMessagesAffectedHistory(&mtproto.Messages_AffectedHistory{
		Pts: pts, PtsCount: ptsCount, Offset: 0,
	}).To_Messages_AffectedHistory(), nil
}

func (c *ApiFullCore) ChannelsToggleParticipantsHidden(in *mtproto.TLChannelsToggleParticipantsHidden) (*mtproto.Updates, error) {
	if in == nil || in.GetEnabled() == nil {
		if _, err := c.requireUserId(); err != nil {
			return nil, err
		}
		return nil, mtproto.ErrChannelInvalid
	}
	enabled := mtproto.FromBool(in.GetEnabled())
	return c.updateChannelSettings(in.GetChannel(), domain.ChannelSettings{ParticipantsHidden: &enabled})
}

func (c *ApiFullCore) updateChannelSettings(channel *mtproto.InputChannel, settings domain.ChannelSettings) (*mtproto.Updates, error) {
	userID, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if channel == nil || channel.GetChannelId() == 0 {
		return nil, mtproto.ErrChannelInvalid
	}
	channelID := channel.GetChannelId()
	if _, err = c.resolveMemberChannel(userID, channel, channelID); err != nil {
		return nil, err
	}
	if err = domain.UpdateChannelSettings(userID, channelID, settings); err != nil {
		switch {
		case errors.Is(err, domain.ErrChannelMissing):
			return nil, mtproto.ErrChannelInvalid
		case errors.Is(err, domain.ErrNotCreator):
			return nil, mtproto.ErrChatAdminRequired
		default:
			return nil, err
		}
	}
	ch, ok, err := domain.LoadChannel(channelID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, mtproto.ErrChannelInvalid
	}
	return mtproto.MakeUpdatesByUpdatesChats([]*mtproto.Chat{channelview.Chat(ch, ch.Creator == userID)}), nil
}

func (c *ApiFullCore) ChannelsJoinChannel24B524C5(in *mtproto.TLChannelsJoinChannel24B524C5) (*mtproto.Updates, error) {
	userID, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	var input *mtproto.InputChannel
	if in != nil {
		input = in.GetChannel()
	}
	if err = c.joinChannel(userID, input); err != nil {
		return nil, err
	}
	return mtproto.MakeEmptyUpdates(), nil
}

func (c *ApiFullCore) ChannelsEditCreator(in *mtproto.TLChannelsEditCreator) (*mtproto.Updates, error) {
	if c.MD == nil || c.MD.UserId == 0 {
		return nil, mtproto.ErrAuthKeyUnregistered
	}
	if err := c.persistChan("ChannelsEditCreator", in); err != nil {
		return nil, err
	}
	return mtproto.MakeEmptyUpdates(), nil
}

func (c *ApiFullCore) ChannelsGetFutureCreatorAfterLeave(in *mtproto.TLChannelsGetFutureCreatorAfterLeave) (*mtproto.User, error) {
	if c.MD == nil || c.MD.UserId == 0 {
		return nil, mtproto.ErrAuthKeyUnregistered
	}
	if err := c.persistChan("ChannelsGetFutureCreatorAfterLeave", in); err != nil {
		return nil, err
	}
	return mtproto.MakeTLUserEmpty(&mtproto.User{}).To_User(), nil
}

func (c *ApiFullCore) ChannelsInviteToChannel199F3A6C(in *mtproto.TLChannelsInviteToChannel199F3A6C) (*mtproto.Updates, error) {
	userID, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	var channel *mtproto.InputChannel
	var users []*mtproto.InputUser
	if in != nil {
		channel = in.GetChannel()
		users = in.GetUsers()
	}
	if err = c.inviteChannelUsers(userID, channel, users); err != nil {
		return nil, err
	}
	return mtproto.MakeEmptyUpdates(), nil
}

func (c *ApiFullCore) ChannelsDeleteHistoryAF369D42(in *mtproto.TLChannelsDeleteHistoryAF369D42) (*mtproto.Bool, error) {
	userId, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	var inputChannel *mtproto.InputChannel
	var channelID int64
	var maxID int32
	if in != nil {
		inputChannel = in.GetChannel()
		channelID = inputChannelID(inputChannel)
		maxID = in.GetMaxId()
	}
	if _, err = c.resolveMemberChannel(userId, inputChannel, channelID); err != nil {
		return nil, err
	}
	if err = c.requireChannelMember(userId, channelID); err != nil {
		return nil, err
	}
	if _, err = channelview.DeleteHistory(userId, channelID, maxID, false); err != nil {
		return nil, err
	}
	return mtproto.BoolTrue, nil
}
