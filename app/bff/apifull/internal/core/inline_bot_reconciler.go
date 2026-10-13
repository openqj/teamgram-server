package core

import (
	"context"
	"errors"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/persist"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/svc"
	syncpb "github.com/teamgram/teamgram-server/app/messenger/sync/sync"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
	"github.com/zeromicro/go-zero/core/logx"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// RunInlineBotQueryReconciler delivers updateBotInlineQuery records from the
// durable query store. A lease makes delivery restartable without allowing two
// workers to settle the same record after one of them has been reclaimed.
func RunInlineBotQueryReconciler(ctx context.Context, svcCtx *svc.ServiceContext) {
	if svcCtx == nil || svcCtx.Dao == nil || svcCtx.Dao.SyncClient == nil {
		return
	}
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	var unavailable bool
	for {
		items, err := persist.ClaimInlineBotQueries(ctx, 50)
		if err != nil {
			if !unavailable {
				logx.WithContext(ctx).Errorf("inline bot query reconciler unavailable: %v", err)
				unavailable = true
			}
		} else {
			if unavailable {
				logx.WithContext(ctx).Info("inline bot query reconciler available")
				unavailable = false
			}
			for _, item := range items {
				if err = deliverInlineBotQuery(ctx, svcCtx, item); err != nil {
					logx.WithContext(ctx).Errorf("inline bot query delivery %d failed: %v", item.QueryID, err)
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

func deliverInlineBotQuery(ctx context.Context, svcCtx *svc.ServiceContext, item persist.InlineBotQuery) error {
	updates, err := inlineBotQueryUpdate(ctx, svcCtx, item)
	if err != nil {
		_ = persist.RetryInlineBotQuery(ctx, item.QueryID, item.ClaimToken, err)
		return err
	}
	callCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	result, pushErr := svcCtx.Dao.SyncClient.SyncPushBotUpdates(callCtx, &syncpb.TLSyncPushBotUpdates{
		UserId: item.BotID, Updates: updates,
	})
	cancel()
	if pushErr == nil && result == nil {
		pushErr = errors.New("sync bot update returned no result")
	}
	if pushErr != nil {
		if retryErr := persist.RetryInlineBotQuery(ctx, item.QueryID, item.ClaimToken, pushErr); retryErr != nil {
			return errors.Join(pushErr, retryErr)
		}
		return pushErr
	}
	return persist.CompleteInlineBotQuery(ctx, item.QueryID, item.ClaimToken)
}

func inlineBotQueryUpdate(ctx context.Context, svcCtx *svc.ServiceContext, item persist.InlineBotQuery) (*mtproto.Updates, error) {
	peerType, err := inlineQueryPeerTypeFromPredicate(item.PeerType)
	if err != nil {
		return nil, err
	}
	var geo *mtproto.GeoPoint
	if item.HasGeo {
		geoData := &mtproto.GeoPoint{Lat: item.GeoLat, Long: item.GeoLong}
		if item.GeoAccuracy > 0 {
			geoData.AccuracyRadius = wrapperspb.Int32(item.GeoAccuracy)
		}
		geo = mtproto.MakeTLGeoPoint(geoData).To_GeoPoint()
	}
	update := mtproto.MakeTLUpdateBotInlineQuery(&mtproto.Update{
		QueryId_INT64: item.QueryID,
		UserId:        item.RequesterID,
		Query:         item.Query,
		Geo:           geo,
		PeerType:      peerType,
		Offset:        item.Offset,
	}).To_Update()
	if svcCtx == nil || svcCtx.Dao == nil || svcCtx.Dao.UserClient == nil {
		return nil, mtproto.ErrMethodNotImpl
	}
	profile, err := svcCtx.Dao.UserGetImmutableUser(ctx, &userpb.TLUserGetImmutableUser{Id: item.RequesterID})
	if err != nil {
		return nil, err
	}
	if profile == nil || profile.GetUser() == nil || profile.Deleted() {
		return nil, mtproto.ErrUserIdInvalid
	}
	requester := profile.ToUser(item.BotID)
	if requester == nil {
		return nil, mtproto.ErrUserIdInvalid
	}
	return mtproto.MakeUpdatesByUpdatesUsers([]*mtproto.User{requester}, update), nil
}

func inlineQueryPeerTypeFromPredicate(predicate string) (*mtproto.InlineQueryPeerType, error) {
	value := &mtproto.InlineQueryPeerType{}
	switch predicate {
	case mtproto.Predicate_inlineQueryPeerTypeSameBotPM:
		return mtproto.MakeTLInlineQueryPeerTypeSameBotPM(value).To_InlineQueryPeerType(), nil
	case mtproto.Predicate_inlineQueryPeerTypePM:
		return mtproto.MakeTLInlineQueryPeerTypePM(value).To_InlineQueryPeerType(), nil
	case mtproto.Predicate_inlineQueryPeerTypeChat:
		return mtproto.MakeTLInlineQueryPeerTypeChat(value).To_InlineQueryPeerType(), nil
	case mtproto.Predicate_inlineQueryPeerTypeMegagroup:
		return mtproto.MakeTLInlineQueryPeerTypeMegagroup(value).To_InlineQueryPeerType(), nil
	case mtproto.Predicate_inlineQueryPeerTypeBroadcast:
		return mtproto.MakeTLInlineQueryPeerTypeBroadcast(value).To_InlineQueryPeerType(), nil
	case mtproto.Predicate_inlineQueryPeerTypeBotPM:
		return mtproto.MakeTLInlineQueryPeerTypeBotPM(value).To_InlineQueryPeerType(), nil
	default:
		return nil, mtproto.ErrInputConstructorInvalid
	}
}
