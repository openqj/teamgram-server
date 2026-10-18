/*
 * Copyright (c) 2024-present,  Teamgram Authors.
 *  All rights reserved.
 *
 * Author: teamgramio (teamgram.io@gmail.com)
 */

package jobs

import (
	"context"
	"time"

	"github.com/teamgram/teamgram-server/app/service/biz/dialog/internal/dal/dao/postgres_dao"
	"github.com/zeromicro/go-zero/core/logx"
)

// RecommendationCleanupJob 推荐清理定时任务
type RecommendationCleanupJob struct {
	store *postgres_dao.Store
}

// NewRecommendationCleanupJob 创建推荐清理任务
func NewRecommendationCleanupJob(store *postgres_dao.Store) *RecommendationCleanupJob {
	return &RecommendationCleanupJob{
		store: store,
	}
}

// Run 执行过期推荐清理
// 每天执行一次，清理已过期的推荐记录
func (j *RecommendationCleanupJob) Run() {
	if j == nil || j.store == nil || j.store.DialogRecommendations == nil {
		logx.Error("RecommendationCleanupJob: PostgreSQL store is required")
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	now := time.Now().Unix()

	logx.Info("RecommendationCleanupJob: starting cleanup of expired recommendations")

	// 删除过期的推荐记录
	deletedCount, err := j.store.DialogRecommendations.DeleteExpired(ctx, now)
	if err != nil {
		logx.Errorf("RecommendationCleanupJob: failed to delete expired recommendations: %v", err)
		return
	}

	logx.Infof("RecommendationCleanupJob: cleaned up %d expired recommendations", deletedCount)

	// TODO: 可选的清理操作
	// 1. 清理长时间未点击的推荐（shown_count > 10 && clicked_count = 0）
	// 2. 清理过期的反馈日志
	// 3. 统计和报告清理结果

	logx.Info("RecommendationCleanupJob: completed cleanup")
}

// Schedule 返回任务执行计划
// 建议配置为每天凌晨2点执行
func (j *RecommendationCleanupJob) Schedule() string {
	// Cron 表达式：每天凌晨2点执行
	// 格式：分 时 日 月 周
	// 0 2 * * * 表示每天凌晨2:00执行
	return "0 2 * * *"
}
