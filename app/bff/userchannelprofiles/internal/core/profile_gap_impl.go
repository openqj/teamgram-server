package core

import (
	"fmt"
	"strconv"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/apifull/persist"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

func profileTabKey(channelID int64) string {
	return "profile-tab:" + strconv.FormatInt(channelID, 10)
}

func contactPhotoKey(ownerID, userID int64) string {
	return fmt.Sprintf("contact-photo:%d:%d", ownerID, userID)
}

// otherUserPeer rejects self and any peer that is not a user.
func otherUserPeer(selfID int64, in *mtproto.InputUser) (int64, error) {
	if in == nil {
		return 0, mtproto.ErrUserIdInvalid
	}
	peer := mtproto.FromInputUser(selfID, in)
	if peer.PeerType == mtproto.PEER_SELF || (peer.PeerId != 0 && peer.PeerId == selfID) {
		return 0, mtproto.ErrUserIdInvalid
	}
	if peer.PeerType != mtproto.PEER_USER || peer.PeerId == 0 {
		return 0, mtproto.ErrUserIdInvalid
	}
	return peer.PeerId, nil
}

func inputFileEmpty(f *mtproto.InputFile) bool {
	if f == nil {
		return true
	}
	return f.GetId_INT64() == 0 && f.GetName() == "" && f.GetParts() == 0 && f.GetId_INPUTDOCUMENT() == nil
}

// requireChannelAdmin allows the call only for a channel admin: the creator
// recorded for this user, or the user's personal channel.
// No channel-service profile-tab RPC exists in this tree.
func (c *UserChannelProfilesCore) requireChannelAdmin(ch *mtproto.InputChannel) (int64, error) {
	if ch == nil || ch.GetPredicateName() == mtproto.Predicate_inputChannelEmpty || ch.GetChannelId() == 0 {
		return 0, mtproto.ErrChannelInvalid
	}
	channelID := ch.GetChannelId()
	uid := c.MD.UserId

	raw, err := persist.Default.Get(fmt.Sprintf("chan:%d:%d", uid, channelID))
	if err != nil {
		return 0, err
	}
	if raw != "" {
		return channelID, nil
	}
	if c.svcCtx != nil && c.svcCtx.Dao != nil {
		me, err := c.svcCtx.Dao.UserClient.UserGetImmutableUser(c.ctx, &userpb.TLUserGetImmutableUser{Id: uid})
		if err != nil {
			return 0, err
		}
		if me.GetUser().GetPersonalChannelId() == channelID {
			return channelID, nil
		}
	}
	return 0, mtproto.ErrChatAdminRequired
}
