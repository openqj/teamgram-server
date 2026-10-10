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
	"strconv"
	"strings"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/domain"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/persist"
	chatpb "github.com/teamgram/teamgram-server/app/service/biz/chat/chat"
	"github.com/teamgram/teamgram-server/app/service/dfs/dfs"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// RPCStoriesServer: Layer 229 methods previously returned ERR_ENTERPRISE_IS_BLOCKED.

// storiesProviderUnavailable is reserved for story operations that require an
// external provider (for example media upload, live streaming, or cross-peer
// aggregation). Self-owned story state is persisted and served by APIFull.
func storiesProviderUnavailable(c *ApiFullCore) error {
	if _, err := c.requireUserId(); err != nil {
		return err
	}
	return mtproto.ErrMethodNotImpl
}

// Per-user story document. JSON at "story:"+userId. Empty Get means none.
type storyAlbumRec struct {
	AlbumId int32   `json:"albumId"`
	Title   string  `json:"title"`
	Stories []int32 `json:"stories"`
}

type storyReportRec struct {
	Kind    string  `json:"kind"`
	Ids     []int32 `json:"ids,omitempty"`
	Message string  `json:"message,omitempty"`
	Option  []byte  `json:"option,omitempty"`
	UserId  int64   `json:"userId,omitempty"`
}

type storyViewerRec struct {
	UserId int64 `json:"userId"`
	Date   int32 `json:"date"`
}

type storyReactionRec struct {
	UserId   int64  `json:"userId"`
	Date     int32  `json:"date"`
	Emoticon string `json:"emoticon,omitempty"`
	Document int64  `json:"document,omitempty"`
}

type userStoryStore struct {
	Next          int32                        `json:"next"`
	Order         []int32                      `json:"order"`
	Items         map[int32]*mtproto.StoryItem `json:"items"`
	ByRand        map[int64]int32              `json:"byRand"`
	AllHidden     bool                         `json:"allHidden"`
	ReadMax       int32                        `json:"readMax"`
	HiddenPeers   map[string]bool              `json:"hiddenPeers"`
	HiddenUsers   map[int64]bool               `json:"hiddenUsers"`
	PinnedTop     []int32                      `json:"pinnedTop"`
	NextAlbum     int32                        `json:"nextAlbum"`
	AlbumOrder    []int32                      `json:"albumOrder"`
	Albums        map[int32]*storyAlbumRec     `json:"albums"`
	Links         map[int32]string             `json:"links"`
	Reports       []storyReportRec             `json:"reports"`
	StealthPast   bool                         `json:"stealthPast"`
	StealthFuture bool                         `json:"stealthFuture"`
	StealthUntil  int32                        `json:"stealthUntil"`
	Viewers       map[int32][]storyViewerRec   `json:"viewers"`
	Reactions     map[int32][]storyReactionRec `json:"reactions"`
}

func (s *userStoryStore) ensure() {
	if s.Items == nil {
		s.Items = map[int32]*mtproto.StoryItem{}
	}
	if s.ByRand == nil {
		s.ByRand = map[int64]int32{}
	}
	if s.HiddenPeers == nil {
		s.HiddenPeers = map[string]bool{}
	}
	if s.HiddenUsers == nil {
		s.HiddenUsers = map[int64]bool{}
	}
	if s.Albums == nil {
		s.Albums = map[int32]*storyAlbumRec{}
	}
	if s.Links == nil {
		s.Links = map[int32]string{}
	}
	if s.Viewers == nil {
		s.Viewers = map[int32][]storyViewerRec{}
	}
	if s.Reactions == nil {
		s.Reactions = map[int32][]storyReactionRec{}
	}
}

func storyKey(userID int64) string {
	return "story:" + strconv.FormatInt(userID, 10)
}

func decodeUserStories(raw string) (*userStoryStore, error) {
	if raw == "" {
		s := &userStoryStore{}
		s.ensure()
		return s, nil
	}
	s := &userStoryStore{}
	if err := json.Unmarshal([]byte(raw), s); err != nil {
		return nil, err
	}
	s.ensure()
	for _, item := range s.Items {
		if item != nil && item.GetMedia() == nil {
			item.Media = mtproto.MakeTLMessageMediaEmpty(&mtproto.MessageMedia{}).To_MessageMedia()
		}
	}
	return s, nil
}

func loadUserStories(userID int64) (*userStoryStore, error) {
	raw, err := persist.Default.Get(storyKey(userID))
	if err != nil {
		return nil, err
	}
	return decodeUserStories(raw)
}

func saveUserStories(userID int64, s *userStoryStore) error {
	b, err := json.Marshal(s)
	if err != nil {
		return err
	}
	return persist.Update(storyKey(userID), func(string) (string, error) { return string(b), nil })
}

func (s *userStoryStore) list(ids []int32, pinnedOnly bool) []*mtproto.StoryItem {
	out := make([]*mtproto.StoryItem, 0)
	take := func(it *mtproto.StoryItem) {
		if it == nil || (pinnedOnly && !it.GetPinned()) {
			return
		}
		out = append(out, it)
	}
	if len(ids) == 0 {
		for _, id := range s.Order {
			take(s.Items[id])
		}
		return out
	}
	for _, id := range ids {
		take(s.Items[id])
	}
	return out
}

func listOwnStories(userID int64, ids []int32, pinnedOnly bool) []*mtproto.StoryItem {
	s, err := loadUserStories(userID)
	if err != nil || s == nil {
		return nil
	}
	return s.list(ids, pinnedOnly)
}

func storyPeerOwned(userID int64, peer *mtproto.InputPeer) bool {
	if peer == nil || peer.GetPredicateName() == mtproto.Predicate_inputPeerSelf {
		return true
	}
	if peer.GetChannelId() != 0 || peer.GetChatId() != 0 {
		return false
	}
	return peer.GetUserId() == 0 || peer.GetUserId() == userID
}

func storyUserOwned(userID int64, user *mtproto.InputUser) bool {
	if user == nil || user.GetPredicateName() == mtproto.Predicate_inputUserSelf {
		return true
	}
	return user.GetUserId() == 0 || user.GetUserId() == userID
}

func storiesBox(items []*mtproto.StoryItem) *mtproto.Stories_Stories {
	if items == nil {
		items = []*mtproto.StoryItem{}
	}
	pinned := make([]int32, 0)
	for _, it := range items {
		if it != nil && it.GetPinned() {
			pinned = append(pinned, it.GetId())
		}
	}
	return mtproto.MakeTLStoriesStories(&mtproto.Stories_Stories{
		Count:       int32(len(items)),
		Stories:     items,
		PinnedToTop: pinned,
		Chats:       []*mtproto.Chat{},
		Users:       []*mtproto.User{},
	}).To_Stories_Stories()
}

func storyUpdates(userID int64, item *mtproto.StoryItem) *mtproto.Updates {
	ups := []*mtproto.Update{}
	if item != nil {
		ups = append(ups, mtproto.MakeTLUpdateStory(&mtproto.Update{
			Peer_PEER: mtproto.MakeTLPeerUser(&mtproto.Peer{UserId: userID}).To_Peer(),
			Story:     item,
			UserId:    userID,
		}).To_Update())
	}
	return mtproto.MakeTLUpdates(&mtproto.Updates{
		Updates: ups,
		Users:   []*mtproto.User{},
		Chats:   []*mtproto.Chat{},
		Date:    int32(time.Now().Unix()),
	}).To_Updates()
}

