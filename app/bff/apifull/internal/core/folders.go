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
	"github.com/teamgram/teamgram-server/app/bff/apifull/channelview"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/domain"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/persist"
	chatpb "github.com/teamgram/teamgram-server/app/service/biz/chat/chat"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/dialog"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

// RPCFoldersServer: Layer 229 methods previously returned ERR_ENTERPRISE_IS_BLOCKED.

func dialogFiltersKey(userId int64) string {
	return fmt.Sprintf("dfilter:%d", userId)
}

func (c *ApiFullCore) loadDialogFilters(userId int64) ([]*mtproto.DialogFilter, error) {
	raw, err := persist.Default.Get(dialogFiltersKey(userId))
	if err != nil || raw == "" {
		return []*mtproto.DialogFilter{}, err
	}
	var list []*mtproto.DialogFilter
	if err := json.Unmarshal([]byte(raw), &list); err != nil {
		return nil, err
	}
	if list == nil {
		list = []*mtproto.DialogFilter{}
	}
	return list, nil
}

func saveDialogFilters(userId int64, list []*mtproto.DialogFilter) error {
	if list == nil {
		list = []*mtproto.DialogFilter{}
	}
	b, err := json.Marshal(list)
	if err != nil {
		return err
	}
	return persist.Default.Set(dialogFiltersKey(userId), string(b))
}

func reorderDialogFilters(list []*mtproto.DialogFilter, order []int32) []*mtproto.DialogFilter {
	if len(order) == 0 {
		if list == nil {
			return []*mtproto.DialogFilter{}
		}
		return list
	}
	byID := make(map[int32]*mtproto.DialogFilter, len(list))
	for _, f := range list {
		if f == nil {
			continue
		}
		byID[f.GetId()] = f
	}
	next := make([]*mtproto.DialogFilter, 0, len(list))
	seen := map[int32]struct{}{}
	for _, id := range order {
		f, ok := byID[id]
		if !ok {
			continue
		}
		if _, dup := seen[id]; dup {
			continue
		}
		seen[id] = struct{}{}
		next = append(next, f)
	}
	for _, f := range list {
		if f == nil {
			continue
		}
		if _, ok := seen[f.GetId()]; ok {
			continue
		}
		next = append(next, f)
	}
	return next
}

func (c *ApiFullCore) dialogFilterTagsEnabled(userId int64) (bool, error) {
	if c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.DialogClient == nil {
		return false, nil
	}
	tags, err := c.svcCtx.Dao.DialogClient.DialogGetDialogFilterTags(c.ctx, &dialog.TLDialogGetDialogFilterTags{
		UserId: userId,
	})
	if err != nil {
		return false, err
	}
	return mtproto.FromBool(tags), nil
}

func (c *ApiFullCore) MessagesGetDialogFiltersEFD48C89(in *mtproto.TLMessagesGetDialogFiltersEFD48C89) (*mtproto.Messages_DialogFilters, error) {
	_ = in
	userId, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	filters, err := c.loadDialogFilters(userId)
	if err != nil {
		return nil, err
	}
	tagsEnabled, err := c.dialogFilterTagsEnabled(userId)
	if err != nil {
		return nil, err
	}
	return mtproto.MakeTLMessagesDialogFilters(&mtproto.Messages_DialogFilters{
		TagsEnabled: tagsEnabled,
		Filters:     filters,
	}).To_Messages_DialogFilters(), nil
}

func (c *ApiFullCore) MessagesGetDialogFiltersF19ED96D(in *mtproto.TLMessagesGetDialogFiltersF19ED96D) (*mtproto.Vector_DialogFilter, error) {
	_ = in
	userId, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	filters, err := c.loadDialogFilters(userId)
	if err != nil {
		return nil, err
	}
	return &mtproto.Vector_DialogFilter{Datas: filters}, nil
}

func (c *ApiFullCore) MessagesGetSuggestedDialogFilters(_ *mtproto.TLMessagesGetSuggestedDialogFilters) (*mtproto.Vector_DialogFilterSuggested, error) {
	if _, err := c.requireUserId(); err != nil {
		return nil, err
	}
	empty := []*mtproto.InputPeer{}
	text := func(value string) *mtproto.TextWithEntities {
		return mtproto.MakeTLTextWithEntities(&mtproto.TextWithEntities{Text: value, Entities: []*mtproto.MessageEntity{}}).To_TextWithEntities()
	}
	filter := func(id int32, title, emoticon string, contacts, groups, broadcasts, bots bool) *mtproto.DialogFilterSuggested {
		return mtproto.MakeTLDialogFilterSuggested(&mtproto.DialogFilterSuggested{
			Filter: mtproto.MakeTLDialogFilter(&mtproto.DialogFilter{
				Contacts:               contacts,
				Groups:                 groups,
				Broadcasts:             broadcasts,
				Bots:                   bots,
				ExcludeMuted:           true,
				ExcludeArchived:        true,
				Id:                     id,
				Title_TEXTWITHENTITIES: text(title),
				Emoticon:               mtproto.MakeFlagsString(emoticon),
				PinnedPeers:            empty,
				IncludePeers:           empty,
				ExcludePeers:           empty,
			}).To_DialogFilter(),
			Description: title,
		}).To_DialogFilterSuggested()
	}
	return &mtproto.Vector_DialogFilterSuggested{Datas: []*mtproto.DialogFilterSuggested{
		filter(2, "Contacts", "👥", true, false, false, false),
		filter(3, "Groups", "👨‍👩‍👧‍👦", false, true, false, false),
		filter(4, "Channels", "📣", false, false, true, false),
	}}, nil
}

func (c *ApiFullCore) MessagesUpdateDialogFilter(in *mtproto.TLMessagesUpdateDialogFilter) (*mtproto.Bool, error) {
	userId, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	if in.GetId() < 2 {
		return nil, mtproto.ErrFilterIdInvalid
	}
	list, err := c.loadDialogFilters(userId)
	if err != nil {
		return nil, err
	}
	if in.GetFilter() == nil {
		next := list[:0]
		for _, f := range list {
			if f == nil || f.GetId() == in.GetId() {
				continue
			}
			next = append(next, f)
		}
		if err := saveDialogFilters(userId, next); err != nil {
			return nil, err
		}
		return mtproto.BoolTrue, nil
	}
	filter := in.GetFilter()
	filter.Id = in.GetId()
	for i := range list {
		if list[i] != nil && list[i].GetId() == in.GetId() {
			list[i] = filter
			if err := saveDialogFilters(userId, list); err != nil {
				return nil, err
			}
			return mtproto.BoolTrue, nil
		}
	}
	list = append(list, filter)
	if err := saveDialogFilters(userId, list); err != nil {
		return nil, err
	}
	return mtproto.BoolTrue, nil
}

