// Copyright 2025 Teamgram Authors
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
)

// UsersSuggestBirthday
// users.suggestBirthday#fc533372 id:InputUser birthday:Birthday = Updates;
func (c *UserChannelProfilesCore) UsersSuggestBirthday(in *mtproto.TLUsersSuggestBirthday) (*mtproto.Updates, error) {
	if c == nil || c.MD == nil || c.MD.UserId <= 0 {
		return nil, mtproto.ErrAuthKeyUnregistered
	}
	if in == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	targetID, err := otherUserPeer(c.MD.UserId, in.GetId())
	if err != nil {
		c.Logger.Errorf("users.suggestBirthday - error: %v", err)
		return nil, err
	}
	if in.GetBirthday() == nil {
		c.Logger.Errorf("users.suggestBirthday - error: empty birthday")
		return nil, mtproto.ErrInputRequestInvalid
	}
	return c.callBirthdaySuggestionProvider(c.ctx, birthdaySuggestionProviderRequest{
		CallerID: c.MD.UserId, TargetID: targetID, Recipient: in.GetId(), Birthday: in.GetBirthday(),
	})
}
