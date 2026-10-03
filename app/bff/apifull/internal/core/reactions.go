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
	"strconv"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/dao"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/domain"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/persist"
	"github.com/teamgram/teamgram-server/app/service/biz/chat/chat"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/dialog"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// RPCReactionsServer: Layer 229 methods previously returned ERR_ENTERPRISE_IS_BLOCKED.

func reactKey(userID int64, op string) string {
	return fmt.Sprintf("react:%d:%s", userID, op)
}

func saveReact(userID int64, op string, in any) error {
	raw, err := json.Marshal(in)
	if err != nil {
		return err
	}
	return persist.Default.Set(reactKey(userID, op), string(raw))
}

func (c *ApiFullCore) apifullDao() *dao.Dao {
	if c == nil || c.svcCtx == nil {
		return nil
	}
	return c.svcCtx.Dao
}

// apifullPeerTypeID maps an input peer the way FromInputPeer2 does when the
// predicate is set, and falls back to the raw id fields tests actually fill.
func apifullPeerTypeID(self int64, p *mtproto.InputPeer) (int32, int64) {
	if p == nil {
		return 0, 0
	}
	if p.PredicateName != "" {
		peer := mtproto.FromInputPeer2(self, p)
		if peer.PeerType != mtproto.PEER_EMPTY && peer.PeerType != mtproto.PEER_UNKNOWN && peer.PeerId != 0 {
			return peer.PeerType, peer.PeerId
		}
	}
	switch {
	case p.ChannelId != 0:
		return mtproto.PEER_CHANNEL, p.ChannelId
	case p.ChatId != 0:
		return mtproto.PEER_CHAT, p.ChatId
	case p.UserId != 0:
		return mtproto.PEER_USER, p.UserId
	default:
		return 0, 0
	}
}

// authorizeReactionPeer validates peers before they are used as shared
// reaction-store keys. Channel reactions need both a valid access hash and
// current channel membership; direct and basic-group reactions retain their
// existing storage behavior.
func (c *ApiFullCore) authorizeReactionPeer(userID int64, peer *mtproto.InputPeer) error {
	if peer == nil {
		return mtproto.ErrPeerIdInvalid
	}
	if peer.GetPredicateName() == mtproto.Predicate_inputPeerChannel && peer.GetChannelId() == 0 {
		return mtproto.ErrPeerIdInvalid
	}
	peerType, peerID := apifullPeerTypeID(userID, peer)
	if peerID == 0 {
		return mtproto.ErrPeerIdInvalid
	}
	switch peerType {
	case mtproto.PEER_USER, mtproto.PEER_CHAT:
		return nil
	case mtproto.PEER_CHANNEL:
		if peer.GetPredicateName() != mtproto.Predicate_inputPeerChannel {
			return mtproto.ErrPeerIdInvalid
		}
		channel, ok, err := domain.LoadChannel(peerID)
		if err != nil {
			return err
		}
		if !ok || channel.AccessHash != peer.GetAccessHash() {
			return mtproto.ErrChannelInvalid
		}
		return c.requireChannelMember(userID, channel.ID)
	default:
		return mtproto.ErrPeerIdInvalid
	}
}

type reactItem struct {
	PeerUser    int64  `json:"peerUser,omitempty"`
	PeerChat    int64  `json:"peerChat,omitempty"`
	PeerChannel int64  `json:"peerChannel,omitempty"`
	Username    string `json:"username,omitempty"`
	SavedUser   int64  `json:"savedUser,omitempty"`
	SavedChat   int64  `json:"savedChat,omitempty"`
	SavedChan   int64  `json:"savedChan,omitempty"`
	MsgId       int32  `json:"msgId"`
	TopMsgId    int32  `json:"topMsgId,omitempty"`
	Emoticon    string `json:"emoticon,omitempty"`
	DocumentId  int64  `json:"documentId,omitempty"`
	Big         bool   `json:"big,omitempty"`
	Unread      bool   `json:"unread,omitempty"`
	Date        int32  `json:"date,omitempty"`
	UserId      int64  `json:"userId"`
}

func reactListKey(userID int64) string {
	return fmt.Sprintf("react:list:%d", userID)
}

// sharedReactListKey identifies the conversation independently from the user
// who is reading it. Reactions belong to the message conversation, while the
// per-user list remains an index for user-specific features such as recents.
func sharedReactListKey(userID int64, peer *mtproto.InputPeer) string {
	peerType, peerID := apifullPeerTypeID(userID, peer)
	switch peerType {
	case mtproto.PEER_USER:
		if peerID < userID {
			userID, peerID = peerID, userID
		}
		return fmt.Sprintf("react:shared:user:%d:%d", userID, peerID)
	case mtproto.PEER_CHAT:
		return fmt.Sprintf("react:shared:chat:%d", peerID)
	case mtproto.PEER_CHANNEL:
		return fmt.Sprintf("react:shared:channel:%d", peerID)
	default:
		user, chat, channel, username := reactPeerOf(peer)
		return fmt.Sprintf("react:shared:raw:%d:%d:%d:%d:%s", userID, user, chat, channel, username)
	}
}

