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
	"strconv"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/persist"
)

// RPCGatewayVerificationMessagesServer: Layer 229 methods previously returned ERR_ENTERPRISE_IS_BLOCKED.

func (c *ApiFullCore) MessagesReportMessagesDelivery(in *mtproto.TLMessagesReportMessagesDelivery) (*mtproto.Bool, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if err = restPut(uid, "messages.reportMessagesDelivery", in); err != nil {
		return nil, err
	}
	id := ""
	if in != nil && len(in.GetId()) > 0 {
		id = strconv.FormatInt(int64(in.GetId()[0]), 10)
	}
	if err = persist.Default.Set(b18Key(uid, "delivery"), id); err != nil {
		return nil, err
	}
	return mtproto.BoolTrue, nil
}
