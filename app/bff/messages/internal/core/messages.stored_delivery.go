package core

import (
	"strings"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/apifull/channelview"
	"github.com/teamgram/teamgram-server/app/bff/apifull/schedstore"
	syncpb "github.com/teamgram/teamgram-server/app/messenger/sync/sync"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func schedulePeer(userID int64, peer *mtproto.PeerUtil) (*mtproto.Peer, error) {
	if userID <= 0 {
		return nil, mtproto.ErrAuthKeyUnregistered
	}
	if peer == nil {
		return nil, mtproto.ErrPeerIdInvalid
	}
	if peer.PeerType == mtproto.PEER_SELF {
		return mtproto.MakePeerUser(userID), nil
	}
	if !peer.IsUserOrChatOrChannel() || peer.PeerId <= 0 {
		return nil, mtproto.ErrPeerIdInvalid
	}
	out := peer.ToPeer()
	if out == nil {
		return nil, mtproto.ErrPeerIdInvalid
	}
	return out, nil
}

// deliverStored handles a scheduled send, or a channel send, before the stock
// messenger path. A non-zero schedule date wins. ok is false when neither applies.
func (c *MessagesCore) deliverStored(inputPeer *mtproto.InputPeer, peer *mtproto.PeerUtil, when int32, text string, replyToMsgID, replyToTopID int32, randomID int64) (*mtproto.Updates, bool, error) {
	if c.MD == nil || c.MD.UserId == 0 {
		return nil, false, mtproto.ErrAuthKeyUnregistered
	}
	if when != 0 {
		if when <= int32(time.Now().Unix()) {
			return nil, true, mtproto.ErrScheduleDateInvalid
		}
		if peer != nil && peer.IsChannel() {
			if _, err := channelview.ValidateInputPeer(c.MD.UserId, inputPeer); err != nil {
				return nil, true, err
			}
		}
		target, err := schedulePeer(c.MD.UserId, peer)
		if err != nil {
			return nil, true, err
		}
		up, err := schedstore.Append(c.MD.UserId, target, text, when)
		return up, true, err
	}
	if peer != nil && peer.IsChannel() {
		up, err := channelview.PostForInputPeerWithReplyAndRandomID(c.MD.UserId, inputPeer, text, time.Now().Unix(), replyToMsgID, replyToTopID, randomID)
		if err != nil {
			return nil, true, err
		}
		if err = c.pushChannelUpdates(up); err != nil {
			return up, true, err
		}
		return up, true, nil
	}
	return nil, false, nil
}

func (c *MessagesCore) pushChannelUpdates(updates *mtproto.Updates) error {
	if c == nil || c.MD == nil || c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.SyncClient == nil {
		return mtproto.ErrMethodNotImpl
	}
	if updates == nil {
		return mtproto.ErrInputRequestInvalid
	}
	var channelID int64
	for _, update := range updates.GetUpdates() {
		message := update.GetMessage_MESSAGE()
		if message == nil || message.GetPeerId() == nil || message.GetPeerId().GetChannelId() <= 0 {
			continue
		}
		if channelID != 0 && channelID != message.GetPeerId().GetChannelId() {
			return mtproto.ErrInputRequestInvalid
		}
		channelID = message.GetPeerId().GetChannelId()
	}
	if channelID <= 0 {
		return mtproto.ErrInputRequestInvalid
	}
	chat, err := channelview.ChatForUpdates(c.MD.UserId, channelID)
	if err != nil {
		return err
	}
	updates.Chats = []*mtproto.Chat{chat}
	userIDs, err := channelview.UpdateRecipientIDs(channelID)
	if err != nil {
		return err
	}
	var excludes []int64
	if c.MD.PermAuthKeyId != 0 {
		excludes = []int64{c.MD.PermAuthKeyId}
	}
	for _, userID := range userIDs {
		if userID <= 0 {
			return mtproto.ErrInternalServerError
		}
		if _, err = c.svcCtx.Dao.SyncClient.SyncPushUpdatesIfNot(c.ctx, &syncpb.TLSyncPushUpdatesIfNot{
			UserId:   userID,
			Excludes: excludes,
			Updates:  updates,
		}); err != nil {
			return err
		}
	}
	return nil
}

func storedReplyIDs(replyTo *mtproto.InputReplyTo, legacy *wrapperspb.Int32Value) (int32, int32) {
	if replyTo != nil {
		return replyTo.GetReplyToMsgId(), replyTo.GetTopMsgId().GetValue()
	}
	if legacy != nil {
		return legacy.GetValue(), 0
	}
	return 0, 0
}

func joinCaptions(in *mtproto.TLMessagesSendMultiMedia) string {
	if in == nil {
		return ""
	}
	parts := make([]string, 0, len(in.GetMultiMedia()))
	for _, media := range in.GetMultiMedia() {
		if media == nil || media.GetMessage() == "" {
			continue
		}
		parts = append(parts, media.GetMessage())
	}
	return strings.Join(parts, "\n")
}
