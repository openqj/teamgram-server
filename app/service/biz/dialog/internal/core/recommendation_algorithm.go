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

	score := 0.0

	// 1. 对话频率分数（最近30天的消息数）
	frequencyScore, err := r.calculateFrequencyScore(ctx, userID, peerType, peerID, 30)
	if err != nil {
		return 0, err
	}
	score += frequencyScore * r.frequencyWeight

	// 2. 最近活跃度分数
	recencyScore, err := r.calculateRecencyScore(ctx, userID, peerType, peerID)
	if err != nil {
		return 0, err
	}
	score += recencyScore * r.recencyWeight

	// 3. 互动程度分数
	engagementScore, err := r.calculateEngagementScore(ctx, userID, peerType, peerID)
	if err != nil {
		return 0, err
	}
	score += engagementScore * r.engagementWeight

	// 4. 社交关系分数
	socialScore, err := r.calculateSocialScore(ctx, userID, peerType, peerID)
	if err != nil {
		return 0, err
	}
	score += socialScore * r.socialWeight

	// 归一化到 0-1
	return math.Min(1.0, math.Max(0.0, score)), nil
}

// calculateFrequencyScore 计算对话频率分数
func (r *RecommendationAlgorithm) calculateFrequencyScore(ctx context.Context,
	userID int64, peerType int32, peerID int64, days int) (float64, error) {

	// TODO: 调用消息服务统计最近N天的消息数
	// 当前返回模拟值，实际需要调用 message service
	// messageCount := getMessageCountInDays(userID, peerType, peerID, days)

	// 归一化：假设日均10条消息为满分
	// return math.Min(1.0, float64(messageCount) / float64(days*10)), nil

	// 临时返回中等分数
	return 0.5, nil
}

// calculateRecencyScore 计算最近活跃度分数
func (r *RecommendationAlgorithm) calculateRecencyScore(ctx context.Context,
	userID int64, peerType int32, peerID int64) (float64, error) {

	// TODO: 获取最后一条消息的时间
	// 实际需要调用 dialog service 获取 top_message 时间
	// lastMessageTime := getLastMessageTime(userID, peerType, peerID)
	// hoursSinceLastMessage := time.Since(lastMessageTime).Hours()

	// 时间衰减：24小时内=1.0, 7天=0.5, 30天=0.1
	// score := 1.0 / (1.0 + hoursSinceLastMessage/24.0)

	// 临时返回中等分数
	return 0.5, nil
}

// calculateEngagementScore 计算互动程度分数
func (r *RecommendationAlgorithm) calculateEngagementScore(ctx context.Context,
	userID int64, peerType int32, peerID int64) (float64, error) {

	// TODO: 考虑因素：
	// - 双向消息比例
	// - 平均回复速度
	// - 消息长度
	// - 是否有媒体内容
	// 实际需要调用 message service 进行分析

	// 临时返回中等分数
	return 0.5, nil
}

// calculateSocialScore 计算社交关系分数
func (r *RecommendationAlgorithm) calculateSocialScore(ctx context.Context,
	userID int64, peerType int32, peerID int64) (float64, error) {

	// TODO: 考虑因素：
	// - 共同好友数（私聊）
	// - 群组活跃度（群聊）
	// - 是否互相关注（频道）
	// 实际需要调用 user/chat service 进行分析

	// 临时返回中等分数
	return 0.5, nil
}

// GenerateRecommendations 为用户生成推荐列表
func (r *RecommendationAlgorithm) GenerateRecommendations(ctx context.Context,
	userID int64, limit int) ([]RecommendedDialog, error) {

	// TODO: 完整实现
	// 1. 获取用户的所有活跃对话
	// dialogs := getUserActiveDialogs(userID)

	// 2. 排除已在推荐列表中的
	// existingRecommendations := getExistingRecommendations(userID)

	// 3. 为每个对话计算分数
	// scored := []struct{
	//     dialog Dialog
	//     score float64
	// }{}
	// for _, dialog := range dialogs {
	//     score, _ := r.CalculateScore(ctx, userID, dialog.PeerType, dialog.PeerID)
	//     scored = append(scored, struct{dialog Dialog; score float64}{dialog, score})
	// }

	// 4. 排序并取 top N
	// sort.Slice(scored, func(i, j int) bool {
	//     return scored[i].score > scored[j].score
	// })

	// 5. 返回推荐
	return []RecommendedDialog{}, nil
}

// getConfigInt 获取整数配置
func (r *RecommendationAlgorithm) getConfigInt(ctx context.Context, key string, defaultValue int) int {
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