func (c *ApiFullCore) MessagesUpdateDialogFiltersOrder(in *mtproto.TLMessagesUpdateDialogFiltersOrder) (*mtproto.Bool, error) {
	userId, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	list, err := c.loadDialogFilters(userId)
	if err != nil {
		return nil, err
	}
	var order []int32
	if in != nil {
		order = in.GetOrder()
	}
	if err := saveDialogFilters(userId, reorderDialogFilters(list, order)); err != nil {
		return nil, err
	}
	return mtproto.BoolTrue, nil
}

func (c *ApiFullCore) FoldersEditPeerFolders(in *mtproto.TLFoldersEditPeerFolders) (*mtproto.Updates, error) {
	userId, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	folderPeers := in.GetFolderPeers()
	if len(folderPeers) == 0 {
		return mtproto.MakeEmptyUpdates(), nil
	}
	if c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.DialogClient == nil {
		return nil, mtproto.ErrInternalServerError
	}
	filters, err := c.loadDialogFilters(userId)
	if err != nil {
		return nil, err
	}
	filterIDs := make(map[int32]struct{}, len(filters))
	for _, filter := range filters {
		if filter != nil {
			filterIDs[filter.GetId()] = struct{}{}
		}
	}

	byFolder := make(map[int32][]int64)
	updatePeers := make([]*mtproto.FolderPeer, 0, len(folderPeers))
	seen := make(map[int64]int32, len(folderPeers))
	for _, fp := range folderPeers {
		if fp == nil || fp.GetPeer() == nil {
			return nil, mtproto.ErrPeerIdInvalid
		}
		if fp.GetFolderId() < 0 {
			return nil, mtproto.ErrFolderIdInvalid
		}
		if fp.GetFolderId() != 0 && fp.GetFolderId() != 1 {
			if _, ok := filterIDs[fp.GetFolderId()]; !ok {
				return nil, mtproto.ErrFolderIdInvalid
			}
		}
		peer := folderInputPeerUtil(userId, fp.GetPeer())
		if peer.PeerType == mtproto.PEER_EMPTY || peer.PeerType == mtproto.PEER_UNKNOWN || peer.PeerId == 0 {
			return nil, mtproto.ErrPeerIdInvalid
		}
		peerType := peer.PeerType
		if peerType == mtproto.PEER_SELF {
			peerType = mtproto.PEER_USER
		}
		dialogId := mtproto.MakePeerDialogId(peerType, peer.PeerId)
		if oldFolder, ok := seen[dialogId]; ok {
			if oldFolder != fp.GetFolderId() {
				return nil, mtproto.ErrInputRequestInvalid
			}
			continue
		}
		seen[dialogId] = fp.GetFolderId()
		dialogExt, err := c.svcCtx.Dao.DialogClient.DialogGetDialogById(c.ctx, &dialog.TLDialogGetDialogById{
			UserId: userId, PeerType: peerType, PeerId: peer.PeerId,
		})
		if err != nil {
			return nil, err
		}
		if dialogExt == nil || dialogExt.GetDialog() == nil {
			return nil, mtproto.ErrPeerIdInvalid
		}
		byFolder[fp.GetFolderId()] = append(byFolder[fp.GetFolderId()], dialogId)
		updatePeers = append(updatePeers, mtproto.MakeTLFolderPeer(&mtproto.FolderPeer{
			Peer: folderInputPeer(userId, fp.GetPeer()), FolderId: fp.GetFolderId(),
		}).To_FolderPeer())
	}
	folderIds := make([]int32, 0, len(byFolder))
	for folderId := range byFolder {
		folderIds = append(folderIds, folderId)
	}
	sort.Slice(folderIds, func(i, j int) bool { return folderIds[i] < folderIds[j] })
	for _, folderId := range folderIds {
		dialogIds := byFolder[folderId]
		result, err := c.svcCtx.Dao.DialogClient.DialogEditPeerFolders(c.ctx, &dialog.TLDialogEditPeerFolders{
			UserId: userId, PeerDialogList: dialogIds, FolderId: folderId,
		})
		if err != nil {
			return nil, err
		}
		if result == nil {
			return nil, mtproto.ErrInternalServerError
		}
	}
	if len(updatePeers) == 0 {
		return mtproto.MakeEmptyUpdates(), nil
	}
	update := mtproto.MakeTLUpdateFolderPeers(&mtproto.Update{
		FolderPeers: updatePeers,
	}).To_Update()
	return mtproto.MakeTLUpdates(&mtproto.Updates{
		Updates: []*mtproto.Update{update}, Users: []*mtproto.User{}, Chats: []*mtproto.Chat{},
		Date: int32(time.Now().Unix()),
	}).To_Updates(), nil
}

func (c *ApiFullCore) FoldersDeleteFolder(in *mtproto.TLFoldersDeleteFolder) (*mtproto.Updates, error) {
	userId, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	if in.GetFolderId() < 2 {
		return nil, mtproto.ErrFolderIdInvalid
	}
	filters, err := c.loadDialogFilters(userId)
	if err != nil {
		return nil, err
	}
	filterIndex := -1
	for i, filter := range filters {
		if filter != nil && filter.GetId() == in.GetFolderId() {
			filterIndex = i
			break
		}
	}
	if filterIndex < 0 {
		return nil, mtproto.ErrFolderIdInvalid
	}
	st, err := loadClist(userId)
	if err != nil {
		return nil, err
	}
	if c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.DialogClient == nil {
		return nil, mtproto.ErrInternalServerError
	}
	folderDialogs, err := c.svcCtx.Dao.DialogClient.DialogGetDialogs(c.ctx, &dialog.TLDialogGetDialogs{
		UserId: userId, FolderId: in.GetFolderId(), ExcludePinned: mtproto.BoolFalse,
	})
	if err != nil {
		return nil, err
	}
	if folderDialogs == nil {
		return nil, mtproto.ErrInternalServerError
	}
	peers := make([]*mtproto.InputPeer, 0, len(folderDialogs.GetDatas()))
	for _, entry := range folderDialogs.GetDatas() {
		if entry == nil || entry.GetDialog() == nil || entry.GetDialog().GetPeer() == nil {
			return nil, mtproto.ErrInternalServerError
		}
		peer := entry.GetDialog().GetPeer()
		switch peer.GetPredicateName() {
		case mtproto.Predicate_peerUser:
			peers = append(peers, mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: peer.GetUserId()}).To_InputPeer())
		case mtproto.Predicate_peerChat:
			peers = append(peers, mtproto.MakeInputPeerChat(peer.GetChatId()))
		case mtproto.Predicate_peerChannel:
			peers = append(peers, mtproto.MakeInputPeerChannel(peer.GetChannelId()))
		default:
			return nil, mtproto.ErrPeerIdInvalid
		}
	}
	folderPeers, err := c.applyFolderPeers(userId, 0, peers)
	if err != nil {
		return nil, err
	}
	deleted, err := c.svcCtx.Dao.DialogClient.DialogDeleteDialogFilter(c.ctx, &dialog.TLDialogDeleteDialogFilter{
		UserId: userId, Id: in.GetFolderId(),
	})
	if err != nil {
		return nil, err
	}
	if deleted == nil || !mtproto.FromBool(deleted) {
		return nil, mtproto.ErrInternalServerError
	}
	filters = append(filters[:filterIndex], filters[filterIndex+1:]...)
	if err := saveDialogFilters(userId, filters); err != nil {
		return nil, err
	}
	joined := st.Joined[:0]
	for _, entry := range st.Joined {
		if entry.FilterID != in.GetFolderId() {
			joined = append(joined, entry)
		}
	}
	st.Joined = joined
	hidden := st.Hidden[:0]
	for _, id := range st.Hidden {
		if id != in.GetFolderId() {
			hidden = append(hidden, id)
		}
	}
	st.Hidden = hidden
	if err := saveClist(userId, st); err != nil {
		return nil, err
	}
	updates := []*mtproto.Update{
		mtproto.MakeTLUpdateDialogFilter(&mtproto.Update{Id_INT32: in.GetFolderId()}).To_Update(),
	}
	if len(folderPeers) > 0 {
		updates = append(updates, mtproto.MakeTLUpdateFolderPeers(&mtproto.Update{FolderPeers: folderPeers}).To_Update())
	}
	return mtproto.MakeTLUpdates(&mtproto.Updates{
		Updates: updates, Users: []*mtproto.User{}, Chats: []*mtproto.Chat{}, Date: int32(time.Now().Unix()),
	}).To_Updates(), nil
}

