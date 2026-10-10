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
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/persist"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// RPCStickersServer: Layer 229 methods previously returned ERR_ENTERPRISE_IS_BLOCKED.

// stickersProviderUnavailable keeps sticker RPCs fail-closed until APIFull is
// connected to an authoritative sticker catalog/media provider.
func stickersProviderUnavailable(c *ApiFullCore) error {
	if _, err := c.requireUserId(); err != nil {
		return err
	}
	return mtproto.ErrMethodNotImpl
}

const stickerKeyPrefix = "stk:"

// Built-in sticker sets are available to every account even when this
// installation has no sticker catalog. Their IDs only need to be stable and
// positive so the client can cache the empty set by ID.
const (
	builtinAnimatedEmojiSetID        int64 = 9000000000000001
	builtinAnimatedEmojiEffectsSetID int64 = 9000000000000002
	builtinPremiumGiftsSetID         int64 = 9000000000000003
	builtinGenericEmojiEffectsSetID  int64 = 9000000000000004
	builtinDefaultStatusEmojiSetID   int64 = 9000000000000005
	builtinDefaultTopicIconsSetID    int64 = 9000000000000006
	builtinChannelDefaultStatusSetID int64 = 9000000000000007
	builtinTonGiftsSetID             int64 = 9000000000000008
	builtinFestiveFontEmojiSetID     int64 = 9000000000000009
	builtinRestrictedEmojiSetID      int64 = 7173162320003080
)

func saveStickerJSON(userID int64, op string, in any) error {
	raw, err := json.Marshal(in)
	if err != nil {
		return err
	}
	return persist.Default.Set(fmt.Sprintf("%s%d:%s", stickerKeyPrefix, userID, op), string(raw))
}

func emptyStickerSet() *mtproto.Messages_StickerSet {
	return mtproto.MakeTLMessagesStickerSetNotModified(nil).To_Messages_StickerSet()
}

func stickerNameKey(userID int64) string {
	return fmt.Sprintf("%s%d:name", stickerKeyPrefix, userID)
}

func installedStickerKey(userID int64) string {
	return fmt.Sprintf("%s%d:installed", stickerKeyPrefix, userID)
}

func stickerSetToken(in *mtproto.TLMessagesInstallStickerSet) string {
	if in == nil || in.GetStickerset() == nil {
		return ""
	}
	return inputStickerSetToken(in.GetStickerset())
}

func inputStickerSetToken(set *mtproto.InputStickerSet) string {
	if set == nil {
		return ""
	}
	switch set.GetPredicateName() {
	case mtproto.Predicate_inputStickerSetShortName:
		return set.GetShortName()
	case mtproto.Predicate_inputStickerSetID:
		if id := set.GetId(); id > 0 {
			return fmt.Sprintf("%d", id)
		}
	}
	return ""
}

func loadInstalledStickerNames(userID int64) ([]string, error) {
	raw, err := persist.Default.Get(installedStickerKey(userID))
	if err != nil || raw == "" {
		return nil, err
	}
	var names []string
	if err = json.Unmarshal([]byte(raw), &names); err != nil {
		return nil, err
	}
	return names, nil
}

func appendInstalledSticker(userID int64, token string) error {
	if token == "" {
		return nil
	}
	names, err := loadInstalledStickerNames(userID)
	if err != nil {
		return err
	}
	for _, name := range names {
		if name == token {
			return nil
		}
	}
	raw, err := json.Marshal(append(names, token))
	if err != nil {
		return err
	}
	return persist.Default.Set(installedStickerKey(userID), string(raw))
}

func removeInstalledSticker(userID int64, token string) error {
	return removeInstalledStickers(userID, []string{token})
}

func removeInstalledStickers(userID int64, tokens []string) error {
	remove := make(map[string]struct{}, len(tokens))
	for _, token := range tokens {
		if token != "" {
			remove[token] = struct{}{}
		}
	}
	if len(remove) == 0 {
		return nil
	}
	names, err := loadInstalledStickerNames(userID)
	if err != nil {
		return err
	}
	kept := make([]string, 0, len(names))
	for _, name := range names {
		if _, shouldRemove := remove[name]; !shouldRemove {
			kept = append(kept, name)
		}
	}
	if len(kept) == len(names) {
		return nil
	}
	raw, err := json.Marshal(kept)
	if err != nil {
		return err
	}
	return persist.Default.Set(installedStickerKey(userID), string(raw))
}

func namedStickerSet(short string) *mtproto.Messages_StickerSet {
	set := mtproto.MakeTLStickerSet(&mtproto.StickerSet{
		Id:        1,
		Title:     short,
		ShortName: short,
		Creator:   true,
	}).To_StickerSet()
	return mtproto.MakeTLMessagesStickerSet(&mtproto.Messages_StickerSet{
		Set:       set,
		Packs:     []*mtproto.StickerPack{},
		Keywords:  []*mtproto.StickerKeyword{},
		Documents: []*mtproto.Document{},
	}).To_Messages_StickerSet()
}

type storedStickerRef struct {
	id      int64
	short   string
	title   string
	creator bool
}

func stickerTokenRef(token string) (storedStickerRef, bool) {
	if token == "" {
		return storedStickerRef{}, false
	}
	if id, err := strconv.ParseInt(token, 10, 64); err == nil && id != 0 && strconv.FormatInt(id, 10) == token {
		return storedStickerRef{id: id, title: token}, true
	}
	return storedStickerRef{id: 1, short: token, title: token}, true
}