func reactRecentKey(userID int64) string {
	return fmt.Sprintf("react:recent:%d", userID)
}

func loadReactItems(userID int64) ([]reactItem, error) {
	raw, err := persist.Default.Get(reactListKey(userID))
	if err != nil || raw == "" {
		return nil, err
	}
	var list []reactItem
	if err := json.Unmarshal([]byte(raw), &list); err != nil {
		return nil, err
	}
	return list, nil
}

func saveReactItems(userID int64, list []reactItem) error {
	if list == nil {
		list = []reactItem{}
	}
	b, err := json.Marshal(list)
	if err != nil {
		return err
	}
	return persist.Default.Set(reactListKey(userID), string(b))
}

func loadSharedReactItems(userID int64, peer *mtproto.InputPeer) ([]reactItem, error) {
	raw, err := persist.Default.Get(sharedReactListKey(userID, peer))
	if err != nil || raw == "" {
		return nil, err
	}
	var list []reactItem
	if err := json.Unmarshal([]byte(raw), &list); err != nil {
		return nil, err
	}
	return list, nil
}

func saveSharedReactItems(userID int64, peer *mtproto.InputPeer, list []reactItem) error {
	if list == nil {
		list = []reactItem{}
	}
	b, err := json.Marshal(list)
	if err != nil {
		return err
	}
	return persist.Default.Set(sharedReactListKey(userID, peer), string(b))
}

func reactItemMatchesConversation(userID int64, peer *mtproto.InputPeer, item reactItem) bool {
	itemPeer := &mtproto.InputPeer{
		UserId:    item.PeerUser,
		ChatId:    item.PeerChat,
		ChannelId: item.PeerChannel,
		Username:  item.Username,
	}
	return sharedReactListKey(userID, peer) == sharedReactListKey(userID, itemPeer)
}

func reactPeerMatch(p *mtproto.InputPeer, user, chat, channel int64, username string) bool {
	if p == nil || (p.UserId == 0 && p.ChatId == 0 && p.ChannelId == 0 && p.Username == "") {
		return true
	}
	return p.UserId == user && p.ChatId == chat && p.ChannelId == channel && p.Username == username
}

func reactPeerOf(p *mtproto.InputPeer) (user, chat, channel int64, username string) {
	if p == nil {
		return 0, 0, 0, ""
	}
	return p.UserId, p.ChatId, p.ChannelId, p.Username
}

func reactionOf(emoticon string, documentID int64) *mtproto.Reaction {
	if documentID != 0 {
		return mtproto.MakeTLReactionCustomEmoji(&mtproto.Reaction{DocumentId: documentID}).To_Reaction()
	}
	return mtproto.MakeTLReactionEmoji(&mtproto.Reaction{Emoticon: emoticon}).To_Reaction()
}

func sentReactItems(in *mtproto.TLMessagesSendReaction) []reactItem {
	if in == nil {
		return nil
	}
	var out []reactItem
	for _, r := range in.GetReaction_FLAGVECTORREACTION() {
		if r == nil {
			continue
		}
		out = append(out, reactItem{Emoticon: r.GetEmoticon(), DocumentId: r.GetDocumentId(), Big: in.GetBig()})
	}
	if s := in.GetReaction_FLAGSTRING(); s != nil && s.GetValue() != "" {
		out = append(out, reactItem{Emoticon: s.GetValue(), Big: in.GetBig()})
	}
	return out
}