func (c *ApiFullCore) requireUserId() (int64, error) {
	if c == nil || c.MD == nil || c.MD.UserId <= 0 {
		return 0, mtproto.ErrAuthKeyUnregistered
	}
	return c.MD.UserId, nil
}

type clInvite struct {
	FilterID int32                `json:"filter_id"`
	Slug     string               `json:"slug"`
	Title    string               `json:"title"`
	Peers    []*mtproto.InputPeer `json:"peers"`
}

type clJoin struct {
	FilterID int32                `json:"filter_id"`
	Slug     string               `json:"slug"`
	Peers    []*mtproto.InputPeer `json:"peers"`
}

type clState struct {
	Next    int        `json:"next"`
	Invites []clInvite `json:"invites"`
	Joined  []clJoin   `json:"joined"`
	Hidden  []int32    `json:"hidden"`
}

func clistKey(uid int64) string {
	return fmt.Sprintf("clist:%d", uid)
}

func clistSlugKey(slug string) string {
	return "clist:slug:" + slug
}

func loadClist(uid int64) (*clState, error) {
	if domain.Ready() {
		raw, err := domain.LoadChatlistState(uid)
		if err != nil {
			return nil, err
		}
		if len(raw) == 0 {
			return &clState{}, nil
		}
		var st clState
		if err := json.Unmarshal(raw, &st); err != nil {
			return nil, err
		}
		return &st, nil
	}
	raw, err := persist.Default.Get(clistKey(uid))
	if err != nil || raw == "" {
		return &clState{}, err
	}
	var st clState
	if err := json.Unmarshal([]byte(raw), &st); err != nil {
		return &clState{}, err
	}
	return &st, nil
}

func saveClist(uid int64, st *clState) error {
	if st == nil {
		st = &clState{}
	}
	b, err := json.Marshal(st)
	if err != nil {
		return err
	}
	if domain.Ready() {
		invites := make([]domain.ChatlistInviteRecord, 0, len(st.Invites))
		for _, invite := range st.Invites {
			peers, err := json.Marshal(invite.Peers)
			if err != nil {
				return err
			}
			invites = append(invites, domain.ChatlistInviteRecord{
				Slug: invite.Slug, FilterID: invite.FilterID, Title: invite.Title, PeersJSON: peers,
			})
		}
		return domain.SaveChatlistState(uid, b, invites)
	}
	return persist.Default.Set(clistKey(uid), string(b))
}

func indexClistSlug(slug string, uid int64) error {
	if slug == "" {
		return nil
	}
	if domain.Ready() {
		return nil
	}
	return persist.Default.Set(clistSlugKey(slug), strconv.FormatInt(uid, 10))
}

func clearClistSlug(slug string) error {
	if slug == "" {
		return nil
	}
	if domain.Ready() {
		return nil
	}
	return persist.Default.Set(clistSlugKey(slug), "")
}

func findClistInvite(slug string) (*clInvite, bool, error) {
	if slug == "" {
		return nil, false, nil
	}
	if domain.Ready() {
		_, record, ok, err := domain.FindChatlistInvite(slug)
		if err != nil || !ok {
			return nil, ok, err
		}
		var peers []*mtproto.InputPeer
		if err := json.Unmarshal(record.PeersJSON, &peers); err != nil {
			return nil, false, err
		}
		return &clInvite{FilterID: record.FilterID, Slug: record.Slug, Title: record.Title, Peers: peers}, true, nil
	}
	raw, err := persist.Default.Get(clistSlugKey(slug))
	if err != nil || raw == "" {
		return nil, false, err
	}
	owner, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return nil, false, nil
	}
	st, err := loadClist(owner)
	if err != nil {
		return nil, false, err
	}
	for i := range st.Invites {
		if st.Invites[i].Slug == slug {
			inv := st.Invites[i]
			return &inv, true, nil
		}
	}
	return nil, false, nil
}

func folderInputPeer(userId int64, p *mtproto.InputPeer) *mtproto.Peer {
	if p == nil {
		return nil
	}
	peer := folderInputPeerUtil(userId, p)
	if peer.PeerType == mtproto.PEER_SELF {
		return mtproto.MakePeerUser(userId)
	}
	return mtproto.MakePeer(peer.PeerType, peer.PeerId)
}

func folderInputPeerUtil(userId int64, p *mtproto.InputPeer) *mtproto.PeerUtil {
	if p == nil {
		return &mtproto.PeerUtil{PeerType: mtproto.PEER_UNKNOWN}
	}
	peer := mtproto.FromInputPeer2(userId, p)
	switch p.GetPredicateName() {
	case mtproto.Predicate_inputPeerUserFromMessage:
		peer.PeerType = mtproto.PEER_USER
		peer.PeerId = p.GetUserId()
	case mtproto.Predicate_inputPeerChannelFromMessage:
		peer.PeerType = mtproto.PEER_CHANNEL
		peer.PeerId = p.GetChannelId()
	}
	return peer
}