func storyPeerKey(peer *mtproto.InputPeer) string {
	if peer == nil || peer.GetPredicateName() == mtproto.Predicate_inputPeerSelf {
		return "self"
	}
	return fmt.Sprintf("%s:%d:%d:%d", peer.GetPredicateName(), peer.GetUserId(), peer.GetChatId(), peer.GetChannelId())
}

func storyRequestOwned(userID int64, peer *mtproto.InputPeer, user *mtproto.InputUser) bool {
	if peer != nil {
		return storyPeerOwned(userID, peer)
	}
	if user != nil {
		return storyUserOwned(userID, user)
	}
	return true
}

func (s *userStoryStore) boxed(ids []int32, pinnedOnly bool) *mtproto.Stories_Stories {
	box := storiesBox(s.list(ids, pinnedOnly))
	if s.PinnedTop != nil {
		box.PinnedToTop = append([]int32(nil), s.PinnedTop...)
	}
	return box
}

// ownStoryStore resolves the only story authority currently available to
// APIFull: the authenticated user's durable story store. Requests for a
// different peer must not be answered with the caller's stories.
func (c *ApiFullCore) ownStoryStore(peer *mtproto.InputPeer, user *mtproto.InputUser) (int64, *userStoryStore, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return 0, nil, err
	}
	if !storyRequestOwned(uid, peer, user) {
		return 0, nil, mtproto.ErrMethodNotImpl
	}
	stories, err := loadUserStories(uid)
	if err != nil {
		return 0, nil, err
	}
	return uid, stories, nil
}

func storyPage(items []*mtproto.StoryItem, offset, limit int32) []*mtproto.StoryItem {
	if offset < 0 {
		offset = 0
	}
	if offset >= int32(len(items)) {
		return []*mtproto.StoryItem{}
	}
	if limit <= 0 || limit > 100 {
		limit = 100
	}
	end := offset + limit
	if end > int32(len(items)) {
		end = int32(len(items))
	}
	return items[offset:end]
}

// storyPageByID applies the exclusive story-ID cursor used by the archive and
// pinned-story RPCs. Story IDs are not dense after deletion, so treating the
// cursor as a slice index skips the wrong records (or returns an empty page).
func storyPageByID(items []*mtproto.StoryItem, offsetID, limit int32) []*mtproto.StoryItem {
	start := 0
	if offsetID > 0 {
		for i, item := range items {
			if item != nil && item.GetId() <= offsetID {
				start = i + 1
			}
		}
	}
	return storyPage(items[start:], 0, limit)
}

func albumTL(a *storyAlbumRec) *mtproto.StoryAlbum {
	if a == nil {
		return mtproto.MakeTLStoryAlbum(&mtproto.StoryAlbum{}).To_StoryAlbum()
	}
	return mtproto.MakeTLStoryAlbum(&mtproto.StoryAlbum{
		AlbumId: a.AlbumId,
		Title:   a.Title,
	}).To_StoryAlbum()
}

func attachAlbum(item *mtproto.StoryItem, albumID int32, add bool) {
	if item == nil {
		return
	}
	next := make([]int32, 0, len(item.Albums)+1)
	for _, id := range item.Albums {
		if id != albumID {
			next = append(next, id)
		}
	}
	if add {
		next = append(next, albumID)
	}
	item.Albums = next
}

func (s *userStoryStore) bindAlbumStories(albumID int32, ids []int32, add bool) {
	for _, id := range ids {
		attachAlbum(s.Items[id], albumID, add)
	}
}

func (s *userStoryStore) touchViews(id int32, viewer int64, now int32) {
	item := s.Items[id]
	if item == nil {
		return
	}
	if item.Views == nil {
		item.Views = mtproto.MakeTLStoryViews(&mtproto.StoryViews{}).To_StoryViews()
	}
	item.Views.ViewsCount++
	item.Views.HasViewers = true
	for _, v := range s.Viewers[id] {
		if v.UserId == viewer {
			return
		}
	}
	s.Viewers[id] = append(s.Viewers[id], storyViewerRec{UserId: viewer, Date: now})
}

func (s *userStoryStore) putReaction(id int32, userID int64, rx *mtproto.Reaction, now int32) {
	item := s.Items[id]
	if item == nil {
		return
	}
	remove := rx == nil || rx.GetPredicateName() == mtproto.Predicate_reactionEmpty
	if remove {
		item.SentReaction = nil
		list := s.Reactions[id]
		kept := list[:0]
		for _, existing := range list {
			if existing.UserId != userID {
				kept = append(kept, existing)
			}
		}
		if len(kept) == 0 {
			delete(s.Reactions, id)
		} else {
			s.Reactions[id] = kept
		}
		return
	}
	item.SentReaction = rx
	list := s.Reactions[id]
	rec := storyReactionRec{UserId: userID, Date: now}
	if rx != nil {
		rec.Emoticon = rx.GetEmoticon()
		rec.Document = rx.GetDocumentId()
	}
	for i := range list {
		if list[i].UserId == userID {
			list[i] = rec
			s.Reactions[id] = list
			return
		}
	}
	s.Reactions[id] = append(list, rec)
}

func reactionFromRec(r storyReactionRec) *mtproto.Reaction {
	if r.Document != 0 {
		return mtproto.MakeTLReactionCustomEmoji(&mtproto.Reaction{DocumentId: r.Document}).To_Reaction()
	}
	if r.Emoticon == "" {
		return nil
	}
	return mtproto.MakeTLReactionEmoji(&mtproto.Reaction{Emoticon: r.Emoticon}).To_Reaction()
}

func mutateOwnStories(userID int64, fn func(*userStoryStore)) (*userStoryStore, error) {
	return mutateOwnStoriesE(userID, func(s *userStoryStore) error {
		fn(s)
		return nil
	})
}

func mutateOwnStoriesE(userID int64, fn func(*userStoryStore) error) (*userStoryStore, error) {
	var result *userStoryStore
	err := persist.Update(storyKey(userID), func(raw string) (string, error) {
		s, err := decodeUserStories(raw)
		if err != nil {
			return "", err
		}
		if err = fn(s); err != nil {
			return "", err
		}
		result = s
		encoded, err := json.Marshal(s)
		return string(encoded), err
	})
	return result, err
}

func storyMaxID(s *userStoryStore) int32 {
	if s == nil {
		return 0
	}
	// Order is a presentation sequence and may be sparse or restored from an
	// older store with a non-monotonic order. The read cursor is an ID, so it
	// must use the greatest persisted story ID rather than the last slice item.
	// Older records may contain only Order, so retain that as a read fallback.
	maxID := int32(0)
	if len(s.Items) > 0 {
		for id := range s.Items {
			if id > maxID {
				maxID = id
			}
		}
		return maxID
	}
	for _, id := range s.Order {
		if id > maxID {
			maxID = id
		}
	}
	return maxID
}

func storyStealthMode(s *userStoryStore) *mtproto.StoriesStealthMode {
	if s == nil {
		return mtproto.MakeTLStoriesStealthMode(&mtproto.StoriesStealthMode{}).To_StoriesStealthMode()
	}
	return mtproto.MakeTLStoriesStealthMode(&mtproto.StoriesStealthMode{
		ActiveUntilDate: func() *wrapperspb.Int32Value {
			if s.StealthUntil > 0 {
				return wrapperspb.Int32(s.StealthUntil)
			}
			return nil
		}(),
	}).To_StoriesStealthMode()
}

