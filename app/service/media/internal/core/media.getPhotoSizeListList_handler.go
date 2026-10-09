/*
 * Created from 'scheme.tl' by 'mtprotoc'
 *
 * Copyright (c) 2021-present,  Teamgram Studio (https://teamgram.io).
 *  All rights reserved.
 *
 * Author: teamgramio (teamgram.io@gmail.com)
 */

package core

import (
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/media/media"
)

// MediaGetPhotoSizeListList
// media.getPhotoSizeListList id_list:Vector<long> = Vector<PhotoSizeList>;
func (c *MediaCore) MediaGetPhotoSizeListList(in *media.TLMediaGetPhotoSizeListList) (*media.Vector_PhotoSizeList, error) {
	if in == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	szListList, err := c.svcCtx.Dao.GetPhotoSizeListListE(c.ctx, in.GetIdList())
	if err != nil {
		c.Logger.Errorf("media.getPhotoSizeListList - error: %v", err)
		return nil, err
	}
	photoSizeListList := &media.Vector_PhotoSizeList{
		Datas: photoSizeListsInRequestOrder(in.GetIdList(), szListList),
	}
	return photoSizeListList, nil
}

func photoSizeListsInRequestOrder(idList []int64, sizesByID map[int64][]*mtproto.PhotoSize) []*media.PhotoSizeList {
	result := make([]*media.PhotoSizeList, 0, len(sizesByID))
	seen := make(map[int64]struct{}, len(idList))
	for _, id := range idList {
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		if sizes, ok := sizesByID[id]; ok {
			result = append(result, &media.PhotoSizeList{SizeId: id, Sizes: sizes, DcId: 1})
		}
	}
	return result
}