func folderPeerKey(p *mtproto.InputPeer) string {
	if p == nil {
		return ""
	}
	switch p.GetPredicateName() {
	case mtproto.Predicate_inputPeerSelf:
		return "user:self"
	case mtproto.Predicate_inputPeerUser, mtproto.Predicate_inputPeerUserFromMessage:
		return "user:" + strconv.FormatInt(p.GetUserId(), 10)
	case mtproto.Predicate_inputPeerChat:
		return "chat:" + strconv.FormatInt(p.GetChatId(), 10)
	case mtproto.Predicate_inputPeerChannel, mtproto.Predicate_inputPeerChannelFromMessage:
		return "channel:" + strconv.FormatInt(p.GetChannelId(), 10)
	default:
		return p.GetPredicateName() + ":" + strconv.FormatInt(p.GetUserId(), 10) + ":" + strconv.FormatInt(p.GetChatId(), 10) + ":" + strconv.FormatInt(p.GetChannelId(), 10)
	}
}

func inputPeersToPeers(userId int64, in []*mtproto.InputPeer) []*mtproto.Peer {
	out := make([]*mtproto.Peer, 0, len(in))
	for _, p := range in {
		if peer := folderInputPeer(userId, p); peer != nil {
			out = append(out, peer)
		}
	}
	return out
}

func mergeInputPeers(parts ...[]*mtproto.InputPeer) []*mtproto.InputPeer {
	seen := map[string]struct{}{}
	out := make([]*mtproto.InputPeer, 0)
	for _, part := range parts {
		for _, p := range part {
			if p == nil {
				continue
			}
			k := folderPeerKey(p)
			if _, ok := seen[k]; ok {
				continue
			}
			seen[k] = struct{}{}
			out = append(out, p)
		}
	}
	return out
}

func peersMissing(invite, joined []*mtproto.InputPeer) []*mtproto.InputPeer {
	have := map[string]struct{}{}
	for _, p := range joined {
		if p == nil {
			continue
		}
		have[folderPeerKey(p)] = struct{}{}
	}
	out := make([]*mtproto.InputPeer, 0)
	seen := map[string]struct{}{}
	for _, p := range invite {
		if p == nil {
			continue
		}
		k := folderPeerKey(p)
		if _, ok := have[k]; ok {
			continue
		}
		if _, ok := seen[k]; ok {
			continue
		}
		seen[k] = struct{}{}
		out = append(out, p)
	}
	return out
}

func chatlistFilterID(cl *mtproto.InputChatlist) int32 {
	if cl == nil {
		return 0
	}
	return cl.GetFilterId()
}

func exportedInvite(userId int64, inv clInvite) *mtproto.ExportedChatlistInvite {
	return mtproto.MakeTLExportedChatlistInvite(&mtproto.ExportedChatlistInvite{
		Title: inv.Title,
		Url:   inv.Slug,
		Peers: inputPeersToPeers(userId, inv.Peers),
	}).To_ExportedChatlistInvite()
}

func (c *ApiFullCore) chatlistEntities(userId int64, peers []*mtproto.InputPeer) ([]*mtproto.User, []*mtproto.Chat, error) {
	if c == nil {
		return nil, nil, mtproto.ErrInternalServerError
	}
	userIDs := make([]int64, 0)
	chatIDs := make([]int64, 0)
	channelPeers := make([]*mtproto.InputPeer, 0)
	seenUsers := make(map[int64]struct{})
	seenChats := make(map[int64]struct{})
	seenChannels := make(map[int64]struct{})
	for _, inputPeer := range peers {
		if inputPeer == nil {
			return nil, nil, mtproto.ErrPeerIdInvalid
		}
		peer := folderInputPeerUtil(userId, inputPeer)
		switch peer.PeerType {
		case mtproto.PEER_SELF, mtproto.PEER_USER:
			if _, ok := seenUsers[peer.PeerId]; !ok {
				seenUsers[peer.PeerId] = struct{}{}
				userIDs = append(userIDs, peer.PeerId)
			}
		case mtproto.PEER_CHAT:
			if _, ok := seenChats[peer.PeerId]; !ok {
				seenChats[peer.PeerId] = struct{}{}
				chatIDs = append(chatIDs, peer.PeerId)
			}
		case mtproto.PEER_CHANNEL:
			if _, ok := seenChannels[peer.PeerId]; !ok {
				seenChannels[peer.PeerId] = struct{}{}
				channelPeers = append(channelPeers, inputPeer)
			}
		default:
			return nil, nil, mtproto.ErrPeerIdInvalid
		}
	}

	users := make([]*mtproto.User, 0, len(userIDs))
	if len(userIDs) > 0 {
		if c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.UserClient == nil {
			return nil, nil, mtproto.ErrInternalServerError
		}
		mutableUsers, err := c.svcCtx.Dao.UserClient.UserGetMutableUsersV2(c.ctx, &userpb.TLUserGetMutableUsersV2{
			Id: userIDs, Privacy: true, HasTo: true, To: []int64{userId},
		})
		if err != nil {
			return nil, nil, err
		}
		if mutableUsers == nil || !mutableUsers.CheckExistUser(userIDs...) {
			return nil, nil, mtproto.ErrUserIdInvalid
		}
		users = mutableUsers.GetUserListByIdList(userId, userIDs...)
	}

	chats := make([]*mtproto.Chat, 0, len(chatIDs)+len(channelPeers))
	if len(chatIDs) > 0 {
		if c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.ChatClient == nil {
			return nil, nil, mtproto.ErrInternalServerError
		}
		mutableChats, err := c.svcCtx.Dao.ChatClient.ChatGetChatListByIdList(c.ctx, &chatpb.TLChatGetChatListByIdList{
			SelfId: userId, IdList: chatIDs,
		})
		if err != nil {
			return nil, nil, err
		}
		if mutableChats == nil {
			return nil, nil, mtproto.ErrChatIdInvalid
		}
		found := make(map[int64]struct{}, len(mutableChats.GetDatas()))
		for _, chat := range mutableChats.GetDatas() {
			if chat != nil {
				found[chat.Id()] = struct{}{}
			}
		}
		for _, id := range chatIDs {
			if _, ok := found[id]; !ok {
				return nil, nil, mtproto.ErrChatIdInvalid
			}
		}
		chats = append(chats, mutableChats.GetChatListByIdList(userId, chatIDs...)...)
	}
	for _, inputPeer := range channelPeers {
		id := folderInputPeerUtil(userId, inputPeer).PeerId
		var chat *mtproto.Chat
		if domain.Ready() {
			stored, ok, err := domain.LoadChannel(id)
			if err != nil {
				return nil, nil, err
			}
			if !ok {
				return nil, nil, mtproto.ErrChannelInvalid
			}
			chat = channelview.Chat(stored, stored.Creator == userId)
		} else {
			stored, ok, err := loadChan(userId, id)
			if err != nil {
				return nil, nil, err
			}
			if ok {
				chat = chanChat(id, stored.Title)
			}
		}
		if chat == nil {
			return nil, nil, mtproto.ErrChannelInvalid
		}
		chats = append(chats, chat)
	}
	return users, chats, nil
}