func loadStoredStickerRefs(userID int64) ([]storedStickerRef, error) {
	var refs []storedStickerRef
	short, err := persist.Default.Get(stickerNameKey(userID))
	if err != nil {
		return nil, err
	}
	if short != "" {
		refs = append(refs, storedStickerRef{id: 1, short: short, title: short, creator: true})
	}
	names, err := loadInstalledStickerNames(userID)
	if err != nil {
		return nil, err
	}
	for _, token := range names {
		if ref, ok := stickerTokenRef(token); ok {
			refs = append(refs, ref)
		}
	}
	raw, err := persist.Default.Get(fmt.Sprintf("%s%d:install", stickerKeyPrefix, userID))
	if err != nil {
		return nil, err
	}
	if raw != "" {
		var saved mtproto.TLMessagesInstallStickerSet
		if json.Unmarshal([]byte(raw), &saved) == nil && saved.GetStickerset() != nil {
			set := saved.GetStickerset()
			title := set.GetShortName()
			if title == "" && set.GetId() != 0 {
				title = strconv.FormatInt(set.GetId(), 10)
			}
			if set.GetId() != 0 || set.GetShortName() != "" {
				refs = append(refs, storedStickerRef{
					id:    set.GetId(),
					short: set.GetShortName(),
					title: title,
				})
			}
		}
	}
	raw, err = persist.Default.Get(fmt.Sprintf("%s%d:create", stickerKeyPrefix, userID))
	if err != nil {
		return nil, err
	}
	if raw != "" {
		var saved mtproto.TLStickersCreateStickerSet
		if json.Unmarshal([]byte(raw), &saved) == nil && saved.GetShortName() != "" {
			title := saved.GetTitle()
			if title == "" {
				title = saved.GetShortName()
			}
			refs = append(refs, storedStickerRef{
				id:      1,
				short:   saved.GetShortName(),
				title:   title,
				creator: true,
			})
		}
	}
	return refs, nil
}

func pickStoredSticker(refs []storedStickerRef, reqID int64, reqShort string) *storedStickerRef {
	if reqShort != "" {
		var loose *storedStickerRef
		for i := range refs {
			if refs[i].short != reqShort {
				continue
			}
			if refs[i].id != 0 && refs[i].id != 1 {
				return &refs[i]
			}
			if loose == nil {
				loose = &refs[i]
			}
		}
		if loose != nil {
			return loose
		}
	}
	if reqID == 0 {
		return nil
	}
	var loose *storedStickerRef
	for i := range refs {
		if refs[i].id != reqID {
			continue
		}
		if refs[i].short == "" {
			return &refs[i]
		}
		if loose == nil {
			loose = &refs[i]
		}
	}
	return loose
}

func stickerRefHash(ref storedStickerRef) int32 {
	h := int64(1)
	h = h*31 + ref.id
	for _, r := range ref.short {
		h = h*31 + int64(r)
	}
	if h < 0 {
		h = -h
	}
	return int32(h & 0x7fffffff)
}

func messagesStickerSetFromRef(ref storedStickerRef, reqHash int32) *mtproto.Messages_StickerSet {
	if reqHash != 0 && reqHash == stickerRefHash(ref) {
		return mtproto.MakeTLMessagesStickerSetNotModified(nil).To_Messages_StickerSet()
	}
	title := ref.title
	if title == "" {
		title = ref.short
	}
	if title == "" {
		title = strconv.FormatInt(ref.id, 10)
	}
	return mtproto.MakeTLMessagesStickerSet(&mtproto.Messages_StickerSet{
		Set: mtproto.MakeTLStickerSet(&mtproto.StickerSet{
			Id:        ref.id,
			Title:     title,
			ShortName: ref.short,
			Creator:   ref.creator,
		}).To_StickerSet(),
		Packs:     []*mtproto.StickerPack{},
		Keywords:  []*mtproto.StickerKeyword{},
		Documents: []*mtproto.Document{},
	}).To_Messages_StickerSet()
}

func builtinStickerRef(set *mtproto.InputStickerSet) (storedStickerRef, bool) {
	if set == nil {
		return storedStickerRef{}, false
	}
	var ref storedStickerRef
	switch set.GetPredicateName() {
	case mtproto.Predicate_inputStickerSetAnimatedEmoji:
		ref = storedStickerRef{id: builtinAnimatedEmojiSetID, title: "Animated Emoji"}
	case mtproto.Predicate_inputStickerSetAnimatedEmojiAnimations:
		ref = storedStickerRef{id: builtinAnimatedEmojiEffectsSetID, title: "Animated Emoji Effects"}
	case mtproto.Predicate_inputStickerSetPremiumGifts:
		ref = storedStickerRef{id: builtinPremiumGiftsSetID, title: "Premium Gifts"}
	case mtproto.Predicate_inputStickerSetEmojiGenericAnimations:
		ref = storedStickerRef{id: builtinGenericEmojiEffectsSetID, title: "Generic Emoji Effects"}
	case mtproto.Predicate_inputStickerSetEmojiDefaultStatuses:
		ref = storedStickerRef{id: builtinDefaultStatusEmojiSetID, title: "Default Status Emojis"}
	case mtproto.Predicate_inputStickerSetEmojiDefaultTopicIcons:
		ref = storedStickerRef{id: builtinDefaultTopicIconsSetID, title: "Default Topic Icons"}
	case mtproto.Predicate_inputStickerSetEmojiChannelDefaultStatuses:
		ref = storedStickerRef{id: builtinChannelDefaultStatusSetID, title: "Channel Status Emojis"}
	case mtproto.Predicate_inputStickerSetTonGifts:
		ref = storedStickerRef{id: builtinTonGiftsSetID, title: "TON Gifts"}
	default:
		return storedStickerRef{}, false
	}
	return ref, true
}

func namedBuiltinStickerRef(shortName string) (storedStickerRef, bool) {
	switch shortName {
	case "FestiveFontEmoji":
		return storedStickerRef{id: builtinFestiveFontEmojiSetID, short: shortName, title: shortName}, true
	case "RestrictedEmoji":
		return storedStickerRef{id: builtinRestrictedEmojiSetID, short: shortName, title: shortName}, true
	default:
		return storedStickerRef{}, false
	}
}

func idListHash(ids []int64) int64 {
	h := int64(1)
	for _, id := range ids {
		h = h*31 + id
	}
	if h < 0 {
		h = -h
	}
	return h
}