func (c *ApiFullCore) MessagesSendReaction(in *mtproto.TLMessagesSendReaction) (*mtproto.Updates, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	peerUser, peerChat, peerChannel, username := reactPeerOf(nil)
	msgID := int32(0)
	big := false
	addRecent := false
	var peer *mtproto.InputPeer
	if in != nil {
		peer = in.GetPeer()
		peerUser, peerChat, peerChannel, username = reactPeerOf(peer)
		msgID = in.GetMsgId()
		big = in.GetBig()
		addRecent = in.GetAddToRecent()
	}
	if err = c.authorizeReactionPeer(uid, peer); err != nil {
		return nil, err
	}
	if err = saveReact(uid, "send", in); err != nil {
		return nil, err
	}
	shared, err := loadSharedReactItems(uid, peer)
	if err != nil {
		return nil, err
	}
	personal, err := loadReactItems(uid)
	if err != nil {
		return nil, err
	}
	nextShared := shared[:0]
	for _, it := range shared {
		if it.UserId == uid && it.MsgId == msgID {
			continue
		}
		nextShared = append(nextShared, it)
	}
	nextPersonal := personal[:0]
	for _, it := range personal {
		if it.UserId == uid && it.MsgId == msgID && reactItemMatchesConversation(uid, peer, it) {
			continue
		}
		nextPersonal = append(nextPersonal, it)
	}
	now := int32(time.Now().Unix())
	var added []string
	for _, it := range sentReactItems(in) {
		it.PeerUser, it.PeerChat, it.PeerChannel, it.Username = peerUser, peerChat, peerChannel, username
		it.MsgId = msgID
		it.Big = big || it.Big
		it.Unread = true
		it.Date = now
		it.UserId = uid
		nextShared = append(nextShared, it)
		nextPersonal = append(nextPersonal, it)
		if it.Emoticon != "" {
			added = append(added, it.Emoticon)
		}
	}
	if err = saveSharedReactItems(uid, peer, nextShared); err != nil {
		return nil, err
	}
	if err = saveReactItems(uid, nextPersonal); err != nil {
		return nil, err
	}
	if addRecent && len(added) > 0 {
		if err = appendRecentReactions(uid, added); err != nil {
			return nil, err
		}
	}
	rows := reactForMessage(nextShared, msgID)
	return mtproto.MakeUpdatesByUpdates(reactUpdate(inPeer(in), msgID, buildMessageReactions(rows, uid))), nil
}

func inPeer(in *mtproto.TLMessagesSendReaction) *mtproto.InputPeer {
	if in == nil {
		return nil
	}
	return in.GetPeer()
}

func reactForMsg(list []reactItem, user, chat, channel int64, username string, msgID int32) []reactItem {
	var rows []reactItem
	for _, it := range list {
		if it.PeerUser == user && it.PeerChat == chat && it.PeerChannel == channel && it.Username == username && it.MsgId == msgID {
			rows = append(rows, it)
		}
	}
	return rows
}

func reactForMessage(list []reactItem, msgID int32) []reactItem {
	rows := make([]reactItem, 0)
	for _, it := range list {
		if it.MsgId == msgID {
			rows = append(rows, it)
		}
	}
	return rows
}

func reactUpdate(peer *mtproto.InputPeer, msgID int32, rx *mtproto.MessageReactions) *mtproto.Update {
	return mtproto.MakeTLUpdateMessageReactions(&mtproto.Update{
		Peer_PEER:                  pollPeer(peer),
		MsgId_INT32:                msgID,
		Reactions_MESSAGEREACTIONS: rx,
	}).To_Update()
}

func peerReaction(it reactItem, viewer int64) *mtproto.MessagePeerReaction {
	return mtproto.MakeTLMessagePeerReaction(&mtproto.MessagePeerReaction{
		Big:               it.Big,
		Unread:            it.Unread,
		My:                it.UserId == viewer,
		PeerId:            mtproto.MakePeerUser(it.UserId),
		Date:              it.Date,
		Reaction_REACTION: reactionOf(it.Emoticon, it.DocumentId),
		Reaction:          it.Emoticon,
	}).To_MessagePeerReaction()
}

func buildMessageReactions(items []reactItem, viewer int64) *mtproto.MessageReactions {
	type key struct {
		em  string
		doc int64
	}
	var order []key
	counts := map[key]int32{}
	chosen := map[key]bool{}
	recent := make([]*mtproto.MessagePeerReaction, 0, len(items))
	for _, it := range items {
		k := key{it.Emoticon, it.DocumentId}
		if _, ok := counts[k]; !ok {
			order = append(order, k)
		}
		counts[k]++
		if it.UserId == viewer {
			chosen[k] = true
		}
		recent = append(recent, peerReaction(it, viewer))
	}
	results := make([]*mtproto.ReactionCount, 0, len(order))
	for _, k := range order {
		results = append(results, mtproto.MakeTLReactionCount(&mtproto.ReactionCount{
			Reaction_REACTION: reactionOf(k.em, k.doc),
			Reaction:          k.em,
			Count:             counts[k],
			Chosen:            chosen[k],
		}).To_ReactionCount())
	}
	return mtproto.MakeTLMessageReactions(&mtproto.MessageReactions{
		CanSeeList:      true,
		Results:         results,
		RecentReactions: recent,
	}).To_MessageReactions()
}

func (c *ApiFullCore) MessagesGetMessagesReactions(in *mtproto.TLMessagesGetMessagesReactions) (*mtproto.Updates, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	var peer *mtproto.InputPeer
	var ids []int32
	if in != nil {
		peer = in.GetPeer()
		ids = in.GetId()
	}
	if err = c.authorizeReactionPeer(uid, peer); err != nil {
		return nil, err
	}
	list, err := loadSharedReactItems(uid, peer)
	if err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		seen := map[int32]struct{}{}
		for _, it := range list {
			if _, ok := seen[it.MsgId]; ok {
				continue
			}
			seen[it.MsgId] = struct{}{}
			ids = append(ids, it.MsgId)
		}
	}
	updates := make([]*mtproto.Update, 0, len(ids))
	for _, id := range ids {
		rows := reactForMessage(list, id)
		updates = append(updates, reactUpdate(peer, id, buildMessageReactions(rows, uid)))
	}
	if len(updates) == 0 {
		return mtproto.MakeEmptyUpdates(), nil
	}
	return mtproto.MakeUpdatesByUpdates(updates...), nil
}