func (c *ApiFullCore) allocateChatlistFilterID(userId int64) (int32, error) {
	filters, err := c.loadDialogFilters(userId)
	if err != nil {
		return 0, err
	}
	used := make(map[int32]struct{}, len(filters))
	for _, filter := range filters {
		if filter != nil {
			used[filter.GetId()] = struct{}{}
		}
	}
	for id := int32(2); id <= 10; id++ {
		if _, ok := used[id]; !ok {
			return id, nil
		}
	}
	return 0, mtproto.ErrFilterIdInvalid
}

func (c *ApiFullCore) ensureChatlistFilter(userId int64, filterId int32, title string, peers []*mtproto.InputPeer) (*mtproto.DialogFilter, error) {
	if filterId < 2 || title == "" {
		return nil, mtproto.ErrFilterIdInvalid
	}
	filters, err := c.loadDialogFilters(userId)
	if err != nil {
		return nil, err
	}
	var filter *mtproto.DialogFilter
	for _, existing := range filters {
		if existing == nil || existing.GetId() != filterId {
			continue
		}
		if existing.GetPredicateName() != mtproto.Predicate_dialogFilterChatlist {
			return nil, mtproto.ErrFilterIdInvalid
		}
		filter = existing
		break
	}
	if filter == nil {
		filter = mtproto.MakeTLDialogFilterChatlist(&mtproto.DialogFilter{
			Id: filterId, PinnedPeers: []*mtproto.InputPeer{},
		}).To_DialogFilter()
	}
	filter.Title_STRING = title
	filter.Title_TEXTWITHENTITIES = mtproto.MakeTLTextWithEntities(&mtproto.TextWithEntities{
		Text: title, Entities: []*mtproto.MessageEntity{},
	}).To_TextWithEntities()
	filter.IncludePeers = peers
	if c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.DialogClient == nil {
		return nil, mtproto.ErrInternalServerError
	}
	updated, err := c.svcCtx.Dao.DialogClient.DialogInsertOrUpdateDialogFilter(c.ctx, &dialog.TLDialogInsertOrUpdateDialogFilter{
		UserId: userId, Id: filterId, DialogFilter: filter,
	})
	if err != nil {
		return nil, err
	}
	if updated == nil || !mtproto.FromBool(updated) {
		return nil, mtproto.ErrInternalServerError
	}
	found := false
	for i, existing := range filters {
		if existing != nil && existing.GetId() == filterId {
			filters[i] = filter
			found = true
			break
		}
	}
	if !found {
		filters = append(filters, filter)
	}
	if err := saveDialogFilters(userId, filters); err != nil {
		return nil, err
	}
	return filter, nil
}

func (c *ApiFullCore) deleteChatlistFilter(userId int64, filterId int32) error {
	filters, err := c.loadDialogFilters(userId)
	if err != nil {
		return err
	}
	idx := -1
	for i, filter := range filters {
		if filter != nil && filter.GetId() == filterId {
			if filter.GetPredicateName() != mtproto.Predicate_dialogFilterChatlist {
				return mtproto.ErrFilterIdInvalid
			}
			idx = i
			break
		}
	}
	if idx < 0 {
		return mtproto.ErrFilterIdInvalid
	}
	if c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.DialogClient == nil {
		return mtproto.ErrInternalServerError
	}
	if _, err := c.svcCtx.Dao.DialogClient.DialogDeleteDialogFilter(c.ctx, &dialog.TLDialogDeleteDialogFilter{
		UserId: userId, Id: filterId,
	}); err != nil {
		return err
	}
	filters = append(filters[:idx], filters[idx+1:]...)
	return saveDialogFilters(userId, filters)
}

func (c *ApiFullCore) applyFolderPeers(userId int64, folderId int32, peers []*mtproto.InputPeer) ([]*mtproto.FolderPeer, error) {
	if len(peers) == 0 {
		return []*mtproto.FolderPeer{}, nil
	}
	if folderId < 0 {
		return nil, mtproto.ErrFolderIdInvalid
	}
	if c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.DialogClient == nil {
		return nil, mtproto.ErrInternalServerError
	}
	dialogIDs := make([]int64, 0, len(peers))
	folderPeers := make([]*mtproto.FolderPeer, 0, len(peers))
	seen := make(map[int64]struct{}, len(peers))
	for _, inputPeer := range peers {
		if inputPeer == nil {
			return nil, mtproto.ErrPeerIdInvalid
		}
		peer := folderInputPeerUtil(userId, inputPeer)
		if peer.PeerType == mtproto.PEER_EMPTY || peer.PeerType == mtproto.PEER_UNKNOWN || peer.PeerId == 0 {
			return nil, mtproto.ErrPeerIdInvalid
		}
		peerType := peer.PeerType
		if peerType == mtproto.PEER_SELF {
			peerType = mtproto.PEER_USER
		}
		dialogID := mtproto.MakePeerDialogId(peerType, peer.PeerId)
		if _, ok := seen[dialogID]; ok {
			continue
		}
		seen[dialogID] = struct{}{}
		dialogExt, err := c.svcCtx.Dao.DialogClient.DialogGetDialogById(c.ctx, &dialog.TLDialogGetDialogById{
			UserId: userId, PeerType: peerType, PeerId: peer.PeerId,
		})
		if err != nil {
			return nil, err
		}
		if dialogExt == nil || dialogExt.GetDialog() == nil {
			return nil, mtproto.ErrPeerIdInvalid
		}
		dialogIDs = append(dialogIDs, dialogID)
		folderPeers = append(folderPeers, mtproto.MakeTLFolderPeer(&mtproto.FolderPeer{
			Peer: folderInputPeer(userId, inputPeer), FolderId: folderId,
		}).To_FolderPeer())
	}
	if len(dialogIDs) == 0 {
		return folderPeers, nil
	}
	result, err := c.svcCtx.Dao.DialogClient.DialogEditPeerFolders(c.ctx, &dialog.TLDialogEditPeerFolders{
		UserId: userId, PeerDialogList: dialogIDs, FolderId: folderId,
	})
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, mtproto.ErrInternalServerError
	}
	return folderPeers, nil
}

