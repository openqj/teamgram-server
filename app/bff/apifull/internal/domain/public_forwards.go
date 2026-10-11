package domain

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/teamgram/proto/mtproto"
)

// PublicForwardMessage is the durable projection consumed by the statistics
// methods. The message consumer owns writes; APIFull only validates and reads
// this projection. Keeping the encoded message lets the projection preserve
// the exact Layer 229 message constructor instead of reconstructing media or
// entities from counters.
type PublicForwardMessage struct {
	ID                   int64
	SourceChannelID      int64
	SourceMessageID      int32
	DestinationPeerType  int32
	DestinationPeerID    int64
	DestinationMessageID int32
	MessageDate          int64
	Message              *mtproto.Message
}

var ErrPublicForwardInvalid = errors.New("invalid public forward")
var ErrPublicForwardCursorInvalid = errors.New("invalid public forward cursor")

// SavePublicForwardMessage records an already-authoritative forwarded
// message. It is idempotent for the source/destination tuple and intentionally
// does not manufacture a message when the caller has no canonical payload.
func SavePublicForwardMessage(sourceChannelID int64, sourceMessageID int32, message *mtproto.Message) error {
	if db == nil {
		return errors.New("domain PostgreSQL is not open")
	}
	if sourceChannelID <= 0 || sourceMessageID <= 0 || message == nil || message.GetId() <= 0 || message.GetPeerId() == nil {
		return ErrPublicForwardInvalid
	}
	peer := message.GetPeerId()
	peerType, peerID := publicForwardPeer(peer)
	if peerType == 0 || peerID <= 0 {
		return ErrPublicForwardInvalid
	}
	if message.GetDate() <= 0 {
		return ErrPublicForwardInvalid
	}
	encoded, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("encode public forward: %w", err)
	}
	_, err = db.Exec(`INSERT INTO apifull_public_forward_message
		(source_channel_id, source_message_id, destination_peer_type,
		 destination_peer_id, destination_message_id, message_date, message_json, created_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7::jsonb,$8)
		ON CONFLICT (source_channel_id, source_message_id, destination_peer_type,
		             destination_peer_id, destination_message_id) DO UPDATE SET
		 message_date=EXCLUDED.message_date, message_json=EXCLUDED.message_json`,
		sourceChannelID, sourceMessageID, peerType, peerID, message.GetId(), message.GetDate(), string(encoded), time.Now().Unix())
	return err
}

// ListPublicForwardMessages reads the source-owned reverse index with a
// stable numeric cursor. An empty result is authoritative and is distinct
// from a missing table/provider error.
func ListPublicForwardMessages(sourceChannelID int64, sourceMessageID int32, offset string, limit int32) ([]PublicForwardMessage, int32, string, error) {
	if db == nil {
		return nil, 0, "", errors.New("domain PostgreSQL is not open")
	}
	if sourceChannelID <= 0 || sourceMessageID <= 0 || limit <= 0 {
		return nil, 0, "", ErrPublicForwardInvalid
	}
	var cursor int64
	if strings.TrimSpace(offset) != "" {
		parsed, err := strconv.ParseInt(strings.TrimSpace(offset), 10, 64)
		if err != nil || parsed < 0 {
			return nil, 0, "", ErrPublicForwardCursorInvalid
		}
		cursor = parsed
	}
	var count int32
	if err := db.QueryRow(`SELECT COUNT(*) FROM apifull_public_forward_message
		WHERE source_channel_id=$1 AND source_message_id=$2`, sourceChannelID, sourceMessageID).Scan(&count); err != nil {
		return nil, 0, "", err
	}
	rows, err := db.Query(`SELECT id, destination_peer_type, destination_peer_id,
		destination_message_id, message_date, message_json
		FROM apifull_public_forward_message
		WHERE source_channel_id=$1 AND source_message_id=$2 AND id>$3
		ORDER BY id ASC LIMIT $4`, sourceChannelID, sourceMessageID, cursor, limit+1)
	if err != nil {
		return nil, 0, "", err
	}
	defer rows.Close()
	out := make([]PublicForwardMessage, 0, limit)
	var next string
	for rows.Next() {
		var row PublicForwardMessage
		var peerType int32
		var raw []byte
		if err := rows.Scan(&row.ID, &peerType, &row.DestinationPeerID, &row.DestinationMessageID, &row.MessageDate, &raw); err != nil {
			return nil, 0, "", err
		}
		var message mtproto.Message
		if err := json.Unmarshal(raw, &message); err != nil {
			return nil, 0, "", fmt.Errorf("decode public forward %d: %w", row.ID, err)
		}
		if message.GetId() <= 0 || message.GetPeerId() == nil {
			return nil, 0, "", ErrPublicForwardInvalid
		}
		row.SourceChannelID = sourceChannelID
		row.SourceMessageID = sourceMessageID
		row.DestinationPeerType = peerType
		row.Message = mtproto.MakeTLMessage(&message).To_Message()
		if int32(len(out)) == limit {
			next = strconv.FormatInt(row.ID, 10)
			break
		}
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, "", err
	}
	return out, count, next, nil
}

