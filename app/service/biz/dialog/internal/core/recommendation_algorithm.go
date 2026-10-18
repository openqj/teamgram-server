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
	"math"
	"sort"
	"time"

	"github.com/teamgram/teamgram-server/app/service/biz/dialog/internal/dal/dataobject"
)

// RecommendationAlgorithm 推荐算法引擎
type RecommendationAlgorithm struct {
	core *DialogCore

	// 权重配置
	frequencyWeight  float64 // 对话频率权重
	recencyWeight    float64 // 最近活跃权重
	engagementWeight float64 // 互动程度权重
	socialWeight     float64 // 社交关系权重
}

// NewRecommendationAlgorithm 创建推荐算法实例
func NewRecommendationAlgorithm(core *DialogCore) *RecommendationAlgorithm {
	return &RecommendationAlgorithm{
		core:             core,
		frequencyWeight:  0.3,
		recencyWeight:    0.3,
		engagementWeight: 0.2,
		socialWeight:     0.2,
	}
}

// RecommendedDialog 推荐对话结果
type RecommendedDialog struct {
	PeerType int32
	PeerId   int64
	Score    float64
}

// CalculateScore 计算推荐分数
func (r *RecommendationAlgorithm) CalculateScore(ctx context.Context,
	userID int64, peerType int32, peerID int64) (float64, error) {
	if r == nil || r.core == nil {
		return 0, fmt.Errorf("dialog recommendations: core is not configured")
	}
	store, err := r.core.pgStore()
	if err != nil {
		return 0, err
	}
	if store.Dialogs == nil {
		return 0, fmt.Errorf("dialog recommendations: dialogs DAO is not configured")
	}
	row, err := store.Dialogs.SelectDialog(ctx, userID, peerType, peerID)
	if err != nil {
		return 0, err
	}
	if row == nil {
		return 0, nil
	}
	return r.scoreDialog(*row), nil
}

// calculateFrequencyScore 计算对话频率分数
func (r *RecommendationAlgorithm) calculateFrequencyScore(ctx context.Context,
	userID int64, peerType int32, peerID int64, days int) (float64, error) {
	if r == nil || r.core == nil {
		return 0, fmt.Errorf("dialog recommendations: core is not configured")
	}
	store, err := r.core.pgStore()
	if err != nil {
		return 0, err
	}
	if store.Dialogs == nil {
		return 0, fmt.Errorf("dialog recommendations: dialogs DAO is not configured")
	}
	row, err := store.Dialogs.SelectDialog(ctx, userID, peerType, peerID)
	if err != nil {
		return 0, err
	}
	if row == nil {
		return 0, nil
	}
	return dialogActivityScore(*row, days), nil
}

// calculateRecencyScore 计算最近活跃度分数
func (r *RecommendationAlgorithm) calculateRecencyScore(ctx context.Context,
	userID int64, peerType int32, peerID int64) (float64, error) {
	if r == nil || r.core == nil {
		return 0, fmt.Errorf("dialog recommendations: core is not configured")
	}
	store, err := r.core.pgStore()
	if err != nil {
		return 0, err
	}
	if store.Dialogs == nil {
		return 0, fmt.Errorf("dialog recommendations: dialogs DAO is not configured")
	}
	row, err := store.Dialogs.SelectDialog(ctx, userID, peerType, peerID)
	if err != nil {
		return 0, err
	}
	if row == nil {
		return 0, nil
	}
	return dialogRecencyScore(row.Date2, time.Now()), nil
}

// calculateEngagementScore 计算互动程度分数
func (r *RecommendationAlgorithm) calculateEngagementScore(ctx context.Context,
	userID int64, peerType int32, peerID int64) (float64, error) {
	if r == nil || r.core == nil {
		return 0, fmt.Errorf("dialog recommendations: core is not configured")
	}
	store, err := r.core.pgStore()
	if err != nil {
		return 0, err
	}
	if store.Dialogs == nil {
		return 0, fmt.Errorf("dialog recommendations: dialogs DAO is not configured")
	}
	row, err := store.Dialogs.SelectDialog(ctx, userID, peerType, peerID)
	if err != nil {
		return 0, err
	}
	if row == nil {
		return 0, nil
	}
	return dialogEngagementScore(*row), nil
}

// calculateSocialScore 计算社交关系分数
func (r *RecommendationAlgorithm) calculateSocialScore(ctx context.Context,
	userID int64, peerType int32, peerID int64) (float64, error) {
	if peerID <= 0 {
		return 0, nil
	}
	// The dialog service does not own social graph data. Use the peer kind as a
	// conservative prior until a user/chat relationship provider is available.
	switch peerType {
	case 1:
		return 1, nil
	case 2:
		return 0.8, nil
	case 3:
		return 0.6, nil
	default:
		return 0, nil
	}
}