func reactionWant(rx *mtproto.Reaction, emoticon string) (string, int64, bool) {
	if rx != nil {
		return rx.GetEmoticon(), rx.GetDocumentId(), true
	}
	if emoticon != "" {
		return emoticon, 0, true
	}
	return "", 0, false
}

func (c *ApiFullCore) MessagesGetMessageReactionsList(in *mtproto.TLMessagesGetMessageReactionsList) (*mtproto.Messages_MessageReactionsList, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	var peer *mtproto.InputPeer
	var msgID int32
	var limit int32
	var offset int
	wantEm := ""
	wantDoc := int64(0)
	filter := false
	if in != nil {
		peer = in.GetPeer()
		msgID = in.GetId()
		limit = in.GetLimit()
		if in.GetOffset() != nil {
			offset, _ = strconv.Atoi(in.GetOffset().GetValue())
		}
		if em, doc, ok := reactionWant(in.GetReaction_FLAGREACTION(), ""); ok {
			wantEm, wantDoc, filter = em, doc, true
		} else if s := in.GetReaction_FLAGSTRING(); s != nil && s.GetValue() != "" {
			wantEm, filter = s.GetValue(), true
		}
	}
	if err = c.authorizeReactionPeer(uid, peer); err != nil {
		return nil, err
	}
	list, err := loadSharedReactItems(uid, peer)
	if err != nil {
		return nil, err
	}
	rows := make([]*mtproto.MessagePeerReaction, 0)
	for _, it := range list {
		if it.MsgId != msgID {
			continue
		}
		if filter && (it.Emoticon != wantEm || it.DocumentId != wantDoc) {
			continue
		}
		rows = append(rows, peerReaction(it, uid))
	}
	count := int32(len(rows))
	if offset < 0 {
		offset = 0
	}
	if offset > len(rows) {
		offset = len(rows)
	}
	rows = rows[offset:]
	var next *wrapperspb.StringValue
	if limit > 0 && int(limit) < len(rows) {
		rows = rows[:limit]
		next = wrapperspb.String(strconv.Itoa(offset + int(limit)))
	}
	return mtproto.MakeTLMessagesMessageReactionsList(&mtproto.Messages_MessageReactionsList{
		Count:      count,
		Reactions:  rows,
		Chats:      []*mtproto.Chat{},
		Users:      []*mtproto.User{},
		NextOffset: next,
	}).To_Messages_MessageReactionsList(), nil
}

func (c *ApiFullCore) pushChatReactions(uid int64, peer *mtproto.InputPeer, cr *mtproto.ChatReactions, extras []string) error {
	d := c.apifullDao()
	if d == nil || d.ChatClient == nil {
		return nil
	}
	pt, pid := apifullPeerTypeID(uid, peer)
	if pt != mtproto.PEER_CHAT || pid == 0 {
		return nil
	}
	typ := mtproto.ChatReactionsTypeNotDefined
	var emos []string
	if cr != nil {
		typ, emos = cr.ToChatReactions()
	}
	if len(extras) > 0 {
		emos = append(emos, extras...)
		if typ == mtproto.ChatReactionsTypeNotDefined {
			typ = mtproto.ChatReactionsTypeSome
		}
	}
	_, err := d.ChatClient.ChatSetChatAvailableReactions(c.ctx, &chat.TLChatSetChatAvailableReactions{
		SelfId:                 uid,
		ChatId:                 pid,
		AvailableReactionsType: typ,
		AvailableReactions:     emos,
	})
	return err
}

func (c *ApiFullCore) MessagesSetChatAvailableReactions(in *mtproto.TLMessagesSetChatAvailableReactions) (*mtproto.Updates, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if err = saveReact(uid, "chat", in); err != nil {
		return nil, err
	}
	var peer *mtproto.InputPeer
	var cr *mtproto.ChatReactions
	var extras []string
	if in != nil {
		peer = in.GetPeer()
		cr = in.GetAvailableReactions_CHATREACTIONS()
		extras = in.GetAvailableReactions_VECTORSTRING()
	}
	raw, err := json.Marshal(struct {
		Peer   string   `json:"peer"`
		Extras []string `json:"extras,omitempty"`
	}{Peer: fmt.Sprint(reactPeerOf(peer)), Extras: extras})
	if err != nil {
		return nil, err
	}
	if err = persist.Default.Set(fmt.Sprintf("react:chat:%d", uid), string(raw)); err != nil {
		return nil, err
	}
	if err = c.pushChatReactions(uid, peer, cr, extras); err != nil {
		return nil, err
	}
	return mtproto.MakeEmptyUpdates(), nil
}

