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
	sharedpersist "github.com/teamgram/teamgram-server/app/bff/apifull/persist"
)

// HelpGetPremiumPromo
// help.getPremiumPromo#b81b93d4 = help.PremiumPromo;
func (c *PremiumCore) HelpGetPremiumPromo(in *mtproto.TLHelpGetPremiumPromo) (*mtproto.Help_PremiumPromo, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	if !premiumPaymentProviderConfigured(c) && sharedpersist.PostgresEnabled() {
		ctx := c.ctx
		if ctx == nil {
			ctx = context.Background()
		}
		promo, found, loadErr := sharedpersist.LoadPremiumPromo(ctx)
		if loadErr != nil {
			return nil, mtproto.ErrInternalServerError
		}
		if found {
			if !normalizePremiumPromo(promo) {
				return nil, mtproto.ErrPaymentProviderInvalid
			}
			return promo, nil
		}
	}
	var result premiumPromoProviderResult
	if err = c.callPaymentProvider("help.getPremiumPromo", map[string]any{"user_id": uid}, &result); err != nil {
		return nil, err
	}
	if !normalizePremiumPromo(result.Promo) {
		return nil, mtproto.ErrPaymentProviderInvalid
	}
	return result.Promo, nil
}
