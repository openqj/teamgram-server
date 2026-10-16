package postgres_dao

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/internal/dal/dataobject"
)

type DialogRecommendationsDAO struct{ db DB }

func NewDialogRecommendationsDAO(db DB) *DialogRecommendationsDAO {
	return &DialogRecommendationsDAO{db: db}
}

const recommendationColumns = `id, user_id, peer_type, peer_id, recommendation_type, weight, score,
	is_promoted, promoted_until, promoted_priority, recommended_at, expires_at, last_updated,
	shown_count, clicked_count, dismissed, dismissed_at, deleted`

func scanRecommendation(row interface{ Scan(...any) error }) (*dataobject.DialogRecommendationsDO, error) {
	do := new(dataobject.DialogRecommendationsDO)
	err := row.Scan(&do.Id, &do.UserId, &do.PeerType, &do.PeerId, &do.RecommendationType,
		&do.Weight, &do.Score, &do.IsPromoted, &do.PromotedUntil, &do.PromotedPriority,
		&do.RecommendedAt, &do.ExpiresAt, &do.LastUpdated, &do.ShownCount, &do.ClickedCount,
		&do.Dismissed, &do.DismissedAt, &do.Deleted)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return do, nil
}

func scanRecommendations(rows pgx.Rows) ([]dataobject.DialogRecommendationsDO, error) {
	return scanRows(rows, func(rows pgx.Rows, do *dataobject.DialogRecommendationsDO) error {
		v, err := scanRecommendation(rows)
		if err != nil {
			return err
		}
		if v != nil {
			*do = *v
		}
		return nil
	})
}

// InsertOrUpdate 插入或更新推荐
func (d *DialogRecommendationsDAO) InsertOrUpdate(ctx context.Context, do *dataobject.DialogRecommendationsDO) (int64, error) {
	var id int64
	err := d.db.QueryRow(ctx, `INSERT INTO dialog_recommendations
		(user_id, peer_type, peer_id, recommendation_type, weight, score, is_promoted,
		 promoted_until, promoted_priority, recommended_at, expires_at, last_updated,
		 shown_count, clicked_count, dismissed, dismissed_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16)
		ON CONFLICT (user_id, peer_type, peer_id) DO UPDATE SET
		 recommendation_type = EXCLUDED.recommendation_type,
		 weight = EXCLUDED.weight,
		 score = EXCLUDED.score,
		 is_promoted = EXCLUDED.is_promoted,
		 promoted_until = EXCLUDED.promoted_until,
		 promoted_priority = EXCLUDED.promoted_priority,
		 recommended_at = EXCLUDED.recommended_at,
		 expires_at = EXCLUDED.expires_at,
		 last_updated = EXCLUDED.last_updated,
		 shown_count = EXCLUDED.shown_count,
		 clicked_count = EXCLUDED.clicked_count,
		 dismissed = EXCLUDED.dismissed,
		 dismissed_at = EXCLUDED.dismissed_at,
		 deleted = FALSE
		RETURNING id`,
		do.UserId, do.PeerType, do.PeerId, do.RecommendationType, do.Weight, do.Score,
		do.IsPromoted, do.PromotedUntil, do.PromotedPriority, do.RecommendedAt,
		do.ExpiresAt, do.LastUpdated, do.ShownCount, do.ClickedCount,
		do.Dismissed, do.DismissedAt).Scan(&id)
	if err != nil {
		return 0, err
	}
	return id, nil
}

// SelectActiveList 获取用户的活跃推荐列表
func (d *DialogRecommendationsDAO) SelectActiveList(ctx context.Context, userID int64, limit int32) ([]dataobject.DialogRecommendationsDO, error) {
	rows, err := d.db.Query(ctx, `SELECT `+recommendationColumns+`
		FROM dialog_recommendations
		WHERE user_id = $1
		  AND deleted = FALSE
		  AND dismissed = FALSE
		  AND (expires_at = 0 OR expires_at > EXTRACT(EPOCH FROM NOW())::BIGINT)
		ORDER BY weight DESC, score DESC
		LIMIT $2`, userID, limit)
	if err != nil {
		return nil, err
	}
	return scanRecommendations(rows)
}

// SelectPromotedList 获取推广对话列表
func (d *DialogRecommendationsDAO) SelectPromotedList(ctx context.Context, userID int64) ([]dataobject.DialogRecommendationsDO, error) {
	rows, err := d.db.Query(ctx, `SELECT `+recommendationColumns+`
		FROM dialog_recommendations
		WHERE user_id = $1
		  AND deleted = FALSE
		  AND dismissed = FALSE
		  AND is_promoted = TRUE
		  AND (promoted_until = 0 OR promoted_until > EXTRACT(EPOCH FROM NOW())::BIGINT)
		ORDER BY promoted_priority DESC, weight DESC
		LIMIT 10`, userID)
	if err != nil {
		return nil, err
	}
	return scanRecommendations(rows)
}

// Select 根据用户和对话获取推荐
func (d *DialogRecommendationsDAO) Select(ctx context.Context, userID int64, peerType int32, peerID int64) (*dataobject.DialogRecommendationsDO, error) {
	return scanRecommendation(d.db.QueryRow(ctx, `SELECT `+recommendationColumns+`
		FROM dialog_recommendations
		WHERE user_id = $1 AND peer_type = $2 AND peer_id = $3 AND deleted = FALSE`,
		userID, peerType, peerID))
}

// MarkDismissed 标记为已隐藏
func (d *DialogRecommendationsDAO) MarkDismissed(ctx context.Context, userID int64, peerType int32, peerID int64) (int64, error) {
	tag, err := d.db.Exec(ctx, `UPDATE dialog_recommendations
		SET dismissed = TRUE, dismissed_at = EXTRACT(EPOCH FROM NOW())::BIGINT
		WHERE user_id = $1 AND peer_type = $2 AND peer_id = $3`,
		userID, peerType, peerID)
	if err != nil {
		return 0, err
	}
	return rowsAffected(tag), nil
}

// UpdateStats 更新展示/点击统计
func (d *DialogRecommendationsDAO) UpdateStats(ctx context.Context, userID int64, peerType int32, peerID int64, shownDelta, clickedDelta int32) (int64, error) {
	tag, err := d.db.Exec(ctx, `UPDATE dialog_recommendations
		SET shown_count = shown_count + $4,
		    clicked_count = clicked_count + $5,
		    last_updated = EXTRACT(EPOCH FROM NOW())::BIGINT
		WHERE user_id = $1 AND peer_type = $2 AND peer_id = $3`,
		userID, peerType, peerID, shownDelta, clickedDelta)
	if err != nil {
		return 0, err
	}
	return rowsAffected(tag), nil
}

// DeleteExpired 清理过期推荐
func (d *DialogRecommendationsDAO) DeleteExpired(ctx context.Context, beforeTimestamp int64) (int64, error) {
	tag, err := d.db.Exec(ctx, `UPDATE dialog_recommendations
		SET deleted = TRUE
		WHERE expires_at > 0 AND expires_at < $1 AND deleted = FALSE`,
		beforeTimestamp)
	if err != nil {
		return 0, err
	}
	return rowsAffected(tag), nil
}

// BatchInsert 批量插入推荐
func (d *DialogRecommendationsDAO) BatchInsert(ctx context.Context, recommendations []dataobject.DialogRecommendationsDO) error {
	if len(recommendations) == 0 {
		return nil
	}

	for i := range recommendations {
		do := &recommendations[i]
		_, err := d.InsertOrUpdate(ctx, do)
		if err != nil {
			return err
		}
	}

	return nil
}