// Unicode emoji reactions shipped by Telegram clients. There is no reaction
// catalog store reachable from this Dao (Dialog/User/Chat/Sync only).
var staticReactionEmojis = []string{
	"👍", "👎", "❤️", "🔥", "🥰", "👏", "😁", "🤔", "🤯", "😱", "🤬", "😢", "🎉", "🤩", "🤮", "💩",
	"🙏", "👌", "🕊", "🤡", "🥱", "🥴", "😍", "🐳", "❤️‍🔥", "🌚", "🌭", "💯", "🤣", "⚡", "🍌", "🏆",
	"💔", "🤨", "😐", "🍓", "🍾", "💋", "🖕", "😈", "😴", "😭", "🤓", "👻", "👨‍💻", "👀", "🎃", "🙈",
	"😇", "😨", "🤝", "✍", "🤗", "🫡", "🎅", "🎄", "☃", "💅", "🤪", "🗿", "🆒", "💘", "🙉", "🦄",
	"😘", "💊", "🙊", "😎", "👾", "🤷‍♂", "🤷", "🤷‍♀", "😡",
}

func emptyReactionDocument() *mtproto.Document {
	return mtproto.MakeTLDocumentEmpty(nil).To_Document()
}

func availableFromEmojis(emojis []string) []*mtproto.AvailableReaction {
	reactions := make([]*mtproto.AvailableReaction, 0, len(emojis))
	for _, emoji := range emojis {
		reactions = append(reactions, mtproto.MakeTLAvailableReaction(&mtproto.AvailableReaction{
			Inactive:          false,
			Premium:           false,
			Reaction:          emoji,
			Title:             emoji,
			StaticIcon:        emptyReactionDocument(),
			AppearAnimation:   emptyReactionDocument(),
			SelectAnimation:   emptyReactionDocument(),
			ActivateAnimation: emptyReactionDocument(),
			EffectAnimation:   emptyReactionDocument(),
		}).To_AvailableReaction())
	}
	return reactions
}

func staticAvailableReactions() []*mtproto.AvailableReaction {
	return availableFromEmojis(staticReactionEmojis)
}

const builtinReactionHash int32 = 1

func loadAvailableReactionList() ([]*mtproto.AvailableReaction, int32, error) {
	raw, err := persist.Default.Get("react:catalog")
	if err != nil {
		return nil, 0, err
	}
	if raw == "" {
		return staticAvailableReactions(), builtinReactionHash, nil
	}
	var emojis []string
	if err := json.Unmarshal([]byte(raw), &emojis); err != nil {
		return staticAvailableReactions(), builtinReactionHash, nil
	}
	h := int32(0)
	for _, e := range emojis {
		for _, r := range e {
			h = h*31 + int32(r)
		}
	}
	if h == 0 {
		h = builtinReactionHash
	}
	return availableFromEmojis(emojis), h, nil
}

func (c *ApiFullCore) MessagesGetAvailableReactions(in *mtproto.TLMessagesGetAvailableReactions) (*mtproto.Messages_AvailableReactions, error) {
	if _, err := c.requireUserId(); err != nil {
		return nil, err
	}
	reactions, hash, err := loadAvailableReactionList()
	if err != nil {
		return nil, err
	}
	if in != nil && in.GetHash() != 0 && in.GetHash() == hash {
		return mtproto.MakeTLMessagesAvailableReactionsNotModified(&mtproto.Messages_AvailableReactions{}).To_Messages_AvailableReactions(), nil
	}
	return mtproto.MakeTLMessagesAvailableReactions(&mtproto.Messages_AvailableReactions{
		Hash:      hash,
		Reactions: reactions,
	}).To_Messages_AvailableReactions(), nil
}

func (c *ApiFullCore) MessagesSetDefaultReaction(in *mtproto.TLMessagesSetDefaultReaction) (*mtproto.Bool, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	emoticon := ""
	if in != nil {
		if in.Reaction_STRING != "" {
			emoticon = in.Reaction_STRING
		} else if in.Reaction_REACTION != nil {
			emoticon = in.Reaction_REACTION.GetEmoticon()
		}
	}
	if err = persist.Default.Set(fmt.Sprintf("react:%d", uid), emoticon); err != nil {
		return nil, err
	}
	return mtproto.BoolTrue, nil
}

func reactMsgIDs(list []reactItem, pred func(reactItem) bool) []int32 {
	seen := map[int32]struct{}{}
	var ids []int32
	for _, it := range list {
		if pred != nil && !pred(it) {
			continue
		}
		if _, ok := seen[it.MsgId]; ok {
			continue
		}
		seen[it.MsgId] = struct{}{}
		ids = append(ids, it.MsgId)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] > ids[j] })
	return ids
}

