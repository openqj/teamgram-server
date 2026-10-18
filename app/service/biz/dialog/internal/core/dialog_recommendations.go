/*
 * Copyright (c) 2024-present,  Teamgram Authors.
 *  All rights reserved.
 *
 * Author: teamgramio (teamgram.io@gmail.com)
 */

package core

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/dialog"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/internal/dal/dataobject"
)

// DialogGetRecommendedDialogs
// dialog.getRecommendedDialogs user_id:long offset:int limit:int = RecommendedDialogs;
func (c *DialogCore) DialogGetRecommendedDialogs(ctx context.Context,
	in *dialog.TLDialogGetRecommendedDialogs) (*dialog.RecommendedDialogs, error) {
	if in == nil || in.UserId <= 0 {
		return nil, mtproto.ErrInputRequestInvalid
	}
	if in.Offset < 0 || in.Offset > 10000 {
		return nil, mtproto.ErrOffsetInvalid
	}

	store, err := c.pgStore()
	if err != nil {
		return nil, err
	}
	if store.DialogRecommendations == nil {
		return nil, mtproto.ErrMethodNotImpl
	}

	userID := in.UserId
	limit := in.Limit
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	fetchLimit := in.Offset + limit
	if fetchLimit < limit || fetchLimit > 10000 {
		return nil, mtproto.ErrOffsetInvalid
	}
	promotedLimit := int(fetchLimit)
	if configured := NewRecommendationAlgorithm(c).getConfigInt(ctx, "promoted_max_slots", 3); configured > 0 && configured < promotedLimit {
		promotedLimit = configured
	}

	// 1. 获取推广对话（优先级最高）
	promotedDialogs, err := store.DialogRecommendations.SelectPromotedListWithLimit(ctx, userID, int32(promotedLimit))
	if err != nil {
		return nil, err
	}

	// 2. 获取算法推荐对话
	recommendedDialogs, err := store.DialogRecommendations.SelectActiveList(ctx, userID, fetchLimit)
	if err != nil {
		return nil, err
	}

	// 3. 合并结果（推广在前）
	merged := make([]*dialog.RecommendedDialog, 0, len(promotedDialogs)+len(recommendedDialogs))
	seen := make(map[string]struct{}, len(promotedDialogs)+len(recommendedDialogs))
	for _, d := range promotedDialogs {
		key := makePeerKey(d.PeerType, d.PeerId)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		merged = append(merged, recommendationToProto(d))
	}

	for _, d := range recommendedDialogs {
		key := makePeerKey(d.PeerType, d.PeerId)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		merged = append(merged, recommendationToProto(d))
	}

	start := int(in.Offset)
	if start > len(merged) {
		start = len(merged)
	}
	end := start + int(limit)
	if end > len(merged) {
		end = len(merged)
	}
	page := merged[start:end]
	result := &dialog.RecommendedDialogs{Dialogs: page}

	// 4. 异步更新当前页面的展示统计
	stats := append([]*dialog.RecommendedDialog(nil), page...)
	go func() {
		bgCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		for _, d := range stats {
			_, _ = store.DialogRecommendations.UpdateStats(bgCtx,
				userID, d.PeerType, d.PeerId, 1, 0)
		}
	}()

	return result, nil
}

// DialogRefreshRecommendations
// dialog.refreshRecommendations user_id:long = Bool;
func (c *DialogCore) DialogRefreshRecommendations(ctx context.Context,
	in *dialog.TLDialogRefreshRecommendations) (*mtproto.Bool, error) {
	if in == nil || in.UserId <= 0 {
		return mtproto.BoolFalse, mtproto.ErrInputRequestInvalid
	}

	store, err := c.pgStore()
	if err != nil {
		return mtproto.BoolFalse, err
	}
	if store.DialogRecommendations == nil {
		return mtproto.BoolFalse, mtproto.ErrMethodNotImpl
	}

	userID := in.UserId

	// 1. 创建算法实例
	algorithm := NewRecommendationAlgorithm(c)
	maxRecommendations := algorithm.getConfigInt(ctx, "max_recommendations", 20)
	if maxRecommendations <= 0 || maxRecommendations > 100 {
		maxRecommendations = 20
	}

	// 2. 生成推荐
	recommendations, err := algorithm.GenerateRecommendations(ctx, userID, maxRecommendations)
	if err != nil {
		c.Logger.Errorf("generate recommendations error: %v", err)
		return mtproto.BoolFalse, err
	}

	// 3. 批量插入/更新
	now := time.Now().Unix()
	ttlDays := algorithm.getConfigInt(ctx, "recommendation_ttl_days", 7)
	if ttlDays <= 0 || ttlDays > 365 {
		ttlDays = 7
	}
	expiresAt := now + int64(ttlDays*24*3600)

	dataObjects := make([]dataobject.DialogRecommendationsDO, 0, len(recommendations))
	for i, rec := range recommendations {
		dataObjects = append(dataObjects, dataobject.DialogRecommendationsDO{
			UserId:             userID,
			PeerType:           rec.PeerType,
			PeerId:             rec.PeerId,
			RecommendationType: 0,                               // 算法推荐
			Weight:             int32(len(recommendations) - i), // 递减权重
			Score:              rec.Score,
			RecommendedAt:      now,
			ExpiresAt:          expiresAt,
			LastUpdated:        now,
		})
	}

	if len(dataObjects) > 0 {
		if c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.Postgres == nil {
			return mtproto.BoolFalse, mtproto.ErrMethodNotImpl
		}
		err = c.svcCtx.Dao.Postgres.InTx(ctx, func(tx pgx.Tx) error {
			return store.DialogRecommendations.BatchInsertOn(ctx, tx, dataObjects)
		})
		if err != nil {
			c.Logger.Errorf("batch insert recommendations error: %v", err)
			return mtproto.BoolFalse, err
		}
	}

	return mtproto.BoolTrue, nil
}

