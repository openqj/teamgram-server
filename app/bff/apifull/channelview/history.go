package channelview

import (
	"errors"
	"sync"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/domain"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func mapDomain(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, domain.ErrChannelMissing):
		return mtproto.ErrChannelInvalid
	case errors.Is(err, domain.ErrNotCreator):
		return mtproto.ErrChatAdminRequired
	case errors.Is(err, domain.ErrNotChannelMember):
		return mtproto.ErrUserNotParticipant
	case errors.Is(err, domain.ErrChannelWriteForbidden):
		return mtproto.ErrChatWriteForbidden
	case errors.Is(err, domain.ErrMessageMissing):
		return mtproto.ErrMessageIdInvalid
	case errors.Is(err, domain.ErrInvalidMessageID):
		return mtproto.ErrMessageIdInvalid
	default:
		return err
	}
}

func messageOf(row domain.ChannelMessage, views int32) *mtproto.Message {
	msg := &mtproto.Message{
		Out:     true,
		Id:      row.MessageID,
		FromId:  mtproto.MakePeerUser(row.Sender),
		PeerId:  mtproto.MakePeerChannel(row.ChannelID),
		Date:    int32(row.Date),
		Message: row.Text,
		Pinned:  row.Pinned,
		Views:   wrapperspb.Int32(views),
		// The current TL schema shares this flag with forwards, so encode both fields.
		Forwards: wrapperspb.Int32(0),
	}
	if row.Edited && row.EditedAt != 0 {
		msg.EditDate = wrapperspb.Int32(int32(row.EditedAt))
	}
	if row.ReplyToMsgID > 0 {
		msg.ReplyTo = mtproto.MakeTLMessageReplyHeader(&mtproto.MessageReplyHeader{
			ReplyToMsgId:           row.ReplyToMsgID,
			ReplyToMsgId_INT32:     row.ReplyToMsgID,
			ReplyToMsgId_FLAGINT32: wrapperspb.Int32(row.ReplyToMsgID),
			ReplyToTopId: func() *wrapperspb.Int32Value {
				if row.ReplyToTopID > 0 {
					return wrapperspb.Int32(row.ReplyToTopID)
				}
				return nil
			}(),
		}).To_MessageReplyHeader()
	}
	return mtproto.MakeTLMessage(msg).To_Message()
}

func updatesNew(rows []domain.ChannelMessage) *mtproto.Updates {
	ups := make([]*mtproto.Update, 0, len(rows))
	for _, row := range rows {
		msg := messageOf(row, 0)
		ups = append(ups, mtproto.MakeTLUpdateNewChannelMessage(&mtproto.Update{
			Message_MESSAGE: msg,
			Pts_INT32:       row.Pts,
			PtsCount:        1,
		}).To_Update())
	}
	return mtproto.MakeUpdatesByUpdates(ups...)
}

func Post(sender, channelID int64, text string, when int64) (*mtproto.Updates, error) {
	row, err := domain.InsertChannelMessage(channelID, sender, when, text)
	if err != nil {
		return nil, mapDomain(err)
	}
	return updatesNew([]domain.ChannelMessage{row}), nil
}

func PostWithReply(sender, channelID int64, text string, when int64, replyToMsgID, replyToTopID int32) (*mtproto.Updates, error) {
	row, err := domain.InsertChannelMessageWithReply(channelID, sender, when, text, replyToMsgID, replyToTopID)
	if err != nil {
		return nil, mapDomain(err)
	}
	return updatesNew([]domain.ChannelMessage{row}), nil
}

func PostAll(sender, channelID int64, texts []string, when int64) (*mtproto.Updates, error) {
	rows := make([]domain.ChannelMessage, 0, len(texts))
	for _, text := range texts {
		row, err := domain.InsertChannelMessage(channelID, sender, when, text)
		if err != nil {
			return nil, mapDomain(err)
		}
		rows = append(rows, row)
	}
	return updatesNew(rows), nil
}

func Edit(sender, channelID int64, id int32, text string) (*mtproto.Updates, error) {
	row, err := domain.UpdateChannelMessage(channelID, sender, id, text)
	if err != nil {
		return nil, mapDomain(err)
	}
	messages, err := messagesOf([]domain.ChannelMessage{row})
	if err != nil {
		return nil, err
	}
	update := mtproto.MakeTLUpdateEditChannelMessage(&mtproto.Update{
		Message_MESSAGE: messages[0],
		Pts_INT32:       row.Pts,
		PtsCount:        1,
	}).To_Update()
	return mtproto.MakeUpdatesByUpdates(update), nil
}

