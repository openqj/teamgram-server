/*
 * Copyright (c) 2024-present,  Teamgram Authors.
 *  All rights reserved.
 *
 * Author: teamgramio (teamgram.io@gmail.com)
 */

package postgres_dao

import (
	"context"
	"testing"
	"time"

	"github.com/teamgram/teamgram-server/app/service/biz/dialog/internal/dal/dataobject"
)

// TestDialogRecommendationsDAO_InsertOrUpdate 测试插入或更新推荐
func TestDialogRecommendationsDAO_InsertOrUpdate(t *testing.T) {
	// 此测试需要实际的数据库连接
	// 在 CI/CD 中应该使用测试数据库
	t.Skip("需要数据库连接，跳过")

	// dao := NewDialogRecommendationsDAO(testDB)
	// ctx := context.Background()
	// now := time.Now().Unix()
	//
	// do := &dataobject.DialogRecommendationsDO{
	// 	UserId:             12345,
	// 	PeerType:           1,
	// 	PeerId:             67890,
	// 	RecommendationType: 0,
	// 	Weight:             100,
	// 	Score:              0.85,
	// 	RecommendedAt:      now,
	// 	ExpiresAt:          now + 7*24*3600,
	// 	LastUpdated:        now,
	// }
	//
	// id, err := dao.InsertOrUpdate(ctx, do)
	// if err != nil {
	// 	t.Fatalf("InsertOrUpdate failed: %v", err)
	// }
	//
	// if id <= 0 {
	// 	t.Errorf("Expected id > 0, got %d", id)
	// }
}

// TestDialogRecommendationsDAO_SelectActiveList 测试获取活跃推荐列表
func TestDialogRecommendationsDAO_SelectActiveList(t *testing.T) {
	t.Skip("需要数据库连接，跳过")

	// dao := NewDialogRecommendationsDAO(testDB)
	// ctx := context.Background()
	//
	// recommendations, err := dao.SelectActiveList(ctx, 12345, 20)
	// if err != nil {
	// 	t.Fatalf("SelectActiveList failed: %v", err)
	// }
	//
	// // 验证结果按 weight DESC, score DESC 排序
	// for i := 1; i < len(recommendations); i++ {
	// 	prev := recommendations[i-1]
	// 	curr := recommendations[i]
	// 	if prev.Weight < curr.Weight {
	// 		t.Errorf("Recommendations not sorted by weight")
	// 	}
	// 	if prev.Weight == curr.Weight && prev.Score < curr.Score {
	// 		t.Errorf("Recommendations not sorted by score")
	// 	}
	// }
}

// TestDialogRecommendationsDAO_MarkDismissed 测试标记为已隐藏
func TestDialogRecommendationsDAO_MarkDismissed(t *testing.T) {
	t.Skip("需要数据库连接，跳过")

	// dao := NewDialogRecommendationsDAO(testDB)
	// ctx := context.Background()
	//
	// affected, err := dao.MarkDismissed(ctx, 12345, 1, 67890)
	// if err != nil {
	// 	t.Fatalf("MarkDismissed failed: %v", err)
	// }
	//
	// if affected != 1 {
	// 	t.Errorf("Expected 1 row affected, got %d", affected)
	// }
	//
	// // 验证已标记为 dismissed
	// rec, err := dao.Select(ctx, 12345, 1, 67890)
	// if err != nil {
	// 	t.Fatalf("Select failed: %v", err)
	// }
	// if !rec.Dismissed {
	// 	t.Errorf("Expected dismissed=true, got false")
	// }
}

// TestDialogRecommendationsDAO_UpdateStats 测试更新统计
func TestDialogRecommendationsDAO_UpdateStats(t *testing.T) {
	t.Skip("需要数据库连接，跳过")

	// dao := NewDialogRecommendationsDAO(testDB)
	// ctx := context.Background()
	//
	// // 增加展示次数
	// _, err := dao.UpdateStats(ctx, 12345, 1, 67890, 1, 0)
	// if err != nil {
	// 	t.Fatalf("UpdateStats failed: %v", err)
	// }
	//
	// // 增加点击次数
	// _, err = dao.UpdateStats(ctx, 12345, 1, 67890, 0, 1)
	// if err != nil {
	// 	t.Fatalf("UpdateStats failed: %v", err)
	// }
	//
	// // 验证统计更新
	// rec, err := dao.Select(ctx, 12345, 1, 67890)
	// if err != nil {
	// 	t.Fatalf("Select failed: %v", err)
	// }
	// if rec.ShownCount == 0 || rec.ClickedCount == 0 {
	// 	t.Errorf("Stats not updated: shown=%d, clicked=%d", rec.ShownCount, rec.ClickedCount)
	// }
}

// TestDialogRecommendationsDAO_DeleteExpired 测试清理过期推荐
func TestDialogRecommendationsDAO_DeleteExpired(t *testing.T) {
	t.Skip("需要数据库连接，跳过")

	// dao := NewDialogRecommendationsDAO(testDB)
	// ctx := context.Background()
	//
	// // 删除一天前过期的记录
	// beforeTimestamp := time.Now().Add(-24 * time.Hour).Unix()
	// affected, err := dao.DeleteExpired(ctx, beforeTimestamp)
	// if err != nil {
	// 	t.Fatalf("DeleteExpired failed: %v", err)
	// }
	//
	// t.Logf("Deleted %d expired recommendations", affected)
}

// TestDialogRecommendationsDAO_BatchInsert 测试批量插入
func TestDialogRecommendationsDAO_BatchInsert(t *testing.T) {
	t.Skip("需要数据库连接，跳过")

	// dao := NewDialogRecommendationsDAO(testDB)
	// ctx := context.Background()
	// now := time.Now().Unix()
	//
	// recommendations := []dataobject.DialogRecommendationsDO{
	// 	{
	// 		UserId:             12345,
	// 		PeerType:           1,
	// 		PeerId:             100,
	// 		RecommendationType: 0,
	// 		Weight:             100,
	// 		Score:              0.9,
	// 		RecommendedAt:      now,
	// 		ExpiresAt:          now + 7*24*3600,
	// 		LastUpdated:        now,
	// 	},
	// 	{
	// 		UserId:             12345,
	// 		PeerType:           1,
	// 		PeerId:             200,
	// 		RecommendationType: 0,
	// 		Weight:             90,
	// 		Score:              0.85,
	// 		RecommendedAt:      now,
	// 		ExpiresAt:          now + 7*24*3600,
	// 		LastUpdated:        now,
	// 	},
	// }
	//
	// err := dao.BatchInsert(ctx, recommendations)
	// if err != nil {
	// 	t.Fatalf("BatchInsert failed: %v", err)
	// }
	//
	// // 验证批量插入成功
	// list, err := dao.SelectActiveList(ctx, 12345, 10)
	// if err != nil {
	// 	t.Fatalf("SelectActiveList failed: %v", err)
	// }
	// if len(list) < 2 {
	// 	t.Errorf("Expected at least 2 recommendations, got %d", len(list))
	// }
}
