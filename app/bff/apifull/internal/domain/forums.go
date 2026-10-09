package domain

import (
	"database/sql"
	"errors"
	"strconv"
	"time"

	"github.com/teamgram/proto/mtproto"
)

type ForumTopic struct {
	ID          int32
	Title       string
	Closed      bool
	Pinned      bool
	Hidden      bool
	Date        int32
	CreatorID   int64
	IconEmojiID int64
	TopMessage  int32
	Position    int64
}

type ForumChannelSettings struct {
	Enabled        bool
	Tabs           bool
	ViewAsMessages bool
}

func forumPeerLock(tx *sql.Tx, peerKey string) error {
	_, err := tx.Exec(`SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, peerKey)
	return err
}

func CreateForumTopic(peerKey string, userID int64, title string) error {
	if db == nil {
		return errors.New("domain PostgreSQL is not open")
	}
	if peerKey == "" || userID <= 0 || title == "" {
		return mtproto.ErrInputRequestInvalid
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err = forumPeerLock(tx, peerKey); err != nil {
		return err
	}
	var position int64
	if err = tx.QueryRow(`SELECT COALESCE(MAX(position), 0) + 1 FROM apifull_forum_topic WHERE peer_key=$1`, peerKey).Scan(&position); err != nil {
		return err
	}
	var topicID int32
	if err = tx.QueryRow(`INSERT INTO apifull_forum_topic (peer_key, title, creator_user_id, topic_date, position)
		VALUES ($1, $2, $3, $4, $5) RETURNING topic_id`, peerKey, title, userID, int32(time.Now().Unix()), position).Scan(&topicID); err != nil {
		return err
	}
	if _, err = tx.Exec(`UPDATE apifull_forum_topic SET top_message=$1 WHERE peer_key=$2 AND topic_id=$3`, topicID, peerKey, topicID); err != nil {
		return err
	}
	if _, err = tx.Exec(`INSERT INTO apifull_forum_user_title (user_id, title) VALUES ($1, $2)
		ON CONFLICT (user_id) DO UPDATE SET title=EXCLUDED.title`, userID, title); err != nil {
		return err
	}
	return tx.Commit()
}

func ListForumTopics(peerKey string) ([]ForumTopic, error) {
	if db == nil {
		return nil, errors.New("domain PostgreSQL is not open")
	}
	rows, err := db.Query(`SELECT topic_id, title, closed, pinned, hidden, topic_date, creator_user_id,
		icon_emoji_id, top_message, position FROM apifull_forum_topic WHERE peer_key=$1 ORDER BY position, topic_id`, peerKey)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	topics := make([]ForumTopic, 0)
	for rows.Next() {
		var topic ForumTopic
		if err = rows.Scan(&topic.ID, &topic.Title, &topic.Closed, &topic.Pinned, &topic.Hidden, &topic.Date,
			&topic.CreatorID, &topic.IconEmojiID, &topic.TopMessage, &topic.Position); err != nil {
			return nil, err
		}
		topics = append(topics, topic)
	}
	return topics, rows.Err()
}

func UpdateForumTopic(peerKey string, topicID int32, update func(*ForumTopic) error) error {
	if db == nil {
		return errors.New("domain PostgreSQL is not open")
	}
	if peerKey == "" || topicID <= 0 {
		return mtproto.ErrTopicIdInvalid
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err = forumPeerLock(tx, peerKey); err != nil {
		return err
	}
	var topic ForumTopic
	err = tx.QueryRow(`SELECT topic_id, title, closed, pinned, hidden, topic_date, creator_user_id,
		icon_emoji_id, top_message, position FROM apifull_forum_topic
		WHERE peer_key=$1 AND topic_id=$2 FOR UPDATE`, peerKey, topicID).Scan(
		&topic.ID, &topic.Title, &topic.Closed, &topic.Pinned, &topic.Hidden, &topic.Date,
		&topic.CreatorID, &topic.IconEmojiID, &topic.TopMessage, &topic.Position)
	if err == sql.ErrNoRows {
		return mtproto.ErrTopicIdInvalid
	}
	if err != nil {
		return err
	}
	if err = update(&topic); err != nil {
		return err
	}
	if _, err = tx.Exec(`UPDATE apifull_forum_topic SET title=$1, closed=$2, pinned=$3, hidden=$4,
		icon_emoji_id=$5 WHERE peer_key=$6 AND topic_id=$7`,
		topic.Title, topic.Closed, topic.Pinned, topic.Hidden, topic.IconEmojiID, peerKey, topicID); err != nil {
		return err
	}
	return tx.Commit()
}

func ReorderForumTopics(peerKey string, order []int32) error {
	if db == nil {
		return errors.New("domain PostgreSQL is not open")
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err = forumPeerLock(tx, peerKey); err != nil {
		return err
	}
	rows, err := tx.Query(`SELECT topic_id FROM apifull_forum_topic WHERE peer_key=$1 ORDER BY position, topic_id FOR UPDATE`, peerKey)
	if err != nil {
		return err
	}
	current := make([]int32, 0)
	byID := make(map[int32]struct{})
	for rows.Next() {
		var id int32
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		current = append(current, id)
		byID[id] = struct{}{}
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	seen := make(map[int32]struct{}, len(order))
	reordered := make([]int32, 0, len(current))
	for _, id := range order {
		if _, ok := byID[id]; !ok {
			return mtproto.ErrTopicIdInvalid
		}
		if _, duplicate := seen[id]; duplicate {
			continue
		}
		seen[id] = struct{}{}
		reordered = append(reordered, id)
	}
	for _, id := range current {
		if _, ok := seen[id]; !ok {
			reordered = append(reordered, id)
		}
	}
	for position, id := range reordered {
		if _, err = tx.Exec(`UPDATE apifull_forum_topic SET position=$1 WHERE peer_key=$2 AND topic_id=$3`, position+1, peerKey, id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func DeleteForumTopic(peerKey string, topicID int32) (bool, error) {
	if db == nil {
		return false, errors.New("domain PostgreSQL is not open")
	}
	tx, err := db.Begin()
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	if err = forumPeerLock(tx, peerKey); err != nil {
		return false, err
	}
	result, err := tx.Exec(`DELETE FROM apifull_forum_topic WHERE peer_key=$1 AND topic_id=$2`, peerKey, topicID)
	if err != nil {
		return false, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	if err = tx.Commit(); err != nil {
		return false, err
	}
	return affected > 0, nil
}

func ForumUserTitle(userID int64) (string, error) {
	if db == nil {
		return "", errors.New("domain PostgreSQL is not open")
	}
	var title string
	err := db.QueryRow(`SELECT title FROM apifull_forum_user_title WHERE user_id=$1`, userID).Scan(&title)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return title, err
}

func ClearForumUserTitleIfNoTopics(userID int64, peerKey string) error {
	if db == nil {
		return errors.New("domain PostgreSQL is not open")
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err = forumPeerLock(tx, peerKey); err != nil {
		return err
	}
	var count int
	if err = tx.QueryRow(`SELECT COUNT(*) FROM apifull_forum_topic WHERE peer_key=$1`, peerKey).Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return tx.Commit()
	}
	if _, err = tx.Exec(`INSERT INTO apifull_forum_user_title (user_id, title) VALUES ($1, '')
		ON CONFLICT (user_id) DO UPDATE SET title=''`, userID); err != nil {
		return err
	}
	return tx.Commit()
}

func LoadForumChannelSettings(channelID int64) (ForumChannelSettings, error) {
	if db == nil {
		return ForumChannelSettings{}, errors.New("domain PostgreSQL is not open")
	}
	var settings ForumChannelSettings
	err := db.QueryRow(`SELECT enabled, tabs, view_as_messages FROM apifull_forum_channel_settings WHERE channel_id=$1`, channelID).
		Scan(&settings.Enabled, &settings.Tabs, &settings.ViewAsMessages)
	if err == sql.ErrNoRows {
		return ForumChannelSettings{}, nil
	}
	return settings, err
}

func UpdateForumChannelSettings(channelID int64, update func(*ForumChannelSettings)) error {
	if db == nil {
		return errors.New("domain PostgreSQL is not open")
	}
	if channelID <= 0 {
		return mtproto.ErrChannelInvalid
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err = forumPeerLock(tx, "channel-settings:"+strconv.FormatInt(channelID, 10)); err != nil {
		return err
	}
	var settings ForumChannelSettings
	err = tx.QueryRow(`SELECT enabled, tabs, view_as_messages FROM apifull_forum_channel_settings WHERE channel_id=$1 FOR UPDATE`, channelID).
		Scan(&settings.Enabled, &settings.Tabs, &settings.ViewAsMessages)
	if err != nil && err != sql.ErrNoRows {
		return err
	}
	update(&settings)
	if _, err = tx.Exec(`INSERT INTO apifull_forum_channel_settings (channel_id, enabled, tabs, view_as_messages)
		VALUES ($1,$2,$3,$4) ON CONFLICT (channel_id) DO UPDATE SET
		enabled=EXCLUDED.enabled, tabs=EXCLUDED.tabs, view_as_messages=EXCLUDED.view_as_messages`,
		channelID, settings.Enabled, settings.Tabs, settings.ViewAsMessages); err != nil {
		return err
	}
	return tx.Commit()
}
