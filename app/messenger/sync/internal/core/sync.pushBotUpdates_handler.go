// Copyright 2025 Teamgram Authors
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

package core

import (
	"context"
	"crypto/sha256"
	"fmt"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/messenger/sync/internal/dao"
	"github.com/teamgram/teamgram-server/app/messenger/sync/sync"
)

// SyncPushBotUpdates
// sync.pushBotUpdates user_id:long updates:Updates = Void;
func (c *SyncCore) SyncPushBotUpdates(in *sync.TLSyncPushBotUpdates) (*mtproto.Void, error) {
	if in == nil || in.GetUserId() <= 0 {
		return nil, mtproto.ErrUserIdInvalid
	}
	if in.GetUpdates() == nil {
		return nil, mtproto.ErrMethodNotImpl
	}
	if c == nil || c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.StatusClient == nil {
		return nil, mtproto.ErrMethodNotImpl
	}
	if c.svcCtx.Dao.Postgres == nil || c.svcCtx.Dao.Postgres.BotAPI == nil {
		return nil, mtproto.ErrMethodNotImpl
	}
	requestCore := c
	if _, ok := dao.DeliveryReceiptFromContext(c.ctx); !ok {
		if receipt, ok := botPaymentDeliveryReceipt(in.GetUpdates()); ok {
			clone := *c
			clone.ctx = dao.WithDeliveryReceipt(c.ctx, receipt)
			requestCore = &clone
		}
	}
	_, err := requestCore.svcCtx.Dao.Postgres.BotAPI.AppendOnce(requestCore.ctx, in.GetUserId(), in.GetUpdates())
	if err != nil {
		return nil, err
	}
	go c.svcCtx.Dao.Postgres.BotAPI.DeliverWebhook(context.Background(), in.GetUserId())

	notification, err := requestCore.processUpdates(syncTypeUser, in.GetUserId(), true, in.GetUpdates())
	if err != nil {
		return nil, err
	}
	if err := requestCore.pushUpdatesToSession(syncTypeUser, in.GetUserId(), 0, nil, nil, nil, in.GetUpdates(), notification); err != nil {
		return nil, err
	}
	if err := requestCore.handleBuiltinBotUpdates(requestCore.ctx, in.GetUserId(), in.GetUpdates()); err != nil {
		return nil, err
	}

	return mtproto.EmptyVoid, nil
}

// botPaymentDeliveryReceipt gives a payment query a stable sync receipt so a
// retry after a successful fan-out is a no-op, while a retry after a failed
// fan-out can reuse the committed prepared updates. The reconciler emits one
// query update per outbox item; mixed batches intentionally do not get this
// identity and retain the ordinary sync behavior.
func botPaymentDeliveryReceipt(updates *mtproto.Updates) (dao.DeliveryReceipt, bool) {
	if updates == nil {
		return dao.DeliveryReceipt{}, false
	}
	var update *mtproto.Update
	switch updates.GetPredicateName() {
	case mtproto.Predicate_updates, mtproto.Predicate_updatesCombined:
		if len(updates.GetUpdates()) != 1 {
			return dao.DeliveryReceipt{}, false
		}
		update = updates.GetUpdates()[0]
	case mtproto.Predicate_updateShort:
		update = updates.GetUpdate()
	default:
		return dao.DeliveryReceipt{}, false
	}
	if update == nil || update.GetQueryId_INT64() <= 0 {
		return dao.DeliveryReceipt{}, false
	}
	var kind string
	switch update.GetPredicateName() {
	case mtproto.Predicate_updateBotShippingQuery:
		kind = "shipping"
	case mtproto.Predicate_updateBotPrecheckoutQuery:
		kind = "precheckout"
	default:
		return dao.DeliveryReceipt{}, false
	}
	queryID := update.GetQueryId_INT64()
	hash := sha256.Sum256([]byte(fmt.Sprintf("bot-payment:%s:%d", kind, queryID)))
	return dao.DeliveryReceipt{
		ConsumerGroup: "bot-payment-reconciler",
		Topic:         "bot-api-" + kind + "-query",
		Partition:     0,
		Offset:        queryID,
		RequestHash:   hash[:],
	}, true
}
