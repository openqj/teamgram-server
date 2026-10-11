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
	"encoding/json"
	"fmt"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/media/internal/dal/dataobject"

	"github.com/zeromicro/go-zero/core/jsonx"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// GetVideoSizeListListE reads video thumbnails and preserves storage or decode
// failures. A missing/corrupt thumbnail must not be turned into a successful
// response with silently incomplete media metadata.
func (m *Dao) GetVideoSizeListListE(ctx context.Context, idList []int64) (map[int64][]*mtproto.VideoSize, error) {
	sizes := make(map[int64][]*mtproto.VideoSize)
	if len(idList) == 0 {
		return sizes, nil
	}

	var decodeErr error
	_, err := m.videoSizesStore().SelectListByVideoSizeIdListWithCB(
		ctx,
		idList,
		func(sz, i int, v *dataobject.VideoSizesDO) {
			decoded, currentErr := getVideoSizeE(v)
			if currentErr != nil {
				// The callback API cannot return an error directly. Keep the
				// first one in the closure and let the caller inspect it below.
				if decodeErr == nil {
					decodeErr = currentErr
				}
				return
			}
			sizes[v.VideoSizeId] = append(sizes[v.VideoSizeId], decoded)
		})
	if err != nil {
		return nil, err
	}
	if decodeErr != nil {
		return nil, decodeErr
	}
	return sizes, nil
}

// GetVideoSizeListList retains the historical best-effort helper for callers
// that cannot return an error. Production hydration uses the E variant above.
func (m *Dao) GetVideoSizeListList(ctx context.Context, idList []int64) (sizes map[int64][]*mtproto.VideoSize) {
	sizes, _ = m.GetVideoSizeListListE(ctx, idList)
	return
}

// GetVideoSizeListE returns all video sizes for one media identity and
// propagates both query and malformed JSON errors.
func (m *Dao) GetVideoSizeListE(ctx context.Context, sizeId int64) ([]*mtproto.VideoSize, error) {
	sizes := make([]*mtproto.VideoSize, 0, 2)
	var decodeErr error
	_, err := m.videoSizesStore().SelectListByVideoSizeIdWithCB(
		ctx,
		sizeId,
		func(sz, i int, v *dataobject.VideoSizesDO) {
			decoded, currentErr := getVideoSizeE(v)
			if currentErr != nil {
				if decodeErr == nil {
					decodeErr = currentErr
				}
				return
			}
			if decoded != nil {
				sizes = append(sizes, decoded)
			}
		})
	if err != nil {
		return nil, err
	}
	if decodeErr != nil {
		return nil, decodeErr
	}
	return sizes, nil
}

// GetVideoSizeList retains the historical best-effort helper for callers that
// cannot return an error. Production hydration uses GetVideoSizeListE.
func (m *Dao) GetVideoSizeList(ctx context.Context, sizeId int64) (sizes []*mtproto.VideoSize) {
	sizes, _ = m.GetVideoSizeListE(ctx, sizeId)
	return
}

func getVideoSize(sz *dataobject.VideoSizesDO) *mtproto.VideoSize {
	videoSize, _ := getVideoSizeE(sz)
	return videoSize
}

func getVideoSizeE(sz *dataobject.VideoSizesDO) (*mtproto.VideoSize, error) {
	if sz == nil {
		return nil, nil
	}
	var (
		videoSize *mtproto.VideoSize
	)

	switch sz.SizeType {
	case "e":
		err := jsonx.UnmarshalFromString(sz.FilePath, &videoSize)
		if err != nil {
			return nil, fmt.Errorf("media: decode emoji video size: %w", err)
		}
	case "s":
		err := jsonx.UnmarshalFromString(sz.FilePath, &videoSize)
		if err != nil {
			return nil, fmt.Errorf("media: decode sticker video size: %w", err)
		}
	default:
		videoSize = mtproto.MakeTLVideoSize(&mtproto.VideoSize{
			Type:         sz.SizeType,
			W:            sz.Width,
			H:            sz.Height,
			Size2:        sz.FileSize,
			VideoStartTs: nil,
		}).To_VideoSize()
		if sz.VideoStartTs > 0 {
			videoSize.VideoStartTs = &wrapperspb.DoubleValue{Value: sz.VideoStartTs}
		}
	}
	if videoSize == nil {
		return nil, fmt.Errorf("media: empty video size for type %q", sz.SizeType)
	}
	return videoSize, nil
}

func (m *Dao) SaveVideoSizeV2(ctx context.Context, szId int64, szList []*mtproto.VideoSize) error {
	sizeDOList, err := makeVideoSizesDO(szId, szList)
	if err != nil {
		return err
	}
	for _, szDO := range sizeDOList {
		if _, _, err := m.videoSizesStore().Insert(ctx, szDO); err != nil {
			return err
		}
	}

	return nil
}

func makeVideoSizesDO(szId int64, szList []*mtproto.VideoSize) ([]*dataobject.VideoSizesDO, error) {
	result := make([]*dataobject.VideoSizesDO, 0, len(szList))
	for _, sz := range szList {
		if sz == nil {
			continue
		}
		var szDO *dataobject.VideoSizesDO
		switch sz.GetPredicateName() {
		case "videoSizeEmojiMarkup", "videoSizeStickerMarkup":
			data, err := json.Marshal(sz)
			if err != nil {
				return nil, err
			}
			sizeType := "e"
			if sz.GetPredicateName() == "videoSizeStickerMarkup" {
				sizeType = "s"
			}
			szDO = &dataobject.VideoSizesDO{VideoSizeId: szId, SizeType: sizeType, FilePath: string(data)}
		default:
			szDO = &dataobject.VideoSizesDO{
				VideoSizeId:  szId,
				SizeType:     sz.Type,
				Width:        sz.W,
				Height:       sz.H,
				FileSize:     sz.Size2,
				VideoStartTs: sz.GetVideoStartTs().GetValue(),
				FilePath:     fmt.Sprintf("%s/%d.dat", sz.Type, szId),
			}
		}
		result = append(result, szDO)
	}
	return result, nil
}
