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

func lookPut(userID int64, method string, in any) error {
	b, err := json.Marshal(in)
	if err != nil {
		return err
	}
	return persist.Default.Set("look:"+strconv.FormatInt(userID, 10)+":"+method, string(b))
}

// RPCWallpapersServer: Layer 229 methods previously returned ERR_ENTERPRISE_IS_BLOCKED.

func decodeWallpaperList(raw string) ([]*mtproto.WallPaper, error) {
	if raw == "" {
		return nil, nil
	}
	var list []*mtproto.WallPaper
	if err := json.Unmarshal([]byte(raw), &list); err != nil {
		return nil, err
	}
	return list, nil
}

func loadWallpapers(userID int64) ([]*mtproto.WallPaper, error) {
	raw, err := persist.LoadWallpaperList(userID, false)
	if err != nil {
		return nil, err
	}
	return decodeWallpaperList(raw)
}

func storeWallpapers(userID int64, list []*mtproto.WallPaper) error {
	if list == nil {
		list = []*mtproto.WallPaper{}
	}
	b, err := json.Marshal(list)
	if err != nil {
		return err
	}
	return persist.StoreWallpaperList(userID, false, string(b))
}

func loadUploadedWallpapers(userID int64) ([]*mtproto.WallPaper, error) {
	raw, err := persist.LoadWallpaperList(userID, true)
	if err != nil {
		return nil, err
	}
	return decodeWallpaperList(raw)
}

func storeUploadedWallpapers(userID int64, list []*mtproto.WallPaper) error {
	if list == nil {
		list = []*mtproto.WallPaper{}
	}
	b, err := json.Marshal(list)
	if err != nil {
		return err
	}
	return persist.StoreWallpaperList(userID, true, string(b))
}

func mutateWallpapers(userID int64, uploaded bool, mutate func([]*mtproto.WallPaper) ([]*mtproto.WallPaper, error)) error {
	_, err := persist.MutateWallpaperList(userID, uploaded, func(raw string) (string, error) {
		list, err := decodeWallpaperList(raw)
		if err != nil {
			return "", err
		}
		next, err := mutate(list)
		if err != nil {
			return "", err
		}
		if next == nil {
			next = []*mtproto.WallPaper{}
		}
		encoded, err := json.Marshal(next)
		if err != nil {
			return "", err
		}
		return string(encoded), nil
	})
	return err
}

func wallpaperMatches(w *mtproto.WallPaper, in *mtproto.InputWallPaper) bool {
	if w == nil || in == nil {
		return false
	}
	if in.GetId() != 0 {
		return w.GetId() == in.GetId()
	}
	if in.GetSlug() != "" {
		return w.GetSlug() == in.GetSlug()
	}
	return false
}

func ownedWallpaperMatches(w *mtproto.WallPaper, in *mtproto.InputWallPaper) bool {
	if !wallpaperMatches(w, in) || w.GetDocument() == nil || w.GetId() <= 0 || w.GetAccessHash() == 0 {
		return false
	}
	document := w.GetDocument()
	if document.GetId() != w.GetId() || document.GetAccessHash() != w.GetAccessHash() {
		return false
	}
	if in.GetId() != 0 {
		return in.GetAccessHash() != 0 && in.GetAccessHash() == w.GetAccessHash()
	}
	return in.GetAccessHash() == 0 || in.GetAccessHash() == w.GetAccessHash()
}

func findOwnedWallpaper(userID int64, in *mtproto.InputWallPaper) (*mtproto.WallPaper, error) {
	if in == nil || (in.GetId() <= 0 && in.GetSlug() == "") {
		return nil, mtproto.ErrWallpaperInvalid
	}
	list, err := loadWallpapers(userID)
	if err != nil {
		return nil, err
	}
	for _, wallpaper := range list {
		if ownedWallpaperMatches(wallpaper, in) {
			return wallpaper, nil
		}
	}
	uploaded, err := loadUploadedWallpapers(userID)
	if err != nil {
		return nil, err
	}
	for _, wallpaper := range uploaded {
		if ownedWallpaperMatches(wallpaper, in) {
			return wallpaper, nil
		}
	}
	return nil, mtproto.ErrWallpaperInvalid
}

