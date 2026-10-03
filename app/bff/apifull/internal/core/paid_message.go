// Copyright 2026 Teamgram Authors
//  All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//
// Author: teamgramio (teamgram.io@gmail.com)

package core

import "github.com/teamgram/proto/mtproto"

// RPCPaidMessageServer: Layer 229 methods previously returned ERR_ENTERPRISE_IS_BLOCKED.

func (c *ApiFullCore) paidMessagesUnavailable() error {
	if _, err := c.requireUserId(); err != nil {
		return err
	}
	return mtproto.ErrMethodNotImpl
}

func (c *ApiFullCore) AccountGetPaidMessagesRevenue(in *mtproto.TLAccountGetPaidMessagesRevenue) (*mtproto.Account_PaidMessagesRevenue, error) {
	_ = in
	return nil, c.paidMessagesUnavailable()
}

func (c *ApiFullCore) AccountToggleNoPaidMessagesException(in *mtproto.TLAccountToggleNoPaidMessagesException) (*mtproto.Bool, error) {
	_ = in
	return nil, c.paidMessagesUnavailable()
}

func (c *ApiFullCore) ChannelsUpdatePaidMessagesPrice(in *mtproto.TLChannelsUpdatePaidMessagesPrice) (*mtproto.Updates, error) {
	_ = in
	return nil, c.paidMessagesUnavailable()
}

func (c *ApiFullCore) AccountAddNoPaidMessagesException(in *mtproto.TLAccountAddNoPaidMessagesException) (*mtproto.Bool, error) {
	_ = in
	return nil, c.paidMessagesUnavailable()
}
