package core

import (
	"context"
	"errors"
	"time"

	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/domain"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/svc"
	syncpb "github.com/teamgram/teamgram-server/app/messenger/sync/sync"
	"github.com/zeromicro/go-zero/core/logx"
)

// RunBusinessConnectionReconciler delivers updateBotBusinessConnect records
// through the same bot update path used by messenger/msg. The PostgreSQL
// outbox is the source of truth across process restarts.
func RunBusinessConnectionReconciler(ctx context.Context, svcCtx *svc.ServiceContext) {
	if svcCtx == nil || svcCtx.Dao == nil || svcCtx.Dao.SyncClient == nil {
		return
	}
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	var unavailable bool
	for {
		deliveries, err := domain.ListDueBusinessConnectionDeliveries(ctx, 50)
		if err != nil {
			if !unavailable {
				logx.WithContext(ctx).Errorf("business connection reconciler unavailable: %v", err)
				unavailable = true
			}
		} else {
			if unavailable {
				logx.WithContext(ctx).Info("business connection reconciler available")
				unavailable = false
			}
			for _, delivery := range deliveries {
				if err := deliverBusinessConnection(ctx, svcCtx, delivery); err != nil {
					logx.WithContext(ctx).Errorf("business connection delivery %d failed: %v", delivery.ID, err)
				}
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func deliverBusinessConnection(ctx context.Context, svcCtx *svc.ServiceContext, delivery domain.BusinessConnectionDelivery) error {
	updates, err := businessConnectionUpdates(delivery.Connection)
	if err != nil {
		_ = domain.RetryBusinessConnectionDelivery(ctx, delivery.ID, err)
		return err
	}
	callCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	result, pushErr := svcCtx.Dao.SyncClient.SyncPushBotUpdates(callCtx, &syncpb.TLSyncPushBotUpdates{
		UserId:  delivery.BotUserID,
		Updates: updates,
	})
	cancel()
	if pushErr == nil && result == nil {
		pushErr = errors.New("sync bot update returned no result")
	}
	if pushErr != nil {
		if retryErr := domain.RetryBusinessConnectionDelivery(ctx, delivery.ID, pushErr); retryErr != nil {
			return errors.Join(pushErr, retryErr)
		}
		return pushErr
	}
	return domain.CompleteBusinessConnectionDelivery(ctx, delivery.ID)
}
