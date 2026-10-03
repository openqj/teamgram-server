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
)

// RPCStoriesServer: Layer 229 methods previously returned ERR_ENTERPRISE_IS_BLOCKED.

// storiesProviderUnavailable keeps the RPC contract honest while APIFull has
// no authoritative story service. Authenticate first, then fail closed rather
// than returning locally synthesized StoryItems or empty success envelopes.
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

func loadUserStories(userID int64) (*userStoryStore, error) {
	raw, err := persist.Default.Get(storyKey(userID))
	if err != nil || raw == "" {
		if err != nil {
			return nil, err
		}
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

func saveUserStories(userID int64, s *userStoryStore) error {
	b, err := json.Marshal(s)
	if err != nil {
		return err
	}
	return persist.Default.Set(storyKey(userID), string(b))
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
	item.SentReaction = rx
	rec := storyReactionRec{UserId: userID, Date: now}
	if rx != nil {
		rec.Emoticon = rx.GetEmoticon()
		rec.Document = rx.GetDocumentId()
	}
	list := s.Reactions[id]
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
	s, err := loadUserStories(userID)
	if err != nil {
		return nil, err
	}
	fn(s)
	if err := saveUserStories(userID, s); err != nil {
		return nil, err
	}
	return s, nil
}

func (c *ApiFullCore) StoriesCanSendStory30EB63F0(in *mtproto.TLStoriesCanSendStory30EB63F0) (*mtproto.Stories_CanSendStoryCount, error) {
	_ = in
	return nil, storiesProviderUnavailable(c)
}

func (c *ApiFullCore) StoriesSendStory(in *mtproto.TLStoriesSendStory) (*mtproto.Updates, error) {
	_ = in
	return nil, storiesProviderUnavailable(c)
}

func (c *ApiFullCore) StoriesEditStory(in *mtproto.TLStoriesEditStory) (*mtproto.Updates, error) {
	_ = in
	return nil, storiesProviderUnavailable(c)
}

func (c *ApiFullCore) StoriesDeleteStories(in *mtproto.TLStoriesDeleteStories) (*mtproto.Vector_Int, error) {
	_ = in
	return nil, storiesProviderUnavailable(c)
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
	stories, err := loadUserStories(uid)
	if err != nil {
		return nil, err
	}
	pinned := in.GetPinned().GetPredicateName() == mtproto.Predicate_boolTrue
	result := make([]int32, 0, len(in.GetId()))
	for _, id := range in.GetId() {
		item := stories.Items[id]
		if item == nil {
			return nil, mtproto.ErrStoryIdEmpty
		}
		item.Pinned = pinned
		result = append(result, id)
	}
	if err = saveUserStories(uid, stories); err != nil {
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
	_ = in
	return nil, storiesProviderUnavailable(c)
}

func (c *ApiFullCore) StoriesGetPinnedStories(in *mtproto.TLStoriesGetPinnedStories) (*mtproto.Stories_Stories, error) {
	if in == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	_, stories, err := c.ownStoryStore(in.GetPeer(), in.GetUserId())
	if err != nil {
		return nil, err
	}
	return storiesBox(storyPage(stories.list(nil, true), in.GetOffsetId(), in.GetLimit())), nil
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
	return storiesBox(storyPage(items, in.GetOffsetId(), in.GetLimit())), nil
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
	stories, err := loadUserStories(uid)
	if err != nil {
		return nil, err
	}
	stories.AllHidden = in.GetHidden().GetPredicateName() == mtproto.Predicate_boolTrue
	if err = saveUserStories(uid, stories); err != nil {
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
	if in.GetMaxId() > stories.ReadMax {
		stories.ReadMax = in.GetMaxId()
	}
	ids := make([]int32, 0)
	for _, id := range stories.Order {
		if id <= in.GetMaxId() {
			ids = append(ids, id)
		}
	}
	if err = saveUserStories(uid, stories); err != nil {
		return nil, err
	}
	return &mtproto.Vector_Int{Datas: ids}, nil
}

func (c *ApiFullCore) StoriesIncrementStoryViews(in *mtproto.TLStoriesIncrementStoryViews) (*mtproto.Bool, error) {
	if in == nil || len(in.GetId()) == 0 {
		return nil, mtproto.ErrStoryIdEmpty
	}
	uid, stories, err := c.ownStoryStore(in.GetPeer(), in.GetUserId())
	if err != nil {
		return nil, err
	}
	now := int32(time.Now().Unix())
	for _, id := range in.GetId() {
		if stories.Items[id] == nil {
			return nil, mtproto.ErrStoryIdEmpty
		}
		stories.touchViews(id, uid, now)
	}
	if err = saveUserStories(uid, stories); err != nil {
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
	reactions := stories.Reactions[in.GetId()]
	return mtproto.MakeTLStoriesStoryViewsList(&mtproto.Stories_StoryViewsList{
		Count:          int32(len(stories.Viewers[in.GetId()])),
		ViewsCount:     stories.Items[in.GetId()].GetViews().GetViewsCount(),
		ReactionsCount: int32(len(reactions)),
		Views:          views,
		Chats:          []*mtproto.Chat{},
		Users:          []*mtproto.User{},
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
		copy := *v
		copy.RecentViewers = append([]int64(nil), v.GetRecentViewers()...)
		if len(copy.RecentViewers) == 0 {
			for _, viewer := range stories.Viewers[id] {
				copy.RecentViewers = append(copy.RecentViewers, viewer.UserId)
			}
		}
		views = append(views, &copy)
	}
	return mtproto.MakeTLStoriesStoryViews(&mtproto.Stories_StoryViews{
		Views: views,
		Users: []*mtproto.User{},
	}).To_Stories_StoryViews(), nil
}

func (c *ApiFullCore) StoriesExportStoryLink(in *mtproto.TLStoriesExportStoryLink) (*mtproto.ExportedStoryLink, error) {
	_ = in
	return nil, storiesProviderUnavailable(c)
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
	_ = in
	return nil, storiesProviderUnavailable(c)
}

func (c *ApiFullCore) StoriesSendReaction(in *mtproto.TLStoriesSendReaction) (*mtproto.Updates, error) {
	_ = in
	return nil, storiesProviderUnavailable(c)
}

func (c *ApiFullCore) StoriesGetPeerStories(in *mtproto.TLStoriesGetPeerStories) (*mtproto.Stories_PeerStories, error) {
	_ = in
	return nil, storiesProviderUnavailable(c)
}

func (c *ApiFullCore) StoriesGetAllReadPeerStories(in *mtproto.TLStoriesGetAllReadPeerStories) (*mtproto.Updates, error) {
	_ = in
	return nil, storiesProviderUnavailable(c)
}

func (c *ApiFullCore) StoriesGetPeerMaxIDs78499170(in *mtproto.TLStoriesGetPeerMaxIDs78499170) (*mtproto.Vector_RecentStory, error) {
	_ = in
	return nil, storiesProviderUnavailable(c)
}

func (c *ApiFullCore) StoriesGetChatsToSend(in *mtproto.TLStoriesGetChatsToSend) (*mtproto.Messages_Chats, error) {
	_ = in
	return nil, storiesProviderUnavailable(c)
}

func (c *ApiFullCore) StoriesTogglePeerStoriesHidden(in *mtproto.TLStoriesTogglePeerStoriesHidden) (*mtproto.Bool, error) {
	if in == nil || in.GetPeer() == nil || in.GetHidden() == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	stories, err := loadUserStories(uid)
	if err != nil {
		return nil, err
	}
	key := storyPeerKey(in.GetPeer())
	if key == "self" {
		return nil, mtproto.ErrInputRequestInvalid
	}
	if in.GetHidden().GetPredicateName() == mtproto.Predicate_boolTrue {
		stories.HiddenPeers[key] = true
	} else {
		delete(stories.HiddenPeers, key)
	}
	if err = saveUserStories(uid, stories); err != nil {
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
		result = append(result, mtproto.MakeTLStoryReaction(&mtproto.StoryReaction{
			PeerId:   mtproto.MakeTLPeerUser(&mtproto.Peer{UserId: reaction.UserId}).To_Peer(),
			Date:     reaction.Date,
			Reaction: reactionFromRec(reaction),
		}).To_StoryReaction())
	}
	limit := in.GetLimit()
	if limit <= 0 || limit > 100 {
		limit = 100
	}
	if int(limit) < len(result) {
		result = result[:limit]
	}
	return mtproto.MakeTLStoriesStoryReactionsList(&mtproto.Stories_StoryReactionsList{
		Count:     int32(len(reactions)),
		Reactions: result,
		Chats:     []*mtproto.Chat{},
		Users:     []*mtproto.User{},
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
	stories, err := loadUserStories(uid)
	if err != nil {
		return nil, err
	}
	if in.GetPeer() != nil && !storyPeerOwned(uid, in.GetPeer()) {
		return nil, mtproto.ErrMethodNotImpl
	}
	for _, id := range in.GetId() {
		if stories.Items[id] == nil {
			return nil, mtproto.ErrStoryIdEmpty
		}
	}
	stories.PinnedTop = append([]int32(nil), in.GetId()...)
	if err = saveUserStories(uid, stories); err != nil {
		return nil, err
	}
	return mtproto.BoolTrue, nil
}

func (c *ApiFullCore) StoriesSearchPosts(in *mtproto.TLStoriesSearchPosts) (*mtproto.Stories_FoundStories, error) {
	_ = in
	return nil, storiesProviderUnavailable(c)
}

func (c *ApiFullCore) StoriesCreateAlbum(in *mtproto.TLStoriesCreateAlbum) (*mtproto.StoryAlbum, error) {
	if in == nil || strings.TrimSpace(in.GetTitle()) == "" {
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
	stories.NextAlbum++
	if stories.NextAlbum <= 0 {
		stories.NextAlbum = 1
	}
	album := &storyAlbumRec{AlbumId: stories.NextAlbum, Title: strings.TrimSpace(in.GetTitle()), Stories: append([]int32(nil), in.GetStories()...)}
	for _, id := range album.Stories {
		if stories.Items[id] == nil {
			return nil, mtproto.ErrStoryIdEmpty
		}
	}
	stories.Albums[album.AlbumId] = album
	stories.AlbumOrder = append(stories.AlbumOrder, album.AlbumId)
	stories.bindAlbumStories(album.AlbumId, album.Stories, true)
	if err = saveUserStories(uid, stories); err != nil {
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
	stories, err := loadUserStories(uid)
	if err != nil {
		return nil, err
	}
	album := stories.Albums[in.GetAlbumId()]
	if album == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	if title := in.GetTitle(); title != nil {
		if strings.TrimSpace(title.GetValue()) == "" {
			return nil, mtproto.ErrInputRequestInvalid
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
				return nil, mtproto.ErrStoryIdEmpty
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
	if err = saveUserStories(uid, stories); err != nil {
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
	stories, err := loadUserStories(uid)
	if err != nil {
		return nil, err
	}
	seen := make(map[int32]struct{}, len(in.GetOrder()))
	order := make([]int32, 0, len(stories.AlbumOrder))
	for _, id := range in.GetOrder() {
		if stories.Albums[id] == nil {
			return nil, mtproto.ErrInputRequestInvalid
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
	if err = saveUserStories(uid, stories); err != nil {
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
	stories, err := loadUserStories(uid)
	if err != nil {
		return nil, err
	}
	album := stories.Albums[in.GetAlbumId()]
	if album == nil {
		return nil, mtproto.ErrInputRequestInvalid
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
	if err = saveUserStories(uid, stories); err != nil {
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
	_ = in
	return nil, storiesProviderUnavailable(c)
}

func (c *ApiFullCore) StoriesGetPeerMaxIDs535983C3(in *mtproto.TLStoriesGetPeerMaxIDs535983C3) (*mtproto.Vector_Int, error) {
	_ = in
	return nil, storiesProviderUnavailable(c)
}

func (c *ApiFullCore) StoriesCanSendStoryC7DFDFDD(in *mtproto.TLStoriesCanSendStoryC7DFDFDD) (*mtproto.Bool, error) {
	_ = in
	return nil, storiesProviderUnavailable(c)
}

func (c *ApiFullCore) StoriesReport1923FA8C(in *mtproto.TLStoriesReport1923FA8C) (*mtproto.Bool, error) {
	if _, err := c.requireUserId(); err != nil {
		return nil, err
	}
	return nil, mtproto.ErrMethodNotImpl
}

func (c *ApiFullCore) UsersGetStoriesMaxIDs(in *mtproto.TLUsersGetStoriesMaxIDs) (*mtproto.Vector_Int, error) {
	_ = in
	return nil, storiesProviderUnavailable(c)
}

func (c *ApiFullCore) ContactsToggleStoriesHidden(in *mtproto.TLContactsToggleStoriesHidden) (*mtproto.Bool, error) {
	_ = in
	return nil, storiesProviderUnavailable(c)
}

func (c *ApiFullCore) StoriesCanSendStoryB100D45D(in *mtproto.TLStoriesCanSendStoryB100D45D) (*mtproto.Bool, error) {
	_ = in
	return nil, storiesProviderUnavailable(c)
}

func (c *ApiFullCore) StoriesGetUserStories(in *mtproto.TLStoriesGetUserStories) (*mtproto.Stories_UserStories, error) {
	_ = in
	return nil, storiesProviderUnavailable(c)
}

func (c *ApiFullCore) StoriesGetAllReadUserStories(in *mtproto.TLStoriesGetAllReadUserStories) (*mtproto.Updates, error) {
	_ = in
	return nil, storiesProviderUnavailable(c)
}

func (c *ApiFullCore) StoriesReportC95BE06A(in *mtproto.TLStoriesReportC95BE06A) (*mtproto.Bool, error) {
	if _, err := c.requireUserId(); err != nil {
		return nil, err
	}
	return nil, mtproto.ErrMethodNotImpl
}
