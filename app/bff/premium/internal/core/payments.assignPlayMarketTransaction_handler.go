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
	"github.com/teamgram/proto/mtproto"
)

// PaymentsAssignPlayMarketTransaction
// payments.assignPlayMarketTransaction#dffd50d3 receipt:DataJSON purpose:InputStorePaymentPurpose = Updates;
func (c *PremiumCore) PaymentsAssignPlayMarketTransaction(in *mtproto.TLPaymentsAssignPlayMarketTransaction) (*mtproto.Updates, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil || in.GetReceipt().GetData() == "" {
		return nil, mtproto.ErrPaymentProviderInvalid
	}
	if in.GetPurpose() == nil && premiumPaymentProviderConfigured(c) {
		return nil, mtproto.ErrInputRequestInvalid
	}

	var result premiumUpdatesProviderResult
	if err = c.callPaymentProvider("payments.assignPlayMarketTransaction", map[string]any{"user_id": uid, "receipt": in.GetReceipt(), "purpose": in.GetPurpose(), "purchase_token": in.GetPurchaseToken()}, &result); err != nil {
		return nil, paymentAssignmentError(err)
	}
	if !result.Accepted || !normalizePremiumUpdates(result.Updates) {
		return nil, mtproto.ErrPaymentProviderInvalid
	}
	return result.Updates, nil
}
