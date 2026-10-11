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

import "github.com/teamgram/proto/mtproto"

// PaymentsCanPurchasePremium
// payments.canPurchasePremium#9fc19eb6 purpose:InputStorePaymentPurpose = Bool;
func (c *PremiumCore) PaymentsCanPurchasePremium(in *mtproto.TLPaymentsCanPurchasePremium) (*mtproto.Bool, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil || in.GetPurpose() == nil {
		if !premiumPaymentProviderConfigured(c) {
			return nil, mtproto.ErrMethodNotImpl
		}
		return nil, mtproto.ErrInputRequestInvalid
	}
	var result premiumBoolProviderResult
	if err = c.callPaymentProvider("payments.canPurchasePremium", map[string]any{"user_id": uid, "purpose": in.GetPurpose()}, &result); err != nil {
		return nil, err
	}
	if result.Bool != nil {
		if !normalizePremiumBool(result.Bool) {
			return nil, mtproto.ErrPaymentProviderInvalid
		}
		return result.Bool, nil
	}
	if result.Allowed {
		return mtproto.BoolTrue, nil
	}
	return mtproto.BoolFalse, nil
}