func publicForwardPeer(peer *mtproto.Peer) (int32, int64) {
	if peer == nil {
		return 0, 0
	}
	switch peer.GetPredicateName() {
	case mtproto.Predicate_peerUser:
		return mtproto.PEER_USER, peer.GetUserId()
	case mtproto.Predicate_peerChat:
		return mtproto.PEER_CHAT, peer.GetChatId()
	case mtproto.Predicate_peerChannel:
		return mtproto.PEER_CHANNEL, peer.GetChannelId()
	}
	if peer.GetChannelId() > 0 {
		return mtproto.PEER_CHANNEL, peer.GetChannelId()
	}
	if peer.GetChatId() > 0 {
		return mtproto.PEER_CHAT, peer.GetChatId()
	}
	if peer.GetUserId() > 0 {
		return mtproto.PEER_USER, peer.GetUserId()
	}
	return 0, 0
}

func publicForwardMessageToTL(row PublicForwardMessage) *mtproto.PublicForward {
	return mtproto.MakeTLPublicForwardMessage(&mtproto.PublicForward{Message: row.Message}).To_PublicForward()
}

func publicForwardMessagesToTL(rows []PublicForwardMessage) []*mtproto.PublicForward {
	out := make([]*mtproto.PublicForward, 0, len(rows))
	for _, row := range rows {
		out = append(out, publicForwardMessageToTL(row))
	}
	return out
}

// PublicForwardMessagesToTL converts the durable projection into the typed
// stats union without exposing storage details to the BFF core package.
func PublicForwardMessagesToTL(rows []PublicForwardMessage) []*mtproto.PublicForward {
	return publicForwardMessagesToTL(rows)
}

func publicForwardMessagesToLegacy(rows []PublicForwardMessage) []*mtproto.Message {
	out := make([]*mtproto.Message, 0, len(rows))
	for _, row := range rows {
		if row.Message != nil {
			out = append(out, row.Message)
		}
	}
	return out
}

// PublicForwardMessagesToLegacy converts the same projection for the legacy
// messages.Messages result constructor.
func PublicForwardMessagesToLegacy(rows []PublicForwardMessage) []*mtproto.Message {
	return publicForwardMessagesToLegacy(rows)
}

// PublicForwardStory is the durable projection consumed by
// stats.getStoryPublicForwards.
type PublicForwardStory struct {
	ID                  int64
	SourceUserID        int64
	SourceStoryID       int32
	DestinationPeerType int32
	DestinationPeerID   int64
	DestinationStoryID  int32
	Peer                *mtproto.Peer
	Story               *mtproto.StoryItem
}

// SavePublicForwardStory records an already-authoritative story forward. The
// source owner is checked by the handler before this projection is read; the
// write API refuses incomplete constructors and is idempotent for the same
// source/destination tuple.
func SavePublicForwardStory(sourceUserID int64, sourceStoryID int32, peer *mtproto.Peer, story *mtproto.StoryItem) error {
	if db == nil {
		return errors.New("domain PostgreSQL is not open")
	}
	if sourceUserID <= 0 || sourceStoryID <= 0 || peer == nil || story == nil || story.GetId() <= 0 {
		return ErrPublicForwardInvalid
	}
	peerType, peerID := publicForwardPeer(peer)
	if peerType == 0 || peerID <= 0 {
		return ErrPublicForwardInvalid
	}
	peerJSON, err := json.Marshal(peer)
	if err != nil {
		return fmt.Errorf("encode public forward story peer: %w", err)
	}
	storyJSON, err := json.Marshal(story)
	if err != nil {
		return fmt.Errorf("encode public forward story: %w", err)
	}
	_, err = db.Exec(`INSERT INTO apifull_public_forward_story
		(source_user_id, source_story_id, destination_peer_type, destination_peer_id,
		 destination_story_id, story_json, peer_json, created_at)
		VALUES ($1,$2,$3,$4,$5,$6::jsonb,$7::jsonb,$8)
		ON CONFLICT (source_user_id, source_story_id, destination_peer_type,
		             destination_peer_id, destination_story_id) DO UPDATE SET
		 story_json=EXCLUDED.story_json, peer_json=EXCLUDED.peer_json`,
		sourceUserID, sourceStoryID, peerType, peerID, story.GetId(), string(storyJSON), string(peerJSON), time.Now().Unix())
	return err
}