func featuredStickerIDs(userID int64) ([]int64, error) {
	raw, err := persist.Default.Get(fmt.Sprintf("%s%d:readFeatured", stickerKeyPrefix, userID))
	if err != nil || raw == "" {
		return nil, err
	}
	var saved mtproto.TLMessagesReadFeaturedStickers
	if err = json.Unmarshal([]byte(raw), &saved); err != nil {
		return nil, err
	}
	return saved.GetId(), nil
}

func (c *ApiFullCore) loadFeaturedStickers(hash int64) (*mtproto.Messages_FeaturedStickers, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	ids, err := featuredStickerIDs(uid)
	if err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return mtproto.MakeTLMessagesFeaturedStickers(&mtproto.Messages_FeaturedStickers{
			Count:  0,
			Hash:   0,
			Sets:   []*mtproto.StickerSetCovered{},
			Unread: []int64{},
		}).To_Messages_FeaturedStickers(), nil
	}
	h := idListHash(ids)
	if hash != 0 && hash == h {
		return mtproto.MakeTLMessagesFeaturedStickersNotModified(&mtproto.Messages_FeaturedStickers{
			Count: int32(len(ids)),
			Hash:  h,
		}).To_Messages_FeaturedStickers(), nil
	}
	sets := make([]*mtproto.StickerSetCovered, 0, len(ids))
	for _, id := range ids {
		sets = append(sets, mtproto.MakeTLStickerSetCovered(&mtproto.StickerSetCovered{
			Cover: mtproto.MakeTLDocumentEmpty(&mtproto.Document{}).To_Document(),
			Set: mtproto.MakeTLStickerSet(&mtproto.StickerSet{
				Id:    id,
				Title: strconv.FormatInt(id, 10),
			}).To_StickerSet(),
		}).To_StickerSetCovered())
	}
	return mtproto.MakeTLMessagesFeaturedStickers(&mtproto.Messages_FeaturedStickers{
		Count:  int32(len(ids)),
		Hash:   h,
		Sets:   sets,
		Unread: append([]int64(nil), ids...),
	}).To_Messages_FeaturedStickers(), nil
}

func (c *ApiFullCore) MessagesGetStickers(in *mtproto.TLMessagesGetStickers) (*mtproto.Messages_Stickers, error) {
	if in == nil {
		return nil, stickersProviderUnavailable(c)
	}
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	docs, err := persist.SearchStickerDocuments(stickerRequestContext(c), uid, "", in.GetEmoticon(), 0, 100)
	if err != nil {
		return nil, stickerProviderError(c, err)
	}
	ids := make([]int64, 0, len(docs))
	out := make([]*mtproto.Document, 0, len(docs))
	for _, d := range docs {
		ids = append(ids, d.ID)
		out = append(out, stickerDocumentFromRecord(d))
	}
	hash := idListHash(ids)
	if in.GetHash() != 0 && in.GetHash() == hash {
		return mtproto.MakeTLMessagesStickersNotModified(nil).To_Messages_Stickers(), nil
	}
	return mtproto.MakeTLMessagesStickers(&mtproto.Messages_Stickers{Hash: hash, Stickers: out}).To_Messages_Stickers(), nil
}

func (c *ApiFullCore) MessagesGetAllStickers(in *mtproto.TLMessagesGetAllStickers) (*mtproto.Messages_AllStickers, error) {
	if in == nil {
		return nil, stickersProviderUnavailable(c)
	}
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	sets, err := persist.ListStickerSets(stickerRequestContext(c), uid, false, false, false, 100)
	if err != nil {
		return nil, stickerProviderError(c, err)
	}
	items := make([]*mtproto.StickerSet, 0, len(sets))
	ids := make([]int64, 0, len(sets))
	for i := range sets {
		set := sets[i]
		ids = append(ids, set.ID)
		items = append(items, mtproto.MakeTLStickerSet(&mtproto.StickerSet{Id: set.ID, AccessHash: set.AccessHash, Title: set.Title, ShortName: set.ShortName, Masks: set.Masks, Emojis: set.Emojis, TextColor: set.TextColor, Creator: set.Creator, Count: int32(len(set.Documents)), Hash: int32(stickerSetHash(&set)), Animated: set.Animated, Videos: set.Videos}).To_StickerSet())
	}
	hash := idListHash(ids)
	if in.GetHash() != 0 && in.GetHash() == hash {
		return mtproto.MakeTLMessagesAllStickersNotModified(nil).To_Messages_AllStickers(), nil
	}
	return mtproto.MakeTLMessagesAllStickers(&mtproto.Messages_AllStickers{Hash: hash, Sets: items}).To_Messages_AllStickers(), nil
}

func (c *ApiFullCore) MessagesGetStickerSet(in *mtproto.TLMessagesGetStickerSet) (*mtproto.Messages_StickerSet, error) {
	if in == nil {
		return nil, stickersProviderUnavailable(c)
	}
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	setID, short := stickerSetTokenID(in.GetStickerset())
	if ref, ok := builtinStickerRef(in.GetStickerset()); ok {
		return messagesStickerSetFromRef(ref, in.GetHash()), nil
	}
	if setID == 0 && short == "" {
		return nil, mtproto.ErrStickersetInvalid
	}
	set, err := persist.GetStickerSet(stickerRequestContext(c), uid, setID, short)
	if err != nil {
		return nil, stickerProviderError(c, err)
	}
	if set == nil {
		return nil, mtproto.ErrStickersetInvalid
	}
	if err := validateStickerSetAccess(in.GetStickerset(), set); err != nil {
		return nil, err
	}
	return stickerSetFromRecord(set, in.GetHash()), nil
}