// DialogReportRecommendationFeedback
// dialog.reportRecommendationFeedback user_id:long peer_type:int peer_id:long feedback_type:int = Bool;
func (c *DialogCore) DialogReportRecommendationFeedback(ctx context.Context,
	in *dialog.TLDialogReportRecommendationFeedback) (*mtproto.Bool, error) {
	if in == nil || in.UserId <= 0 {
		return mtproto.BoolFalse, mtproto.ErrInputRequestInvalid
	}
	if err := validateRecommendationPeer(in.PeerType, in.PeerId); err != nil {
		return mtproto.BoolFalse, err
	}
	if in.FeedbackType < 1 || in.FeedbackType > 5 {
		return mtproto.BoolFalse, mtproto.ErrInputRequestInvalid
	}

	store, err := c.pgStore()
	if err != nil {
		return mtproto.BoolFalse, err
	}
	if store.DialogRecommendations == nil {
		return mtproto.BoolFalse, mtproto.ErrMethodNotImpl
	}

	if c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.Postgres == nil {
		return mtproto.BoolFalse, mtproto.ErrMethodNotImpl
	}
	err = c.svcCtx.Dao.Postgres.InTx(ctx, func(tx pgx.Tx) error {
		switch in.FeedbackType {
		case 1: // clicked
			if _, err := store.DialogRecommendations.UpdateStatsOn(ctx, tx, in.UserId, in.PeerType, in.PeerId, 0, 1); err != nil {
				return err
			}
		case 2: // dismissed
			if _, err := store.DialogRecommendations.MarkDismissedOn(ctx, tx, in.UserId, in.PeerType, in.PeerId); err != nil {
				return err
			}
		case 3: // pinned
			if _, err := store.DialogRecommendations.UpdateStatsOn(ctx, tx, in.UserId, in.PeerType, in.PeerId, 0, 1); err != nil {
				return err
			}
		}
		_, err := store.DialogRecommendations.InsertFeedbackLogOn(ctx, tx,
			in.UserId, in.PeerType, in.PeerId, in.FeedbackType, time.Now().Unix(), "", []byte(`{}`))
		return err
	})
	if err != nil {
		c.Logger.Errorf("report recommendation feedback error: %v", err)
		return mtproto.BoolFalse, err
	}

	return mtproto.BoolTrue, nil
}

