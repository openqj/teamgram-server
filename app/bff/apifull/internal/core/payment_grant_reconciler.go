package core

import (
	"context"
	"errors"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/domain"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/svc"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
	"github.com/zeromicro/go-zero/core/logx"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func RunPremiumGrantReconciler(ctx context.Context, svcCtx *svc.ServiceContext) {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	var unavailable bool
	for {
		err := reconcilePremiumGrants(ctx, svcCtx)
		if err != nil && !unavailable {
			logx.WithContext(ctx).Errorf("premium grant reconciler unavailable: %v", err)
			unavailable = true
		} else if err == nil && unavailable {
			logx.WithContext(ctx).Info("premium grant reconciler available")
			unavailable = false
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func reconcilePremiumGrants(ctx context.Context, svcCtx *svc.ServiceContext) error {
	if svcCtx == nil || svcCtx.Dao == nil || svcCtx.Dao.UserClient == nil {
		return nil
	}
	if err := domain.CheckPremiumGrantOutbox(ctx); err != nil {
		return err
	}
	release, acquired, err := domain.LockPremiumGrantReconciler(ctx)
	if err != nil {
		return err
	}
	if !acquired {
		return nil
	}
	defer release()

	grants, err := domain.ListDuePremiumGrants(ctx, 50)
	if err != nil {
		return err
	}
	for _, grant := range grants {
		callCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		result, grantErr := svcCtx.Dao.UserClient.UserUpdatePremium(callCtx, &userpb.TLUserUpdatePremium{
			UserId:        grant.UserID,
			Premium:       mtproto.ToBool(true),
			Months:        wrapperspb.Int32(grant.Months),
			Provider:      grant.Provider,
			TransactionId: grant.TransactionID,
		})
		cancel()
		if grantErr == nil && result != nil && mtproto.FromBool(result) {
			completeCtx, completeCancel := context.WithTimeout(context.Background(), 2*time.Second)
			completeErr := domain.CompletePremiumGrant(completeCtx, grant.RequestID)
			completeCancel()
			if completeErr == nil {
				continue
			}
			grantErr = completeErr
		}
		if grantErr == nil {
			grantErr = errors.New("user service returned no successful premium grant")
		}
		retryCtx, retryCancel := context.WithTimeout(context.Background(), 2*time.Second)
		if retryErr := domain.RetryPremiumGrant(retryCtx, grant.RequestID, grantErr); retryErr != nil {
			logx.WithContext(ctx).Errorf("premium grant retry scheduling failed for request %d: %v", grant.RequestID, retryErr)
		}
		retryCancel()
		logx.WithContext(ctx).Errorf("premium grant reconciliation failed for request %d: %v", grant.RequestID, grantErr)
	}
	return nil
}