func folderPeersUpdates(peers []*mtproto.FolderPeer, users []*mtproto.User, chats []*mtproto.Chat) *mtproto.Updates {
	if len(peers) == 0 {
		return mtproto.MakeEmptyUpdates()
	}
	update := mtproto.MakeTLUpdateFolderPeers(&mtproto.Update{FolderPeers: peers}).To_Update()
	return mtproto.MakeTLUpdates(&mtproto.Updates{
		Updates: []*mtproto.Update{update}, Users: users, Chats: chats, Date: int32(time.Now().Unix()),
	}).To_Updates()
}

func chatlistJoinUpdates(filter *mtproto.DialogFilter, peers []*mtproto.FolderPeer, users []*mtproto.User, chats []*mtproto.Chat) *mtproto.Updates {
	updates := []*mtproto.Update{
		mtproto.MakeTLUpdateDialogFilter(&mtproto.Update{Id_INT32: filter.GetId(), Filter: filter}).To_Update(),
	}
	if len(peers) > 0 {
		updates = append(updates, mtproto.MakeTLUpdateFolderPeers(&mtproto.Update{FolderPeers: peers}).To_Update())
	}
	return mtproto.MakeTLUpdates(&mtproto.Updates{
		Updates: updates, Users: users, Chats: chats, Date: int32(time.Now().Unix()),
	}).To_Updates()
}

func (c *ApiFullCore) dialogFilterByID(userId int64, id int32) (*mtproto.DialogFilter, error) {
	filters, err := c.loadDialogFilters(userId)
	if err != nil {
		return nil, err
	}
	for _, f := range filters {
		if f != nil && f.GetId() == id {
			return f, nil
		}
	}
	return nil, mtproto.ErrFilterIdInvalid
}

func (c *ApiFullCore) requireChatlistFilter(userId int64, id int32) error {
	filter, err := c.dialogFilterByID(userId, id)
	if err != nil {
		return err
	}
	if filter.GetPredicateName() != mtproto.Predicate_dialogFilterChatlist {
		return mtproto.ErrFilterIdInvalid
	}
	return nil
}

func emptyChatlistInvite() *mtproto.Chatlists_ChatlistInvite {
	return mtproto.MakeTLChatlistsChatlistInvite(&mtproto.Chatlists_ChatlistInvite{
		Title_TEXTWITHENTITIES: mtproto.MakeTLTextWithEntities(&mtproto.TextWithEntities{
			Entities: []*mtproto.MessageEntity{},
		}).To_TextWithEntities(),
		MissingPeers: []*mtproto.Peer{},
		AlreadyPeers: []*mtproto.Peer{},
		Chats:        []*mtproto.Chat{},
		Users:        []*mtproto.User{},
		Peers:        []*mtproto.Peer{},
	}).To_Chatlists_ChatlistInvite()
}

func (c *ApiFullCore) ChatlistsExportChatlistInvite(in *mtproto.TLChatlistsExportChatlistInvite) (*mtproto.Chatlists_ExportedChatlistInvite, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil || in.GetChatlist() == nil || len(in.GetPeers()) == 0 {
		return nil, mtproto.ErrInputRequestInvalid
	}
	if _, _, err := c.chatlistEntities(uid, in.GetPeers()); err != nil {
		return nil, err
	}
	filterID := chatlistFilterID(in.GetChatlist())
	filter, err := c.dialogFilterByID(uid, filterID)
	if err != nil {
		return nil, err
	}
	st, err := loadClist(uid)
	if err != nil {
		return nil, err
	}
	slug := fmt.Sprintf("%d-%d", uid, st.Next)
	st.Next++
	inv := clInvite{FilterID: filterID, Slug: slug, Title: in.GetTitle(), Peers: in.GetPeers()}
	st.Invites = append(st.Invites, inv)
	if err := saveClist(uid, st); err != nil {
		return nil, err
	}
	if err := indexClistSlug(slug, uid); err != nil {
		st.Invites = st.Invites[:len(st.Invites)-1]
		_ = saveClist(uid, st)
		return nil, err
	}
	return mtproto.MakeTLChatlistsExportedChatlistInvite(&mtproto.Chatlists_ExportedChatlistInvite{
		Filter: filter,
		Invite: exportedInvite(uid, inv),
	}).To_Chatlists_ExportedChatlistInvite(), nil
}

func (c *ApiFullCore) ChatlistsDeleteExportedInvite(in *mtproto.TLChatlistsDeleteExportedInvite) (*mtproto.Bool, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	slug := in.GetSlug()
	if slug == "" {
		return nil, mtproto.ErrInviteHashInvalid
	}
	st, err := loadClist(uid)
	if err != nil {
		return nil, err
	}
	next := st.Invites[:0]
	removed := false
	for _, inv := range st.Invites {
		if inv.Slug == slug {
			removed = true
			continue
		}
		next = append(next, inv)
	}
	if removed {
		if err := clearClistSlug(slug); err != nil {
			return nil, err
		}
		st.Invites = next
		if err := saveClist(uid, st); err != nil {
			return nil, err
		}
	}
	return mtproto.BoolTrue, nil
}

func (c *ApiFullCore) ChatlistsEditExportedInvite(in *mtproto.TLChatlistsEditExportedInvite) (*mtproto.ExportedChatlistInvite, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	slug := in.GetSlug()
	if slug == "" {
		return nil, mtproto.ErrInviteHashInvalid
	}
	var filterID int32
	filterID = chatlistFilterID(in.GetChatlist())
	st, err := loadClist(uid)
	if err != nil {
		return nil, err
	}
	idx := -1
	for i := range st.Invites {
		if st.Invites[i].Slug == slug {
			idx = i
			break
		}
	}
	if idx < 0 {
		return nil, mtproto.ErrInviteHashInvalid
	}
	previous := st.Invites[idx]
	if in.GetTitle() != nil {
		st.Invites[idx].Title = in.GetTitle().GetValue()
	}
	if in.GetPeers() != nil {
		if _, _, err := c.chatlistEntities(uid, in.GetPeers()); err != nil {
			return nil, err
		}
		st.Invites[idx].Peers = in.GetPeers()
	}
	if in.GetChatlist() != nil {
		if _, err := c.dialogFilterByID(uid, filterID); err != nil {
			return nil, err
		}
		st.Invites[idx].FilterID = filterID
	}
	if err := saveClist(uid, st); err != nil {
		return nil, err
	}
	if err := indexClistSlug(slug, uid); err != nil {
		st.Invites[idx] = previous
		_ = saveClist(uid, st)
		return nil, err
	}
	return exportedInvite(uid, st.Invites[idx]), nil
}