func (c *ApiFullCore) MessagesInstallStickerSet(in *mtproto.TLMessagesInstallStickerSet) (*mtproto.Messages_StickerSetInstallResult, error) {
	if in == nil {
		return nil, stickersProviderUnavailable(c)
	}
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	setID, short := stickerSetTokenID(in.GetStickerset())
	set, err := persist.GetStickerSet(stickerRequestContext(c), uid, setID, short)
	if err != nil {
		return nil, stickerProviderError(c, err)
	}
	if set == nil {
		return nil, mtproto.ErrStickersetInvalid
	}
	if err := validateStickerSetAccess(in.GetStickerset(), set); err != nil {
		return nil, err
	}
	archived := in.GetArchived() != nil && in.GetArchived().GetPredicateName() == mtproto.Predicate_boolTrue
	if err = persist.SetInstalled(stickerRequestContext(c), uid, set.ID, true, archived); err != nil {
		return nil, stickerProviderError(c, err)
	}
	set.Installed, set.Archived = true, archived
	return mtproto.MakeTLMessagesStickerSetInstallResultSuccess(&mtproto.Messages_StickerSetInstallResult{Sets: []*mtproto.StickerSetCovered{stickerCovered(*set)}}).To_Messages_StickerSetInstallResult(), nil
}

func (c *ApiFullCore) MessagesUninstallStickerSet(in *mtproto.TLMessagesUninstallStickerSet) (*mtproto.Bool, error) {
	if in == nil {
		return nil, stickersProviderUnavailable(c)
	}
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	setID, short := stickerSetTokenID(in.GetStickerset())
	set, err := persist.GetStickerSet(stickerRequestContext(c), uid, setID, short)
	if err != nil {
		return nil, stickerProviderError(c, err)
	}
	if set == nil {
		return nil, mtproto.ErrStickersetInvalid
	}
	if err := validateStickerSetAccess(in.GetStickerset(), set); err != nil {
		return nil, err
	}
	if err = persist.SetInstalled(stickerRequestContext(c), uid, set.ID, false, false); err != nil {
		return nil, stickerProviderError(c, err)
	}
	return mtproto.BoolTrue, nil
}

func (c *ApiFullCore) MessagesReorderStickerSets(in *mtproto.TLMessagesReorderStickerSets) (*mtproto.Bool, error) {
	if in == nil {
		return nil, stickersProviderUnavailable(c)
	}
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if err = persist.ReorderStickerSets(stickerRequestContext(c), uid, in.GetOrder()); err != nil {
		return nil, stickerProviderError(c, err)
	}
	return mtproto.BoolTrue, nil
}

func (c *ApiFullCore) MessagesGetFeaturedStickers(in *mtproto.TLMessagesGetFeaturedStickers) (*mtproto.Messages_FeaturedStickers, error) {
	if in == nil {
		return nil, stickersProviderUnavailable(c)
	}
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	sets, err := persist.ListStickerSets(stickerRequestContext(c), uid, false, false, true, 100)
	if err != nil {
		return nil, stickerProviderError(c, err)
	}
	ids := make([]int64, 0, len(sets))
	covers := make([]*mtproto.StickerSetCovered, 0, len(sets))
	unread := make([]int64, 0, len(sets))
	for _, set := range sets {
		ids = append(ids, set.ID)
		covers = append(covers, stickerCovered(set))
		if set.Unread {
			unread = append(unread, set.ID)
		}
	}
	hash := idListHash(ids)
	if in.GetHash() != 0 && in.GetHash() == hash {
		return mtproto.MakeTLMessagesFeaturedStickersNotModified(&mtproto.Messages_FeaturedStickers{Count: int32(len(sets)), Hash: hash}).To_Messages_FeaturedStickers(), nil
	}
	return mtproto.MakeTLMessagesFeaturedStickers(&mtproto.Messages_FeaturedStickers{Count: int32(len(sets)), Hash: hash, Sets: covers, Unread: unread}).To_Messages_FeaturedStickers(), nil
}

func (c *ApiFullCore) MessagesReadFeaturedStickers(in *mtproto.TLMessagesReadFeaturedStickers) (*mtproto.Bool, error) {
	if in == nil {
		return nil, stickersProviderUnavailable(c)
	}
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if err = persist.SetFeaturedRead(stickerRequestContext(c), uid, in.GetId()); err != nil {
		return nil, stickerProviderError(c, err)
	}
	return mtproto.BoolTrue, nil
}

func (c *ApiFullCore) MessagesGetRecentStickers(in *mtproto.TLMessagesGetRecentStickers) (*mtproto.Messages_RecentStickers, error) {
	if in == nil {
		return nil, stickersProviderUnavailable(c)
	}
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	docs, err := persist.ListRecentStickers(stickerRequestContext(c), uid, in.GetAttached(), 100)
	if err != nil {
		return nil, stickerProviderError(c, err)
	}
	items := make([]*mtproto.Document, 0, len(docs))
	ids := make([]int64, 0, len(docs))
	dates := make([]int32, 0, len(docs))
	for _, d := range docs {
		items = append(items, stickerDocumentFromRecord(d))
		ids = append(ids, d.ID)
		dates = append(dates, d.Date)
	}
	hash := idListHash(ids)
	if in.GetHash() != 0 && in.GetHash() == hash {
		return mtproto.MakeTLMessagesRecentStickersNotModified(&mtproto.Messages_RecentStickers{Hash: hash}).To_Messages_RecentStickers(), nil
	}
	return mtproto.MakeTLMessagesRecentStickers(&mtproto.Messages_RecentStickers{Hash: hash, Packs: []*mtproto.StickerPack{}, Stickers: items, Dates: dates}).To_Messages_RecentStickers(), nil
}

func (c *ApiFullCore) MessagesSaveRecentSticker(in *mtproto.TLMessagesSaveRecentSticker) (*mtproto.Bool, error) {
	if in == nil {
		return nil, stickersProviderUnavailable(c)
	}
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in.GetId() == nil || inputDocumentID(in.GetId()) == 0 || in.GetId().GetAccessHash() == 0 {
		return nil, mtproto.ErrStickerIdInvalid
	}
	unsave := in.GetUnsave() != nil && in.GetUnsave().GetPredicateName() == mtproto.Predicate_boolTrue
	if err = persist.SaveRecentStickerWithAccessHash(stickerRequestContext(c), uid, inputDocumentID(in.GetId()), in.GetId().GetAccessHash(), in.GetAttached(), unsave); err != nil {
		return nil, stickerProviderError(c, err)
	}
	return mtproto.BoolTrue, nil
}