func DeleteMessages(userID, channelID int64, ids []int32) (*mtproto.Messages_AffectedMessages, error) {
	if _, ok, err := domain.LoadChannel(channelID); err != nil {
		return nil, mapDomain(err)
	} else if !ok {
		return nil, mtproto.ErrChannelInvalid
	}
	deleted, pts, err := domain.DeleteChannelMessages(channelID, userID, ids)
	if err != nil {
		return nil, mapDomain(err)
	}
	return mtproto.MakeTLMessagesAffectedMessages(&mtproto.Messages_AffectedMessages{
		Pts:      pts,
		PtsCount: int32(len(deleted)),
	}).To_Messages_AffectedMessages(), nil
}

func DeleteHistory(userID, channelID int64, maxID int32, forEveryone bool) (*mtproto.Updates, error) {
	if _, ok, err := domain.LoadChannel(channelID); err != nil {
		return nil, mapDomain(err)
	} else if !ok {
		return nil, mtproto.ErrChannelInvalid
	}
	if !forEveryone {
		if _, err := domain.HideChannelHistory(userID, channelID, maxID); err != nil {
			return nil, mapDomain(err)
		}
		return mtproto.MakeEmptyUpdates(), nil
	}
	deleted, pts, err := domain.DeleteChannelHistory(channelID, userID, maxID)
	if err != nil {
		return nil, mapDomain(err)
	}
	if len(deleted) == 0 {
		return mtproto.MakeEmptyUpdates(), nil
	}
	update := mtproto.MakeTLUpdateDeleteChannelMessages(&mtproto.Update{
		ChannelId: channelID,
		Messages:  deleted,
		Pts_INT32: pts,
		PtsCount:  int32(len(deleted)),
	}).To_Update()
	return mtproto.MakeUpdatesByUpdates(update), nil
}

func History(userID, channelID int64, offsetID, limit int32) (*mtproto.Messages_Messages, error) {
	ch, ok, err := domain.LoadChannel(channelID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, mtproto.ErrChannelInvalid
	}
	rows, err := domain.ListChannelMessages(userID, channelID, offsetID, limit)
	if err != nil {
		return nil, err
	}
	return messageBox(ch, userID, rows)
}

// ValidateInputPeer verifies the supplied channel access hash and membership.
func ValidateInputPeer(userID int64, peer *mtproto.InputPeer) (int64, error) {
	if peer == nil || peer.GetPredicateName() != mtproto.Predicate_inputPeerChannel {
		return 0, mtproto.ErrChannelInvalid
	}

	ch, ok, err := domain.LoadChannel(peer.GetChannelId())
	if err != nil {
		return 0, stored(err)
	}
	if !ok || ch.AccessHash != peer.GetAccessHash() {
		return 0, mtproto.ErrChannelInvalid
	}
	member, err := domain.ChannelIsMember(ch.ID, userID)
	if err != nil {
		return 0, stored(err)
	}
	if !member {
		return 0, mtproto.ErrUserNotParticipant
	}
	return ch.ID, nil
}

// HistoryForInputPeer verifies the supplied channel access hash and membership
// before exposing APIFull's local channel messages.
func HistoryForInputPeer(userID int64, peer *mtproto.InputPeer, offsetID, limit int32) (*mtproto.Messages_Messages, error) {
	channelID, err := ValidateInputPeer(userID, peer)
	if err != nil {
		return nil, err
	}
	return History(userID, channelID, offsetID, limit)
}

// PostForInputPeer verifies the exact channel peer before adding a stored
// channel message. The generic message sender has already normalized this peer
// into a PeerUtil, which does not retain the access hash.
func PostForInputPeer(userID int64, peer *mtproto.InputPeer, text string, when int64) (*mtproto.Updates, error) {
	return PostForInputPeerWithReply(userID, peer, text, when, 0, 0)
}

