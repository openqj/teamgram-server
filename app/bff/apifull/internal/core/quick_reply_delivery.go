package core

import (
	"time"

	"github.com/teamgram/proto/mtproto"
	msgpb "github.com/teamgram/teamgram-server/app/messenger/msg/msg/msg"
	"google.golang.org/protobuf/proto"
)

func quickReplyTarget(userID int64, input *mtproto.InputPeer) (*mtproto.PeerUtil, error) {
	if userID <= 0 || input == nil {
		return nil, mtproto.ErrPeerIdInvalid
	}
	peer := mtproto.FromInputPeer2(userID, input)
	if peer == nil || peer.PeerId <= 0 {
		return nil, mtproto.ErrPeerIdInvalid
	}
	switch peer.PeerType {
	case mtproto.PEER_SELF:
		peer.PeerType = mtproto.PEER_USER
	case mtproto.PEER_USER:
		if input.GetPredicateName() == mtproto.Predicate_inputPeerUser && input.GetAccessHash() == 0 {
			return nil, mtproto.ErrPeerIdInvalid
		}
	case mtproto.PEER_CHAT:
	case mtproto.PEER_CHANNEL:
		return peer, nil
	default:
		return nil, mtproto.ErrPeerIdInvalid
	}
	return peer, nil
}

func quickReplyIDs(userID int64, shortcutID int32, requested []int32) ([]int32, error) {
	if shortcutID <= 0 || len(requested) == 0 {
		return nil, mtproto.ErrInputRequestInvalid
	}
	items, err := loadQuickReplyMsgs(userID)
	if err != nil {
		return nil, err
	}
	var stored []int32
	for _, item := range items {
		if item.ShortcutId == shortcutID {
			stored = append([]int32(nil), item.Ids...)
			break
		}
	}
	if len(stored) == 0 {
		return nil, mtproto.ErrMessageIdInvalid
	}
	available := make(map[int32]struct{}, len(stored))
	for _, id := range stored {
		if id <= 0 {
			return nil, mtproto.ErrMessageIdInvalid
		}
		available[id] = struct{}{}
	}
	seen := make(map[int32]struct{}, len(requested))
	for _, id := range requested {
		if id <= 0 {
			return nil, mtproto.ErrMessageIdInvalid
		}
		if _, ok := available[id]; !ok {
			return nil, mtproto.ErrMessageIdInvalid
		}
		if _, duplicate := seen[id]; duplicate {
			return nil, mtproto.ErrInputRequestInvalid
		}
		seen[id] = struct{}{}
	}
	return append([]int32(nil), requested...), nil
}

func (c *ApiFullCore) MessagesSendQuickReplyMessages(in *mtproto.TLMessagesSendQuickReplyMessages) (*mtproto.Updates, error) {
	userID, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil || in.GetPeer() == nil || len(in.GetId()) == 0 || len(in.GetId()) != len(in.GetRandomId()) {
		return nil, mtproto.ErrInputRequestInvalid
	}
	peer, err := quickReplyTarget(userID, in.GetPeer())
	if err != nil {
		return nil, err
	}
	if peer.IsChannel() {
		if err = c.authorizeReactionPeer(userID, in.GetPeer()); err != nil {
			return nil, err
		}
	}
	for _, randomID := range in.GetRandomId() {
		if randomID == 0 {
			return nil, mtproto.ErrInputRequestInvalid
		}
	}
	ids, err := quickReplyIDs(userID, in.GetShortcutId(), in.GetId())
	if err != nil {
		return nil, err
	}
	d := c.apifullDao()
	if d == nil || d.PollMessageReader == nil || d.ScheduledMessageSender == nil {
		return nil, mtproto.ErrMethodNotImpl
	}
	messages, err := c.hydrateQuickReplyMessages(userID, ids)
	if err != nil {
		return nil, err
	}
	outbox := make([]*msgpb.OutboxMessage, 0, len(messages))
	target := peer.ToPeer()
	if target == nil {
		return nil, mtproto.ErrPeerIdInvalid
	}
	for i, source := range messages {
		if source == nil || !source.GetOut() || source.GetFromId() == nil || source.GetFromId().GetUserId() != userID {
			return nil, mtproto.ErrMessageIdInvalid
		}
		message := proto.Clone(source).(*mtproto.Message)
		message.Id = 0
		message.Out = true
		message.FromId = mtproto.MakePeerUser(userID)
		message.PeerId = target
		message.Date = int32(time.Now().Unix())
		message.FromScheduled = false
		message.FwdFrom = nil
		message.ReplyTo = nil
		message.SavedPeerId = nil
		outbox = append(outbox, msgpb.MakeTLOutboxMessage(&msgpb.OutboxMessage{
			NoWebpage: true,
			RandomId:  in.GetRandomId()[i],
			Message:   message,
		}).To_OutboxMessage())
	}
	updates, err := d.ScheduledMessageSender.MsgSendMessageV2(callContext(c), &msgpb.TLMsgSendMessageV2{
		UserId:    userID,
		AuthKeyId: c.MD.PermAuthKeyId,
		PeerType:  peer.PeerType,
		PeerId:    peer.PeerId,
		Message:   outbox,
	})
	if err != nil {
		return nil, err
	}
	if updates == nil {
		return nil, mtproto.ErrInternalServerError
	}
	return updates, nil
}

func (c *ApiFullCore) MessagesDeleteQuickReplyMessages(in *mtproto.TLMessagesDeleteQuickReplyMessages) (*mtproto.Updates, error) {
	userID, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil || in.GetShortcutId() <= 0 || len(in.GetId()) == 0 {
		return nil, mtproto.ErrInputRequestInvalid
	}
	requested := make(map[int32]struct{}, len(in.GetId()))
	for _, id := range in.GetId() {
		if id <= 0 {
			return nil, mtproto.ErrMessageIdInvalid
		}
		if _, duplicate := requested[id]; duplicate {
			return nil, mtproto.ErrInputRequestInvalid
		}
		requested[id] = struct{}{}
	}
	quickReplyMu.Lock()
	defer quickReplyMu.Unlock()
	items, err := loadQuickReplyMsgs(userID)
	if err != nil {
		return nil, err
	}
	found := false
	changed := false
	for index := range items {
		if items[index].ShortcutId != in.GetShortcutId() {
			continue
		}
		found = true
		kept := make([]int32, 0, len(items[index].Ids))
		for _, id := range items[index].Ids {
			if _, remove := requested[id]; remove {
				changed = true
				delete(requested, id)
				continue
			}
			kept = append(kept, id)
		}
		if len(kept) == 0 {
			items = append(items[:index], items[index+1:]...)
		} else {
			items[index].Ids = kept
		}
		break
	}
	if !found || len(requested) != 0 {
		return nil, mtproto.ErrMessageIdInvalid
	}
	if changed {
		if err = saveQuickReplyMsgs(userID, items); err != nil {
			return nil, err
		}
	}
	return callUpdates(), nil
}