func (c *ApiFullCore) MessagesClearRecentStickers(in *mtproto.TLMessagesClearRecentStickers) (*mtproto.Bool, error) {
	if in == nil {
		return nil, stickersProviderUnavailable(c)
	}
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if err = persist.ClearRecentStickers(stickerRequestContext(c), uid, in.GetAttached()); err != nil {
		return nil, stickerProviderError(c, err)
	}
	return mtproto.BoolTrue, nil
}

func (c *ApiFullCore) MessagesGetArchivedStickers(in *mtproto.TLMessagesGetArchivedStickers) (*mtproto.Messages_ArchivedStickers, error) {
	if in == nil {
		return nil, stickersProviderUnavailable(c)
	}
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	sets, err := persist.ListStickerSets(stickerRequestContext(c), uid, false, true, false, in.GetLimit())
	if err != nil {
		return nil, stickerProviderError(c, err)
	}
	covers := make([]*mtproto.StickerSetCovered, 0, len(sets))
	for _, set := range sets {
		covers = append(covers, stickerCovered(set))
	}
	return mtproto.MakeTLMessagesArchivedStickers(&mtproto.Messages_ArchivedStickers{Count: int32(len(covers)), Sets: covers}).To_Messages_ArchivedStickers(), nil
}

func (c *ApiFullCore) MessagesGetMaskStickers(in *mtproto.TLMessagesGetMaskStickers) (*mtproto.Messages_AllStickers, error) {
	if in == nil {
		return nil, stickersProviderUnavailable(c)
	}
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	sets, err := persist.ListStickerSets(stickerRequestContext(c), uid, false, false, false, 100)
	if err != nil {
		return nil, stickerProviderError(c, err)
	}
	items := make([]*mtproto.StickerSet, 0)
	ids := make([]int64, 0)
	for _, set := range sets {
		if !set.Masks {
			continue
		}
		items = append(items, mtproto.MakeTLStickerSet(&mtproto.StickerSet{Id: set.ID, AccessHash: set.AccessHash, Title: set.Title, ShortName: set.ShortName, Masks: true, Count: int32(len(set.Documents)), Hash: int32(stickerSetHash(&set))}).To_StickerSet())
		ids = append(ids, set.ID)
	}
	hash := idListHash(ids)
	if in.GetHash() != 0 && in.GetHash() == hash {
		return mtproto.MakeTLMessagesAllStickersNotModified(nil).To_Messages_AllStickers(), nil
	}
	return mtproto.MakeTLMessagesAllStickers(&mtproto.Messages_AllStickers{Hash: hash, Sets: items}).To_Messages_AllStickers(), nil
}

func (c *ApiFullCore) MessagesGetAttachedStickers(in *mtproto.TLMessagesGetAttachedStickers) (*mtproto.Vector_StickerSetCovered, error) {
	if in == nil {
		return nil, stickersProviderUnavailable(c)
	}
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in.GetMedia() == nil || in.GetMedia().GetId_INPUTDOCUMENT() == nil || in.GetMedia().GetId_INPUTDOCUMENT().GetId() == 0 {
		return &mtproto.Vector_StickerSetCovered{Datas: []*mtproto.StickerSetCovered{}}, nil
	}
	doc := in.GetMedia().GetId_INPUTDOCUMENT()
	if doc.GetAccessHash() == 0 {
		return nil, mtproto.ErrStickerIdInvalid
	}
	sets, err := persist.StickerSetsForDocument(stickerRequestContext(c), uid, doc.GetId(), doc.GetAccessHash())
	if err != nil {
		return nil, stickerProviderError(c, err)
	}
	covered := make([]*mtproto.StickerSetCovered, 0, len(sets))
	for _, set := range sets {
		covered = append(covered, stickerCovered(set))
	}
	return &mtproto.Vector_StickerSetCovered{Datas: covered}, nil
}

func (c *ApiFullCore) MessagesGetFavedStickers(in *mtproto.TLMessagesGetFavedStickers) (*mtproto.Messages_FavedStickers, error) {
	if in == nil {
		return nil, stickersProviderUnavailable(c)
	}
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	docs, err := persist.ListFavouriteStickers(stickerRequestContext(c), uid, 100)
	if err != nil {
		return nil, stickerProviderError(c, err)
	}
	items := make([]*mtproto.Document, 0, len(docs))
	ids := make([]int64, 0, len(docs))
	for _, d := range docs {
		items = append(items, stickerDocumentFromRecord(d))
		ids = append(ids, d.ID)
	}
	hash := idListHash(ids)
	if in.GetHash() != 0 && in.GetHash() == hash {
		return mtproto.MakeTLMessagesFavedStickersNotModified(&mtproto.Messages_FavedStickers{Hash: hash}).To_Messages_FavedStickers(), nil
	}
	return mtproto.MakeTLMessagesFavedStickers(&mtproto.Messages_FavedStickers{Hash: hash, Packs: []*mtproto.StickerPack{}, Stickers: items}).To_Messages_FavedStickers(), nil
}

func (c *ApiFullCore) MessagesFaveSticker(in *mtproto.TLMessagesFaveSticker) (*mtproto.Bool, error) {
	if in == nil {
		return nil, stickersProviderUnavailable(c)
	}
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in.GetId() == nil || inputDocumentID(in.GetId()) == 0 || in.GetId().GetAccessHash() == 0 {
		return nil, mtproto.ErrStickerIdInvalid
	}
	unfave := in.GetUnfave() != nil && in.GetUnfave().GetPredicateName() == mtproto.Predicate_boolTrue
	if err = persist.SaveFavouriteStickerWithAccessHash(stickerRequestContext(c), uid, inputDocumentID(in.GetId()), in.GetId().GetAccessHash(), unfave); err != nil {
		return nil, stickerProviderError(c, err)
	}
	return mtproto.BoolTrue, nil
}

