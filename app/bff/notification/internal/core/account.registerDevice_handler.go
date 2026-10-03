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

type pushDevice struct {
	TokenType  int32   `json:"token_type"`
	Token      string  `json:"token"`
	NoMuted    bool    `json:"no_muted"`
	AppSandbox bool    `json:"app_sandbox"`
	Secret     []byte  `json:"secret,omitempty"`
	OtherUids  []int64 `json:"other_uids,omitempty"`
}

func deviceStoreKey(userID int64, tokenType int32, token string) string {
	return fmt.Sprintf("device:%d:%d:%s", userID, tokenType, token)
}

func validateDeviceOwnerIDs(self int64, other []int64) error {
	for _, id := range other {
		if id != 0 && id != self {
			return mtproto.ErrUserIdInvalid
		}
	}

	return nil
}

func deviceUserID(c *NotificationCore) (int64, error) {
	if c == nil || c.MD == nil || c.MD.GetUserId() <= 0 {
		return 0, mtproto.ErrAuthKeyUnregistered
	}
	return c.MD.GetUserId(), nil
}

// AccountRegisterDevice
// account.registerDevice#ec86017a flags:# no_muted:flags.0?true token_type:int token:string app_sandbox:Bool secret:bytes other_uids:Vector<long> = Bool;
func (c *NotificationCore) AccountRegisterDevice(in *mtproto.TLAccountRegisterDevice) (*mtproto.Bool, error) {
	userID, err := deviceUserID(c)
	if err != nil {
		return nil, err
	}
	if err = validateDeviceOwnerIDs(userID, in.GetOtherUids()); err != nil {
		return nil, err
	}

	raw, err := json.Marshal(pushDevice{
		TokenType:  in.GetTokenType(),
		Token:      in.GetToken(),
		NoMuted:    in.GetNoMuted(),
		AppSandbox: mtproto.FromBool(in.GetAppSandbox()),
		Secret:     in.GetSecret(),
		OtherUids:  in.GetOtherUids(),
	})
	if err != nil {
		c.Logger.Errorf("account.registerDevice - error: %v", err)
		return nil, err
	}

	if err = persist.Default.Set(deviceStoreKey(userID, in.GetTokenType(), in.GetToken()), string(raw)); err != nil {
		c.Logger.Errorf("account.registerDevice - error: %v", err)
		return nil, err
	}

	return mtproto.BoolTrue, nil
}