// GenerateRecommendations 为用户生成推荐列表
func (r *RecommendationAlgorithm) GenerateRecommendations(ctx context.Context,
	userID int64, limit int) ([]RecommendedDialog, error) {
	if r == nil || r.core == nil {
		return nil, fmt.Errorf("dialog recommendations: core is not configured")
	}
	if userID <= 0 {
		return nil, fmt.Errorf("invalid user id: %d", userID)
	}
	store, err := r.core.pgStore()
	if err != nil {
		return nil, err
	}
	if store.Dialogs == nil || store.DialogRecommendations == nil {
		return nil, fmt.Errorf("dialog recommendations: required DAOs are not configured")
	}
	if limit <= 0 {
		limit = r.getConfigInt(ctx, "max_recommendations", 20)
	}
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	dialogs, err := store.Dialogs.SelectAllDialogs(ctx, userID)
	if err != nil {
		return nil, err
	}
	existing, err := store.DialogRecommendations.SelectAllList(ctx, userID)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	blocked := make(map[string]struct{}, len(existing))
	for _, row := range existing {
		activePromotion := row.IsPromoted && (row.PromotedUntil == 0 || row.PromotedUntil > now.Unix())
		if row.Dismissed || activePromotion {
			blocked[makePeerKey(row.PeerType, row.PeerId)] = struct{}{}
		}
	}

	type scoredDialog struct {
		dialog dataobject.DialogsDO
		score  float64
	}
	scored := make([]scoredDialog, 0, len(dialogs))
	for _, row := range dialogs {
		if row.PeerId <= 0 || row.PeerType < 1 || row.PeerType > 3 {
			continue
		}
		if row.PeerType == 1 && row.PeerId == userID {
			continue
		}
		if _, ok := blocked[makePeerKey(row.PeerType, row.PeerId)]; ok {
			continue
		}
		scored = append(scored, scoredDialog{dialog: row, score: r.scoreDialogAt(row, now)})
	}
	sort.SliceStable(scored, func(i, j int) bool {
		if scored[i].score != scored[j].score {
			return scored[i].score > scored[j].score
		}
		if scored[i].dialog.Date2 != scored[j].dialog.Date2 {
			return scored[i].dialog.Date2 > scored[j].dialog.Date2
		}
		if scored[i].dialog.PeerType != scored[j].dialog.PeerType {
			return scored[i].dialog.PeerType < scored[j].dialog.PeerType
		}
		return scored[i].dialog.PeerId < scored[j].dialog.PeerId
	})

	if len(scored) > limit {
		scored = scored[:limit]
	}
	result := make([]RecommendedDialog, 0, len(scored))
	for _, item := range scored {
		result = append(result, RecommendedDialog{
			PeerType: item.dialog.PeerType,
			PeerId:   item.dialog.PeerId,
			Score:    item.score,
		})
	}
	return result, nil
}

func (r *RecommendationAlgorithm) scoreDialog(row dataobject.DialogsDO) float64 {
	return r.scoreDialogAt(row, time.Now())
}

func (r *RecommendationAlgorithm) scoreDialogAt(row dataobject.DialogsDO, now time.Time) float64 {
	frequency := dialogActivityScore(row, 30)
	recency := dialogRecencyScore(row.Date2, now)
	engagement := dialogEngagementScore(row)
	social, _ := r.calculateSocialScore(context.Background(), row.UserId, row.PeerType, row.PeerId)
	score := frequency*r.frequencyWeight + recency*r.recencyWeight + engagement*r.engagementWeight + social*r.socialWeight
	return math.Min(1, math.Max(0, score))
}

func dialogActivityScore(row dataobject.DialogsDO, days int) float64 {
	if row.TopMessage <= 0 {
		return 0
	}
	if days <= 0 {
		days = 30
	}
	// Dialog rows do not contain a message count. Treat the presence of a top
	// message as the baseline activity signal and add a bounded unread signal.
	unread := row.UnreadCount + row.UnreadMentionsCount + row.UnreadReactionsCount
	signal := 0.5
	if unread > 0 {
		signal += math.Min(0.5, math.Log1p(float64(unread))/math.Log1p(float64(days)))
	}
	return math.Min(1, signal)
}

func dialogRecencyScore(date2 int64, now time.Time) float64 {
	if date2 <= 0 {
		return 0
	}
	age := now.Unix() - date2
	if age < 0 {
		age = 0
	}
	return 1 / (1 + float64(age)/86400)
}

func dialogEngagementScore(row dataobject.DialogsDO) float64 {
	unread := row.UnreadCount + row.UnreadMentionsCount + row.UnreadReactionsCount
	score := math.Min(1, math.Log1p(float64(maxInt32(unread)))/math.Log1p(20))
	if row.Pinned > 0 || row.FolderPinned > 0 {
		score = math.Min(1, score+0.2)
	}
	return score
}

func maxInt32(value int32) int32 {
	if value < 0 {
		return 0
	}
	return value
}

// getConfigInt 获取整数配置
func (r *RecommendationAlgorithm) getConfigInt(ctx context.Context, key string, defaultValue int) int {
	if r == nil || r.core == nil {
		return defaultValue
	}
	store, err := r.core.pgStore()
	if err != nil || store.RecommendationConfigs == nil {
		return defaultValue
	}

	config, err := store.RecommendationConfigs.SelectByKey(ctx, key)
	if err != nil || config == nil {
		return defaultValue
	}

	var value int
	if _, err := fmt.Sscanf(config.ConfigValue, "%d", &value); err != nil {
		return defaultValue
	}
	return value
}