func PostForInputPeerWithReply(userID int64, peer *mtproto.InputPeer, text string, when int64, replyToMsgID, replyToTopID int32) (*mtproto.Updates, error) {
	channelID, err := ValidateInputPeer(userID, peer)
	if err != nil {
		return nil, err
	}
	// Replies to broadcast posts are stored in the linked megagroup. The
	// broadcast root remains authoritative for validation, while the linked
	// channel owns the reply rows returned by messages.getReplies.
	targetID := channelID
	if replyToMsgID > 0 || replyToTopID > 0 {
		linkedID, linkErr := domain.DiscussionGroupID(channelID)
		if linkErr != nil {
			return nil, mapDomain(linkErr)
		}
		if linkedID > 0 {
			targetID = linkedID
		}
	}
	return PostWithReply(userID, targetID, text, when, replyToMsgID, replyToTopID)
}

func PersonalHistory(viewerID, ownerID, channelID int64, minID, maxID, limit int32) (*mtproto.Messages_Messages, error) {
	ch, ok, err := domain.LoadChannel(channelID)
	if err != nil {
		return nil, err
	}
	if !ok || ch.Creator != ownerID {
		return nil, mtproto.ErrChannelInvalid
	}
	if limit <= 0 {
		limit = 20
	}
	if limit > 50 {
		limit = 50
	}
	rows, count, err := domain.ListChannelMessagesRange(viewerID, channelID, minID, maxID, limit)
	if err != nil {
		return nil, err
	}
	messages, err := messagesOf(rows)
	if err != nil {
		return nil, err
	}
	box := &mtproto.Messages_Messages{
		Messages: messages,
		Topics:   []*mtproto.ForumTopic{},
		Chats:    []*mtproto.Chat{Chat(ch, ch.Creator == viewerID)},
		Users:    []*mtproto.User{},
	}
	if int32(len(rows)) == limit {
		box.Count = count
		return mtproto.MakeTLMessagesMessagesSlice(box).To_Messages_Messages(), nil
	}
	return mtproto.MakeTLMessagesMessages(box).To_Messages_Messages(), nil
}

func ReadHistory(userID, channelID int64, maxID int32) (int32, error) {
	if _, ok, err := domain.LoadChannel(channelID); err != nil {
		return 0, err
	} else if !ok {
		return 0, mtproto.ErrChannelInvalid
	}
	readMax, err := domain.MarkChannelReadHistory(userID, channelID, maxID)
	if err != nil {
		return 0, mapDomain(err)
	}
	return readMax, nil
}

// Ready reports whether this process has opened the APIFull MySQL store.
// Callers that also serve the basic message API use this to avoid routing a
// channel read into the generic message store when the channel store is not
// configured in that process.
func Ready() bool {
	return domain.Ready()
}

// ReadHistoryForInputPeer validates a channel input peer and records its read
// cursor in the APIFull channel store. The affected-messages result uses the
// channel's persisted pts sequence, and only reports a pts change when the
// caller advanced its cursor.
func ReadHistoryForInputPeer(userID int64, peer *mtproto.InputPeer, maxID int32) (*mtproto.Messages_AffectedMessages, error) {
	channelID, err := ValidateInputPeer(userID, peer)
	if err != nil {
		return nil, err
	}
	previous, err := domain.ChannelReadMaxID(userID, channelID)
	if err != nil {
		return nil, stored(err)
	}
	readMax, err := ReadHistory(userID, channelID, maxID)
	if err != nil {
		return nil, err
	}
	pts, err := domain.ChannelMessagePTS(channelID)
	if err != nil {
		return nil, stored(err)
	}
	ptsCount := int32(0)
	if readMax > previous {
		ptsCount = 1
	}
	return mtproto.MakeTLMessagesAffectedMessages(&mtproto.Messages_AffectedMessages{
		Pts:      pts,
		PtsCount: ptsCount,
	}).To_Messages_AffectedMessages(), nil
}

// PresentIDs reports which of ids are stored for the channel. A missing id is absent, not an error.
func PresentIDs(userID, channelID int64, ids []int32) (map[int32]struct{}, error) {
	_, ok, err := domain.LoadChannel(channelID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, mtproto.ErrChannelInvalid
	}
	rows, err := domain.ChannelMessagesByID(userID, channelID, ids)
	if err != nil {
		return nil, err
	}
	found := make(map[int32]struct{}, len(rows))
	for _, row := range rows {
		found[row.MessageID] = struct{}{}
	}
	return found, nil
}

