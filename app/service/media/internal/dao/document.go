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

package dao

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/teamgram/marmota/pkg/hack"
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/media/internal/dal/dataobject"

	"github.com/zeromicro/go-zero/core/jsonx"
	"github.com/zeromicro/go-zero/core/logx"
	"github.com/zeromicro/go-zero/core/mr"
)

var (
	cacheDocumentPrefix   = "document"
	GenCacheDocumentKey   = genCacheDocumentKey
	ParseCacheDocumentKey = parseCacheDocumentKey
)

func genCacheDocumentKey(id int64) string {
	return fmt.Sprintf("%s_%d", cacheDocumentPrefix, id)
}

func parseCacheDocumentKey(k string) int64 {
	if strings.HasPrefix(k, cacheDocumentPrefix+"_") {
		v, _ := strconv.ParseInt(k[len(cacheDocumentPrefix)+1:], 10, 64)
		return v
	}

	return 0
}

// MakeDocumentByDO
/*
document#1e87342b flags:#
	id:long
	access_hash:long
	file_reference:bytes
	date:int
	mime_type:string
	size:int
	thumbs:flags.0?Vector<PhotoSize>
	video_thumbs:flags.1?Vector<VideoSize>
	dc_id:int
	attributes:Vector<DocumentAttribute> = Document;
*/
func (m *Dao) MakeDocumentByDO(
	ctx context.Context,
	document *mtproto.Document,
	id int64,
	do *dataobject.DocumentsDO,
	thumbs []*mtproto.PhotoSize,
	videoThumbs []*mtproto.VideoSize) {
	document.Id = do.DocumentId

	if do == nil {
		document.Id = id
		mtproto.MakeTLDocumentEmpty(&mtproto.Document{
			Id: id,
		})
		return
	}

	mtproto.MakeTLDocument(document)
	document.AccessHash = do.AccessHash
	document.FileReference = []byte{}
	document.Date = int32(do.Date2)
	if document.Date == 0 {
		document.Date = int32(time.Now().Unix())
	}
	document.MimeType = do.MimeType
	document.Size2_INT32 = int32(do.FileSize)
	document.Size2_INT64 = do.FileSize

	if do.ThumbId != 0 && do.VideoThumbId != 0 {
		if len(thumbs) > 0 && len(videoThumbs) > 0 {
			document.Thumbs = thumbs
			document.VideoThumbs = videoThumbs
		} else {
			mr.FinishVoid(
				func() {
					document.Thumbs = m.GetPhotoSizeListV2(ctx, do.ThumbId)
				},
				func() {
					document.VideoThumbs = m.GetVideoSizeList(ctx, do.VideoThumbId)
				})
		}
	} else {
		// thumbs
		if do.ThumbId != 0 {
			if len(thumbs) > 0 {
				document.Thumbs = thumbs
			} else {
				document.Thumbs = m.GetPhotoSizeListV2(ctx, do.ThumbId)
			}
		}

		// video_thumbs
		if do.VideoThumbId != 0 {
			if len(videoThumbs) > 0 {
				document.VideoThumbs = videoThumbs
			} else {
				document.VideoThumbs = m.GetVideoSizeList(ctx, do.VideoThumbId)
			}
		}
	}

	document.DcId = 1
	err := jsonx.UnmarshalFromString(do.Attributes, &document.Attributes)
	if err != nil {
		logx.WithContext(ctx).Errorf("makeDocumentByDO - error: %v", err)
	}
	if document.Attributes == nil {
		document.Attributes = []*mtproto.DocumentAttribute{}
	}
	document = document.FixData()
}

func (m *Dao) GetDocumentById(ctx context.Context, id int64) *mtproto.Document {
	do, err := m.documentsStore().SelectByDocumentId(ctx, id)
	if err != nil {
		logx.WithContext(ctx).Errorf("GetDocumentById(%d) - error: %v", id, err)
		return mtproto.MakeTLDocumentEmpty(&mtproto.Document{Id: id}).To_Document()
	}
	if do == nil {
		logx.WithContext(ctx).Infof("not found document by id: %d", id)
		return mtproto.MakeTLDocumentEmpty(&mtproto.Document{Id: id}).To_Document()
	}
	document := new(mtproto.Document)
	m.MakeDocumentByDO(ctx, document, id, do, nil, nil)
	return document.FixData()
}