func (c *ApiFullCore) StoriesCanSendStory30EB63F0(in *mtproto.TLStoriesCanSendStory30EB63F0) (*mtproto.Stories_CanSendStoryCount, error) {
	if in == nil {
		return nil, storiesProviderUnavailable(c)
	}
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if !storyPeerOwned(uid, in.GetPeer()) {
		return nil, mtproto.ErrMethodNotImpl
	}
	stories, err := loadUserStories(uid)
	if err != nil {
		return nil, err
	}
	remaining := int32(100 - len(stories.Order))
	if remaining < 0 {
		remaining = 0
	}
	return mtproto.MakeTLStoriesCanSendStoryCount(&mtproto.Stories_CanSendStoryCount{CountRemains: remaining}).To_Stories_CanSendStoryCount(), nil
}

func (c *ApiFullCore) StoriesSendStory(in *mtproto.TLStoriesSendStory) (*mtproto.Updates, error) {
	if in == nil {
		return nil, storiesProviderUnavailable(c)
	}
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if !storyPeerOwned(uid, in.GetPeer()) {
		return nil, mtproto.ErrPeerIdInvalid
	}
	if in.GetMedia() == nil {
		return nil, mtproto.ErrMediaEmpty
	}
	stories, err := loadUserStories(uid)
	if err != nil {
		return nil, err
	}
	if in.GetRandomId() != 0 {
		if existing := stories.ByRand[in.GetRandomId()]; existing != 0 {
			return storyUpdates(uid, stories.Items[existing]), nil
		}
	}
	for _, albumID := range in.GetAlbums() {
		if stories.Albums[albumID] == nil {
			return nil, mtproto.ErrInputRequestInvalid
		}
	}
	media, err := c.uploadStoryMedia(uid, in.GetMedia())
	if err != nil {
		return nil, err
	}
	var item *mtproto.StoryItem
	_, err = mutateOwnStoriesE(uid, func(next *userStoryStore) error {
		if in.GetRandomId() != 0 {
			if existing := next.ByRand[in.GetRandomId()]; existing != 0 {
				item = next.Items[existing]
				return nil
			}
		}
		next.Next++
		if next.Next <= 0 {
			next.Next = 1
		}
		period := int32(86400)
		if in.GetPeriod() != nil && in.GetPeriod().GetValue() > 0 {
			period = in.GetPeriod().GetValue()
		}
		item = mtproto.MakeTLStoryItem(&mtproto.StoryItem{
			Id:         next.Next,
			Date:       int32(time.Now().Unix()),
			ExpireDate: int32(time.Now().Unix()) + period,
			Pinned:     in.GetPinned(),
			Noforwards: in.GetNoforwards(),
			Out:        true,
			Public:     true,
			FromId:     mtproto.MakeTLPeerUser(&mtproto.Peer{UserId: uid}).To_Peer(),
			Caption:    in.GetCaption(),
			Entities:   in.GetEntities(),
			Media:      media,
			MediaAreas: in.GetMediaAreas(),
			Albums:     append([]int32(nil), in.GetAlbums()...),
		}).To_StoryItem()
		next.Items[item.GetId()] = item
		next.Order = append(next.Order, item.GetId())
		if in.GetRandomId() != 0 {
			next.ByRand[in.GetRandomId()] = item.GetId()
		}
		for _, albumID := range in.GetAlbums() {
			next.bindAlbumStories(albumID, []int32{item.GetId()}, true)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return storyUpdates(uid, item), nil
}

func (c *ApiFullCore) uploadStoryMedia(uid int64, input *mtproto.InputMedia) (*mtproto.MessageMedia, error) {
	if input == nil {
		return nil, mtproto.ErrMediaEmpty
	}
	d := c.apifullDao()
	if d == nil || d.DfsClient == nil {
		return nil, mtproto.ErrMethodNotImpl
	}
	if input.GetFile() == nil {
		return nil, mtproto.ErrMediaInvalid
	}
	switch input.GetPredicateName() {
	case mtproto.Predicate_inputMediaUploadedPhoto:
		photo, err := d.DfsUploadPhotoFileV2(callContext(c), &dfs.TLDfsUploadPhotoFileV2{Creator: uid, File: input.GetFile()})
		if err != nil {
			return nil, err
		}
		if photo == nil || photo.GetId() == 0 {
			return nil, mtproto.ErrMediaInvalid
		}
		return mtproto.MakeTLMessageMediaPhoto(&mtproto.MessageMedia{Photo_FLAGPHOTO: photo, TtlSeconds: input.GetTtlSeconds()}).To_MessageMedia(), nil
	case mtproto.Predicate_inputMediaUploadedDocument:
		document, err := d.DfsUploadDocumentFileV2(callContext(c), &dfs.TLDfsUploadDocumentFileV2{Creator: uid, Media: input})
		if err != nil {
			return nil, err
		}
		if document == nil || document.GetId() == 0 {
			return nil, mtproto.ErrMediaInvalid
		}
		return mtproto.MakeTLMessageMediaDocument(&mtproto.MessageMedia{Document: document, TtlSeconds: input.GetTtlSeconds()}).To_MessageMedia(), nil
	default:
		return nil, mtproto.ErrMediaInvalid
	}
}

func (c *ApiFullCore) StoriesEditStory(in *mtproto.TLStoriesEditStory) (*mtproto.Updates, error) {
	if in == nil {
		return nil, storiesProviderUnavailable(c)
	}
	if in.GetId() <= 0 {
		return nil, mtproto.ErrStoryIdEmpty
	}
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if !storyPeerOwned(uid, in.GetPeer()) {
		return nil, mtproto.ErrMethodNotImpl
	}
	if in.GetMedia() != nil {
		return nil, mtproto.ErrMethodNotImpl
	}
	var item *mtproto.StoryItem
	_, err = mutateOwnStoriesE(uid, func(stories *userStoryStore) error {
		item = stories.Items[in.GetId()]
		if item == nil {
			return mtproto.ErrStoryIdEmpty
		}
		if in.GetCaption() != nil {
			item.Caption = in.GetCaption()
		}
		if in.GetEntities() != nil {
			item.Entities = in.GetEntities()
		}
		item.Edited = true
		return nil
	})
	if err != nil {
		return nil, err
	}
	return storyUpdates(uid, item), nil
}

func (c *ApiFullCore) StoriesDeleteStories(in *mtproto.TLStoriesDeleteStories) (*mtproto.Vector_Int, error) {
	if in == nil {
		return nil, storiesProviderUnavailable(c)
	}
	if len(in.GetId()) == 0 {
		return nil, mtproto.ErrStoryIdEmpty
	}
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if !storyPeerOwned(uid, in.GetPeer()) {
		return nil, mtproto.ErrMethodNotImpl
	}
	_, err = mutateOwnStoriesE(uid, func(stories *userStoryStore) error {
		for _, id := range in.GetId() {
			if stories.Items[id] == nil {
				return mtproto.ErrStoryIdEmpty
			}
		}
		deleted := make(map[int32]struct{}, len(in.GetId()))
		for _, id := range in.GetId() {
			delete(stories.Items, id)
			delete(stories.Viewers, id)
			delete(stories.Reactions, id)
			delete(stories.Links, id)
			deleted[id] = struct{}{}
		}
		order := stories.Order[:0]
		for _, id := range stories.Order {
			if _, ok := deleted[id]; !ok {
				order = append(order, id)
			}
		}
		stories.Order = order
		for randomID, id := range stories.ByRand {
			if _, ok := deleted[id]; ok {
				delete(stories.ByRand, randomID)
			}
		}
		for _, album := range stories.Albums {
			kept := album.Stories[:0]
			for _, id := range album.Stories {
				if _, ok := deleted[id]; !ok {
					kept = append(kept, id)
				}
			}
			album.Stories = kept
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &mtproto.Vector_Int{Datas: append([]int32(nil), in.GetId()...)}, nil
}

func (c *ApiFullCore) StoriesTogglePinned(in *mtproto.TLStoriesTogglePinned) (*mtproto.Vector_Int, error) {
	if in == nil || len(in.GetId()) == 0 || in.GetPinned() == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in.GetPeer() != nil && !storyPeerOwned(uid, in.GetPeer()) {
		return nil, mtproto.ErrMethodNotImpl
	}
	pinned := in.GetPinned().GetPredicateName() == mtproto.Predicate_boolTrue
	result := make([]int32, 0, len(in.GetId()))
	_, err = mutateOwnStoriesE(uid, func(stories *userStoryStore) error {
		for _, id := range in.GetId() {
			item := stories.Items[id]
			if item == nil {
				return mtproto.ErrStoryIdEmpty
			}
			item.Pinned = pinned
			result = append(result, id)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &mtproto.Vector_Int{Datas: result}, nil
}

func storyListState(items []*mtproto.StoryItem) string {
	if len(items) == 0 {
		return ""
	}
	parts := make([]string, 0, len(items))
	for _, it := range items {
		if it == nil {
			continue
		}
		parts = append(parts, fmt.Sprintf("%d:%s:%t", it.GetId(), it.GetCaption().GetValue(), it.GetEdited()))
	}
	return strings.Join(parts, ",")
}

func (c *ApiFullCore) StoriesGetAllStories(in *mtproto.TLStoriesGetAllStories) (*mtproto.Stories_AllStories, error) {
	if in == nil {
		return nil, storiesProviderUnavailable(c)
	}
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	stories, err := loadUserStories(uid)
	if err != nil {
		return nil, err
	}
	items := stories.list(nil, false)
	userStories := mtproto.MakeTLUserStories(&mtproto.UserStories{
		UserId: uid, MaxReadId: wrapperspb.Int32(stories.ReadMax), Stories: items,
	}).To_UserStories()
	return mtproto.MakeTLStoriesAllStories(&mtproto.Stories_AllStories{
		State: storyListState(items), StealthMode: storyStealthMode(stories),
		HasMore: false, Count: int32(len(items)), UserStories: []*mtproto.UserStories{userStories},
		Chats: []*mtproto.Chat{}, Users: []*mtproto.User{}, PeerStories: []*mtproto.PeerStories{},
	}).To_Stories_AllStories(), nil
}

func (c *ApiFullCore) StoriesGetPinnedStories(in *mtproto.TLStoriesGetPinnedStories) (*mtproto.Stories_Stories, error) {
	if in == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	_, stories, err := c.ownStoryStore(in.GetPeer(), in.GetUserId())
	if err != nil {
		return nil, err
	}
	return storiesBox(storyPageByID(stories.list(nil, true), in.GetOffsetId(), in.GetLimit())), nil
}

func (c *ApiFullCore) StoriesGetStoriesArchive(in *mtproto.TLStoriesGetStoriesArchive) (*mtproto.Stories_Stories, error) {
	if in == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	_, stories, err := c.ownStoryStore(in.GetPeer(), nil)
	if err != nil {
		return nil, err
	}
	items := stories.list(nil, false)
	box := storiesBox(storyPageByID(items, in.GetOffsetId(), in.GetLimit()))
	// stories.getStoriesArchive does not return pinned_to_top. TDLib treats a
	// non-empty field here as a malformed archive response.
	box.PinnedToTop = nil
	return box, nil
}

func (c *ApiFullCore) StoriesGetStoriesByID(in *mtproto.TLStoriesGetStoriesByID) (*mtproto.Stories_Stories, error) {
	if in == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	_, stories, err := c.ownStoryStore(in.GetPeer(), in.GetUserId())
	if err != nil {
		return nil, err
	}
	return storiesBox(stories.list(in.GetId(), false)), nil
}

func (c *ApiFullCore) StoriesToggleAllStoriesHidden(in *mtproto.TLStoriesToggleAllStoriesHidden) (*mtproto.Bool, error) {
	if in == nil || in.GetHidden() == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	_, err = mutateOwnStoriesE(uid, func(stories *userStoryStore) error {
		stories.AllHidden = in.GetHidden().GetPredicateName() == mtproto.Predicate_boolTrue
		return nil
	})
	if err != nil {
		return nil, err
	}
	return mtproto.BoolTrue, nil
}

func (c *ApiFullCore) StoriesReadStories(in *mtproto.TLStoriesReadStories) (*mtproto.Vector_Int, error) {
	if in == nil || in.GetMaxId() <= 0 {
		return nil, mtproto.ErrStoryIdEmpty
	}
	uid, stories, err := c.ownStoryStore(in.GetPeer(), in.GetUserId())
	if err != nil {
		return nil, err
	}
	ids := make([]int32, 0)
	for _, id := range stories.Order {
		if id <= in.GetMaxId() {
			ids = append(ids, id)
		}
	}
	_, err = mutateOwnStoriesE(uid, func(next *userStoryStore) error {
		if in.GetMaxId() > next.ReadMax {
			next.ReadMax = in.GetMaxId()
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &mtproto.Vector_Int{Datas: ids}, nil
}

func (c *ApiFullCore) StoriesIncrementStoryViews(in *mtproto.TLStoriesIncrementStoryViews) (*mtproto.Bool, error) {
	if in == nil || len(in.GetId()) == 0 {
		return nil, mtproto.ErrStoryIdEmpty
	}
	uid, _, err := c.ownStoryStore(in.GetPeer(), in.GetUserId())
	if err != nil {
		return nil, err
	}
	now := int32(time.Now().Unix())
	_, err = mutateOwnStoriesE(uid, func(stories *userStoryStore) error {
		for _, id := range in.GetId() {
			if stories.Items[id] == nil {
				return mtproto.ErrStoryIdEmpty
			}
			stories.touchViews(id, uid, now)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return mtproto.BoolTrue, nil
}

func (c *ApiFullCore) StoriesGetStoryViewsList(in *mtproto.TLStoriesGetStoryViewsList) (*mtproto.Stories_StoryViewsList, error) {
	if in == nil || in.GetId() <= 0 {
		return nil, mtproto.ErrStoryIdEmpty
	}
	_, stories, err := c.ownStoryStore(in.GetPeer(), nil)
	if err != nil {
		return nil, err
	}
	if stories.Items[in.GetId()] == nil {
		return nil, mtproto.ErrStoryIdEmpty
	}
	viewers := stories.Viewers[in.GetId()]
	views := make([]*mtproto.StoryView, 0, len(viewers))
	for _, viewer := range viewers {
		views = append(views, mtproto.MakeTLStoryView(&mtproto.StoryView{
			UserId: viewer.UserId,
			Date:   viewer.Date,
		}).To_StoryView())
	}
	limit := in.GetLimit()
	if limit <= 0 || limit > 100 {
		limit = 100
	}
	start := 0
	if raw := strings.TrimSpace(in.GetOffset()); raw != "" {
		parsed, parseErr := strconv.Atoi(raw)
		if parseErr != nil || parsed < 0 {
			return nil, mtproto.ErrOffsetInvalid
		}
		start = parsed
	}
	if in.GetOffsetId() > 0 {
		for start < len(views) && int64(views[start].GetUserId()) <= in.GetOffsetId() {
			start++
		}
	}
	if start > len(views) {
		start = len(views)
	}
	end := start + int(limit)
	if end > len(views) {
		end = len(views)
	}
	views = views[start:end]
	var nextOffset *wrapperspb.StringValue
	if end < len(stories.Viewers[in.GetId()]) {
		nextOffset = wrapperspb.String(strconv.Itoa(end))
	}
	reactions := stories.Reactions[in.GetId()]
	return mtproto.MakeTLStoriesStoryViewsList(&mtproto.Stories_StoryViewsList{
		Count:          int32(len(stories.Viewers[in.GetId()])),
		ViewsCount:     stories.Items[in.GetId()].GetViews().GetViewsCount(),
		ReactionsCount: int32(len(reactions)),
		Views:          views,
		Chats:          []*mtproto.Chat{},
		Users:          []*mtproto.User{},
		NextOffset:     nextOffset,
	}).To_Stories_StoryViewsList(), nil
}

func (c *ApiFullCore) StoriesGetStoriesViews(in *mtproto.TLStoriesGetStoriesViews) (*mtproto.Stories_StoryViews, error) {
	if in == nil || len(in.GetId()) == 0 {
		return nil, mtproto.ErrStoryIdEmpty
	}
	_, stories, err := c.ownStoryStore(in.GetPeer(), nil)
	if err != nil {
		return nil, err
	}
	views := make([]*mtproto.StoryViews, 0, len(in.GetId()))
	for _, id := range in.GetId() {
		item := stories.Items[id]
		if item == nil {
			return nil, mtproto.ErrStoryIdEmpty
		}
		v := item.GetViews()
		if v == nil {
			v = mtproto.MakeTLStoryViews(&mtproto.StoryViews{}).To_StoryViews()
		}
		copy := proto.Clone(v).(*mtproto.StoryViews)
		copy.RecentViewers = append([]int64(nil), v.GetRecentViewers()...)
		if len(copy.RecentViewers) == 0 {
			for _, viewer := range stories.Viewers[id] {
				copy.RecentViewers = append(copy.RecentViewers, viewer.UserId)
			}
		}
		views = append(views, copy)
	}
	return mtproto.MakeTLStoriesStoryViews(&mtproto.Stories_StoryViews{
		Views: views,
		Users: []*mtproto.User{},
	}).To_Stories_StoryViews(), nil
}

func (c *ApiFullCore) StoriesExportStoryLink(in *mtproto.TLStoriesExportStoryLink) (*mtproto.ExportedStoryLink, error) {
	if in == nil {
		return nil, storiesProviderUnavailable(c)
	}
	if in.GetId() <= 0 {
		return nil, mtproto.ErrStoryIdEmpty
	}
	uid, stories, err := c.ownStoryStore(in.GetPeer(), in.GetUserId())
	if err != nil {
		return nil, err
	}
	if stories.Items[in.GetId()] == nil {
		return nil, mtproto.ErrStoryIdEmpty
	}
	link := stories.Links[in.GetId()]
	if link == "" {
		link = fmt.Sprintf("https://t.me/story/%d/%d", uid, in.GetId())
		_, err = mutateOwnStoriesE(uid, func(next *userStoryStore) error {
			if next.Items[in.GetId()] == nil {
				return mtproto.ErrStoryIdEmpty
			}
			if next.Links[in.GetId()] == "" {
				next.Links[in.GetId()] = link
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	return mtproto.MakeTLExportedStoryLink(&mtproto.ExportedStoryLink{Link: link}).To_ExportedStoryLink(), nil
}

func (c *ApiFullCore) StoriesReport19D8EB45(in *mtproto.TLStoriesReport19D8EB45) (*mtproto.ReportResult, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if !domain.Ready() {
		return nil, mtproto.ErrMethodNotImpl
	}
	if in == nil || !reportMessageIDs(in.GetId()) {
		return nil, mtproto.ErrInputRequestInvalid
	}
	target, err := reportPeerTarget(uid, in.GetPeer())
	if err != nil {
		return nil, err
	}
	if err = c.recordReport(uid, "stories.report", target, in); err != nil {
		return nil, err
	}
	return mtproto.MakeTLReportResultReported(nil).To_ReportResult(), nil
}

func (c *ApiFullCore) StoriesActivateStealthMode(in *mtproto.TLStoriesActivateStealthMode) (*mtproto.Updates, error) {
	if in == nil {
		return nil, storiesProviderUnavailable(c)
	}
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	_, err = mutateOwnStories(uid, func(stories *userStoryStore) {
		stories.StealthPast = in.GetPast()
		stories.StealthFuture = in.GetFuture()
		if stories.StealthPast || stories.StealthFuture {
			stories.StealthUntil = int32(time.Now().Unix()) + 3600
		} else {
			stories.StealthUntil = 0
		}
	})
	if err != nil {
		return nil, err
	}
	return storyUpdates(uid, nil), nil
}

func (c *ApiFullCore) StoriesSendReaction(in *mtproto.TLStoriesSendReaction) (*mtproto.Updates, error) {
	if in == nil {
		return nil, storiesProviderUnavailable(c)
	}
	if in.GetStoryId() <= 0 {
		return nil, mtproto.ErrStoryIdEmpty
	}
	uid, _, err := c.ownStoryStore(in.GetPeer(), in.GetUserId())
	if err != nil {
		return nil, err
	}
	if in.GetReaction() == nil || in.GetReaction().GetPredicateName() == "" {
		return nil, mtproto.ErrReactionEmpty
	}
	var updated *userStoryStore
	updated, err = mutateOwnStoriesE(uid, func(next *userStoryStore) error {
		if next.Items[in.GetStoryId()] == nil {
			return mtproto.ErrStoryIdEmpty
		}
		next.putReaction(in.GetStoryId(), uid, in.GetReaction(), int32(time.Now().Unix()))
		return nil
	})
	if err != nil {
		return nil, err
	}
	return storyUpdates(uid, updated.Items[in.GetStoryId()]), nil
}

func (c *ApiFullCore) StoriesGetPeerStories(in *mtproto.TLStoriesGetPeerStories) (*mtproto.Stories_PeerStories, error) {
	if in == nil {
		return nil, storiesProviderUnavailable(c)
	}
	uid, stories, err := c.ownStoryStore(in.GetPeer(), nil)
	if err != nil {
		return nil, err
	}
	peerStories := mtproto.MakeTLPeerStories(&mtproto.PeerStories{
		Peer:      mtproto.MakeTLPeerUser(&mtproto.Peer{UserId: uid}).To_Peer(),
		MaxReadId: wrapperspb.Int32(stories.ReadMax), Stories: stories.list(nil, false),
	}).To_PeerStories()
	return mtproto.MakeTLStoriesPeerStories(&mtproto.Stories_PeerStories{
		Stories: peerStories, Chats: []*mtproto.Chat{}, Users: []*mtproto.User{},
	}).To_Stories_PeerStories(), nil
}

func (c *ApiFullCore) StoriesGetAllReadPeerStories(in *mtproto.TLStoriesGetAllReadPeerStories) (*mtproto.Updates, error) {
	if in == nil {
		return nil, storiesProviderUnavailable(c)
	}
	uid, stories, err := c.ownStoryStore(nil, nil)
	if err != nil {
		return nil, err
	}
	maxID := storyMaxID(stories)
	if maxID > stories.ReadMax {
		if _, err = mutateOwnStoriesE(uid, func(next *userStoryStore) error {
			next.ReadMax = maxID
			return nil
		}); err != nil {
			return nil, err
		}
	}
	if maxID == 0 {
		return mtproto.MakeUpdatesByUpdates(), nil
	}
	update := mtproto.MakeTLUpdateReadStories(&mtproto.Update{
		Peer_PEER: mtproto.MakeTLPeerUser(&mtproto.Peer{UserId: uid}).To_Peer(),
		MaxId:     maxID,
	}).To_Update()
	return mtproto.MakeUpdatesByUpdates(update), nil
}

func (c *ApiFullCore) StoriesGetPeerMaxIDs78499170(in *mtproto.TLStoriesGetPeerMaxIDs78499170) (*mtproto.Vector_RecentStory, error) {
	if in == nil {
		return nil, storiesProviderUnavailable(c)
	}
	uid, stories, err := c.ownStoryStore(nil, nil)
	if err != nil {
		return nil, err
	}
	result := make([]*mtproto.RecentStory, 0, len(in.GetId()))
	for _, peer := range in.GetId() {
		if !storyPeerOwned(uid, peer) {
			return nil, mtproto.ErrMethodNotImpl
		}
		result = append(result, mtproto.MakeTLRecentStory(&mtproto.RecentStory{
			MaxId: wrapperspb.Int32(storyMaxID(stories)),
		}).To_RecentStory())
	}
	return &mtproto.Vector_RecentStory{Datas: result}, nil
}

func (c *ApiFullCore) StoriesGetChatsToSend(in *mtproto.TLStoriesGetChatsToSend) (*mtproto.Messages_Chats, error) {
	if in == nil {
		return nil, storiesProviderUnavailable(c)
	}
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	d := c.apifullDao()
	if d == nil || d.ChatClient == nil {
		return nil, mtproto.ErrMethodNotImpl
	}
	chats, err := d.ChatGetMyChatList(callContext(c), &chatpb.TLChatGetMyChatList{
		UserId: uid, IsCreator: mtproto.BoolTrue,
	})
	if err != nil {
		return nil, err
	}
	if chats == nil {
		return nil, mtproto.ErrInternalServerError
	}
	result := make([]*mtproto.Chat, 0, len(chats.GetDatas()))
	for _, mutable := range chats.GetDatas() {
		if mutable == nil {
			continue
		}
		result = append(result, mutable.ToUnsafeChat(uid))
	}
	return mtproto.MakeTLMessagesChats(&mtproto.Messages_Chats{Chats: result, Count: int32(len(result))}).To_Messages_Chats(), nil
}

func (c *ApiFullCore) StoriesTogglePeerStoriesHidden(in *mtproto.TLStoriesTogglePeerStoriesHidden) (*mtproto.Bool, error) {
	if in == nil || in.GetPeer() == nil || in.GetHidden() == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	key := storyPeerKey(in.GetPeer())
	if key == "self" {
		return nil, mtproto.ErrInputRequestInvalid
	}
	_, err = mutateOwnStories(uid, func(stories *userStoryStore) {
		if in.GetHidden().GetPredicateName() == mtproto.Predicate_boolTrue {
			stories.HiddenPeers[key] = true
		} else {
			delete(stories.HiddenPeers, key)
		}
	})
	if err != nil {
		return nil, err
	}
	return mtproto.BoolTrue, nil
}

func (c *ApiFullCore) StoriesGetStoryReactionsList(in *mtproto.TLStoriesGetStoryReactionsList) (*mtproto.Stories_StoryReactionsList, error) {
	if in == nil || in.GetId() <= 0 {
		return nil, mtproto.ErrStoryIdEmpty
	}
	_, stories, err := c.ownStoryStore(in.GetPeer(), nil)
	if err != nil {
		return nil, err
	}
	if stories.Items[in.GetId()] == nil {
		return nil, mtproto.ErrStoryIdEmpty
	}
	reactions := stories.Reactions[in.GetId()]
	result := make([]*mtproto.StoryReaction, 0, len(reactions))
	for _, reaction := range reactions {
		converted := reactionFromRec(reaction)
		if in.GetReaction() != nil && !proto.Equal(converted, in.GetReaction()) {
			continue
		}
		result = append(result, mtproto.MakeTLStoryReaction(&mtproto.StoryReaction{
			PeerId:   mtproto.MakeTLPeerUser(&mtproto.Peer{UserId: reaction.UserId}).To_Peer(),
			Date:     reaction.Date,
			Reaction: converted,
		}).To_StoryReaction())
	}
	filteredCount := len(result)
	limit := in.GetLimit()
	if limit <= 0 || limit > 100 {
		limit = 100
	}
	start := 0
	if raw := strings.TrimSpace(in.GetOffset().GetValue()); raw != "" {
		parsed, parseErr := strconv.Atoi(raw)
		if parseErr != nil || parsed < 0 {
			return nil, mtproto.ErrOffsetInvalid
		}
		start = parsed
	}
	if start > len(result) {
		start = len(result)
	}
	end := start + int(limit)
	if end > len(result) {
		end = len(result)
	}
	page := result[start:end]
	var nextOffset *wrapperspb.StringValue
	if end < len(result) {
		nextOffset = wrapperspb.String(strconv.Itoa(end))
	}
	return mtproto.MakeTLStoriesStoryReactionsList(&mtproto.Stories_StoryReactionsList{
		Count:      int32(filteredCount),
		Reactions:  page,
		Chats:      []*mtproto.Chat{},
		Users:      []*mtproto.User{},
		NextOffset: nextOffset,
	}).To_Stories_StoryReactionsList(), nil
}

func (c *ApiFullCore) StoriesTogglePinnedToTop(in *mtproto.TLStoriesTogglePinnedToTop) (*mtproto.Bool, error) {
	if in == nil || len(in.GetId()) == 0 {
		return nil, mtproto.ErrStoryIdEmpty
	}
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in.GetPeer() != nil && !storyPeerOwned(uid, in.GetPeer()) {
		return nil, mtproto.ErrMethodNotImpl
	}
	_, err = mutateOwnStoriesE(uid, func(stories *userStoryStore) error {
		for _, id := range in.GetId() {
			if stories.Items[id] == nil {
				return mtproto.ErrStoryIdEmpty
			}
		}
		stories.PinnedTop = append([]int32(nil), in.GetId()...)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return mtproto.BoolTrue, nil
}

func (c *ApiFullCore) StoriesSearchPosts(in *mtproto.TLStoriesSearchPosts) (*mtproto.Stories_FoundStories, error) {
	if in == nil {
		return nil, storiesProviderUnavailable(c)
	}
	uid, stories, err := c.ownStoryStore(in.GetPeer(), nil)
	if err != nil {
		return nil, err
	}
	query := strings.TrimSpace(in.GetHashtag().GetValue())
	if query == "" {
		query = strings.TrimSpace(in.GetOffset())
	}
	if query == "" {
		return nil, mtproto.ErrSearchQueryEmpty
	}
	query = strings.TrimPrefix(strings.ToLower(query), "#")
	found := make([]*mtproto.FoundStory, 0)
	for _, id := range stories.Order {
		item := stories.Items[id]
		if item == nil || !strings.Contains(strings.ToLower(item.GetCaption().GetValue()), query) {
			continue
		}
		found = append(found, mtproto.MakeTLFoundStory(&mtproto.FoundStory{
			Peer: mtproto.MakeTLPeerUser(&mtproto.Peer{UserId: uid}).To_Peer(), Story: item,
		}).To_FoundStory())
	}
	limit := in.GetLimit()
	if limit <= 0 || limit > 100 {
		limit = 100
	}
	offset := 0
	if raw := strings.TrimSpace(in.GetOffset()); raw != "" {
		if parsed, parseErr := strconv.Atoi(raw); parseErr == nil && parsed >= 0 {
			offset = parsed
		}
	}
	if offset > len(found) {
		offset = len(found)
	}
	end := offset + int(limit)
	if end > len(found) {
		end = len(found)
	}
	page := found[offset:end]
	var next *wrapperspb.StringValue
	if end < len(found) {
		next = wrapperspb.String(strconv.Itoa(end))
	}
	return mtproto.MakeTLStoriesFoundStories(&mtproto.Stories_FoundStories{
		Count: int32(len(found)), Stories: page, NextOffset: next,
	}).To_Stories_FoundStories(), nil
}

func (c *ApiFullCore) StoriesCreateAlbum(in *mtproto.TLStoriesCreateAlbum) (*mtproto.StoryAlbum, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil || strings.TrimSpace(in.GetTitle()) == "" {
		return nil, mtproto.ErrInputRequestInvalid
	}
	if in.GetPeer() != nil && !storyPeerOwned(uid, in.GetPeer()) {
		return nil, mtproto.ErrMethodNotImpl
	}
	var album *storyAlbumRec
	_, err = mutateOwnStoriesE(uid, func(stories *userStoryStore) error {
		stories.NextAlbum++
		if stories.NextAlbum <= 0 {
			stories.NextAlbum = 1
		}
		album = &storyAlbumRec{AlbumId: stories.NextAlbum, Title: strings.TrimSpace(in.GetTitle()), Stories: append([]int32(nil), in.GetStories()...)}
		for _, id := range album.Stories {
			if stories.Items[id] == nil {
				return mtproto.ErrStoryIdEmpty
			}
		}
		stories.Albums[album.AlbumId] = album
		stories.AlbumOrder = append(stories.AlbumOrder, album.AlbumId)
		stories.bindAlbumStories(album.AlbumId, album.Stories, true)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return albumTL(album), nil
}

func (c *ApiFullCore) StoriesUpdateAlbum(in *mtproto.TLStoriesUpdateAlbum) (*mtproto.StoryAlbum, error) {
	if in == nil || in.GetAlbumId() <= 0 {
		return nil, mtproto.ErrInputRequestInvalid
	}
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in.GetPeer() != nil && !storyPeerOwned(uid, in.GetPeer()) {
		return nil, mtproto.ErrMethodNotImpl
	}
	var album *storyAlbumRec
	_, err = mutateOwnStoriesE(uid, func(stories *userStoryStore) error {
		album = stories.Albums[in.GetAlbumId()]
		if album == nil {
			return mtproto.ErrInputRequestInvalid
		}
		if title := in.GetTitle(); title != nil {
			if strings.TrimSpace(title.GetValue()) == "" {
				return mtproto.ErrInputRequestInvalid
			}
			album.Title = strings.TrimSpace(title.GetValue())
		}
		if len(in.GetDeleteStories()) > 0 {
			stories.bindAlbumStories(album.AlbumId, in.GetDeleteStories(), false)
			removed := make(map[int32]struct{}, len(in.GetDeleteStories()))
			for _, id := range in.GetDeleteStories() {
				removed[id] = struct{}{}
			}
			kept := album.Stories[:0]
			for _, id := range album.Stories {
				if _, ok := removed[id]; !ok {
					kept = append(kept, id)
				}
			}
			album.Stories = kept
		}
		if len(in.GetAddStories()) > 0 {
			for _, id := range in.GetAddStories() {
				if stories.Items[id] == nil {
					return mtproto.ErrStoryIdEmpty
				}
				found := false
				for _, existing := range album.Stories {
					if existing == id {
						found = true
						break
					}
				}
				if !found {
					album.Stories = append(album.Stories, id)
				}
			}
			stories.bindAlbumStories(album.AlbumId, in.GetAddStories(), true)
		}
		if len(in.GetOrder()) > 0 {
			seen := make(map[int32]struct{}, len(album.Stories))
			ordered := make([]int32, 0, len(in.GetOrder()))
			for _, id := range in.GetOrder() {
				for _, existing := range album.Stories {
					if id == existing {
						if _, ok := seen[id]; !ok {
							ordered = append(ordered, id)
							seen[id] = struct{}{}
						}
					}
				}
			}
			for _, id := range album.Stories {
				if _, ok := seen[id]; !ok {
					ordered = append(ordered, id)
				}
			}
			album.Stories = ordered
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return albumTL(album), nil
}

func (c *ApiFullCore) StoriesReorderAlbums(in *mtproto.TLStoriesReorderAlbums) (*mtproto.Bool, error) {
	if in == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in.GetPeer() != nil && !storyPeerOwned(uid, in.GetPeer()) {
		return nil, mtproto.ErrMethodNotImpl
	}
	_, err = mutateOwnStoriesE(uid, func(stories *userStoryStore) error {
		seen := make(map[int32]struct{}, len(in.GetOrder()))
		order := make([]int32, 0, len(stories.AlbumOrder))
		for _, id := range in.GetOrder() {
			if stories.Albums[id] == nil {
				return mtproto.ErrInputRequestInvalid
			}
			if _, ok := seen[id]; !ok {
				order = append(order, id)
				seen[id] = struct{}{}
			}
		}
		for _, id := range stories.AlbumOrder {
			if _, ok := seen[id]; !ok {
				order = append(order, id)
			}
		}
		stories.AlbumOrder = order
		return nil
	})
	if err != nil {
		return nil, err
	}
	return mtproto.BoolTrue, nil
}

func (c *ApiFullCore) StoriesDeleteAlbum(in *mtproto.TLStoriesDeleteAlbum) (*mtproto.Bool, error) {
	if in == nil || in.GetAlbumId() <= 0 {
		return nil, mtproto.ErrInputRequestInvalid
	}
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in.GetPeer() != nil && !storyPeerOwned(uid, in.GetPeer()) {
		return nil, mtproto.ErrMethodNotImpl
	}
	_, err = mutateOwnStoriesE(uid, func(stories *userStoryStore) error {
		album := stories.Albums[in.GetAlbumId()]
		if album == nil {
			return mtproto.ErrInputRequestInvalid
		}
		stories.bindAlbumStories(album.AlbumId, album.Stories, false)
		delete(stories.Albums, album.AlbumId)
		order := stories.AlbumOrder[:0]
		for _, id := range stories.AlbumOrder {
			if id != album.AlbumId {
				order = append(order, id)
			}
		}
		stories.AlbumOrder = order
		return nil
	})
	if err != nil {
		return nil, err
	}
	return mtproto.BoolTrue, nil
}

func (c *ApiFullCore) StoriesGetAlbums(in *mtproto.TLStoriesGetAlbums) (*mtproto.Stories_Albums, error) {
	if in == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in.GetPeer() != nil && !storyPeerOwned(uid, in.GetPeer()) {
		return nil, mtproto.ErrMethodNotImpl
	}
	stories, err := loadUserStories(uid)
	if err != nil {
		return nil, err
	}
	albums := make([]*mtproto.StoryAlbum, 0, len(stories.AlbumOrder))
	for _, id := range stories.AlbumOrder {
		if album := stories.Albums[id]; album != nil {
			albums = append(albums, albumTL(album))
		}
	}
	return mtproto.MakeTLStoriesAlbums(&mtproto.Stories_Albums{Hash: int64(len(albums)), Albums: albums}).To_Stories_Albums(), nil
}

func (c *ApiFullCore) StoriesGetAlbumStories(in *mtproto.TLStoriesGetAlbumStories) (*mtproto.Stories_Stories, error) {
	if in == nil || in.GetAlbumId() <= 0 {
		return nil, mtproto.ErrInputRequestInvalid
	}
	_, stories, err := c.ownStoryStore(in.GetPeer(), nil)
	if err != nil {
		return nil, err
	}
	album := stories.Albums[in.GetAlbumId()]
	if album == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	items := stories.list(album.Stories, false)
	return storiesBox(storyPage(items, in.GetOffset(), in.GetLimit())), nil
}

func (c *ApiFullCore) StoriesStartLive(in *mtproto.TLStoriesStartLive) (*mtproto.Updates, error) {
	if in == nil {
		return nil, storiesProviderUnavailable(c)
	}
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if !storyPeerOwned(uid, in.GetPeer()) {
		return nil, mtproto.ErrMethodNotImpl
	}
	// The APIFull DAO has no media/live-stream provider. Do not persist a
	// synthetic story that clients could not actually consume.
	return nil, mtproto.ErrMethodNotImpl
}

func (c *ApiFullCore) StoriesGetPeerMaxIDs535983C3(in *mtproto.TLStoriesGetPeerMaxIDs535983C3) (*mtproto.Vector_Int, error) {
	if in == nil {
		return nil, storiesProviderUnavailable(c)
	}
	uid, stories, err := c.ownStoryStore(nil, nil)
	if err != nil {
		return nil, err
	}
	result := make([]int32, 0, len(in.GetId()))
	for _, peer := range in.GetId() {
		if !storyPeerOwned(uid, peer) {
			return nil, mtproto.ErrMethodNotImpl
		}
		result = append(result, storyMaxID(stories))
	}
	return &mtproto.Vector_Int{Datas: result}, nil
}

func (c *ApiFullCore) StoriesCanSendStoryC7DFDFDD(in *mtproto.TLStoriesCanSendStoryC7DFDFDD) (*mtproto.Bool, error) {
	if in == nil {
		return nil, storiesProviderUnavailable(c)
	}
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if !storyPeerOwned(uid, in.GetPeer()) {
		return nil, mtproto.ErrMethodNotImpl
	}
	return mtproto.BoolTrue, nil
}

func (c *ApiFullCore) StoriesReport1923FA8C(in *mtproto.TLStoriesReport1923FA8C) (*mtproto.Bool, error) {
	if in == nil {
		return nil, storiesProviderUnavailable(c)
	}
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if !reportMessageIDs(in.GetId()) || !reportReasonValid(in.GetReason()) {
		return nil, mtproto.ErrInputRequestInvalid
	}
	target, err := reportPeerTarget(uid, in.GetPeer())
	if err != nil {
		return nil, err
	}
	if err = c.recordReport(uid, "stories.report", target, in); err != nil {
		return nil, err
	}
	return mtproto.BoolTrue, nil
}

func (c *ApiFullCore) UsersGetStoriesMaxIDs(in *mtproto.TLUsersGetStoriesMaxIDs) (*mtproto.Vector_Int, error) {
	if in == nil {
		return nil, storiesProviderUnavailable(c)
	}
	uid, stories, err := c.ownStoryStore(nil, nil)
	if err != nil {
		return nil, err
	}
	result := make([]int32, 0, len(in.GetId()))
	for _, user := range in.GetId() {
		if !storyUserOwned(uid, user) {
			return nil, mtproto.ErrMethodNotImpl
		}
		result = append(result, storyMaxID(stories))
	}
	return &mtproto.Vector_Int{Datas: result}, nil
}

func (c *ApiFullCore) ContactsToggleStoriesHidden(in *mtproto.TLContactsToggleStoriesHidden) (*mtproto.Bool, error) {
	if in == nil || in.GetId() == nil || in.GetHidden() == nil {
		if in == nil {
			return nil, storiesProviderUnavailable(c)
		}
		return nil, mtproto.ErrInputRequestInvalid
	}
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	userID := in.GetId().GetUserId()
	if userID <= 0 || userID == uid {
		return nil, mtproto.ErrInputRequestInvalid
	}
	_, err = mutateOwnStoriesE(uid, func(stories *userStoryStore) error {
		if in.GetHidden().GetPredicateName() == mtproto.Predicate_boolTrue {
			stories.HiddenUsers[userID] = true
		} else {
			delete(stories.HiddenUsers, userID)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return mtproto.BoolTrue, nil
}

func (c *ApiFullCore) StoriesCanSendStoryB100D45D(in *mtproto.TLStoriesCanSendStoryB100D45D) (*mtproto.Bool, error) {
	if in == nil {
		return nil, storiesProviderUnavailable(c)
	}
	if _, err := c.requireUserId(); err != nil {
		return nil, err
	}
	return mtproto.BoolTrue, nil
}

func (c *ApiFullCore) StoriesGetUserStories(in *mtproto.TLStoriesGetUserStories) (*mtproto.Stories_UserStories, error) {
	if in == nil {
		return nil, storiesProviderUnavailable(c)
	}
	uid, stories, err := c.ownStoryStore(nil, in.GetUserId())
	if err != nil {
		return nil, err
	}
	return mtproto.MakeTLStoriesUserStories(&mtproto.Stories_UserStories{
		Stories: mtproto.MakeTLUserStories(&mtproto.UserStories{UserId: uid, MaxReadId: wrapperspb.Int32(stories.ReadMax), Stories: stories.list(nil, false)}).To_UserStories(),
		Users:   []*mtproto.User{},
	}).To_Stories_UserStories(), nil
}

func (c *ApiFullCore) StoriesGetAllReadUserStories(in *mtproto.TLStoriesGetAllReadUserStories) (*mtproto.Updates, error) {
	if in == nil {
		return nil, storiesProviderUnavailable(c)
	}
	if _, err := c.requireUserId(); err != nil {
		return nil, err
	}
	return mtproto.MakeTLUpdates(&mtproto.Updates{Updates: []*mtproto.Update{}, Users: []*mtproto.User{}, Chats: []*mtproto.Chat{}, Date: int32(time.Now().Unix())}).To_Updates(), nil
}

func (c *ApiFullCore) StoriesReportC95BE06A(in *mtproto.TLStoriesReportC95BE06A) (*mtproto.Bool, error) {
	if in == nil {
		return nil, storiesProviderUnavailable(c)
	}
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if !storyUserOwned(uid, in.GetUserId()) || !reportMessageIDs(in.GetId()) {
		return nil, mtproto.ErrInputRequestInvalid
	}
	target := reportTarget{typ: "user", id: uid}
	if err = c.recordReport(uid, "stories.reportUser", target, in); err != nil {
		return nil, err
	}
	return mtproto.BoolTrue, nil
}
