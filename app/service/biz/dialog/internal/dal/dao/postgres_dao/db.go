// Package postgres_dao contains the PostgreSQL persistence boundary for the
// dialog aggregate and uses pgx directly.
package postgres_dao

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// DB is implemented by pgxpool.Pool and pgx.Tx. It lets callers keep dialog
// state changes in the same transaction as message/update mutations.
type DB interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

type Store struct {
	Pool                   *pgxpool.Pool
	Dialogs                *DialogsDAO
	DialogFilters          *DialogFiltersDAO
	Drafts                 *DraftsDAO
	SavedDialogs           *SavedDialogsDAO
	DialogFilterTags       *DialogFilterTagsDAO
	DialogRecommendations  *DialogRecommendationsDAO
	RecommendationConfigs  *RecommendationConfigsDAO
}

func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{
		Pool:                  pool,
		Dialogs:               NewDialogsDAO(pool),
		DialogFilters:         NewDialogFiltersDAO(pool),
		Drafts:                NewDraftsDAO(pool),
		SavedDialogs:          NewSavedDialogsDAO(pool),
		DialogFilterTags:      NewDialogFilterTagsDAO(pool),
		DialogRecommendations: NewDialogRecommendationsDAO(pool),
		RecommendationConfigs: NewRecommendationConfigsDAO(pool),
	}
}

func rowsAffected(tag pgconn.CommandTag) int64 { return tag.RowsAffected() }

func scanRows[T any](rows pgx.Rows, scan func(pgx.Rows, *T) error) ([]T, error) {
	defer rows.Close()
	result := make([]T, 0)
	for rows.Next() {
		var value T
		if err := scan(rows, &value); err != nil {
			return nil, err
		}
		result = append(result, value)
	}
	return result, rows.Err()
}