func ownedWallpapers(list []*mtproto.WallPaper) []*mtproto.WallPaper {
	out := make([]*mtproto.WallPaper, 0, len(list))
	for _, wallpaper := range list {
		if ownedWallpaperMatches(wallpaper, mtproto.MakeTLInputWallPaper(&mtproto.InputWallPaper{
			Id:         wallpaper.GetId(),
			AccessHash: wallpaper.GetAccessHash(),
		}).To_InputWallPaper()) {
			out = append(out, wallpaper)
		}
	}
	return out
}

func wallpaperHash(list []*mtproto.WallPaper) int64 {
	var hash int64 = 1
	for _, wallpaper := range list {
		if wallpaper == nil {
			continue
		}
		hash = hash*31 + wallpaper.GetId()
		hash = hash*31 + wallpaper.GetAccessHash()
	}
	if hash < 0 {
		return -hash
	}
	return hash
}

func upsertWallpaper(userID int64, src *mtproto.InputWallPaper, settings *mtproto.WallPaperSettings) error {
	owned, err := findOwnedWallpaper(userID, src)
	if err != nil {
		return err
	}
	wp := proto.Clone(owned).(*mtproto.WallPaper)
	wp.Settings = settings
	return mutateWallpapers(userID, false, func(list []*mtproto.WallPaper) ([]*mtproto.WallPaper, error) {
		for i, w := range list {
			if wallpaperMatches(w, src) {
				list[i] = wp
				return list, nil
			}
		}
		return append(list, wp), nil
	})
}

func (c *ApiFullCore) AccountGetWallPapers(in *mtproto.TLAccountGetWallPapers) (*mtproto.Account_WallPapers, error) {
	userID, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	list, err := loadWallpapers(userID)
	if err != nil {
		return nil, err
	}
	list = ownedWallpapers(list)
	hash := wallpaperHash(list)
	if in != nil && in.GetHash() != 0 && in.GetHash() == hash {
		return mtproto.MakeTLAccountWallPapersNotModified(&mtproto.Account_WallPapers{}).To_Account_WallPapers(), nil
	}
	return mtproto.MakeTLAccountWallPapers(&mtproto.Account_WallPapers{
		Hash:       hash,
		Wallpapers: list,
	}).To_Account_WallPapers(), nil
}

func (c *ApiFullCore) AccountGetWallPaper(in *mtproto.TLAccountGetWallPaper) (*mtproto.WallPaper, error) {
	userID, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil || in.GetWallpaper() == nil {
		return nil, mtproto.ErrWallpaperInvalid
	}
	return findOwnedWallpaper(userID, in.GetWallpaper())
}

func (c *ApiFullCore) AccountUploadWallPaper(in *mtproto.TLAccountUploadWallPaper) (*mtproto.WallPaper, error) {
	userID, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil || in.GetFile() == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	d := c.apifullDao()
	if d == nil || d.DfsClient == nil {
		return nil, mtproto.ErrMethodNotImpl
	}
	mimeType := in.GetMimeType()
	if mimeType == "" {
		mimeType = "image/jpeg"
	}
	document, err := d.DfsUploadWallPaperFile(callContext(c), &dfs.TLDfsUploadWallPaperFile{
		Creator:  userID,
		File:     in.GetFile(),
		MimeType: mimeType,
		Admin:    mtproto.BoolFalse,
	})
	if err != nil {
		return nil, err
	}
	if document == nil || document.GetId() == 0 || document.GetAccessHash() == 0 {
		return nil, mtproto.ErrInternalServerError
	}
	wallpaper := mtproto.MakeTLWallPaper(&mtproto.WallPaper{
		Id:         document.GetId(),
		Creator:    true,
		AccessHash: document.GetAccessHash(),
		Document:   document,
		Settings:   in.GetSettings(),
	}).To_WallPaper()
	if err = mutateWallpapers(userID, true, func(uploaded []*mtproto.WallPaper) ([]*mtproto.WallPaper, error) {
		replaced := false
		for i, candidate := range uploaded {
			if candidate != nil && candidate.GetId() == wallpaper.GetId() {
				uploaded[i] = wallpaper
				replaced = true
				break
			}
		}
		if !replaced {
			uploaded = append(uploaded, wallpaper)
		}
		return uploaded, nil
	}); err != nil {
		return nil, err
	}
	return wallpaper, nil
}

