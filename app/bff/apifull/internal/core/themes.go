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
	"strconv"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/persist"
	dialogpb "github.com/teamgram/teamgram-server/app/service/biz/dialog/dialog"
	"github.com/teamgram/teamgram-server/app/service/dfs/dfs"
	"google.golang.org/protobuf/proto"
)

// RPCThemesServer: Layer 229 methods previously returned ERR_ENTERPRISE_IS_BLOCKED.

func themeKey(userID int64) string {
	return "theme:" + strconv.FormatInt(userID, 10)
}

func themeUploadKey(userID, documentID int64) string {
	return "theme:uploaded:" + strconv.FormatInt(userID, 10) + ":" + strconv.FormatInt(documentID, 10)
}

func loadThemes(userID int64) ([]*mtproto.Theme, error) {
	raw, err := persist.Default.Get(themeKey(userID))
	if err != nil || raw == "" {
		return nil, err
	}
	var list []*mtproto.Theme
	if err := json.Unmarshal([]byte(raw), &list); err != nil {
		return nil, err
	}
	return list, nil
}

func storeThemes(userID int64, list []*mtproto.Theme) error {
	if list == nil {
		list = []*mtproto.Theme{}
	}
	b, err := json.Marshal(list)
	if err != nil {
		return err
	}
	return persist.Default.Set(themeKey(userID), string(b))
}

func themeMatches(t *mtproto.Theme, in *mtproto.InputTheme) bool {
	if t == nil || in == nil {
		return false
	}
	if in.GetId() != 0 {
		return t.GetId() == in.GetId()
	}
	if in.GetSlug() != "" {
		return t.GetSlug() == in.GetSlug()
	}
	return false
}

// ownedThemeMatches only accepts a theme whose real document was persisted
// for this account. Without a catalogue provider, an id/slug alone is not
// enough to claim ownership of a theme.
func ownedThemeMatches(t *mtproto.Theme, in *mtproto.InputTheme) bool {
	if !themeMatches(t, in) || t.GetDocument() == nil || t.GetId() <= 0 || t.GetAccessHash() == 0 {
		return false
	}
	if t.GetDocument().GetId() != t.GetId() || t.GetDocument().GetAccessHash() != t.GetAccessHash() {
		return false
	}
	if in.GetId() != 0 {
		return in.GetAccessHash() != 0 && in.GetAccessHash() == t.GetAccessHash()
	}
	return true
}

func findOwnedTheme(userID int64, in *mtproto.InputTheme) (*mtproto.Theme, error) {
	if in == nil || (in.GetId() <= 0 && in.GetSlug() == "") {
		return nil, mtproto.ErrThemeInvalid
	}
	list, err := loadThemes(userID)
	if err != nil {
		return nil, err
	}
	for _, theme := range list {
		if ownedThemeMatches(theme, in) {
			return theme, nil
		}
	}
	return nil, mtproto.ErrThemeInvalid
}

func ownedThemes(list []*mtproto.Theme) []*mtproto.Theme {
	out := make([]*mtproto.Theme, 0, len(list))
	for _, theme := range list {
		if theme != nil && ownedThemeMatches(theme, mtproto.MakeTLInputTheme(&mtproto.InputTheme{
			Id:         theme.GetId(),
			AccessHash: theme.GetAccessHash(),
		}).To_InputTheme()) {
			out = append(out, theme)
		}
	}
	return out
}

func themeHash(list []*mtproto.Theme) int64 {
	var hash int64 = 1
	for _, theme := range list {
		if theme == nil {
			continue
		}
		hash = hash*31 + theme.GetId()
		hash = hash*31 + theme.GetAccessHash()
	}
	if hash < 0 {
		return -hash
	}
	return hash
}