func MessagesBox(userID, channelID int64, ids []int32) (*mtproto.Messages_Messages, error) {
	ch, ok, err := domain.LoadChannel(channelID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, mtproto.ErrChannelInvalid
	}
	if len(ids) == 0 {
		return messageBox(ch, userID, nil)
	}
	rows, err := domain.ChannelMessagesByID(userID, channelID, ids)
	if err != nil {
		return nil, err
	}
	byID := map[int32]domain.ChannelMessage{}
	for _, row := range rows {
		byID[row.MessageID] = row
	}
	ordered := make([]domain.ChannelMessage, 0, len(ids))
	for _, id := range ids {
		row, found := byID[id]
		if !found {
			return nil, mtproto.ErrMessageIdInvalid
		}
		ordered = append(ordered, row)
	}
	return messageBox(ch, userID, ordered)
}

// MessageViewsForInputPeer verifies channel access and returns persisted view
// counts for each requested, visible message in request order. When increment
// is true, it first advances the caller's persisted read cursor to the highest
// requested message ID.
func MessageViewsForInputPeer(userID int64, peer *mtproto.InputPeer, ids []int32, increment bool) ([]*mtproto.MessageViews, error) {
	if _, err := HistoryForInputPeer(userID, peer, 0, 1); err != nil {
		return nil, err
	}
	if _, err := MessagesBox(userID, peer.GetChannelId(), ids); err != nil {
		return nil, err
	}
	if increment && len(ids) > 0 {
		maxID := ids[0]
		for _, id := range ids[1:] {
			if id > maxID {
				maxID = id
			}
		}
		if _, err := domain.MarkChannelReadHistory(userID, peer.GetChannelId(), maxID); err != nil {
			return nil, stored(err)
		}
	}
	counts, err := domain.ChannelMessageViewCounts(peer.GetChannelId(), ids)
	if err != nil {
		return nil, stored(err)
	}
	views := make([]*mtproto.MessageViews, 0, len(ids))
	for _, id := range ids {
		count, ok := counts[id]
		if !ok {
			return nil, mtproto.ErrMessageIdInvalid
		}
		views = append(views, mtproto.MakeTLMessageViews(&mtproto.MessageViews{
			Views: wrapperspb.Int32(count),
		}).To_MessageViews())
	}
	return views, nil
}

func GetMessageEditData(userID int64, peer *mtproto.InputPeer, messageID int32) (*mtproto.Messages_MessageEditData, error) {
	if peer == nil || peer.GetPredicateName() != mtproto.Predicate_inputPeerChannel {
		return nil, mtproto.ErrChannelInvalid
	}
	if messageID <= 0 {
		return nil, mtproto.ErrMessageIdInvalid
	}

	channelID := peer.GetChannelId()
	ch, ok, err := domain.LoadChannel(channelID)
	if err != nil {
		return nil, err
	}
	if !ok || ch.AccessHash != peer.GetAccessHash() {
		return nil, mtproto.ErrChannelInvalid
	}
	member, err := domain.ChannelIsMember(channelID, userID)
	if err != nil {
		return nil, err
	}
	if !member {
		return nil, mtproto.ErrUserNotParticipant
	}
	if ch.Creator != userID {
		return nil, mtproto.ErrChatAdminRequired
	}
	if _, err = MessagesBox(userID, channelID, []int32{messageID}); err != nil {
		return nil, err
	}
	return mtproto.MakeTLMessagesMessageEditData(&mtproto.Messages_MessageEditData{Caption: false}).To_Messages_MessageEditData(), nil
}

func Search(userID, channelID int64, q string, sender int64, beforeID, addOffset, minDate, maxDate, minID, maxID, limit int32) (*mtproto.Messages_Messages, error) {
	ch, ok, err := domain.LoadChannel(channelID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, mtproto.ErrChannelInvalid
	}
	rows, total, err := domain.SearchChannelMessages(userID, channelID, q, sender, beforeID, addOffset, minDate, maxDate, minID, maxID, limit)
	if err != nil {
		return nil, err
	}
	box, err := messageBox(ch, userID, rows)
	if err != nil {
		return nil, err
	}
	if total > int32(len(rows)) {
		box.Count = total
		box.PredicateName = mtproto.Predicate_messages_messagesSlice
	}
	return box, nil
}

// SearchForInputPeer verifies the supplied channel access hash and membership
// before exposing APIFull's local channel message search.
func SearchForInputPeer(userID int64, peer *mtproto.InputPeer, q string, sender int64, beforeID, addOffset, minDate, maxDate, minID, maxID, limit int32) (*mtproto.Messages_Messages, error) {
	channelID, err := ValidateInputPeer(userID, peer)
	if err != nil {
		return nil, err
	}
	return Search(userID, channelID, q, sender, beforeID, addOffset, minDate, maxDate, minID, maxID, limit)
}