func (c *ApiFullCore) MessagesSearchStickerSets(in *mtproto.TLMessagesSearchStickerSets) (*mtproto.Messages_FoundStickerSets, error) {
	if in == nil {
		return nil, stickersProviderUnavailable(c)
	}
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	sets, err := persist.SearchStickerSets(stickerRequestContext(c), uid, in.GetQ(), false, 100)
	if err != nil {
		return nil, stickerProviderError(c, err)
	}
	covers := make([]*mtproto.StickerSetCovered, 0, len(sets))
	ids := make([]int64, 0, len(sets))
	for _, set := range sets {
		ids = append(ids, set.ID)
		covers = append(covers, stickerCovered(set))
	}
	hash := idListHash(ids)
	if in.GetHash() != 0 && in.GetHash() == hash {
		return mtproto.MakeTLMessagesFoundStickerSetsNotModified(&mtproto.Messages_FoundStickerSets{Hash: hash}).To_Messages_FoundStickerSets(), nil
	}
	return mtproto.MakeTLMessagesFoundStickerSets(&mtproto.Messages_FoundStickerSets{Hash: hash, Sets: covers}).To_Messages_FoundStickerSets(), nil
}

func (c *ApiFullCore) MessagesToggleStickerSets(in *mtproto.TLMessagesToggleStickerSets) (*mtproto.Bool, error) {
	if in == nil {
		return nil, stickersProviderUnavailable(c)
	}
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	for _, input := range in.GetStickersets() {
		setID, short := stickerSetTokenID(input)
		set, getErr := persist.GetStickerSet(stickerRequestContext(c), uid, setID, short)
		if getErr != nil {
			return nil, stickerProviderError(c, getErr)
		}
		if set == nil {
			return nil, mtproto.ErrStickersetInvalid
		}
		if err := validateStickerSetAccess(input, set); err != nil {
			return nil, err
		}
		installed := !in.GetUninstall()
		archived := in.GetArchive() && installed
		if in.GetUnarchive() {
			archived = false
		}
		if err = persist.SetInstalled(stickerRequestContext(c), uid, set.ID, installed, archived); err != nil {
			return nil, stickerProviderError(c, err)
		}
	}
	return mtproto.BoolTrue, nil
}

func (c *ApiFullCore) MessagesGetOldFeaturedStickers(in *mtproto.TLMessagesGetOldFeaturedStickers) (*mtproto.Messages_FeaturedStickers, error) {
	if in == nil {
		return nil, stickersProviderUnavailable(c)
	}
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in.GetOffset() < 0 {
		return nil, mtproto.ErrOffsetInvalid
	}
	if in.GetLimit() < 0 {
		return nil, mtproto.ErrLimitInvalid
	}
	sets, total, err := persist.ListFeaturedStickerSets(stickerRequestContext(c), uid, in.GetOffset(), in.GetLimit())
	if err != nil {
		return nil, stickerProviderError(c, err)
	}
	covers := make([]*mtproto.StickerSetCovered, 0, len(sets))
	ids := make([]int64, 0, len(sets))
	for _, set := range sets {
		ids = append(ids, set.ID)
		covers = append(covers, stickerCovered(set))
	}
	hash := idListHash(ids)
	count := int32(total)
	if total > int64(^uint32(0)>>1) {
		count = int32(^uint32(0) >> 1)
	}
	if in.GetHash() != 0 && in.GetHash() == hash {
		return mtproto.MakeTLMessagesFeaturedStickersNotModified(&mtproto.Messages_FeaturedStickers{Count: count, Hash: hash}).To_Messages_FeaturedStickers(), nil
	}
	return mtproto.MakeTLMessagesFeaturedStickers(&mtproto.Messages_FeaturedStickers{Count: count, Hash: hash, Sets: covers}).To_Messages_FeaturedStickers(), nil
}

func (c *ApiFullCore) MessagesSearchEmojiStickerSets(in *mtproto.TLMessagesSearchEmojiStickerSets) (*mtproto.Messages_FoundStickerSets, error) {
	if in == nil {
		return nil, stickersProviderUnavailable(c)
	}
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	sets, err := persist.SearchStickerSets(stickerRequestContext(c), uid, in.GetQ(), true, 100)
	if err != nil {
		return nil, stickerProviderError(c, err)
	}
	covers := make([]*mtproto.StickerSetCovered, 0, len(sets))
	for _, set := range sets {
		covers = append(covers, stickerCovered(set))
	}
	return mtproto.MakeTLMessagesFoundStickerSets(&mtproto.Messages_FoundStickerSets{Sets: covers, Hash: int64(len(covers))}).To_Messages_FoundStickerSets(), nil
}

func (c *ApiFullCore) MessagesGetMyStickers(in *mtproto.TLMessagesGetMyStickers) (*mtproto.Messages_MyStickers, error) {
	if in == nil {
		return nil, stickersProviderUnavailable(c)
	}
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	sets, err := persist.ListStickerSets(stickerRequestContext(c), uid, false, false, false, in.GetLimit())
	if err != nil {
		return nil, stickerProviderError(c, err)
	}
	covers := make([]*mtproto.StickerSetCovered, 0, len(sets))
	for _, set := range sets {
		if set.OwnerUserID == uid {
			covers = append(covers, stickerCovered(set))
		}
	}
	return mtproto.MakeTLMessagesMyStickers(&mtproto.Messages_MyStickers{Count: int32(len(covers)), Sets: covers}).To_Messages_MyStickers(), nil
}

func (c *ApiFullCore) MessagesSearchStickers(in *mtproto.TLMessagesSearchStickers) (*mtproto.Messages_FoundStickers, error) {
	if in == nil {
		return nil, stickersProviderUnavailable(c)
	}
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	docs, err := persist.SearchStickerDocuments(stickerRequestContext(c), uid, in.GetQ(), in.GetEmoticon(), in.GetOffset(), in.GetLimit())
	if err != nil {
		return nil, stickerProviderError(c, err)
	}
	items := make([]*mtproto.Document, 0, len(docs))
	for _, d := range docs {
		items = append(items, stickerDocumentFromRecord(d))
	}
	next := int32(in.GetOffset()) + int32(len(items))
	return mtproto.MakeTLMessagesFoundStickers(&mtproto.Messages_FoundStickers{NextOffset: wrapperspb.Int32(next), Hash: int64(len(items)), Stickers: items}).To_Messages_FoundStickers(), nil
}

