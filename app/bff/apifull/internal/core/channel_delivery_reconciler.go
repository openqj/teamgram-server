package core

import (
	"context"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/apifull/channelview"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/domain"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/svc"
	syncpb "github.com/teamgram/teamgram-server/app/messenger/sync/sync"
	"github.com/zeromicro/go-zero/core/logx"
)

func RunChannelDeliveryReconciler(ctx context.Context, svcCtx *svc.ServiceContext) {
	if svcCtx == nil || svcCtx.Dao == nil || svcCtx.Dao.SyncClient == nil {
		return
	}
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	var unavailable bool
	for {
		deliveries, err := domain.ListDueChannelDeliveries(ctx, 50)
		if err != nil {
			if !unavailable {
				logx.WithContext(ctx).Errorf("channel delivery reconciler unavailable: %v", err)
				unavailable = true
			}
		} else {
			if unavailable {
				logx.WithContext(ctx).Info("channel delivery reconciler available")
				unavailable = false
			}
			for _, delivery := range deliveries {
				_, dispatchErr := channelview.DeliverPendingChannelUpdates(ctx, delivery.Key, true, func(pushCtx context.Context, userID int64, excludes []int64, updates *mtproto.Updates) error {
					callCtx, cancel := context.WithTimeout(pushCtx, 10*time.Second)
					defer cancel()
					result, pushErr := svcCtx.Dao.SyncClient.SyncPushUpdatesIfNot(callCtx, &syncpb.TLSyncPushUpdatesIfNot{
						UserId:   userID,
						Excludes: excludes,
						Updates:  updates,
					})
					if pushErr == nil && result == nil {
						return mtproto.ErrMethodNotImpl
					}
					return pushErr
				})
				if dispatchErr != nil {
					logx.WithContext(ctx).Errorf("channel delivery reconciliation failed for channel %d pts %d-%d: %v", delivery.Key.ChannelID, delivery.Key.PTSFrom, delivery.Key.PTSTo, dispatchErr)
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
