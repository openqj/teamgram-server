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

	dialogclient "github.com/teamgram/teamgram-server/app/service/biz/dialog/client"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/dialog"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/internal/dal/dao/postgres_dao"
	"github.com/zeromicro/go-zero/core/logx"
)

// RecommendationRefreshJob 推荐刷新定时任务
type RecommendationRefreshJob struct {
	dialogClient dialogclient.DialogClient
	store        *postgres_dao.Store
}

// NewRecommendationRefreshJob 创建推荐刷新任务
func NewRecommendationRefreshJob(dialogClient dialogclient.DialogClient, store *postgres_dao.Store) *RecommendationRefreshJob {
	return &RecommendationRefreshJob{
		dialogClient: dialogClient,
		store:        store,
	}
}

// Run 执行推荐刷新
// 每6小时执行一次，为活跃用户刷新推荐列表
func (j *RecommendationRefreshJob) Run() {
	logx.Info("RecommendationRefreshJob: starting recommendation refresh")
	if j == nil || j.dialogClient == nil || j.store == nil || j.store.Dialogs == nil {
		logx.Error("RecommendationRefreshJob: dialog client and PostgreSQL store are required")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	userIDs, err := j.store.Dialogs.SelectActiveUserIDs(ctx, time.Now().Add(-7*24*time.Hour).Unix(), 10000)
	if err != nil {
		logx.Errorf("RecommendationRefreshJob: failed to load active users: %v", err)
		return
	}

	for _, userID := range userIDs {
		if _, err := j.dialogClient.DialogRefreshRecommendations(ctx, &dialog.TLDialogRefreshRecommendations{UserId: userID}); err != nil {
			logx.Errorf("RecommendationRefreshJob: failed to refresh recommendations for user %d: %v", userID, err)
		}
		select {
		case <-ctx.Done():
			logx.Errorf("RecommendationRefreshJob: stopped after context deadline: %v", ctx.Err())
			return
		case <-time.After(10 * time.Millisecond):
		}
	}

	logx.Infof("RecommendationRefreshJob: completed recommendation refresh for %d users", len(userIDs))
}

// Schedule 返回任务执行计划
// 建议配置为每6小时执行一次
func (j *RecommendationRefreshJob) Schedule() string {
	// Cron 表达式：每6小时执行一次
	// 格式：分 时 日 月 周
	// 0 */6 * * * 表示每6小时的整点执行
	return "0 */6 * * *"
}
