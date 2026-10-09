// Copyright (c) 2021-present,  Teamgram Studio (https://teamgram.io).
//  All rights reserved.
//
// Author: teamgramio (teamgram.io@gmail.com)
//

package mq

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"

	kafka "github.com/teamgram/marmota/pkg/mq"
	"github.com/teamgram/teamgram-server/app/messenger/sync/internal/core"
	"github.com/teamgram/teamgram-server/app/messenger/sync/internal/dao"
	"github.com/teamgram/teamgram-server/app/messenger/sync/internal/svc"
	"github.com/teamgram/teamgram-server/app/messenger/sync/sync"
	"github.com/teamgram/teamgram-server/pkg/mqconsumer"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

func New(svcCtx *svc.ServiceContext, conf kafka.KafkaConsumerConf) (*mqconsumer.Consumer, error) {
	return mqconsumer.New(conf, func(ctx context.Context, method, key string, value []byte) error {
		if metadata, ok := mqconsumer.MetadataFromContext(ctx); ok {
			hash := sha256.Sum256(append(append([]byte(method), 0), value...))
			ctx = dao.WithDeliveryReceipt(ctx, dao.DeliveryReceipt{
				ConsumerGroup: metadata.ConsumerGroup, Topic: metadata.Topic,
				Partition: metadata.Partition, Offset: metadata.Offset, RequestHash: hash[:],
			})
		}
		return handleMessage(ctx, svcCtx, method, key, value)
	})
}

func handleMessage(ctx context.Context, svcCtx *svc.ServiceContext, method, key string, value []byte) error {
	var request proto.Message
	switch protoreflect.FullName(method) {
	case proto.MessageName((*sync.TLSyncUpdatesMe)(nil)):
		request = new(sync.TLSyncUpdatesMe)
	case proto.MessageName((*sync.TLSyncUpdatesNotMe)(nil)):
		request = new(sync.TLSyncUpdatesNotMe)
	case proto.MessageName((*sync.TLSyncPushUpdates)(nil)):
		request = new(sync.TLSyncPushUpdates)
	case proto.MessageName((*sync.TLSyncPushUpdatesIfNot)(nil)):
		request = new(sync.TLSyncPushUpdatesIfNot)
	case proto.MessageName((*sync.TLSyncPushBotUpdates)(nil)):
		request = new(sync.TLSyncPushBotUpdates)
	case proto.MessageName((*sync.TLSyncPushRpcResult)(nil)):
		request = new(sync.TLSyncPushRpcResult)
	case proto.MessageName((*sync.TLSyncBroadcastUpdates)(nil)):
		request = new(sync.TLSyncBroadcastUpdates)
	default:
		return fmt.Errorf("sync: invalid Kafka method %q for key %q", method, key)
	}
	if err := json.Unmarshal(value, request); err != nil {
		return err
	}
	c := core.New(ctx, svcCtx)
	var err error
	switch in := request.(type) {
	case *sync.TLSyncUpdatesMe:
		_, err = c.SyncUpdatesMe(in)
	case *sync.TLSyncUpdatesNotMe:
		_, err = c.SyncUpdatesNotMe(in)
	case *sync.TLSyncPushUpdates:
		_, err = c.SyncPushUpdates(in)
	case *sync.TLSyncPushUpdatesIfNot:
		_, err = c.SyncPushUpdatesIfNot(in)
	case *sync.TLSyncPushBotUpdates:
		_, err = c.SyncPushBotUpdates(in)
	case *sync.TLSyncPushRpcResult:
		_, err = c.SyncPushRpcResult(in)
	case *sync.TLSyncBroadcastUpdates:
		_, err = c.SyncBroadcastUpdates(in)
	}
	return err
}