// DialogAddPromotedDialog
// dialog.addPromotedDialog peer_type:int peer_id:long priority:int expires_at:long target_user_ids:Vector<long> = Bool;
func (c *DialogCore) DialogAddPromotedDialog(ctx context.Context,
	in *dialog.TLDialogAddPromotedDialog) (*mtproto.Bool, error) {
	if in == nil {
		return mtproto.BoolFalse, mtproto.ErrInputRequestInvalid
	}
	if err := validateRecommendationPeer(in.PeerType, in.PeerId); err != nil {
		return mtproto.BoolFalse, err
	}

	store, err := c.pgStore()
	if err != nil {
		return mtproto.BoolFalse, err
	}
	if store.DialogRecommendations == nil {
		return mtproto.BoolFalse, mtproto.ErrMethodNotImpl
	}

	now := time.Now().Unix()
	expiresAt := in.ExpiresAt
	if expiresAt == 0 {
		expiresAt = now + 30*24*3600 // 默认30天
	}
	if expiresAt <= now {
		return mtproto.BoolFalse, mtproto.ErrInputRequestInvalid
	}

	targetUserIDs := in.TargetUserIds
	if len(targetUserIDs) == 0 {
		if store.Dialogs == nil {
			return mtproto.BoolFalse, mtproto.ErrMethodNotImpl
		}
		targetUserIDs, err = store.Dialogs.SelectActiveUserIDs(ctx, now-7*24*3600, 10000)
		if err != nil {
			return mtproto.BoolFalse, err
		}
	}
	targetUserIDs = uniquePositiveIDs(targetUserIDs)
	if len(targetUserIDs) == 0 {
		return mtproto.BoolTrue, nil
	}

	// 批量插入推广
	dataObjects := make([]dataobject.DialogRecommendationsDO, 0, len(targetUserIDs))
	for _, userID := range targetUserIDs {
		dataObjects = append(dataObjects, dataobject.DialogRecommendationsDO{
			UserId:             userID,
			PeerType:           in.PeerType,
			PeerId:             in.PeerId,
			RecommendationType: 1,                  // 系统推广
			Weight:             1000 + in.Priority, // 推广对话高权重
			IsPromoted:         true,
			PromotedUntil:      expiresAt,
			PromotedPriority:   in.Priority,
			RecommendedAt:      now,
			ExpiresAt:          expiresAt,
			LastUpdated:        now,
		})
	}

	if c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.Postgres == nil {
		return mtproto.BoolFalse, mtproto.ErrMethodNotImpl
	}
	err = c.svcCtx.Dao.Postgres.InTx(ctx, func(tx pgx.Tx) error {
		return store.DialogRecommendations.BatchInsertOn(ctx, tx, dataObjects)
	})
	if err != nil {
		c.Logger.Errorf("batch insert promoted dialogs error: %v", err)
		return mtproto.BoolFalse, err
	}

	c.Logger.Infof("added promoted dialog peer_type=%d peer_id=%d for %d users",
		in.PeerType, in.PeerId, len(targetUserIDs))

	return mtproto.BoolTrue, nil
}

// DialogRemovePromotedDialog
// dialog.removePromotedDialog peer_type:int peer_id:long target_user_ids:Vector<long> = Bool;
func (c *DialogCore) DialogRemovePromotedDialog(ctx context.Context,
	in *dialog.TLDialogRemovePromotedDialog) (*mtproto.Bool, error) {
	if in == nil {
		return mtproto.BoolFalse, mtproto.ErrInputRequestInvalid
	}
	if err := validateRecommendationPeer(in.PeerType, in.PeerId); err != nil {
		return mtproto.BoolFalse, err
	}

	store, err := c.pgStore()
	if err != nil {
		return mtproto.BoolFalse, err
	}
	if store.DialogRecommendations == nil {
		return mtproto.BoolFalse, mtproto.ErrMethodNotImpl
	}

	targetUserIDs := in.TargetUserIds
	if len(targetUserIDs) == 0 {
		if store.Dialogs == nil {
			return mtproto.BoolFalse, mtproto.ErrMethodNotImpl
		}
		targetUserIDs, err = store.Dialogs.SelectActiveUserIDs(ctx, time.Now().Add(-7*24*time.Hour).Unix(), 10000)
		if err != nil {
			return mtproto.BoolFalse, err
		}
	}
	targetUserIDs = uniquePositiveIDs(targetUserIDs)
	if len(targetUserIDs) == 0 {
		return mtproto.BoolTrue, nil
	}

	if c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.Postgres == nil {
		return mtproto.BoolFalse, mtproto.ErrMethodNotImpl
	}
	err = c.svcCtx.Dao.Postgres.InTx(ctx, func(tx pgx.Tx) error {
		for _, userID := range targetUserIDs {
			if _, err := store.DialogRecommendations.RemovePromotedOn(ctx, tx, userID, in.PeerType, in.PeerId); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return mtproto.BoolFalse, err
	}

	c.Logger.Infof("removed promoted dialog peer_type=%d peer_id=%d for %d users",
		in.PeerType, in.PeerId, len(targetUserIDs))

	return mtproto.BoolTrue, nil
}

// makePeerKey 生成对话的唯一键
func makePeerKey(peerType int32, peerID int64) string {
	return fmt.Sprintf("%d_%d", peerType, peerID)
}

func recommendationToProto(d dataobject.DialogRecommendationsDO) *dialog.RecommendedDialog {
	return &dialog.RecommendedDialog{
		PeerType:           d.PeerType,
		PeerId:             d.PeerId,
		RecommendationType: d.RecommendationType,
		IsPromoted:         d.IsPromoted,
		Score:              d.Score,
	}
}

func validateRecommendationPeer(peerType int32, peerID int64) error {
	if peerID <= 0 {
		return mtproto.ErrPeerIdInvalid
	}
	switch peerType {
	case 1, 2, 3:
		return nil
	default:
		return mtproto.ErrPeerIdInvalid
	}
}

func uniquePositiveIDs(ids []int64) []int64 {
	seen := make(map[int64]struct{}, len(ids))
	result := make([]int64, 0, len(ids))
	for _, id := range ids {
		if id <= 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		result = append(result, id)
	}
	return result
}
