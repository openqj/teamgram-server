package persist

import (
	"database/sql"
	"encoding/json"
	"sort"
)

type GameScore struct {
	UserID int64
	Score  int32
}

// StoreGameScore persists one player's score. The PostgreSQL path updates a
// row atomically; the blob path remains for isolated in-memory unit tests.
func StoreGameScore(scope, key string, userID int64, score int32, force bool) (bool, error) {
	if store, ok := Default.(*postgresStore); ok {
		return store.storeGameScore(scope, key, userID, score, force)
	}

	updated := false
	err := Update(gameScoreBlobKey(scope, key), func(raw string) (string, error) {
		users := map[int64]int32{}
		if raw != "" {
			if err := json.Unmarshal([]byte(raw), &users); err != nil {
				return "", err
			}
		}
		if previous, ok := users[userID]; ok && !force && score <= previous {
			return raw, nil
		}
		users[userID] = score
		data, err := json.Marshal(users)
		if err != nil {
			return "", err
		}
		updated = true
		return string(data), nil
	})
	return updated, err
}

// LoadGameScores returns a scoreboard ordered by score, then user ID.
func LoadGameScores(scope, key string) ([]GameScore, error) {
	if store, ok := Default.(*postgresStore); ok {
		return store.loadGameScores(scope, key)
	}
	raw, err := Default.Get(gameScoreBlobKey(scope, key))
	if err != nil {
		return nil, err
	}
	users := map[int64]int32{}
	if raw != "" {
		if err = json.Unmarshal([]byte(raw), &users); err != nil {
			return nil, err
		}
	}
	scores := make([]GameScore, 0, len(users))
	for userID, score := range users {
		scores = append(scores, GameScore{UserID: userID, Score: score})
	}
	sort.Slice(scores, func(i, j int) bool {
		if scores[i].Score != scores[j].Score {
			return scores[i].Score > scores[j].Score
		}
		return scores[i].UserID < scores[j].UserID
	})
	return scores, nil
}

func gameScoreBlobKey(scope, key string) string {
	if scope == "inline" {
		return "game:inline:" + key
	}
	return "game:" + key
}

func (s *postgresStore) storeGameScore(scope, key string, userID int64, score int32, force bool) (bool, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()

	var stored int32
	err = tx.QueryRow(`INSERT INTO apifull_game_score (scope, game_key, user_id, score)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (scope, game_key, user_id) DO UPDATE
		SET score = EXCLUDED.score, updated_at = CURRENT_TIMESTAMP
		WHERE $5 OR apifull_game_score.score < EXCLUDED.score
		RETURNING score`, scope, key, userID, score, force).Scan(&stored)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if err = tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}

func (s *postgresStore) loadGameScores(scope, key string) ([]GameScore, error) {
	rows, err := s.db.Query(`SELECT user_id, score FROM apifull_game_score
		WHERE scope = $1 AND game_key = $2 ORDER BY score DESC, user_id ASC`, scope, key)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	scores := make([]GameScore, 0)
	for rows.Next() {
		var score GameScore
		if err := rows.Scan(&score.UserID, &score.Score); err != nil {
			return nil, err
		}
		scores = append(scores, score)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return scores, nil
}
