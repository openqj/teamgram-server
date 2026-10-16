// Copyright 2022 Teamgram Authors
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
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/messenger/sync/sync"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
	mediapb "github.com/teamgram/teamgram-server/app/service/media/media"
	"github.com/teamgram/teamgram-server/pkg/rpc/businessctx"
)

// PhotosDeletePhotos
// photos.deletePhotos#87cf7f2f id:Vector<InputPhoto> = Vector<long>;
func (c *UserChannelProfilesCore) PhotosDeletePhotos(in *mtproto.TLPhotosDeletePhotos) (*mtproto.Vector_Long, error) {
	if c == nil || c.MD == nil || c.MD.UserId <= 0 {
		return nil, mtproto.ErrAuthKeyUnregistered
	}
	if in == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	actorID := c.MD.UserId
	if scope, ok := businessctx.ScopeFromContext(c.ctx); ok {
		if scope.OwnerID <= 0 || !scope.CanEditPhoto {
			return nil, mtproto.ErrChatAdminRequired
		}
		actorID = scope.OwnerID
	}
	var (
		photo        *mtproto.Photo
		deleteIdList = make([]int64, 0, len(in.GetId()))
	)

	for _, id := range in.GetId() {
		if id == nil || id.GetId() <= 0 {
			return nil, mtproto.ErrPhotoIdInvalid
		}
		deleteIdList = append(deleteIdList, id.GetId())
	}
	if len(deleteIdList) == 0 {
		return &mtproto.Vector_Long{Datas: deleteIdList}, nil
	}
	if c.svcCtx == nil || c.svcCtx.Dao == nil ||
		c.svcCtx.Dao.UserClient == nil || c.svcCtx.Dao.MediaClient == nil ||
		c.svcCtx.Dao.SyncClient == nil {
		return nil, mtproto.ErrInternalServerError
	}

	// TODO: ALBUM_PHOTOS_TOO_MANY
	mainId, err := c.svcCtx.Dao.UserClient.UserDeleteProfilePhotos(c.ctx, &userpb.TLUserDeleteProfilePhotos{
		UserId: actorID,
		Id:     deleteIdList,
	})
	if err != nil {
		c.Logger.Errorf("photos.deletePhotos - error: %v", err)
		return nil, err
	}
	if mainId == nil {
		return nil, mtproto.ErrInternalServerError
	}

	if mainId.V != 0 {
		photo, err = c.svcCtx.Dao.MediaClient.MediaGetPhoto(c.ctx, &mediapb.TLMediaGetPhoto{
			PhotoId: mainId.V,
		})
		if err != nil {
			c.Logger.Errorf("photos.deletePhotos - error: %v", err)
			return nil, err
		}
		if photo == nil {
			return nil, mtproto.ErrInternalServerError
		}
	}

	if photo == nil {
		photo = mtproto.MakeTLPhotoEmpty(nil).To_Photo()
	}

	me, err := c.svcCtx.Dao.UserClient.UserGetImmutableUser(
		c.ctx,
		&userpb.TLUserGetImmutableUser{
			Id:       actorID,
			Privacy:  false,
			Contacts: nil,
		})
	if err != nil {
		c.Logger.Errorf("photos.updateProfilePhoto - error: %v", err)
		return nil, err
	}
	if me == nil {
		return nil, mtproto.ErrInternalServerError
	}

	syncReply, err := c.svcCtx.Dao.SyncClient.SyncPushUpdates(c.ctx, &sync.TLSyncPushUpdates{
		UserId: actorID,
		Updates: mtproto.MakeUpdatesByUpdatesUsers(
			[]*mtproto.User{me.ToSelfUser()},
			mtproto.MakeTLUpdateUser(&mtproto.Update{
				UserId: actorID,
			}).To_Update()),
	})
	if err != nil {
		c.Logger.Errorf("photos.deletePhotos - sync error: %v", err)
		return nil, err
	}
	if syncReply == nil {
		return nil, mtproto.ErrInternalServerError
	}

	return &mtproto.Vector_Long{
		Datas: deleteIdList,
	}, nil
}
