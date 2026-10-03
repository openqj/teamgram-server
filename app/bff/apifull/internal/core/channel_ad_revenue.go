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
	"github.com/teamgram/proto/mtproto"
)

// RPCChannelAdRevenueServer: Layer 229 methods previously returned ERR_ENTERPRISE_IS_BLOCKED.

func (c *ApiFullCore) StatsGetBroadcastRevenueStats(in *mtproto.TLStatsGetBroadcastRevenueStats) (*mtproto.Stats_BroadcastRevenueStats, error) {
	if _, err := c.requireUserId(); err != nil {
		return nil, err
	}
	_ = in
	return nil, mtproto.ErrMethodNotImpl
}

func (c *ApiFullCore) StatsGetBroadcastRevenueWithdrawalUrl(in *mtproto.TLStatsGetBroadcastRevenueWithdrawalUrl) (*mtproto.Stats_BroadcastRevenueWithdrawalUrl, error) {
	if _, err := c.requireUserId(); err != nil {
		return nil, err
	}
	if in == nil {
		return nil, mtproto.ErrPeerIdInvalid
	}
	if in.GetConstructor() == mtproto.TLConstructor_CRC32_stats_getBroadcastRevenueWithdrawalUrl_2a65ef73 {
		if inputChannelID(in.GetChannel()) == 0 {
			return nil, mtproto.ErrChannelInvalid
		}
		return nil, mtproto.ErrMethodNotImpl
	}
	if in.GetPeer() == nil || statsInputPeerID(in.GetPeer()) == 0 {
		return nil, mtproto.ErrPeerIdInvalid
	}
	password := in.GetPassword()
	if password == nil || password.GetPredicateName() != mtproto.Predicate_inputCheckPasswordSRP ||
		password.GetSrpId() == 0 || len(password.GetA()) == 0 || len(password.GetM1()) == 0 {
		return nil, mtproto.ErrPasswordHashInvalid
	}
	return nil, mtproto.ErrMethodNotImpl
}

func (c *ApiFullCore) StatsGetBroadcastRevenueTransactions(in *mtproto.TLStatsGetBroadcastRevenueTransactions) (*mtproto.Stats_BroadcastRevenueTransactions, error) {
	if _, err := c.requireUserId(); err != nil {
		return nil, err
	}
	_ = in
	return nil, mtproto.ErrMethodNotImpl
}