func (c *ApiFullCore) ChatlistsGetExportedInvites(in *mtproto.TLChatlistsGetExportedInvites) (*mtproto.Chatlists_ExportedInvites, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	st, err := loadClist(uid)
	if err != nil {
		return nil, err
	}
	var filterID int32
	filterByID := false
	if in != nil && in.GetChatlist() != nil {
		filterByID = true
		filterID = in.GetChatlist().GetFilterId()
	}
	invites := make([]*mtproto.ExportedChatlistInvite, 0)
	var peers []*mtproto.InputPeer
	for _, inv := range st.Invites {
		if filterByID && inv.FilterID != filterID {
			continue
		}
		invites = append(invites, exportedInvite(uid, inv))
		peers = append(peers, inv.Peers...)
	}
	users, chats, err := c.chatlistEntities(uid, mergeInputPeers(peers))
	if err != nil {
		return nil, err
	}
	return mtproto.MakeTLChatlistsExportedInvites(&mtproto.Chatlists_ExportedInvites{
		Invites: invites,
		Chats:   chats,
		Users:   users,
	}).To_Chatlists_ExportedInvites(), nil
}

func (c *ApiFullCore) ChatlistsCheckChatlistInvite(in *mtproto.TLChatlistsCheckChatlistInvite) (*mtproto.Chatlists_ChatlistInvite, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	slug := ""
	if in != nil {
		slug = in.GetSlug()
	}
	inv, ok, err := findClistInvite(slug)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, mtproto.ErrInviteHashInvalid
	}
	st, err := loadClist(uid)
	if err != nil {
		return nil, err
	}
	var joined *clJoin
	for i := range st.Joined {
		if st.Joined[i].Slug == slug {
			joined = &st.Joined[i]
			break
		}
	}
	users, chats, err := c.chatlistEntities(uid, inv.Peers)
	if err != nil {
		return nil, err
	}
	peers := inputPeersToPeers(uid, inv.Peers)
	if joined == nil {
		return mtproto.MakeTLChatlistsChatlistInvite(&mtproto.Chatlists_ChatlistInvite{
			Title_STRING: inv.Title,
			Title_TEXTWITHENTITIES: mtproto.MakeTLTextWithEntities(&mtproto.TextWithEntities{
				Text: inv.Title,
			}).To_TextWithEntities(),
			Peers:        peers,
			FilterId:     inv.FilterID,
			MissingPeers: []*mtproto.Peer{},
			AlreadyPeers: []*mtproto.Peer{},
			Chats:        chats,
			Users:        users,
		}).To_Chatlists_ChatlistInvite(), nil
	}
	if err := c.requireChatlistFilter(uid, joined.FilterID); err != nil {
		return nil, err
	}
	return mtproto.MakeTLChatlistsChatlistInviteAlready(&mtproto.Chatlists_ChatlistInvite{
		FilterId:     joined.FilterID,
		AlreadyPeers: inputPeersToPeers(uid, joined.Peers),
		MissingPeers: inputPeersToPeers(uid, peersMissing(inv.Peers, joined.Peers)),
		Chats:        chats,
		Users:        users,
	}).To_Chatlists_ChatlistInvite(), nil
}

func (c *ApiFullCore) ChatlistsJoinChatlistInvite(in *mtproto.TLChatlistsJoinChatlistInvite) (*mtproto.Updates, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	slug := ""
	var peers []*mtproto.InputPeer
	if in != nil {
		slug = in.GetSlug()
		peers = in.GetPeers()
	}
	inv, ok, err := findClistInvite(slug)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, mtproto.ErrInviteHashInvalid
	}
	if len(peers) == 0 {
		peers = inv.Peers
	}
	if len(peers) == 0 || len(peersMissing(peers, inv.Peers)) != 0 {
		return nil, mtproto.ErrPeerIdInvalid
	}
	users, chats, err := c.chatlistEntities(uid, peers)
	if err != nil {
		return nil, err
	}
	st, err := loadClist(uid)
	if err != nil {
		return nil, err
	}
	filters, err := c.loadDialogFilters(uid)
	if err != nil {
		return nil, err
	}
	localFilterID := int32(0)
	selectedPeers := peers
	for _, joined := range st.Joined {
		if joined.Slug != slug {
			continue
		}
		selectedPeers = mergeInputPeers(joined.Peers, peers)
		for _, filter := range filters {
			if filter != nil && filter.GetId() == joined.FilterID && filter.GetPredicateName() == mtproto.Predicate_dialogFilterChatlist {
				localFilterID = joined.FilterID
				break
			}
		}
		break
	}
	if localFilterID == 0 {
		localFilterID, err = c.allocateChatlistFilterID(uid)
		if err != nil {
			return nil, err
		}
	}
	filter, err := c.ensureChatlistFilter(uid, localFilterID, inv.Title, selectedPeers)
	if err != nil {
		return nil, err
	}
	found := false
	for i := range st.Joined {
		if st.Joined[i].Slug == slug {
			st.Joined[i].FilterID = localFilterID
			st.Joined[i].Peers = selectedPeers
			found = true
			break
		}
	}
	if !found {
		st.Joined = append(st.Joined, clJoin{FilterID: localFilterID, Slug: slug, Peers: selectedPeers})
	}
	folderPeers, err := c.applyFolderPeers(uid, localFilterID, peers)
	if err != nil {
		return nil, err
	}
	if err := saveClist(uid, st); err != nil {
		return nil, err
	}
	return chatlistJoinUpdates(filter, folderPeers, users, chats), nil
}

func (c *ApiFullCore) invitePeersForFilter(st *clState, filterID int32) ([]*mtproto.InputPeer, error) {
	var own []*mtproto.InputPeer
	hasInvite := false
	for _, inv := range st.Invites {
		if inv.FilterID == filterID {
			hasInvite = true
			own = append(own, inv.Peers...)
		}
	}
	parts := [][]*mtproto.InputPeer{own}
	for _, j := range st.Joined {
		if j.FilterID != filterID || j.Slug == "" {
			continue
		}
		inv, ok, err := findClistInvite(j.Slug)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, mtproto.ErrInviteHashInvalid
		}
		hasInvite = true
		parts = append(parts, inv.Peers)
	}
	if !hasInvite {
		return nil, mtproto.ErrMethodNotImpl
	}
	return mergeInputPeers(parts...), nil
}

