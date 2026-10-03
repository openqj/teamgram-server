package core

import (
	"google.golang.org/protobuf/types/known/wrapperspb"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/apifull/channelview"
	messagepb "github.com/teamgram/teamgram-server/app/service/biz/message/message"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

func messageReplyToPeer(userID int64, replyTo *mtproto.InputReplyTo) (*mtproto.Peer, error) {
	if replyTo == nil {
		return nil, nil
	}
	if replyTo.PredicateName == mtproto.Predicate_inputReplyToStory {
		return nil, mtproto.ErrMethodNotImpl
	}
	if replyTo.PredicateName != mtproto.Predicate_inputReplyToMessage {
		return nil, mtproto.ErrPeerIdInvalid
	}
	if replyTo.GetReplyToPeerId() == nil {
		return nil, nil
	}

	peer := mtproto.FromInputPeer2(userID, replyTo.GetReplyToPeerId())
	if peer.PeerId <= 0 {
		return nil, mtproto.ErrPeerIdInvalid
	}

	switch peer.PeerType {
	case mtproto.PEER_SELF, mtproto.PEER_USER, mtproto.PEER_CHAT, mtproto.PEER_CHANNEL:
		return peer.ToPeer(), nil
	default:
		return nil, mtproto.ErrPeerIdInvalid
	}
}

func messagePeersMatch(left, right *mtproto.Peer) bool {
	if left == nil || right == nil || left.PredicateName != right.PredicateName {
		return false
	}

	switch left.PredicateName {
	case mtproto.Predicate_peerUser:
		return left.UserId == right.UserId
	case mtproto.Predicate_peerChat:
		return left.ChatId == right.ChatId
	case mtproto.Predicate_peerChannel:
		return left.ChannelId == right.ChannelId
	default:
		return false
	}
}

func (c *MessagesCore) validateMessageReplySource(peer *mtproto.Peer, messageID int32) error {
	if messageID <= 0 {
		return mtproto.ErrMessageIdInvalid
	}
	if peer == nil {
		return mtproto.ErrPeerIdInvalid
	}
	// Native APIFull channel messages are not indexed by the generic message
	// service. Validate their existence against the channel store before the
	// channel delivery path persists the reply.
	if peer.GetPredicateName() == mtproto.Predicate_peerChannel {
		found, err := channelview.PresentIDs(c.MD.UserId, peer.GetChannelId(), []int32{messageID})
		if err != nil {
			return err
		}
		if _, ok := found[messageID]; !ok {
			return mtproto.ErrMessageIdInvalid
		}
		return nil
	}

	box, err := c.svcCtx.Dao.MessageGetUserMessage(c.ctx, &messagepb.TLMessageGetUserMessage{
		UserId: c.MD.UserId,
		Id:     messageID,
	})
	if err != nil {
		return err
	}
	if box == nil || box.GetMessage() == nil {
		return mtproto.ErrInternalServerError
	}
	if box.GetMessage().GetId() != messageID || !messagePeersMatch(box.GetMessage().GetPeerId(), peer) {
		return mtproto.ErrMessageIdInvalid
	}
	return nil
}

func (c *MessagesCore) resolveMessageReplyPeer(sendPeer *mtproto.PeerUtil, replyTo *mtproto.InputReplyTo, legacyReplyToMsgID *wrapperspb.Int32Value) (*mtproto.Peer, error) {
	if replyTo == nil {
		if legacyReplyToMsgID == nil {
			return nil, nil
		}
		if sendPeer == nil {
			return nil, mtproto.ErrPeerIdInvalid
		}
		return nil, c.validateMessageReplySource(sendPeer.ToPeer(), legacyReplyToMsgID.GetValue())
	}

	peer, err := messageReplyToPeer(c.MD.UserId, replyTo)
	if err != nil {
		return nil, err
	}

	replyToMsgId := replyTo.GetReplyToMsgId()
	if replyToMsgId <= 0 {
		return nil, mtproto.ErrMessageIdInvalid
	}
	if replyTo.GetReplyToPeerId() == nil {
		if sendPeer == nil {
			return nil, mtproto.ErrPeerIdInvalid
		}
		if err = c.validateMessageReplySource(sendPeer.ToPeer(), replyToMsgId); err != nil {
			return nil, err
		}
		return nil, nil
	}
	if peer == nil {
		return nil, mtproto.ErrPeerIdInvalid
	}

	inputPeer := replyTo.GetReplyToPeerId()
	switch inputPeer.PredicateName {
	case mtproto.Predicate_inputPeerUser:
		users, err := c.svcCtx.Dao.UserGetMutableUsers(c.ctx, &userpb.TLUserGetMutableUsers{
			Id: []int64{inputPeer.GetUserId()},
			To: []int64{c.MD.UserId},
		})
		if err != nil {
			return nil, err
		}
		validHash := false
		for _, user := range users.GetDatas() {
			if user.GetUser().GetId() == inputPeer.GetUserId() && user.GetUser().GetAccessHash() == inputPeer.GetAccessHash() {
				validHash = true
				break
			}
		}
		if !validHash {
			return nil, mtproto.ErrPeerIdInvalid
		}
	case mtproto.Predicate_inputPeerChat:
		if !c.svcCtx.Dao.ChatClient.CheckParticipantIsExist(c.ctx, c.MD.UserId, []int64{inputPeer.GetChatId()}) {
			return nil, mtproto.ErrPeerIdInvalid
		}
	case mtproto.Predicate_inputPeerChannel:
		if _, err = channelview.HistoryForInputPeer(c.MD.UserId, inputPeer, 0, 1); err != nil {
			return nil, err
		}
		found, err := channelview.PresentIDs(c.MD.UserId, inputPeer.GetChannelId(), []int32{replyToMsgId})
		if err != nil {
			return nil, err
		}
		if _, ok := found[replyToMsgId]; !ok {
			return nil, mtproto.ErrMessageIdInvalid
		}
		return peer, nil
	case mtproto.Predicate_inputPeerSelf:
	default:
		return nil, mtproto.ErrPeerIdInvalid
	}

	if err = c.validateMessageReplySource(peer, replyToMsgId); err != nil {
		return nil, err
	}
	return peer, nil
}