func Pinned(userID, channelID int64, limit int32) (*mtproto.Messages_Messages, error) {
	ch, ok, err := domain.LoadChannel(channelID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, mtproto.ErrChannelInvalid
	}
	rows, err := domain.ListPinnedChannelMessages(userID, channelID, limit)
	if err != nil {
		return nil, err
	}
	return messageBox(ch, userID, rows)
}

func Empty(userID, channelID int64) (*mtproto.Messages_Messages, error) {
	ch, ok, err := domain.LoadChannel(channelID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, mtproto.ErrChannelInvalid
	}
	return messageBox(ch, userID, nil)
}

func Texts(userID, channelID int64, ids []int32) ([]string, error) {
	_, ok, err := domain.LoadChannel(channelID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, mtproto.ErrChannelInvalid
	}
	rows, err := domain.ChannelMessagesByID(userID, channelID, ids)
	if err != nil {
		return nil, err
	}
	byID := map[int32]string{}
	for _, row := range rows {
		byID[row.MessageID] = row.Text
	}
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		text, found := byID[id]
		if !found {
			return nil, mtproto.ErrMessageIdInvalid
		}
		out = append(out, text)
	}
	return out, nil
}

func messageBox(ch domain.Channel, userID int64, rows []domain.ChannelMessage) (*mtproto.Messages_Messages, error) {
	msgs, err := messagesOf(rows)
	if err != nil {
		return nil, err
	}
	return mtproto.MakeTLMessagesMessages(&mtproto.Messages_Messages{
		Messages: msgs,
		Topics:   []*mtproto.ForumTopic{},
		Chats:    []*mtproto.Chat{Chat(ch, ch.Creator == userID)},
		Users:    []*mtproto.User{},
	}).To_Messages_Messages(), nil
}

func messagesOf(rows []domain.ChannelMessage) ([]*mtproto.Message, error) {
	msgs := make([]*mtproto.Message, 0, len(rows))
	if len(rows) == 0 {
		return msgs, nil
	}
	ids := make([]int32, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.MessageID)
	}
	counts, err := domain.ChannelMessageViewCounts(rows[0].ChannelID, ids)
	if err != nil {
		return nil, stored(err)
	}
	for _, row := range rows {
		views, ok := counts[row.MessageID]
		if !ok {
			return nil, mtproto.ErrMessageIdInvalid
		}
		msgs = append(msgs, messageOf(row, views))
	}
	return msgs, nil
}

var (
	openOnce sync.Once
	openErr  error
)

// Open connects this process to the same MySQL the BFF reads.
// A failed first attempt stays failed until the process restarts.
func Open(dsn string) error {
	if domain.Ready() {
		return nil
	}
	if dsn == "" {
		return errors.New("domain mysql is not open")
	}
	openOnce.Do(func() {
		openErr = domain.Open(dsn)
	})
	return openErr
}

func Pin(sender, channelID int64, id int32, unpin bool) (*mtproto.Updates, error) {
	row, err := domain.SetChannelMessagePinned(channelID, sender, id, !unpin)
	if err != nil {
		return nil, mapDomain(err)
	}
	return mtproto.MakeTLUpdates(&mtproto.Updates{
		Updates: []*mtproto.Update{
			mtproto.MakeTLUpdatePinnedChannelMessages(&mtproto.Update{
				Pinned:    row.Pinned,
				ChannelId: channelID,
				Messages:  []int32{row.MessageID},
				Pts_INT32: row.Pts,
				PtsCount:  1,
			}).To_Update(),
		},
		Users: []*mtproto.User{},
		Chats: []*mtproto.Chat{},
	}).To_Updates(), nil
}

// UnpinAll removes all creator-pinned channel messages and returns the
// affected-history cursor used by messages.unpinAllMessages.
func UnpinAll(sender, channelID int64) (*mtproto.Messages_AffectedHistory, error) {
	ids, pts, err := domain.ClearChannelMessagePins(channelID, sender)
	if err != nil {
		return nil, mapDomain(err)
	}
	return mtproto.MakeTLMessagesAffectedHistory(&mtproto.Messages_AffectedHistory{
		Pts:      pts,
		PtsCount: int32(len(ids)),
		Offset:   0,
	}).To_Messages_AffectedHistory(), nil
}
