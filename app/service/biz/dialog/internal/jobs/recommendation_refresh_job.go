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

	"github.com/teamgram/teamgram-server/app/service/biz/dialog/dialog"
	"github.com/zeromicro/go-zero/core/logx"
)

// RecommendationRefreshJob 推荐刷新定时任务
type RecommendationRefreshJob struct {
	dialogClient dialog.DialogClient
}

// NewRecommendationRefreshJob 创建推荐刷新任务
func NewRecommendationRefreshJob(dialogClient dialog.DialogClient) *RecommendationRefreshJob {
	return &RecommendationRefreshJob{
		dialogClient: dialogClient,
	}
}

// Run 执行推荐刷新
// 每6小时执行一次，为活跃用户刷新推荐列表
func (j *RecommendationRefreshJob) Run() {
	ctx := context.Background()
	logx.Info("RecommendationRefreshJob: starting recommendation refresh")

	// TODO: 获取需要刷新推荐的用户列表
	// 策略：
	// 1. 最近7天活跃的用户
	// 2. 推荐列表为空或接近过期的用户
	// 3. 批量处理，避免一次性加载所有用户

	// 示例：假设从配置或数据库获取活跃用户列表
	// activeUserIds := getActiveUsers(ctx)

	// for _, userId := range activeUserIds {
	// 	_, err := j.dialogClient.DialogRefreshRecommendations(ctx, &dialog.TLDialogRefreshRecommendations{
	// 		UserId: userId,
	// 	})
	// 	if err != nil {
	// 		logx.Errorf("RecommendationRefreshJob: failed to refresh recommendations for user %d: %v", userId, err)
	// 		continue
	// 	}
	//
	// 	// 避免瞬时压力过大，添加小延迟
	// 	time.Sleep(10 * time.Millisecond)
	// }

	logx.Info("RecommendationRefreshJob: completed recommendation refresh")
}

// Schedule 返回任务执行计划
// 建议配置为每6小时执行一次
func (j *RecommendationRefreshJob) Schedule() string {
	// Cron 表达式：每6小时执行一次
	// 格式：分 时 日 月 周
	// 0 */6 * * * 表示每6小时的整点执行
	return "0 */6 * * *"
}
