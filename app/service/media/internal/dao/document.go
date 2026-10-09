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
	"crypto/sha256"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/teamgram/marmota/pkg/hack"
	"github.com/teamgram/proto/mtproto"
	dfs_client "github.com/teamgram/teamgram-server/app/service/dfs/client"
	"github.com/teamgram/teamgram-server/app/service/dfs/dfs"
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
	videoThumbs []*mtproto.VideoSize) error {
	if do == nil {
		document.Id = id
		mtproto.MakeTLDocumentEmpty(&mtproto.Document{
			Id: id,
		})
		return nil
	}
	document.Id = do.DocumentId

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
	var photoSizeErr error

	if do.ThumbId != 0 && do.VideoThumbId != 0 {
		if len(thumbs) > 0 && len(videoThumbs) > 0 {
			document.Thumbs = thumbs
			document.VideoThumbs = videoThumbs
		} else {
			mr.FinishVoid(
				func() {
					document.Thumbs, photoSizeErr = m.GetPhotoSizeListV2(ctx, do.ThumbId)
				},
				func() {
					document.VideoThumbs = m.GetVideoSizeList(ctx, do.VideoThumbId)
				})
			if photoSizeErr != nil {
				return photoSizeErr
			}
		}
	} else {
		// thumbs
		if do.ThumbId != 0 {
			if len(thumbs) > 0 {
				document.Thumbs = thumbs
			} else {
				var err error
				document.Thumbs, err = m.GetPhotoSizeListV2(ctx, do.ThumbId)
				if err != nil {
					return err
				}
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
	return nil
}

func (m *Dao) GetDocumentById(ctx context.Context, id int64) (*mtproto.Document, error) {
	do, err := m.documentsStore().SelectByDocumentId(ctx, id)
	if err != nil {
		logx.WithContext(ctx).Errorf("GetDocumentById(%d) - error: %v", id, err)
		return nil, err
	}
	if do == nil {
		logx.WithContext(ctx).Infof("not found document by id: %d", id)
		return mtproto.MakeTLDocumentEmpty(&mtproto.Document{Id: id}).To_Document(), nil
	}
	document := new(mtproto.Document)
	if err := m.MakeDocumentByDO(ctx, document, id, do, nil, nil); err != nil {
		return nil, err
	}
	return document.FixData(), nil
}

// GetDocumentByHash resolves the canonical Telegram document hash identity.
func (m *Dao) GetDocumentByHash(ctx context.Context, hash []byte, size int64, mimeType string) (*mtproto.Document, error) {
	return m.LookupDocumentByHash(ctx, hash, size, mimeType)
}

func (m *Dao) LookupDocumentByHash(ctx context.Context, hash []byte, size int64, mimeType string) (*mtproto.Document, error) {
	if len(hash) != sha256.Size || size < 0 || mimeType == "" {
		return nil, mtproto.ErrDocumentInvalid
	}
	do, err := m.documentsStore().SelectByHash(ctx, hash, size, mimeType)
	if err != nil {
		return nil, err
	}
	if do == nil {
		return nil, nil
	}
	document := new(mtproto.Document)
	if err := m.MakeDocumentByDO(ctx, document, do.DocumentId, do, nil, nil); err != nil {
		return nil, err
	}
	return document.FixData(), nil
}

func (m *Dao) GetDocumentListByIdList(ctx context.Context, idList []int64) ([]*mtproto.Document, error) {
	if len(idList) == 0 {
		return []*mtproto.Document{}, nil
	}
	doList, err := m.documentsStore().SelectByDocumentIdListWithCB(ctx, idList, nil)
	if err != nil {
		logx.WithContext(ctx).Errorf("findListByIdList - %v", err)
		return nil, err
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
		photoSizeErr           error
	)
	if len(thumbSizeIdList) > 0 && len(videoThumbSizeIdList) > 0 {
		mr.FinishVoid(
			func() {
				thumbSizeListList, photoSizeErr = m.GetPhotoSizeListListE(ctx, thumbSizeIdList)
			},
			func() {
				videoThumbSizeListList = m.GetVideoSizeListList(ctx, videoThumbSizeIdList)
			})
	} else {
		if len(thumbSizeIdList) != 0 {
			thumbSizeListList, photoSizeErr = m.GetPhotoSizeListListE(ctx, thumbSizeIdList)
		}
		if len(videoThumbSizeIdList) != 0 {
			videoThumbSizeListList = m.GetVideoSizeListList(ctx, videoThumbSizeIdList)
		}
	}
	if photoSizeErr != nil {
		return nil, photoSizeErr
	}

	documents := make([]*mtproto.Document, 0, len(idList))
	for _, id := range idList {
		do := doByID[id]
		if do == nil {
			continue
		}
		document := new(mtproto.Document)
		if err := m.MakeDocumentByDO(ctx, document, do.DocumentId, do, thumbSizeListList[do.ThumbId], videoThumbSizeListList[do.VideoThumbId]); err != nil {
			return nil, err
		}
		documents = append(documents, document.FixData())
	}
	return documents, nil
}

func hashDocumentContent(ctx context.Context, client dfs_client.DfsClient, document *mtproto.Document, size int64) ([]byte, error) {
	if document == nil || document.GetId() <= 0 || size < 0 || client == nil {
		return nil, mtproto.ErrMediaInvalid
	}
	hasher := sha256.New()
	for offset := int64(0); offset < size; {
		remaining := size - offset
		limit := int32(128 * 1024)
		if remaining < int64(limit) {
			limit = int32(remaining)
		}
		part, err := client.DfsDownloadFile(ctx, &dfs.TLDfsDownloadFile{
			Location: mtproto.MakeTLInputDocumentFileLocation(&mtproto.InputFileLocation{
				Id: document.Id, AccessHash: document.AccessHash,
			}).To_InputFileLocation(),
			Offset: offset,
			Limit:  limit,
		})
		if err != nil || part == nil {
			if err == nil {
				err = mtproto.ErrMediaInvalid
			}
			return nil, err
		}
		bytes := part.GetBytes()
		if len(bytes) == 0 {
			return nil, mtproto.ErrMediaInvalid
		}
		if int64(len(bytes)) > remaining {
			bytes = bytes[:remaining]
		}
		if _, err = hasher.Write(bytes); err != nil {
			return nil, err
		}
		offset += int64(len(bytes))
	}
	return hasher.Sum(nil), nil
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
	hash, err := hashDocumentContent(ctx, m.DfsClient, document, data.FileSize)
	if err != nil {
		return err
	}
	data.Sha256 = hash

	data.Id, _, err = m.documentsStore().Insert(ctx, data)
	return err
}