func loadUploadedThemeDocument(userID int64, in *mtproto.InputDocument) (*mtproto.Document, error) {
	if in == nil || in.GetPredicateName() == mtproto.Predicate_inputDocumentEmpty || in.GetId() <= 0 || in.GetAccessHash() == 0 {
		return nil, mtproto.ErrDocumentInvalid
	}
	raw, err := persist.Default.Get(themeUploadKey(userID, in.GetId()))
	if err != nil {
		return nil, err
	}
	if raw == "" {
		return nil, mtproto.ErrDocumentInvalid
	}
	document := &mtproto.Document{}
	if err = json.Unmarshal([]byte(raw), document); err != nil {
		return nil, err
	}
	if document.GetId() != in.GetId() || document.GetAccessHash() != in.GetAccessHash() {
		return nil, mtproto.ErrDocumentInvalid
	}
	return document, nil
}

func themeSettingsFromInput(userID int64, input []*mtproto.InputThemeSettings) ([]*mtproto.ThemeSettings, error) {
	if input == nil {
		return nil, nil
	}
	settings := make([]*mtproto.ThemeSettings, 0, len(input))
	for _, src := range input {
		if src == nil {
			continue
		}
		setting := &mtproto.ThemeSettings{
			MessageColorsAnimated: src.GetMessageColorsAnimated(),
			BaseTheme:             src.GetBaseTheme(),
			AccentColor:           src.GetAccentColor(),
			OutboxAccentColor:     src.GetOutboxAccentColor(),
			MessageColors:         append([]int32(nil), src.GetMessageColors()...),
		}
		if wallpaper := src.GetWallpaper(); wallpaper != nil {
			candidate, findErr := findOwnedWallpaper(userID, wallpaper)
			if findErr != nil {
				return nil, findErr
			}
			setting.Wallpaper = candidate
		}
		settings = append(settings, mtproto.MakeTLThemeSettings(setting).To_ThemeSettings())
	}
	return settings, nil
}

func themeFromCreatedInput(userID int64, in *mtproto.TLAccountCreateTheme) (*mtproto.Theme, error) {
	if in == nil || in.GetSlug() == "" || in.GetTitle() == "" {
		return nil, mtproto.ErrThemeInvalid
	}
	document, err := loadUploadedThemeDocument(userID, in.GetDocument())
	if err != nil {
		return nil, err
	}
	settings, err := themeSettingsFromInput(userID, in.GetSettings())
	if err != nil {
		return nil, err
	}
	return mtproto.MakeTLTheme(&mtproto.Theme{
		Creator:    true,
		Id:         document.GetId(),
		AccessHash: document.GetAccessHash(),
		Slug:       in.GetSlug(),
		Title:      in.GetTitle(),
		Document:   document,
		Settings:   settings,
	}).To_Theme(), nil
}

func (c *ApiFullCore) AccountUploadTheme(in *mtproto.TLAccountUploadTheme) (*mtproto.Document, error) {
	userID, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil || in.GetFile() == nil || in.GetFileName() == "" {
		return nil, mtproto.ErrInputRequestInvalid
	}
	d := c.apifullDao()
	if d == nil || d.DfsClient == nil {
		return nil, mtproto.ErrMethodNotImpl
	}
	mimeType := in.GetMimeType()
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}
	document, err := d.DfsUploadThemeFile(callContext(c), &dfs.TLDfsUploadThemeFile{
		Creator:  userID,
		File:     in.GetFile(),
		Thumb:    in.GetThumb(),
		MimeType: mimeType,
		FileName: in.GetFileName(),
	})
	if err != nil {
		return nil, err
	}
	if document == nil || document.GetId() <= 0 || document.GetAccessHash() == 0 {
		return nil, mtproto.ErrInternalServerError
	}
	raw, err := json.Marshal(document)
	if err != nil {
		return nil, err
	}
	if err = persist.Default.Set(themeUploadKey(userID, document.GetId()), string(raw)); err != nil {
		return nil, err
	}
	return document, nil
}