// ListPublicForwardStories reads the source-owned story index using the same
// stable identity cursor as message forwards.
func ListPublicForwardStories(sourceUserID int64, sourceStoryID int32, offset string, limit int32) ([]PublicForwardStory, int32, string, error) {
	if db == nil {
		return nil, 0, "", errors.New("domain PostgreSQL is not open")
	}
	if sourceUserID <= 0 || sourceStoryID <= 0 || limit <= 0 {
		return nil, 0, "", ErrPublicForwardInvalid
	}
	var cursor int64
	if strings.TrimSpace(offset) != "" {
		parsed, err := strconv.ParseInt(strings.TrimSpace(offset), 10, 64)
		if err != nil || parsed < 0 {
			return nil, 0, "", ErrPublicForwardCursorInvalid
		}
		cursor = parsed
	}
	var count int32
	if err := db.QueryRow(`SELECT COUNT(*) FROM apifull_public_forward_story
		WHERE source_user_id=$1 AND source_story_id=$2`, sourceUserID, sourceStoryID).Scan(&count); err != nil {
		return nil, 0, "", err
	}
	rows, err := db.Query(`SELECT id, destination_peer_type, destination_peer_id,
		destination_story_id, story_json, peer_json
		FROM apifull_public_forward_story
		WHERE source_user_id=$1 AND source_story_id=$2 AND id>$3
		ORDER BY id ASC LIMIT $4`, sourceUserID, sourceStoryID, cursor, limit+1)
	if err != nil {
		return nil, 0, "", err
	}
	defer rows.Close()
	out := make([]PublicForwardStory, 0, limit)
	var next string
	for rows.Next() {
		var row PublicForwardStory
		var rawStory, rawPeer []byte
		if err := rows.Scan(&row.ID, &row.DestinationPeerType, &row.DestinationPeerID, &row.DestinationStoryID, &rawStory, &rawPeer); err != nil {
			return nil, 0, "", err
		}
		var story mtproto.StoryItem
		var peer mtproto.Peer
		if err := json.Unmarshal(rawStory, &story); err != nil {
			return nil, 0, "", fmt.Errorf("decode public forward story %d: %w", row.ID, err)
		}
		if err := json.Unmarshal(rawPeer, &peer); err != nil {
			return nil, 0, "", fmt.Errorf("decode public forward story peer %d: %w", row.ID, err)
		}
		peerType, peerID := publicForwardPeer(&peer)
		if story.GetId() <= 0 || peerType == 0 || peerID <= 0 {
			return nil, 0, "", ErrPublicForwardInvalid
		}
		if peer.GetPredicateName() == "" {
			switch peerType {
			case mtproto.PEER_USER:
				peer = *mtproto.MakePeerUser(peerID)
			case mtproto.PEER_CHAT:
				peer = *mtproto.MakePeerChat(peerID)
			case mtproto.PEER_CHANNEL:
				peer = *mtproto.MakePeerChannel(peerID)
			}
		}
		storyTL := mtproto.MakeTLStoryItem(&story).To_StoryItem()
		row.SourceUserID = sourceUserID
		row.SourceStoryID = sourceStoryID
		row.Peer = &peer
		row.Story = storyTL
		if int32(len(out)) == limit {
			next = strconv.FormatInt(row.ID, 10)
			break
		}
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, "", err
	}
	return out, count, next, nil
}

// PublicForwardStoriesToTL converts durable story rows into publicForwardStory
// constructors for stats responses.
func PublicForwardStoriesToTL(rows []PublicForwardStory) []*mtproto.PublicForward {
	out := make([]*mtproto.PublicForward, 0, len(rows))
	for _, row := range rows {
		out = append(out, mtproto.MakeTLPublicForwardStory(&mtproto.PublicForward{Peer: row.Peer, Story: row.Story}).To_PublicForward())
	}
	return out
}