func (c *ApiFullCore) AccountSaveWallPaper(in *mtproto.TLAccountSaveWallPaper) (*mtproto.Bool, error) {
	userID, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil || in.GetWallpaper() == nil {
		return nil, mtproto.ErrWallpaperInvalid
	}
	src := in.GetWallpaper()
	if _, err = findOwnedWallpaper(userID, src); err != nil {
		return nil, err
	}
	if mtproto.FromBool(in.GetUnsave()) {
		if err = mutateWallpapers(userID, false, func(list []*mtproto.WallPaper) ([]*mtproto.WallPaper, error) {
			kept := make([]*mtproto.WallPaper, 0, len(list))
			for _, w := range list {
				if !wallpaperMatches(w, src) {
					kept = append(kept, w)
				}
			}
			return kept, nil
		}); err != nil {
			return nil, err
		}
		return mtproto.BoolTrue, nil
	}
	if err = upsertWallpaper(userID, src, in.GetSettings()); err != nil {
		return nil, err
	}
	return mtproto.BoolTrue, nil
}

func (c *ApiFullCore) AccountInstallWallPaper(in *mtproto.TLAccountInstallWallPaper) (*mtproto.Bool, error) {
	userID, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil || in.GetWallpaper() == nil {
		return nil, mtproto.ErrWallpaperInvalid
	}
	if _, err = findOwnedWallpaper(userID, in.GetWallpaper()); err != nil {
		return nil, err
	}
	if err = upsertWallpaper(userID, in.GetWallpaper(), in.GetSettings()); err != nil {
		return nil, err
	}
	return mtproto.BoolTrue, nil
}

func (c *ApiFullCore) AccountResetWallPapers(in *mtproto.TLAccountResetWallPapers) (*mtproto.Bool, error) {
	userID, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if err = lookPut(userID, "account.resetWallPapers", in); err != nil {
		return nil, err
	}
	if err = mutateWallpapers(userID, false, func([]*mtproto.WallPaper) ([]*mtproto.WallPaper, error) {
		return []*mtproto.WallPaper{}, nil
	}); err != nil {
		return nil, err
	}
	return mtproto.BoolTrue, nil
}

func (c *ApiFullCore) AccountGetMultiWallPapers(in *mtproto.TLAccountGetMultiWallPapers) (*mtproto.Vector_WallPaper, error) {
	userID, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	out := make([]*mtproto.WallPaper, 0)
	if in != nil {
		for _, src := range in.GetWallpapers() {
			if wallpaper, findErr := findOwnedWallpaper(userID, src); findErr == nil {
				out = append(out, wallpaper)
			}
		}
	}
	return &mtproto.Vector_WallPaper{Datas: out}, nil
}

func (c *ApiFullCore) MessagesSetChatWallPaper(in *mtproto.TLMessagesSetChatWallPaper) (*mtproto.Updates, error) {
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
	peerType, peerID := apifullPeerTypeID(userID, in.GetPeer())
	if peerID <= 0 {
		return nil, mtproto.ErrPeerIdInvalid
	}
	wallpaperID := int64(0)
	if !in.GetRevert() {
		if in.GetWallpaper() == nil {
			return nil, mtproto.ErrWallpaperInvalid
		}
		wallpaper, findErr := findOwnedWallpaper(userID, in.GetWallpaper())
		if findErr != nil {
			return nil, findErr
		}
		wallpaperID = wallpaper.GetId()
	}
	d := c.apifullDao()
	if d == nil || d.DialogClient == nil {
		return nil, mtproto.ErrMethodNotImpl
	}
	if _, err = d.DialogSetChatWallpaper(callContext(c), &dialogpb.TLDialogSetChatWallpaper{
		UserId:              userID,
		PeerType:            peerType,
		PeerId:              peerID,
		WallpaperId:         wallpaperID,
		WallpaperOverridden: in.GetForBoth(),
	}); err != nil {
		return nil, err
	}
	return callUpdates(), nil
}