func pageIDs(ids []int32, offset, add, limit, maxID, minID int32) []int32 {
	kept := make([]int32, 0, len(ids))
	for _, id := range ids {
		if offset != 0 && id >= offset {
			continue
		}
		if maxID != 0 && id >= maxID {
			continue
		}
		if minID != 0 && id <= minID {
			continue
		}
		kept = append(kept, id)
	}
	if add < 0 {
		add = 0
	}
	if int(add) >= len(kept) {
		return nil
	}
	kept = kept[add:]
	if limit > 0 && int(limit) < len(kept) {
		kept = kept[:limit]
	}
	return kept
}

func emptyMessages() *mtproto.Messages_Messages {
	return mtproto.MakeTLMessagesMessages(&mtproto.Messages_Messages{
		Messages: []*mtproto.Message{},
		Topics:   []*mtproto.ForumTopic{},
		Chats:    []*mtproto.Chat{},
		Users:    []*mtproto.User{},
	}).To_Messages_Messages()
}

func (c *ApiFullCore) MessagesGetUnreadReactions(in *mtproto.TLMessagesGetUnreadReactions) (*mtproto.Messages_Messages, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	list, err := loadReactItems(uid)
	if err != nil {
		return nil, err
	}
	var peer, saved *mtproto.InputPeer
	var top, offset, add, limit, maxID, minID int32
	if in != nil {
		peer = in.GetPeer()
		saved = in.GetSavedPeerId()
		offset, add, limit = in.GetOffsetId(), in.GetAddOffset(), in.GetLimit()
		maxID, minID = in.GetMaxId(), in.GetMinId()
		if v := in.GetTopMsgId(); v != nil {
			top = v.GetValue()
		}
	}
	ids := reactMsgIDs(list, func(it reactItem) bool {
		if !it.Unread {
			return false
		}
		if !reactPeerMatch(peer, it.PeerUser, it.PeerChat, it.PeerChannel, it.Username) {
			return false
		}
		if saved != nil && (saved.UserId != 0 || saved.ChatId != 0 || saved.ChannelId != 0) {
			if it.SavedUser != saved.UserId || it.SavedChat != saved.ChatId || it.SavedChan != saved.ChannelId {
				return false
			}
		}
		if top != 0 && it.TopMsgId != 0 && it.TopMsgId != top {
			return false
		}
		return true
	})
	ids = pageIDs(ids, offset, add, limit, maxID, minID)
	msgs := make([]*mtproto.Message, 0, len(ids))
	for _, id := range ids {
		var rows []reactItem
		for _, it := range list {
			if !it.Unread || it.MsgId != id {
				continue
			}
			if !reactPeerMatch(peer, it.PeerUser, it.PeerChat, it.PeerChannel, it.Username) {
				continue
			}
			rows = append(rows, it)
		}
		msgPeer := pollPeer(peer)
		if msgPeer == nil && len(rows) > 0 {
			msgPeer = pollPeer(&mtproto.InputPeer{UserId: rows[0].PeerUser, ChatId: rows[0].PeerChat, ChannelId: rows[0].PeerChannel, Username: rows[0].Username})
		}
		msgs = append(msgs, mtproto.MakeTLMessage(&mtproto.Message{
			Id:        id,
			PeerId:    msgPeer,
			Reactions: buildMessageReactions(rows, uid),
		}).To_Message())
	}
	box := emptyMessages()
	box.Messages = msgs
	return box, nil
}

func (c *ApiFullCore) pushUnreadReactions(uid int64, peer *mtproto.InputPeer, left int32) {
	d := c.apifullDao()
	if d == nil || d.DialogClient == nil {
		return
	}
	pt, pid := apifullPeerTypeID(uid, peer)
	if pt == 0 || pid == 0 {
		return
	}
	_, _ = d.DialogClient.DialogUpdateUnreadCount(c.ctx, &dialog.TLDialogUpdateUnreadCount{
		UserId:               uid,
		PeerType:             pt,
		PeerId:               pid,
		UnreadReactionsCount: wrapperspb.Int32(left),
	})
}