func (c *ApiFullCore) StickersCreateStickerSet(in *mtproto.TLStickersCreateStickerSet) (*mtproto.Messages_StickerSet, error) {
	if in == nil {
		return nil, stickersProviderUnavailable(c)
	}
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	set, err := persist.CreateStickerSet(stickerRequestContext(c), uid, in.GetTitle(), in.GetShortName(), in.GetMasks(), in.GetEmojis(), in.GetTextColor(), in.GetAnimated(), in.GetVideos(), inputStickerDocs(in.GetStickers()))
	if err != nil {
		return nil, stickerProviderError(c, err)
	}
	return stickerSetFromRecord(set, 0), nil
}

func (c *ApiFullCore) StickersRemoveStickerFromSet(in *mtproto.TLStickersRemoveStickerFromSet) (*mtproto.Messages_StickerSet, error) {
	if in == nil {
		return nil, stickersProviderUnavailable(c)
	}
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in.GetSticker() == nil || in.GetSticker().GetId() == 0 {
		return nil, mtproto.ErrStickerIdInvalid
	}
	set, err := persist.RemoveSticker(stickerRequestContext(c), uid, in.GetSticker().GetId())
	if err != nil {
		return nil, stickerProviderError(c, err)
	}
	return stickerSetFromRecord(set, 0), nil
}

func (c *ApiFullCore) StickersChangeStickerPosition(in *mtproto.TLStickersChangeStickerPosition) (*mtproto.Messages_StickerSet, error) {
	if in == nil {
		return nil, stickersProviderUnavailable(c)
	}
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in.GetSticker() == nil || in.GetSticker().GetId() == 0 {
		return nil, mtproto.ErrStickerIdInvalid
	}
	set, err := persist.MoveSticker(stickerRequestContext(c), uid, in.GetSticker().GetId(), in.GetPosition())
	if err != nil {
		return nil, stickerProviderError(c, err)
	}
	return stickerSetFromRecord(set, 0), nil
}

func (c *ApiFullCore) StickersAddStickerToSet(in *mtproto.TLStickersAddStickerToSet) (*mtproto.Messages_StickerSet, error) {
	if in == nil {
		return nil, stickersProviderUnavailable(c)
	}
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	setID, short := stickerSetTokenID(in.GetStickerset())
	set, err := persist.GetStickerSet(stickerRequestContext(c), uid, setID, short)
	if err != nil {
		return nil, stickerProviderError(c, err)
	}
	if set == nil {
		return nil, mtproto.ErrStickersetInvalid
	}
	if err := validateStickerSetAccess(in.GetStickerset(), set); err != nil {
		return nil, err
	}
	items := inputStickerDocs([]*mtproto.InputStickerSetItem{in.GetSticker()})
	if len(items) == 0 {
		return nil, mtproto.ErrStickersEmpty
	}
	updated, err := persist.AddSticker(stickerRequestContext(c), uid, set.ID, items[0])
	if err != nil {
		return nil, stickerProviderError(c, err)
	}
	return stickerSetFromRecord(updated, 0), nil
}

func (c *ApiFullCore) StickersSetStickerSetThumb(in *mtproto.TLStickersSetStickerSetThumb) (*mtproto.Messages_StickerSet, error) {
	if in == nil {
		return nil, stickersProviderUnavailable(c)
	}
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	setID, short := stickerSetTokenID(in.GetStickerset())
	set, err := persist.GetStickerSet(stickerRequestContext(c), uid, setID, short)
	if err != nil {
		return nil, stickerProviderError(c, err)
	}
	if set == nil {
		return nil, mtproto.ErrStickersetInvalid
	}
	if err := validateStickerSetAccess(in.GetStickerset(), set); err != nil {
		return nil, err
	}
	documentID := inputDocumentID(in.GetThumb())
	if in.GetThumbDocumentId() != nil {
		documentID = in.GetThumbDocumentId().GetValue()
	}
	updated, err := persist.SetStickerSetThumb(stickerRequestContext(c), uid, set.ID, documentID)
	if err != nil {
		return nil, stickerProviderError(c, err)
	}
	return stickerSetFromRecord(updated, 0), nil
}

func (c *ApiFullCore) StickersCheckShortName(in *mtproto.TLStickersCheckShortName) (*mtproto.Bool, error) {
	if in == nil {
		return nil, stickersProviderUnavailable(c)
	}
	if _, err := c.requireUserId(); err != nil {
		return nil, err
	}
	available, err := persist.StickerShortNameAvailable(stickerRequestContext(c), in.GetShortName())
	if err != nil {
		return nil, stickerProviderError(c, err)
	}
	if available {
		return mtproto.BoolTrue, nil
	}
	return mtproto.BoolFalse, nil
}

func (c *ApiFullCore) StickersSuggestShortName(in *mtproto.TLStickersSuggestShortName) (*mtproto.Stickers_SuggestedShortName, error) {
	if in == nil {
		return nil, stickersProviderUnavailable(c)
	}
	if _, err := c.requireUserId(); err != nil {
		return nil, err
	}
	short := strings.ToLower(strings.TrimSpace(in.GetTitle()))
	short = strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' {
			return r
		}
		return '_'
	}, short)
	short = strings.Trim(short, "_")
	if short == "" {
		short = "sticker_set"
	}
	return mtproto.MakeTLStickersSuggestedShortName(&mtproto.Stickers_SuggestedShortName{ShortName: short}).To_Stickers_SuggestedShortName(), nil
}

