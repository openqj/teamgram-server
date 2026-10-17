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
	return d.InsertOrUpdateOn(ctx, d.db, do)
}

// InsertOrUpdateOn inserts or updates a recommendation using the supplied
// PostgreSQL connection or transaction.
func (d *DialogRecommendationsDAO) InsertOrUpdateOn(ctx context.Context, db DB, do *dataobject.DialogRecommendationsDO) (int64, error) {
	var id int64
	err := db.QueryRow(ctx, `INSERT INTO dialog_recommendations
		(user_id, peer_type, peer_id, recommendation_type, weight, score, is_promoted,
		 promoted_until, promoted_priority, recommended_at, expires_at, last_updated,
		 shown_count, clicked_count, dismissed, dismissed_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16)
		ON CONFLICT (user_id, peer_type, peer_id) DO UPDATE SET
			 recommendation_type = CASE WHEN EXCLUDED.is_promoted
				THEN EXCLUDED.recommendation_type
				WHEN dialog_recommendations.is_promoted AND
					(dialog_recommendations.promoted_until = 0 OR
					 dialog_recommendations.promoted_until > EXTRACT(EPOCH FROM NOW())::BIGINT)
				THEN dialog_recommendations.recommendation_type ELSE EXCLUDED.recommendation_type END,
		 weight = EXCLUDED.weight,
		 score = EXCLUDED.score,
			 is_promoted = EXCLUDED.is_promoted OR
				(dialog_recommendations.is_promoted AND
				 (dialog_recommendations.promoted_until = 0 OR
				  dialog_recommendations.promoted_until > EXTRACT(EPOCH FROM NOW())::BIGINT)),
			 promoted_until = CASE WHEN EXCLUDED.is_promoted
				THEN EXCLUDED.promoted_until
				WHEN dialog_recommendations.promoted_until > EXTRACT(EPOCH FROM NOW())::BIGINT
				THEN dialog_recommendations.promoted_until ELSE 0 END,
			 promoted_priority = CASE WHEN EXCLUDED.is_promoted
				THEN EXCLUDED.promoted_priority
				WHEN dialog_recommendations.promoted_until > EXTRACT(EPOCH FROM NOW())::BIGINT
				THEN dialog_recommendations.promoted_priority ELSE 0 END,
		 recommended_at = EXCLUDED.recommended_at,
			 expires_at = CASE WHEN dialog_recommendations.is_promoted AND NOT EXCLUDED.is_promoted
				AND dialog_recommendations.promoted_until > EXTRACT(EPOCH FROM NOW())::BIGINT
				THEN dialog_recommendations.expires_at ELSE EXCLUDED.expires_at END,
		 last_updated = EXCLUDED.last_updated,
			 shown_count = dialog_recommendations.shown_count,
			 clicked_count = dialog_recommendations.clicked_count,
			 dismissed = CASE WHEN EXCLUDED.is_promoted THEN EXCLUDED.dismissed
				ELSE dialog_recommendations.dismissed OR EXCLUDED.dismissed END,
			 dismissed_at = CASE WHEN EXCLUDED.is_promoted THEN EXCLUDED.dismissed_at
				WHEN dialog_recommendations.dismissed THEN dialog_recommendations.dismissed_at
				ELSE EXCLUDED.dismissed_at END,
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
		ORDER BY weight DESC, score DESC, peer_type, peer_id
		LIMIT $2`, userID, limit)
	if err != nil {
		return nil, err
	}
	return scanRecommendations(rows)
}

// SelectPromotedList 获取推广对话列表
func (d *DialogRecommendationsDAO) SelectPromotedList(ctx context.Context, userID int64) ([]dataobject.DialogRecommendationsDO, error) {
	return d.SelectPromotedListWithLimit(ctx, userID, 10)
}

// SelectPromotedListWithLimit returns promoted rows with an explicit page
// capacity for recommendation pagination.
func (d *DialogRecommendationsDAO) SelectPromotedListWithLimit(ctx context.Context, userID int64, limit int32) ([]dataobject.DialogRecommendationsDO, error) {
	if limit <= 0 {
		limit = 10
	}
	rows, err := d.db.Query(ctx, `SELECT `+recommendationColumns+`
		FROM dialog_recommendations
		WHERE user_id = $1
		  AND deleted = FALSE
		  AND dismissed = FALSE
		  AND is_promoted = TRUE
		  AND (promoted_until = 0 OR promoted_until > EXTRACT(EPOCH FROM NOW())::BIGINT)
		ORDER BY promoted_priority DESC, weight DESC, peer_type, peer_id
		LIMIT $2`, userID, limit)
	if err != nil {
		return nil, err
	}
	return scanRecommendations(rows)
}

