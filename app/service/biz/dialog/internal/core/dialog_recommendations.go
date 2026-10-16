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

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/dialog"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/internal/dal/dataobject"
)

// DialogGetRecommendedDialogs
// dialog.getRecommendedDialogs user_id:long offset:int limit:int = RecommendedDialogs;
func (c *DialogCore) DialogGetRecommendedDialogs(ctx context.Context,
	in *dialog.TLDialogGetRecommendedDialogs) (*dialog.RecommendedDialogs, error) {

	store, err := c.pgStore()
	if err != nil {
		return nil, err
	}

	userID := in.UserId
	limit := in.Limit
	if limit <= 0 || limit > 50 {
		limit = 20
	}

	// 1. 获取推广对话（优先级最高）
	promotedDialogs, err := store.DialogRecommendations.SelectPromotedList(ctx, userID)
	if err != nil {
		c.Logger.Errorf("get promoted dialogs error: %v", err)
		promotedDialogs = []dataobject.DialogRecommendationsDO{}
	}

	// 2. 获取算法推荐对话
	recommendedDialogs, err := store.DialogRecommendations.SelectActiveList(ctx, userID, limit)
	if err != nil {
		c.Logger.Errorf("get recommended dialogs error: %v", err)
		recommendedDialogs = []dataobject.DialogRecommendationsDO{}
	}

	// 3. 合并结果（推广在前）
	result := &dialog.RecommendedDialogs{
		Dialogs: make([]*dialog.RecommendedDialog, 0, len(promotedDialogs)+len(recommendedDialogs)),
	}

	// 添加推广对话
	for _, d := range promotedDialogs {
		result.Dialogs = append(result.Dialogs, &dialog.RecommendedDialog{
			PeerType:           d.PeerType,
			PeerId:             d.PeerId,
			RecommendationType: d.RecommendationType,
			IsPromoted:         d.IsPromoted,
			Score:              d.Score,
		})
	}

	// 添加算法推荐对话（避免重复）
	promotedMap := make(map[string]bool)
	for _, d := range promotedDialogs {
		key := makePeerKey(d.PeerType, d.PeerId)
		promotedMap[key] = true
	}

	for _, d := range recommendedDialogs {
		key := makePeerKey(d.PeerType, d.PeerId)
		if !promotedMap[key] {
			result.Dialogs = append(result.Dialogs, &dialog.RecommendedDialog{
				PeerType:           d.PeerType,
				PeerId:             d.PeerId,
				RecommendationType: d.RecommendationType,
				IsPromoted:         d.IsPromoted,
				Score:              d.Score,
			})
		}
	}

	// 4. 异步更新展示统计
	go func() {
		bgCtx := context.Background()
		for _, d := range result.Dialogs {
			_ = store.DialogRecommendations.UpdateStats(bgCtx,
				userID, d.PeerType, d.PeerId, 1, 0)
		}
	}()

	return result, nil
}

// DialogRefreshRecommendations
// dialog.refreshRecommendations user_id:long = Bool;
func (c *DialogCore) DialogRefreshRecommendations(ctx context.Context,
	in *dialog.TLDialogRefreshRecommendations) (*mtproto.Bool, error) {

	store, err := c.pgStore()
	if err != nil {
		return mtproto.BoolFalse, err
	}

	userID := in.UserId

	// 1. 创建算法实例
	algorithm := NewRecommendationAlgorithm(c)

	// 2. 生成推荐
	recommendations, err := algorithm.GenerateRecommendations(ctx, userID, 20)
	if err != nil {
		c.Logger.Errorf("generate recommendations error: %v", err)
		return mtproto.BoolFalse, err
	}

	// 3. 批量插入/更新
	now := time.Now().Unix()
	ttlDays := 7 // 从配置读取
	expiresAt := now + int64(ttlDays*24*3600)

	dataObjects := make([]dataobject.DialogRecommendationsDO, 0, len(recommendations))
	for i, rec := range recommendations {
		dataObjects = append(dataObjects, dataobject.DialogRecommendationsDO{
			UserId:             userID,
			PeerType:           rec.PeerType,
			PeerId:             rec.PeerId,
			RecommendationType: 0, // 算法推荐
			Weight:             int32(len(recommendations) - i), // 递减权重
			Score:              rec.Score,
			RecommendedAt:      now,
			ExpiresAt:          expiresAt,
			LastUpdated:        now,
		})
	}

	if len(dataObjects) > 0 {
		err = store.DialogRecommendations.BatchInsert(ctx, dataObjects)
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

	store, err := c.pgStore()
	if err != nil {
		return mtproto.BoolFalse, err
	}

	userID := in.UserId
	peerType := in.PeerType
	peerID := in.PeerId
	feedbackType := in.FeedbackType

	switch feedbackType {
	case 1: // clicked
		_, err = store.DialogRecommendations.UpdateStats(ctx, userID, peerType, peerID, 0, 1)
	case 2: // dismissed
		_, err = store.DialogRecommendations.MarkDismissed(ctx, userID, peerType, peerID)
	case 3: // pinned
		// 用户将推荐对话置顶，表示推荐成功
		_, err = store.DialogRecommendations.UpdateStats(ctx, userID, peerType, peerID, 0, 1)
	default:
		c.Logger.Warnf("unknown feedback type: %d", feedbackType)
	}

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

	store, err := c.pgStore()
	if err != nil {
		return mtproto.BoolFalse, err
	}

	now := time.Now().Unix()
	expiresAt := in.ExpiresAt
	if expiresAt == 0 {
		expiresAt = now + 30*24*3600 // 默认30天
	}

	targetUserIDs := in.TargetUserIds
	if len(targetUserIDs) == 0 {
		// TODO: 获取所有活跃用户 ID
		// 这里需要实现获取活跃用户列表的逻辑
		c.Logger.Warn("no target users specified for promoted dialog")
		return mtproto.BoolFalse, nil
	}

	// 批量插入推广
	dataObjects := make([]dataobject.DialogRecommendationsDO, 0, len(targetUserIDs))
	for _, userID := range targetUserIDs {
		dataObjects = append(dataObjects, dataobject.DialogRecommendationsDO{
			UserId:             userID,
			PeerType:           in.PeerType,
			PeerId:             in.PeerId,
			RecommendationType: 1, // 系统推广
			Weight:             1000 + in.Priority, // 推广对话高权重
			IsPromoted:         true,
			PromotedUntil:      expiresAt,
			PromotedPriority:   in.Priority,
			RecommendedAt:      now,
			ExpiresAt:          expiresAt,
			LastUpdated:        now,
		})
	}

	err = store.DialogRecommendations.BatchInsert(ctx, dataObjects)
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

	store, err := c.pgStore()
	if err != nil {
		return mtproto.BoolFalse, err
	}

	targetUserIDs := in.TargetUserIds
	if len(targetUserIDs) == 0 {
		c.Logger.Warn("no target users specified for removing promoted dialog")
		return mtproto.BoolFalse, nil
	}

	// 标记为已删除
	for _, userID := range targetUserIDs {
		_, err := store.DialogRecommendations.MarkDismissed(ctx, userID, in.PeerType, in.PeerId)
		if err != nil {
			c.Logger.Errorf("remove promoted dialog error: %v", err)
		}
	}

	c.Logger.Infof("removed promoted dialog peer_type=%d peer_id=%d for %d users",
		in.PeerType, in.PeerId, len(targetUserIDs))

	return mtproto.BoolTrue, nil
}

// makePeerKey 生成对话的唯一键
func makePeerKey(peerType int32, peerID int64) string {
	return fmt.Sprintf("%d_%d", peerType, peerID)
}
