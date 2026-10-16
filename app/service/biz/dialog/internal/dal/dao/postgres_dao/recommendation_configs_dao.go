package postgres_dao

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/internal/dal/dataobject"
)

type RecommendationConfigsDAO struct{ db DB }

func NewRecommendationConfigsDAO(db DB) *RecommendationConfigsDAO {
	return &RecommendationConfigsDAO{db: db}
}

const configColumns = `id, config_key, config_value, enabled, description, created_at, updated_at`

func scanConfig(row interface{ Scan(...any) error }) (*dataobject.RecommendationConfigsDO, error) {
	do := new(dataobject.RecommendationConfigsDO)
	err := row.Scan(&do.Id, &do.ConfigKey, &do.ConfigValue, &do.Enabled,
		&do.Description, &do.CreatedAt, &do.UpdatedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return do, nil
}

// SelectByKey 根据配置键获取配置
func (d *RecommendationConfigsDAO) SelectByKey(ctx context.Context, key string) (*dataobject.RecommendationConfigsDO, error) {
	return scanConfig(d.db.QueryRow(ctx, `SELECT `+configColumns+`
		FROM recommendation_configs
		WHERE config_key = $1 AND enabled = TRUE`, key))
}

// SelectAll 获取所有启用的配置
func (d *RecommendationConfigsDAO) SelectAll(ctx context.Context) ([]dataobject.RecommendationConfigsDO, error) {
	rows, err := d.db.Query(ctx, `SELECT `+configColumns+`
		FROM recommendation_configs
		WHERE enabled = TRUE
		ORDER BY config_key`)
	if err != nil {
		return nil, err
	}

	var configs []dataobject.RecommendationConfigsDO
	for rows.Next() {
		config, err := scanConfig(rows)
		if err != nil {
			return nil, err
		}
		if config != nil {
			configs = append(configs, *config)
		}
	}
	return configs, rows.Err()
}

// UpdateValue 更新配置值
func (d *RecommendationConfigsDAO) UpdateValue(ctx context.Context, key, value string) (int64, error) {
	tag, err := d.db.Exec(ctx, `UPDATE recommendation_configs
		SET config_value = $2, updated_at = EXTRACT(EPOCH FROM NOW())::BIGINT
		WHERE config_key = $1`, key, value)
	if err != nil {
		return 0, err
	}
	return rowsAffected(tag), nil
}
