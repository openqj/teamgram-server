// Copyright 2026 Teamgram Authors
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
	"strconv"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/domain"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/persist"
)

// RPCGatewayVerificationMessagesServer: Layer 229 methods previously returned ERR_ENTERPRISE_IS_BLOCKED.

func (c *ApiFullCore) MessagesReportMessagesDelivery(in *mtproto.TLMessagesReportMessagesDelivery) (*mtproto.Bool, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	ids := []int32(nil)
	if in != nil {
		ids = in.GetId()
	}
	if len(ids) == 0 {
		return nil, mtproto.ErrMessageIdInvalid
	}
	for _, id := range ids {
		if id <= 0 {
			return nil, mtproto.ErrMessageIdInvalid
		}
	}
	if domain.Ready() {
		ctx := c.ctx
		if ctx == nil {
			ctx = context.Background()
		}
		if err = domain.RecordMessagesDelivery(ctx, uid, ids); err != nil {
			return nil, err
		}
	}
	// Keep the compatibility mirror after validation and the durable write.
	// Invalid requests or failed PostgreSQL writes must not leave state.
	if err = restPut(uid, "messages.reportMessagesDelivery", in); err != nil {
		return nil, err
	}
	if err = persist.Default.Set(b18Key(uid, "delivery"), strconv.FormatInt(int64(ids[0]), 10)); err != nil {
		return nil, err
	}
	return mtproto.BoolTrue, nil
}