func (c *ApiFullCore) AccountCreateTheme(in *mtproto.TLAccountCreateTheme) (*mtproto.Theme, error) {
	userID, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	theme, err := themeFromCreatedInput(userID, in)
	if err != nil {
		return nil, err
	}
	list, err := loadThemes(userID)
	if err != nil {
		return nil, err
	}
	for _, existing := range list {
		if existing == nil || (!themeMatches(existing, mtproto.MakeTLInputTheme(&mtproto.InputTheme{Id: theme.GetId()}).To_InputTheme()) && existing.GetSlug() != theme.GetSlug()) {
			continue
		}
		if existing.GetAccessHash() != theme.GetAccessHash() || existing.GetSlug() != theme.GetSlug() || existing.GetTitle() != theme.GetTitle() {
			return nil, mtproto.ErrThemeInvalid
		}
		return existing, nil
	}
	if err = storeThemes(userID, append(list, theme)); err != nil {
		return nil, err
	}
	return theme, nil
}

func (c *ApiFullCore) AccountUpdateTheme(in *mtproto.TLAccountUpdateTheme) (*mtproto.Theme, error) {
	userID, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil || in.GetTheme() == nil {
		return nil, mtproto.ErrThemeInvalid
	}
	list, err := loadThemes(userID)
	if err != nil {
		return nil, err
	}
	var index = -1
	for i, existing := range list {
		if ownedThemeMatches(existing, in.GetTheme()) {
			if in.GetTheme().GetAccessHash() != 0 && in.GetTheme().GetAccessHash() != existing.GetAccessHash() {
				return nil, mtproto.ErrThemeInvalid
			}
			index = i
			break
		}
	}
	if index < 0 {
		return nil, mtproto.ErrThemeInvalid
	}
	updated := proto.Clone(list[index]).(*mtproto.Theme)
	if in.GetSlug() != nil {
		updated.Slug = in.GetSlug().GetValue()
	}
	if in.GetTitle() != nil {
		updated.Title = in.GetTitle().GetValue()
	}
	if in.GetDocument() != nil {
		document, loadErr := loadUploadedThemeDocument(userID, in.GetDocument())
		if loadErr != nil {
			return nil, loadErr
		}
		updated.Id = document.GetId()
		updated.AccessHash = document.GetAccessHash()
		updated.Document = document
	}
	if in.GetSettings() != nil {
		settings, settingsErr := themeSettingsFromInput(userID, in.GetSettings())
		if settingsErr != nil {
			return nil, settingsErr
		}
		updated.Settings = settings
	}
	if updated.GetSlug() == "" || updated.GetTitle() == "" {
		return nil, mtproto.ErrThemeInvalid
	}
	list[index] = updated
	if err = storeThemes(userID, list); err != nil {
		return nil, err
	}
	return updated, nil
}

func (c *ApiFullCore) AccountSaveTheme(in *mtproto.TLAccountSaveTheme) (*mtproto.Bool, error) {
	userID, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil || in.GetTheme() == nil {
		return nil, mtproto.ErrThemeInvalid
	}
	src := in.GetTheme()
	if _, err = findOwnedTheme(userID, src); err != nil {
		return nil, err
	}
	if mtproto.FromBool(in.GetUnsave()) {
		list, err := loadThemes(userID)
		if err != nil {
			return nil, err
		}
		kept := make([]*mtproto.Theme, 0, len(list))
		for _, t := range list {
			if !themeMatches(t, src) {
				kept = append(kept, t)
			}
		}
		if err := storeThemes(userID, kept); err != nil {
			return nil, err
		}
		return mtproto.BoolTrue, nil
	}
	return mtproto.BoolTrue, nil
}

func (c *ApiFullCore) AccountInstallTheme(in *mtproto.TLAccountInstallTheme) (*mtproto.Bool, error) {
	userID, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil || in.GetTheme() == nil {
		return nil, mtproto.ErrThemeInvalid
	}
	if _, err = findOwnedTheme(userID, in.GetTheme()); err != nil {
		return nil, err
	}
	return mtproto.BoolTrue, nil
}

