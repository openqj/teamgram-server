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
	"fmt"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/apifull/persist"
)

func autoDownloadKey(userID int64, slot string) string {
	return fmt.Sprintf("auto_download:%d:%s", userID, slot)
}

func autoDownloadSlot(in *mtproto.TLAccountSaveAutoDownloadSettings) string {
	if in.GetLow() {
		return "low"
	}
	if in.GetHigh() {
		return "high"
	}
	return "medium"
}

// AccountSaveAutoDownloadSettings
// account.saveAutoDownloadSettings#76f36233 flags:# low:flags.0?true high:flags.1?true settings:AutoDownloadSettings = Bool;
func (c *AutoDownloadCore) AccountSaveAutoDownloadSettings(in *mtproto.TLAccountSaveAutoDownloadSettings) (*mtproto.Bool, error) {
	settings := in.GetSettings()
	if settings == nil {
		return mtproto.BoolTrue, nil
	}
	raw, err := json.Marshal(settings)
	if err != nil {
		c.Logger.Errorf("account.saveAutoDownloadSettings - error: %v", err)
		return nil, err
	}
	if err = persist.Default.Set(autoDownloadKey(c.MD.UserId, autoDownloadSlot(in)), string(raw)); err != nil {
		c.Logger.Errorf("account.saveAutoDownloadSettings - error: %v", err)
		return nil, err
	}
	return mtproto.BoolTrue, nil
}
