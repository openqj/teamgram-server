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

// RPCStarSubscriptionsServer: Layer 229 methods previously returned ERR_ENTERPRISE_IS_BLOCKED.

func (c *ApiFullCore) starSubscriptionsUnavailable() error {
	if _, err := c.requireUserId(); err != nil {
		return err
	}
	return mtproto.ErrMethodNotImpl
}

func (c *ApiFullCore) PaymentsGetStarsSubscriptions(in *mtproto.TLPaymentsGetStarsSubscriptions) (*mtproto.Payments_StarsStatus, error) {
	_ = in
	return nil, c.starSubscriptionsUnavailable()
}

func (c *ApiFullCore) PaymentsChangeStarsSubscription(in *mtproto.TLPaymentsChangeStarsSubscription) (*mtproto.Bool, error) {
	_ = in
	return nil, c.starSubscriptionsUnavailable()
}

func (c *ApiFullCore) PaymentsFulfillStarsSubscription(in *mtproto.TLPaymentsFulfillStarsSubscription) (*mtproto.Bool, error) {
	_ = in
	return nil, c.starSubscriptionsUnavailable()
}

func (c *ApiFullCore) PaymentsBotCancelStarsSubscription(in *mtproto.TLPaymentsBotCancelStarsSubscription) (*mtproto.Bool, error) {
	_ = in
	return nil, c.starSubscriptionsUnavailable()
}