func (c *ApiFullCore) MessagesReadReactions(in *mtproto.TLMessagesReadReactions) (*mtproto.Messages_AffectedHistory, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if err = saveReact(uid, "read", in); err != nil {
		return nil, err
	}
	list, err := loadReactItems(uid)
	if err != nil {
		return nil, err
	}
	var peer, saved *mtproto.InputPeer
	var top int32
	if in != nil {
		peer = in.GetPeer()
		saved = in.GetSavedPeerId()
		if v := in.GetTopMsgId(); v != nil {
			top = v.GetValue()
		}
	}
	var cleared int32
	var left int32
	for i := range list {
		match := list[i].Unread && reactPeerMatch(peer, list[i].PeerUser, list[i].PeerChat, list[i].PeerChannel, list[i].Username)
		if saved != nil && (saved.UserId != 0 || saved.ChatId != 0 || saved.ChannelId != 0) {
			if list[i].SavedUser != saved.UserId || list[i].SavedChat != saved.ChatId || list[i].SavedChan != saved.ChannelId {
				match = false
			}
		}
		if top != 0 && list[i].TopMsgId != 0 && list[i].TopMsgId != top {
			match = false
		}
		if match {
			list[i].Unread = false
			cleared++
			continue
		}
		if list[i].Unread && reactPeerMatch(peer, list[i].PeerUser, list[i].PeerChat, list[i].PeerChannel, list[i].Username) {
			left++
		}
	}
	if err = saveReactItems(uid, list); err != nil {
		return nil, err
	}
	c.pushUnreadReactions(uid, peer, left)
	return mtproto.MakeTLMessagesAffectedHistory(&mtproto.Messages_AffectedHistory{
		Pts:      cleared,
		PtsCount: cleared,
	}).To_Messages_AffectedHistory(), nil
}

func (c *ApiFullCore) MessagesReportReaction(in *mtproto.TLMessagesReportReaction) (*mtproto.Bool, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	// Reporting a reaction is an intake operation. Keep the same durable,
	// idempotent provider used by the other report methods instead of writing a
	// caller-local marker that no worker can consume.
	if !domain.Ready() {
		return nil, mtproto.ErrMethodNotImpl
	}
	if in == nil || in.GetId() <= 0 || in.GetPeer() == nil || in.GetReactionPeer() == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	target, err := reportPeerTarget(uid, in.GetPeer())
	if err != nil {
		return nil, err
	}
	if _, err = reportPeerTarget(uid, in.GetReactionPeer()); err != nil {
		return nil, err
	}
	if err = c.recordReport(uid, "messages.reportReaction", target, in); err != nil {
		return nil, err
	}
	return mtproto.BoolTrue, nil
}

func reactionsWithDefault(userID int64) ([]*mtproto.Reaction, error) {
	raw, err := persist.Default.Get(fmt.Sprintf("react:%d", userID))
	if err != nil {
		return nil, err
	}
	if raw == "" {
		return []*mtproto.Reaction{}, nil
	}
	return []*mtproto.Reaction{
		mtproto.MakeTLReactionEmoji(&mtproto.Reaction{Emoticon: raw}).To_Reaction(),
	}, nil
}

func reactionListHash(list []*mtproto.Reaction) int64 {
	var h int64 = 1
	for _, r := range list {
		if r == nil {
			continue
		}
		s := r.GetEmoticon()
		if s == "" {
			s = fmt.Sprintf("#%d", r.GetDocumentId())
		}
		for _, ch := range s {
			h = h*131 + int64(ch)
		}
	}
	return h
}

func applyReactionLimit(list []*mtproto.Reaction, limit int32) []*mtproto.Reaction {
	if limit > 0 && int(limit) < len(list) {
		return list[:limit]
	}
	return list
}

func reactionsResult(list []*mtproto.Reaction, hash int64) (*mtproto.Messages_Reactions, error) {
	h := reactionListHash(list)
	if hash != 0 && hash == h {
		return mtproto.MakeTLMessagesReactionsNotModified(&mtproto.Messages_Reactions{}).To_Messages_Reactions(), nil
	}
	return mtproto.MakeTLMessagesReactions(&mtproto.Messages_Reactions{
		Hash:      h,
		Reactions: list,
	}).To_Messages_Reactions(), nil
}

func (c *ApiFullCore) MessagesGetTopReactions(in *mtproto.TLMessagesGetTopReactions) (*mtproto.Messages_Reactions, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	reactions, err := reactionsWithDefault(uid)
	if err != nil {
		return nil, err
	}
	var limit int32
	var hash int64
	if in != nil {
		limit = in.GetLimit()
		hash = in.GetHash()
	}
	return reactionsResult(applyReactionLimit(reactions, limit), hash)
}

func loadRecentReactions(userID int64) ([]string, error) {
	raw, err := persist.Default.Get(reactRecentKey(userID))
	if err != nil || raw == "" {
		return nil, err
	}
	var list []string
	if err := json.Unmarshal([]byte(raw), &list); err != nil {
		return nil, err
	}
	return list, nil
}

func appendRecentReactions(userID int64, emojis []string) error {
	list, err := loadRecentReactions(userID)
	if err != nil {
		return err
	}
	seen := map[string]struct{}{}
	for _, e := range list {
		seen[e] = struct{}{}
	}
	for _, e := range emojis {
		if e == "" {
			continue
		}
		if _, ok := seen[e]; ok {
			continue
		}
		seen[e] = struct{}{}
		list = append(list, e)
	}
	if len(list) > 50 {
		list = list[len(list)-50:]
	}
	b, err := json.Marshal(list)
	if err != nil {
		return err
	}
	return persist.Default.Set(reactRecentKey(userID), string(b))
}

