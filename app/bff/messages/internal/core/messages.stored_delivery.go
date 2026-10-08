package core

import (
	"context"
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
		up, err := channelview.PostForInputPeerWithReplyAndRandomIDForDelivery(c.MD.UserId, inputPeer, text, time.Now().Unix(), replyToMsgID, replyToTopID, randomID, c.MD.PermAuthKeyId)
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
	key, err := channelview.DeliveryKeyFromUpdates(updates)
	if err != nil {
		return err
	}
	chat, err := channelview.ChatForUpdates(c.MD.UserId, key.ChannelID)
	if err != nil {
		return err
	}
	updates.Chats = []*mtproto.Chat{chat}
	_, err = channelview.DeliverPendingChannelUpdates(c.ctx, key, false, func(pushCtx context.Context, userID int64, excludes []int64, recipientUpdates *mtproto.Updates) error {
		callCtx, cancel := context.WithTimeout(pushCtx, 10*time.Second)
		defer cancel()
		result, pushErr := c.svcCtx.Dao.SyncClient.SyncPushUpdatesIfNot(callCtx, &syncpb.TLSyncPushUpdatesIfNot{
			UserId:   userID,
			Excludes: excludes,
			Updates:  recipientUpdates,
		})
		if pushErr == nil && result == nil {
			return mtproto.ErrMethodNotImpl
		}
		return pushErr
	})
	return err
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