func (c *ApiFullCore) StickersChangeSticker(in *mtproto.TLStickersChangeSticker) (*mtproto.Messages_StickerSet, error) {
	if in == nil {
		return nil, stickersProviderUnavailable(c)
	}
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in.GetSticker() == nil || in.GetSticker().GetId() == 0 {
		return nil, mtproto.ErrStickerIdInvalid
	}
	// Telegram exposes emoji/keywords edits on the existing document. The
	// provider keeps the update transactional and returns the rebuilt set.
	emoji, keywords := "", ""
	if in.GetEmoji() != nil {
		emoji = in.GetEmoji().GetValue()
	}
	if in.GetKeywords() != nil {
		keywords = in.GetKeywords().GetValue()
	}
	set, err := persist.ChangeSticker(stickerRequestContext(c), uid, in.GetSticker().GetId(), emoji, keywords)
	if err != nil {
		return nil, stickerProviderError(c, err)
	}
	return stickerSetFromRecord(set, 0), nil
}

func (c *ApiFullCore) StickersRenameStickerSet(in *mtproto.TLStickersRenameStickerSet) (*mtproto.Messages_StickerSet, error) {
	if in == nil {
		return nil, stickersProviderUnavailable(c)
	}
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	setID, short := stickerSetTokenID(in.GetStickerset())
	set, err := persist.GetStickerSet(stickerRequestContext(c), uid, setID, short)
	if err != nil {
		return nil, stickerProviderError(c, err)
	}
	if set == nil {
		return nil, mtproto.ErrStickersetInvalid
	}
	if err := validateStickerSetAccess(in.GetStickerset(), set); err != nil {
		return nil, err
	}
	updated, err := persist.UpdateStickerSetTitle(stickerRequestContext(c), uid, set.ID, in.GetTitle())
	if err != nil {
		return nil, stickerProviderError(c, err)
	}
	return stickerSetFromRecord(updated, 0), nil
}

func (c *ApiFullCore) StickersDeleteStickerSet(in *mtproto.TLStickersDeleteStickerSet) (*mtproto.Bool, error) {
	if in == nil {
		return nil, stickersProviderUnavailable(c)
	}
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	setID, short := stickerSetTokenID(in.GetStickerset())
	set, err := persist.GetStickerSet(stickerRequestContext(c), uid, setID, short)
	if err != nil {
		return nil, stickerProviderError(c, err)
	}
	if set == nil {
		return nil, mtproto.ErrStickersetInvalid
	}
	if err := validateStickerSetAccess(in.GetStickerset(), set); err != nil {
		return nil, err
	}
	if err = persist.DeleteStickerSet(stickerRequestContext(c), uid, set.ID); err != nil {
		return nil, stickerProviderError(c, err)
	}
	return mtproto.BoolTrue, nil
}

func (c *ApiFullCore) StickersReplaceSticker(in *mtproto.TLStickersReplaceSticker) (*mtproto.Messages_StickerSet, error) {
	if in == nil {
		return nil, stickersProviderUnavailable(c)
	}
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in.GetSticker() == nil || in.GetSticker().GetId() == 0 || in.GetNewSticker() == nil {
		return nil, mtproto.ErrStickerIdInvalid
	}
	items := inputStickerDocs([]*mtproto.InputStickerSetItem{in.GetNewSticker()})
	if len(items) == 0 {
		return nil, mtproto.ErrStickersEmpty
	}
	set, err := persist.ReplaceSticker(stickerRequestContext(c), uid, in.GetSticker().GetId(), items[0])
	if err != nil {
		return nil, stickerProviderError(c, err)
	}
	return stickerSetFromRecord(set, 0), nil
}

type storedStickerDoc struct {
	ID    int64  `json:"id"`
	Alt   string `json:"alt"`
	Short string `json:"short"`
}

func stickerDocsKey(userID int64) string {
	return fmt.Sprintf("%s%d:docs", stickerKeyPrefix, userID)
}

func loadStickerDocs(userID int64) ([]storedStickerDoc, error) {
	raw, err := persist.Default.Get(stickerDocsKey(userID))
	if err != nil || raw == "" {
		return nil, err
	}
	var docs []storedStickerDoc
	if err = json.Unmarshal([]byte(raw), &docs); err != nil {
		return nil, err
	}
	return docs, nil
}

func saveCreatedStickerDocs(userID int64, short string, in *mtproto.TLStickersCreateStickerSet) error {
	var docs []storedStickerDoc
	if in != nil {
		for i, item := range in.GetStickers() {
			if item == nil {
				continue
			}
			alt := item.GetEmoji()
			if alt == "" {
				alt = short
			}
			docs = append(docs, storedStickerDoc{ID: int64(i + 1), Alt: alt, Short: short})
		}
	}
	if len(docs) == 0 && short != "" {
		docs = []storedStickerDoc{{ID: 1, Alt: short, Short: short}}
	}
	raw, err := json.Marshal(docs)
	if err != nil {
		return err
	}
	return persist.Default.Set(stickerDocsKey(userID), string(raw))
}

func matchingStickerDocuments(userID int64, query string) ([]*mtproto.Document, error) {
	docs, err := loadStickerDocs(userID)
	if err != nil {
		return nil, err
	}
	needle := strings.ToLower(query)
	out := make([]*mtproto.Document, 0, len(docs))
	now := int32(time.Now().Unix())
	for _, doc := range docs {
		if needle != "" && !strings.Contains(strings.ToLower(doc.Alt), needle) && !strings.Contains(strings.ToLower(doc.Short), needle) {
			continue
		}
		out = append(out, mtproto.MakeTLDocument(&mtproto.Document{
			Id:            doc.ID,
			AccessHash:    doc.ID,
			FileReference: []byte{},
			Date:          now,
			MimeType:      "image/webp",
			Size2_INT64:   1,
			DcId:          2,
			Attributes: []*mtproto.DocumentAttribute{
				mtproto.MakeTLDocumentAttributeSticker(&mtproto.DocumentAttribute{
					Alt: doc.Alt,
					Stickerset: mtproto.MakeTLInputStickerSetID(&mtproto.InputStickerSet{
						Id:         doc.ID,
						AccessHash: doc.ID,
					}).To_InputStickerSet(),
				}).To_DocumentAttribute(),
			},
		}).To_Document())
	}
	return out, nil
}