func (c *ApiFullCore) MessagesGetRecentReactions(in *mtproto.TLMessagesGetRecentReactions) (*mtproto.Messages_Reactions, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	reactions, err := reactionsWithDefault(uid)
	if err != nil {
		return nil, err
	}
	recent, err := loadRecentReactions(uid)
	if err != nil {
		return nil, err
	}
	have := map[string]struct{}{}
	for _, r := range reactions {
		if r != nil {
			have[r.GetEmoticon()] = struct{}{}
		}
	}
	for _, e := range recent {
		if _, ok := have[e]; ok {
			continue
		}
		have[e] = struct{}{}
		reactions = append(reactions, reactionOf(e, 0))
	}
	var limit int32
	var hash int64
	if in != nil {
		limit = in.GetLimit()
		hash = in.GetHash()
	}
	return reactionsResult(applyReactionLimit(reactions, limit), hash)
}

func (c *ApiFullCore) MessagesClearRecentReactions(in *mtproto.TLMessagesClearRecentReactions) (*mtproto.Bool, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if err = saveReact(uid, "clear", in); err != nil {
		return nil, err
	}
	if err = persist.Default.Set(reactRecentKey(uid), "[]"); err != nil {
		return nil, err
	}
	return mtproto.BoolTrue, nil
}

func (c *ApiFullCore) MessagesSendPaidReaction(in *mtproto.TLMessagesSendPaidReaction) (*mtproto.Updates, error) {
	_ = in
	if _, err := c.requireUserId(); err != nil {
		return nil, err
	}
	return nil, mtproto.ErrMethodNotImpl
}

func (c *ApiFullCore) MessagesTogglePaidReactionPrivacy(in *mtproto.TLMessagesTogglePaidReactionPrivacy) (*mtproto.Bool, error) {
	_ = in
	if _, err := c.requireUserId(); err != nil {
		return nil, err
	}
	return nil, mtproto.ErrMethodNotImpl
}

func (c *ApiFullCore) MessagesGetPaidReactionPrivacy(in *mtproto.TLMessagesGetPaidReactionPrivacy) (*mtproto.Updates, error) {
	_ = in
	if _, err := c.requireUserId(); err != nil {
		return nil, err
	}
	return nil, mtproto.ErrMethodNotImpl
}

func participantID(p *mtproto.InputPeer) int64 {
	if p == nil {
		return 0
	}
	if p.UserId != 0 {
		return p.UserId
	}
	if p.ChatId != 0 {
		return p.ChatId
	}
	return p.ChannelId
}

func (c *ApiFullCore) MessagesDeleteParticipantReactions(in *mtproto.TLMessagesDeleteParticipantReactions) (*mtproto.Bool, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	var peer, who *mtproto.InputPeer
	if in != nil {
		peer = in.GetPeer()
		who = in.GetParticipant()
	}
	if err = c.authorizeReactionPeer(uid, peer); err != nil {
		return nil, err
	}
	if err = saveReact(uid, "delAll", in); err != nil {
		return nil, err
	}
	list, err := loadSharedReactItems(uid, peer)
	if err != nil {
		return nil, err
	}
	pid := participantID(who)
	next := list[:0]
	for _, it := range list {
		if pid == 0 || it.UserId == pid {
			continue
		}
		next = append(next, it)
	}
	if err = saveSharedReactItems(uid, peer, next); err != nil {
		return nil, err
	}
	return mtproto.BoolTrue, nil
}

func (c *ApiFullCore) MessagesDeleteParticipantReaction(in *mtproto.TLMessagesDeleteParticipantReaction) (*mtproto.Updates, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	var peer, who *mtproto.InputPeer
	var msgID int32
	if in != nil {
		peer = in.GetPeer()
		who = in.GetParticipant()
		msgID = in.GetMsgId()
	}
	if err = c.authorizeReactionPeer(uid, peer); err != nil {
		return nil, err
	}
	if err = saveReact(uid, "delOne", in); err != nil {
		return nil, err
	}
	list, err := loadSharedReactItems(uid, peer)
	if err != nil {
		return nil, err
	}
	pid := participantID(who)
	next := list[:0]
	for _, it := range list {
		if it.MsgId == msgID && (pid == 0 || it.UserId == pid) {
			continue
		}
		next = append(next, it)
	}
	if err = saveSharedReactItems(uid, peer, next); err != nil {
		return nil, err
	}
	rows := reactForMessage(next, msgID)
	return mtproto.MakeUpdatesByUpdates(reactUpdate(peer, msgID, buildMessageReactions(rows, uid))), nil
}
