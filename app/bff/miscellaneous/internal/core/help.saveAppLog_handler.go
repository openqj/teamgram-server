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
	"context"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/apifull/persist"
)

// HelpSaveAppLog
// help.saveAppLog#6f02f748 events:Vector<InputAppEvent> = Bool;
func (c *MiscellaneousCore) HelpSaveAppLog(in *mtproto.TLHelpSaveAppLog) (*mtproto.Bool, error) {
	var events []*mtproto.InputAppEvent
	if in != nil {
		events = in.GetEvents()
	}
	// The composite BFF opens the shared PostgreSQL store before registering
	// this service. Standalone miscellaneous startup does the same in server.
	// Keep direct unit callers compatible when no service context is present.
	if c != nil && c.svcCtx != nil && c.svcCtx.Config.PostgresDSN != "" {
		userID := int64(0)
		if c.MD != nil {
			userID = c.MD.UserId
		}
		ctx := c.ctx
		if ctx == nil {
			ctx = context.Background()
		}
		if err := persist.SaveAppLogEvents(ctx, userID, events); err != nil {
			if c.Logger != nil {
				c.Logger.Errorf("help.saveAppLog - error: %v", err)
			}
			return nil, err
		}
	}
	if c != nil && c.Logger != nil {
		n := 0
		for _, ev := range events {
			if ev != nil && ev.GetType() != "" {
				n++
			}
		}
		c.Logger.Infof("help.saveAppLog events=%d", n)
	}

	return mtproto.BoolTrue, nil
}
