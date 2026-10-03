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

// RPCGiftCollectionsServer: Layer 229 methods previously returned ERR_ENTERPRISE_IS_BLOCKED.

func (c *ApiFullCore) giftCollectionsUnavailable() error {
	if _, err := c.requireUserId(); err != nil {
		return err
	}
	return mtproto.ErrMethodNotImpl
}

func (c *ApiFullCore) PaymentsCreateStarGiftCollection(in *mtproto.TLPaymentsCreateStarGiftCollection) (*mtproto.StarGiftCollection, error) {
	_ = in
	return nil, c.giftCollectionsUnavailable()
}

func (c *ApiFullCore) PaymentsUpdateStarGiftCollection(in *mtproto.TLPaymentsUpdateStarGiftCollection) (*mtproto.StarGiftCollection, error) {
	_ = in
	return nil, c.giftCollectionsUnavailable()
}

func (c *ApiFullCore) PaymentsReorderStarGiftCollections(in *mtproto.TLPaymentsReorderStarGiftCollections) (*mtproto.Bool, error) {
	_ = in
	return nil, c.giftCollectionsUnavailable()
}

func (c *ApiFullCore) PaymentsDeleteStarGiftCollection(in *mtproto.TLPaymentsDeleteStarGiftCollection) (*mtproto.Bool, error) {
	_ = in
	return nil, c.giftCollectionsUnavailable()
}

func (c *ApiFullCore) PaymentsGetStarGiftCollections(in *mtproto.TLPaymentsGetStarGiftCollections) (*mtproto.Payments_StarGiftCollections, error) {
	_ = in
	return nil, c.giftCollectionsUnavailable()
}