// SelectAllList returns all non-deleted recommendations, including dismissed
// rows. Refresh uses this state to avoid reintroducing a user's dismissal.
func (d *DialogRecommendationsDAO) SelectAllList(ctx context.Context, userID int64) ([]dataobject.DialogRecommendationsDO, error) {
	rows, err := d.db.Query(ctx, `SELECT `+recommendationColumns+`
		FROM dialog_recommendations
		WHERE user_id = $1 AND deleted = FALSE
		ORDER BY weight DESC, score DESC, peer_type, peer_id`, userID)
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
	return d.MarkDismissedOn(ctx, d.db, userID, peerType, peerID)
}

func (d *DialogRecommendationsDAO) MarkDismissedOn(ctx context.Context, db DB, userID int64, peerType int32, peerID int64) (int64, error) {
	tag, err := db.Exec(ctx, `UPDATE dialog_recommendations
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
	return d.UpdateStatsOn(ctx, d.db, userID, peerType, peerID, shownDelta, clickedDelta)
}

func (d *DialogRecommendationsDAO) UpdateStatsOn(ctx context.Context, db DB, userID int64, peerType int32, peerID int64, shownDelta, clickedDelta int32) (int64, error) {
	tag, err := db.Exec(ctx, `UPDATE dialog_recommendations
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

// RemovePromoted marks a promoted row deleted so a later algorithm refresh can
// reuse the unique user/peer key without retaining promotion state.
func (d *DialogRecommendationsDAO) RemovePromoted(ctx context.Context, userID int64, peerType int32, peerID int64) (int64, error) {
	return d.RemovePromotedOn(ctx, d.db, userID, peerType, peerID)
}

func (d *DialogRecommendationsDAO) RemovePromotedOn(ctx context.Context, db DB, userID int64, peerType int32, peerID int64) (int64, error) {
	tag, err := db.Exec(ctx, `UPDATE dialog_recommendations
		SET deleted = TRUE, is_promoted = FALSE, promoted_until = 0,
			promoted_priority = 0, dismissed = FALSE, dismissed_at = 0,
			last_updated = EXTRACT(EPOCH FROM NOW())::BIGINT
		WHERE user_id = $1 AND peer_type = $2 AND peer_id = $3 AND is_promoted = TRUE`,
		userID, peerType, peerID)
	if err != nil {
		return 0, err
	}
	return rowsAffected(tag), nil
}

// InsertFeedbackLog appends an immutable user feedback event.
func (d *DialogRecommendationsDAO) InsertFeedbackLog(ctx context.Context, userID int64, peerType int32, peerID int64, feedbackType int32, timestamp int64, sessionID string, deviceInfo []byte) (int64, error) {
	return d.InsertFeedbackLogOn(ctx, d.db, userID, peerType, peerID, feedbackType, timestamp, sessionID, deviceInfo)
}

func (d *DialogRecommendationsDAO) InsertFeedbackLogOn(ctx context.Context, db DB, userID int64, peerType int32, peerID int64, feedbackType int32, timestamp int64, sessionID string, deviceInfo []byte) (int64, error) {
	if len(deviceInfo) == 0 {
		deviceInfo = []byte(`{}`)
	}
	var id int64
	err := db.QueryRow(ctx, `INSERT INTO recommendation_feedback_logs
		(user_id, peer_type, peer_id, feedback_type, timestamp, session_id, device_info)
		VALUES ($1,$2,$3,$4,$5,$6,$7::jsonb)
		RETURNING id`, userID, peerType, peerID, feedbackType, timestamp, sessionID, deviceInfo).Scan(&id)
	if err != nil {
		return 0, err
	}
	return id, nil
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
	return d.BatchInsertOn(ctx, d.db, recommendations)
}

func (d *DialogRecommendationsDAO) BatchInsertOn(ctx context.Context, db DB, recommendations []dataobject.DialogRecommendationsDO) error {
	if len(recommendations) == 0 {
		return nil
	}

	for i := range recommendations {
		do := &recommendations[i]
		_, err := d.InsertOrUpdateOn(ctx, db, do)
		if err != nil {
			return err
		}
	}

	return nil
}