func (c *ApiFullCore) ChatlistsGetChatlistUpdates(in *mtproto.TLChatlistsGetChatlistUpdates) (*mtproto.Chatlists_ChatlistUpdates, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil || in.GetChatlist() == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	filterID := chatlistFilterID(in.GetChatlist())
	if err := c.requireChatlistFilter(uid, filterID); err != nil {
		return nil, err
	}
	empty := mtproto.MakeTLChatlistsChatlistUpdates(&mtproto.Chatlists_ChatlistUpdates{
		MissingPeers: []*mtproto.Peer{},
		Chats:        []*mtproto.Chat{},
		Users:        []*mtproto.User{},
	}).To_Chatlists_ChatlistUpdates()
	st, err := loadClist(uid)
	if err != nil {
		return nil, err
	}
	for _, id := range st.Hidden {
		if id == filterID {
			return empty, nil
		}
	}
	invitePeers, err := c.invitePeersForFilter(st, filterID)
	if err != nil {
		return nil, err
	}
	var joined []*mtproto.InputPeer
	hasJoin := false
	for _, j := range st.Joined {
		if j.FilterID != filterID {
			continue
		}
		hasJoin = true
		joined = append(joined, j.Peers...)
	}
	var missing []*mtproto.InputPeer
	if !hasJoin {
		missing = invitePeers
	} else {
		missing = peersMissing(invitePeers, joined)
	}
	users, chats, err := c.chatlistEntities(uid, missing)
	if err != nil {
		return nil, err
	}
	return mtproto.MakeTLChatlistsChatlistUpdates(&mtproto.Chatlists_ChatlistUpdates{
		MissingPeers: inputPeersToPeers(uid, missing),
		Chats:        chats,
		Users:        users,
	}).To_Chatlists_ChatlistUpdates(), nil
}

func (c *ApiFullCore) ChatlistsJoinChatlistUpdates(in *mtproto.TLChatlistsJoinChatlistUpdates) (*mtproto.Updates, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil || in.GetChatlist() == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	filterID := chatlistFilterID(in.GetChatlist())
	peers := in.GetPeers()
	if err := c.requireChatlistFilter(uid, filterID); err != nil {
		return nil, err
	}
	if len(peers) == 0 {
		return mtproto.MakeEmptyUpdates(), nil
	}
	st, err := loadClist(uid)
	if err != nil {
		return nil, err
	}
	invitePeers, err := c.invitePeersForFilter(st, filterID)
	if err != nil {
		return nil, err
	}
	if len(peersMissing(peers, invitePeers)) != 0 {
		return nil, mtproto.ErrPeerIdInvalid
	}
	users, chats, err := c.chatlistEntities(uid, peers)
	if err != nil {
		return nil, err
	}
	folderPeers, err := c.applyFolderPeers(uid, filterID, peers)
	if err != nil {
		return nil, err
	}
	found := false
	for i := range st.Joined {
		if st.Joined[i].FilterID != filterID {
			continue
		}
		st.Joined[i].Peers = mergeInputPeers(st.Joined[i].Peers, peers)
		found = true
	}
	if !found {
		st.Joined = append(st.Joined, clJoin{FilterID: filterID, Peers: peers})
	}
	hidden := st.Hidden[:0]
	for _, id := range st.Hidden {
		if id != filterID {
			hidden = append(hidden, id)
		}
	}
	st.Hidden = hidden
	if err := saveClist(uid, st); err != nil {
		return nil, err
	}
	return folderPeersUpdates(folderPeers, users, chats), nil
}

func (c *ApiFullCore) ChatlistsHideChatlistUpdates(in *mtproto.TLChatlistsHideChatlistUpdates) (*mtproto.Bool, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil || in.GetChatlist() == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	filterID := chatlistFilterID(in.GetChatlist())
	if err := c.requireChatlistFilter(uid, filterID); err != nil {
		return nil, err
	}
	st, err := loadClist(uid)
	if err != nil {
		return nil, err
	}
	for _, id := range st.Hidden {
		if id == filterID {
			return mtproto.BoolTrue, nil
		}
	}
	st.Hidden = append(st.Hidden, filterID)
	if err := saveClist(uid, st); err != nil {
		return nil, err
	}
	return mtproto.BoolTrue, nil
}

func (c *ApiFullCore) ChatlistsGetLeaveChatlistSuggestions(in *mtproto.TLChatlistsGetLeaveChatlistSuggestions) (*mtproto.Vector_Peer, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil || in.GetChatlist() == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	filterID := chatlistFilterID(in.GetChatlist())
	if err := c.requireChatlistFilter(uid, filterID); err != nil {
		return nil, err
	}
	state, err := loadClist(uid)
	if err != nil {
		return nil, err
	}
	var joined []*mtproto.InputPeer
	for _, entry := range state.Joined {
		if entry.FilterID == filterID {
			joined = append(joined, entry.Peers...)
		}
	}
	return &mtproto.Vector_Peer{Datas: inputPeersToPeers(uid, mergeInputPeers(joined))}, nil
}

func (c *ApiFullCore) ChatlistsLeaveChatlist(in *mtproto.TLChatlistsLeaveChatlist) (*mtproto.Updates, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil || in.GetChatlist() == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	filterID := chatlistFilterID(in.GetChatlist())
	if err := c.requireChatlistFilter(uid, filterID); err != nil {
		return nil, err
	}
	st, err := loadClist(uid)
	if err != nil {
		return nil, err
	}
	var joinedPeers []*mtproto.InputPeer
	for _, joined := range st.Joined {
		if joined.FilterID == filterID {
			joinedPeers = append(joinedPeers, joined.Peers...)
		}
	}
	joinedPeers = mergeInputPeers(joinedPeers)
	peers := in.GetPeers()
	if len(peers) == 0 {
		peers = joinedPeers
	} else if len(peersMissing(peers, joinedPeers)) != 0 {
		return nil, mtproto.ErrPeerIdInvalid
	}
	if len(peers) == 0 {
		return nil, mtproto.ErrFilterIdInvalid
	}
	users, chats, err := c.chatlistEntities(uid, peers)
	if err != nil {
		return nil, err
	}
	folderPeers, err := c.applyFolderPeers(uid, 0, peers)
	if err != nil {
		return nil, err
	}
	requested := make(map[string]struct{}, len(peers))
	for _, peer := range peers {
		requested[folderPeerKey(peer)] = struct{}{}
	}
	next := make([]clJoin, 0, len(st.Joined))
	for _, joined := range st.Joined {
		if joined.FilterID != filterID {
			next = append(next, joined)
			continue
		}
		remaining := make([]*mtproto.InputPeer, 0, len(joined.Peers))
		for _, peer := range joined.Peers {
			if _, leave := requested[folderPeerKey(peer)]; !leave {
				remaining = append(remaining, peer)
			}
		}
		if len(remaining) > 0 {
			joined.Peers = remaining
			next = append(next, joined)
		}
	}
	st.Joined = next
	hasJoinedFilter := false
	for _, joined := range st.Joined {
		if joined.FilterID == filterID {
			hasJoinedFilter = true
			break
		}
	}
	if !hasJoinedFilter {
		if err := c.deleteChatlistFilter(uid, filterID); err != nil {
			return nil, err
		}
	}
	if err := saveClist(uid, st); err != nil {
		return nil, err
	}
	return folderPeersUpdates(folderPeers, users, chats), nil
}
