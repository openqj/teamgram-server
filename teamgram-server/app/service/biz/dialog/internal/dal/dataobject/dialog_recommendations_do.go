/*
 * Copyright (c) 2024-present,  Teamgram Authors.
 *  All rights reserved.
 *
 * Author: teamgramio (teamgram.io@gmail.com)
 */

package dataobject

type DialogRecommendationsDO struct {
	Id                 int64   `db:"id" json:"id"`
	UserId             int64   `db:"user_id" json:"user_id"`
	PeerType           int32   `db:"peer_type" json:"peer_type"`
	PeerId             int64   `db:"peer_id" json:"peer_id"`
	RecommendationType int32   `db:"recommendation_type" json:"recommendation_type"`
	Weight             int32   `db:"weight" json:"weight"`
	Score              float64 `db:"score" json:"score"`
	IsPromoted         bool    `db:"is_promoted" json:"is_promoted"`
	PromotedUntil      int64   `db:"promoted_until" json:"promoted_until"`
	PromotedPriority   int32   `db:"promoted_priority" json:"promoted_priority"`
	RecommendedAt      int64   `db:"recommended_at" json:"recommended_at"`
	ExpiresAt          int64   `db:"expires_at" json:"expires_at"`
	LastUpdated        int64   `db:"last_updated" json:"last_updated"`
	ShownCount         int32   `db:"shown_count" json:"shown_count"`
	ClickedCount       int32   `db:"clicked_count" json:"clicked_count"`
	Dismissed          bool    `db:"dismissed" json:"dismissed"`
	DismissedAt        int64   `db:"dismissed_at" json:"dismissed_at"`
	Deleted            bool    `db:"deleted" json:"deleted"`
}