func (m *Dao) GetDocumentListByIdList(ctx context.Context, idList []int64) []*mtproto.Document {
	if len(idList) == 0 {
		return []*mtproto.Document{}
	}
	doList, err := m.documentsStore().SelectByDocumentIdListWithCB(ctx, idList, nil)
	if err != nil {
		logx.WithContext(ctx).Errorf("findListByIdList - %v", err)
		return []*mtproto.Document{}
	}
	var (
		thumbSizeIdList      = make([]int64, 0)
		videoThumbSizeIdList = make([]int64, 0)
	)
	doByID := make(map[int64]*dataobject.DocumentsDO, len(doList))
	for i := range doList {
		do := &doList[i]
		doByID[do.DocumentId] = do
		if do.ThumbId != 0 {
			thumbSizeIdList = append(thumbSizeIdList, do.ThumbId)
		}
		if do.VideoThumbId != 0 {
			videoThumbSizeIdList = append(videoThumbSizeIdList, do.VideoThumbId)
		}
	}

	var (
		thumbSizeListList      map[int64][]*mtproto.PhotoSize
		videoThumbSizeListList map[int64][]*mtproto.VideoSize
	)
	if len(thumbSizeIdList) > 0 && len(videoThumbSizeIdList) > 0 {
		mr.FinishVoid(
			func() {
				thumbSizeListList = m.GetPhotoSizeListList(ctx, thumbSizeIdList)
			},
			func() {
				videoThumbSizeListList = m.GetVideoSizeListList(ctx, videoThumbSizeIdList)
			})
	} else {
		if len(thumbSizeIdList) != 0 {
			thumbSizeListList = m.GetPhotoSizeListList(ctx, thumbSizeIdList)
		}
		if len(videoThumbSizeIdList) != 0 {
			videoThumbSizeListList = m.GetVideoSizeListList(ctx, videoThumbSizeIdList)
		}
	}

	documents := make([]*mtproto.Document, 0, len(idList))
	for _, id := range idList {
		do := doByID[id]
		if do == nil {
			continue
		}
		document := new(mtproto.Document)
		m.MakeDocumentByDO(ctx, document, do.DocumentId, do, thumbSizeListList[do.ThumbId], videoThumbSizeListList[do.VideoThumbId])
		documents = append(documents, document.FixData())
	}
	return documents
}

func (m *Dao) SaveDocumentV2(ctx context.Context, fileName string, document *mtproto.Document) error {
	if document == nil || document.Id == 0 {
		return mtproto.ErrMediaInvalid
	}
	var (
		aStr string
	)

	if document.GetAttributes() != nil {
		aBuf, err := jsonx.Marshal(document.GetAttributes())
		if err != nil {
			return err
		}
		aStr = hack.String(aBuf)
	}

	data := &dataobject.DocumentsDO{
		DocumentId:       document.Id,
		AccessHash:       document.AccessHash,
		DcId:             document.DcId,
		FilePath:         fmt.Sprintf("%d.dat", document.Id),
		FileSize:         document.GetFixedSize(), // TODO: check
		UploadedFileName: fileName,
		Ext:              getFileExtName(fileName),
		MimeType:         document.MimeType,
		ThumbId:          0,
		VideoThumbId:     0,
		Version:          0,
		Attributes:       aStr,
		Date2:            int64(document.Date),
	}
	if len(document.GetThumbs()) > 0 {
		data.ThumbId = document.Id
	}
	if len(document.GetVideoThumbs()) > 0 {
		data.VideoThumbId = document.Id
	}

	var err error
	data.Id, _, err = m.documentsStore().Insert(ctx, data)
	return err
}
