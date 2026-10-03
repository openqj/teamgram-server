package core

import (
	"github.com/teamgram/proto/mtproto"
)

type savedPeerKey struct {
	peerType int32
	peerId   int64
}

func savedPeerAllowed(p *mtproto.PeerUtil) bool {
	if p == nil || p.PeerId == 0 {
		return false
	}
	switch p.PeerType {
	case mtproto.PEER_SELF, mtproto.PEER_USER, mtproto.PEER_CHAT, mtproto.PEER_CHANNEL:
		return true
	default:
		return false
	}
}

// savedPeerKeyOf folds "self" and "user == me" into one key. Saved dialogs are
// encoded with MakePeer, which emits peerUser for both PEER_SELF and PEER_USER.
func savedPeerKeyOf(peerType int32, peerId, self int64) savedPeerKey {
	if peerType == mtproto.PEER_SELF || (peerType == mtproto.PEER_USER && peerId == self) {
		return savedPeerKey{peerType: mtproto.PEER_USER, peerId: self}
	}
	return savedPeerKey{peerType: peerType, peerId: peerId}
}

func savedPeerKeyFromPeer(p *mtproto.Peer, self int64) (savedPeerKey, bool) {
	if p == nil {
		return savedPeerKey{}, false
	}
	switch p.PredicateName {
	case mtproto.Predicate_peerUser:
		if p.UserId == 0 {
			return savedPeerKey{}, false
		}
		return savedPeerKeyOf(mtproto.PEER_USER, p.UserId, self), true
	case mtproto.Predicate_peerChat:
		if p.ChatId == 0 {
			return savedPeerKey{}, false
		}
		return savedPeerKey{peerType: mtproto.PEER_CHAT, peerId: p.ChatId}, true
	case mtproto.Predicate_peerChannel:
		if p.ChannelId == 0 {
			return savedPeerKey{}, false
		}
		return savedPeerKey{peerType: mtproto.PEER_CHANNEL, peerId: p.ChannelId}, true
	default:
		return savedPeerKey{}, false
	}
}

func emptyUser() *mtproto.User {
	return mtproto.MakeTLUserEmpty(&mtproto.User{}).To_User()
}

func inputChannelID(ch *mtproto.InputChannel) int64 {
	if ch == nil {
		return 0
	}
	switch ch.PredicateName {
	case mtproto.Predicate_inputChannel, mtproto.Predicate_inputChannelFromMessage:
		return ch.ChannelId
	default:
		return 0
	}
}

func boxInChannel(box *mtproto.MessageBox, channelID int64) bool {
	if box == nil || channelID == 0 {
		return false
	}
	if box.PeerType == mtproto.PEER_CHANNEL && box.PeerId == channelID {
		return true
	}
	if box.Message != nil && box.Message.PeerId != nil &&
		box.Message.PeerId.PredicateName == mtproto.Predicate_peerChannel &&
		box.Message.PeerId.ChannelId == channelID {
		return true
	}
	return false
}

// messageAuthorUserID returns the user in from_id. A channel or chat from_id
// (anonymous / signed as the channel) has no user author.
func messageAuthorUserID(box *mtproto.MessageBox) int64 {
	if box == nil {
		return 0
	}
	if box.Message != nil && box.Message.FromId != nil {
		from := box.Message.FromId
		if from.PredicateName == mtproto.Predicate_peerUser && from.UserId > 0 {
			return from.UserId
		}
		return 0
	}
	if box.SenderUserId > 0 {
		return box.SenderUserId
	}
	return 0
}
