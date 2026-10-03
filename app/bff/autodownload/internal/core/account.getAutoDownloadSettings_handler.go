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
	"encoding/json"
	"math"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/apifull/persist"
)

// AccountGetAutoDownloadSettings
// account.getAutoDownloadSettings#56da0b3f = account.AutoDownloadSettings;
func (c *AutoDownloadCore) AccountGetAutoDownloadSettings(in *mtproto.TLAccountGetAutoDownloadSettings) (*mtproto.Account_AutoDownloadSettings, error) {
	_ = in

	makeAutoDownloadSettings := func(disabled, videoPreloadLarge, audioPreloadNext, phonecallsLessData bool, photoSizeMax, videoSizeMax, fileSizeMax int32) *mtproto.AutoDownloadSettings {
		return mtproto.MakeTLAutoDownloadSettings(&mtproto.AutoDownloadSettings{
			Disabled:           disabled,
			VideoPreloadLarge:  videoPreloadLarge,
			AudioPreloadNext:   audioPreloadNext,
			PhonecallsLessData: phonecallsLessData,
			PhotoSizeMax:       photoSizeMax,
			VideoSizeMax_INT32: videoSizeMax,
			VideoSizeMax_INT64: int64(videoSizeMax),
			FileSizeMax_INT32:  fileSizeMax,
			FileSizeMax_INT64:  int64(fileSizeMax),
		}).To_AutoDownloadSettings()
	}

	return mtproto.MakeTLAccountAutoDownloadSettings(&mtproto.Account_AutoDownloadSettings{
		Low: loadAutoDownload(c.MD.UserId, "low", makeAutoDownloadSettings(
			false,
			true,
			true,
			true,
			1048576,
			512000,
			512000)),
		Medium: loadAutoDownload(c.MD.UserId, "medium", makeAutoDownloadSettings(
			false,
			true,
			true,
			false,
			1048576,
			10485760,
			1048576)),
		High: loadAutoDownload(c.MD.UserId, "high", makeAutoDownloadSettings(
			false,
			true,
			true,
			false,
			1048576,
			15728640,
			3145728)),
	}).To_Account_AutoDownloadSettings(), nil
}

func loadAutoDownload(userID int64, slot string, fallback *mtproto.AutoDownloadSettings) *mtproto.AutoDownloadSettings {
	if persist.Default == nil {
		return fallback
	}
	raw, err := persist.Default.Get(autoDownloadKey(userID, slot))
	if err != nil || raw == "" {
		return fallback
	}
	var saved mtproto.AutoDownloadSettings
	if err = json.Unmarshal([]byte(raw), &saved); err != nil {
		return fallback
	}
	return normalizeAutoDownload(&saved)
}

func normalizeAutoDownload(s *mtproto.AutoDownloadSettings) *mtproto.AutoDownloadSettings {
	video := s.GetVideoSizeMax_INT64()
	if video == 0 {
		video = int64(s.GetVideoSizeMax_INT32())
	}
	file := s.GetFileSizeMax_INT64()
	if file == 0 {
		file = int64(s.GetFileSizeMax_INT32())
	}
	video32 := s.GetVideoSizeMax_INT32()
	if video32 == 0 && video <= math.MaxInt32 {
		video32 = int32(video)
	}
	file32 := s.GetFileSizeMax_INT32()
	if file32 == 0 && file <= math.MaxInt32 {
		file32 = int32(file)
	}
	return mtproto.MakeTLAutoDownloadSettings(&mtproto.AutoDownloadSettings{
		Disabled:                      s.GetDisabled(),
		VideoPreloadLarge:             s.GetVideoPreloadLarge(),
		AudioPreloadNext:              s.GetAudioPreloadNext(),
		PhonecallsLessData:            s.GetPhonecallsLessData(),
		StoriesPreload:                s.GetStoriesPreload(),
		PhotoSizeMax:                  s.GetPhotoSizeMax(),
		VideoSizeMax_INT32:            video32,
		VideoSizeMax_INT64:            video,
		FileSizeMax_INT32:             file32,
		FileSizeMax_INT64:             file,
		VideoUploadMaxbitrate:         s.GetVideoUploadMaxbitrate(),
		SmallQueueActiveOperationsMax: s.GetSmallQueueActiveOperationsMax(),
		LargeQueueActiveOperationsMax: s.GetLargeQueueActiveOperationsMax(),
	}).To_AutoDownloadSettings()
}