func (c *ApiFullCore) AccountGetTheme(in *mtproto.TLAccountGetTheme) (*mtproto.Theme, error) {
	userID, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil || in.GetTheme() == nil {
		return nil, mtproto.ErrThemeInvalid
	}
	if theme, findErr := findOwnedTheme(userID, in.GetTheme()); findErr == nil {
		return theme, nil
	} else {
		return nil, findErr
	}
}

func (c *ApiFullCore) AccountGetThemes(in *mtproto.TLAccountGetThemes) (*mtproto.Account_Themes, error) {
	userID, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	_ = in
	themes, err := loadThemes(userID)
	if err != nil {
		return nil, err
	}
	themes = ownedThemes(themes)
	hash := themeHash(themes)
	if in != nil && in.GetHash() != 0 && in.GetHash() == hash {
		return mtproto.MakeTLAccountThemesNotModified(&mtproto.Account_Themes{}).To_Account_Themes(), nil
	}
	return mtproto.MakeTLAccountThemes(&mtproto.Account_Themes{
		Hash:   hash,
		Themes: themes,
	}).To_Account_Themes(), nil
}

func (c *ApiFullCore) AccountGetChatThemes(in *mtproto.TLAccountGetChatThemes) (*mtproto.Account_Themes, error) {
	userID, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	// This deployment has no global Telegram chat-theme catalogue. It does
	// have an authoritative per-user catalogue for themes created from an
	// uploaded document, so expose only those persisted, access-checked rows.
	themes, err := loadThemes(userID)
	if err != nil {
		return nil, err
	}
	themes = ownedThemes(themes)
	hash := themeHash(themes)
	if in != nil && in.GetHash() != 0 && in.GetHash() == hash {
		return mtproto.MakeTLAccountThemesNotModified(&mtproto.Account_Themes{
			Hash: hash,
		}).To_Account_Themes(), nil
	}
	return mtproto.MakeTLAccountThemes(&mtproto.Account_Themes{
		Hash:   hash,
		Themes: themes,
	}).To_Account_Themes(), nil
}

func (c *ApiFullCore) AccountGetUniqueGiftChatThemes(in *mtproto.TLAccountGetUniqueGiftChatThemes) (*mtproto.Account_ChatThemes, error) {
	if _, err := c.requireUserId(); err != nil {
		return nil, err
	}
	_ = in
	return nil, mtproto.ErrMethodNotImpl
}

func (c *ApiFullCore) MessagesSetChatTheme(in *mtproto.TLMessagesSetChatTheme) (*mtproto.Updates, error) {
	userID, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil || in.GetPeer() == nil {
		return nil, mtproto.ErrPeerIdInvalid
	}
	if err = c.authorizeReactionPeer(userID, in.GetPeer()); err != nil {
		return nil, err
	}
	emoticon := in.GetEmoticon()
	if theme := in.GetTheme(); theme != nil {
		if theme.GetEmoticon() != "" {
			emoticon = theme.GetEmoticon()
		} else if theme.GetSlug() != "" {
			return nil, mtproto.ErrMethodNotImpl
		}
	}
	if emoticon == "" && (in.GetTheme() == nil || in.GetTheme().GetPredicateName() != mtproto.Predicate_inputChatThemeEmpty) {
		return nil, mtproto.ErrInputConstructorInvalid
	}
	peerType, peerID := apifullPeerTypeID(userID, in.GetPeer())
	if peerID <= 0 {
		return nil, mtproto.ErrPeerIdInvalid
	}
	d := c.apifullDao()
	if d == nil || d.DialogClient == nil {
		return nil, mtproto.ErrMethodNotImpl
	}
	if _, err = d.DialogSetChatTheme(callContext(c), &dialogpb.TLDialogSetChatTheme{
		UserId:        userID,
		PeerType:      peerType,
		PeerId:        peerID,
		ThemeEmoticon: emoticon,
	}); err != nil {
		return nil, err
	}
	return callUpdates(), nil
}
