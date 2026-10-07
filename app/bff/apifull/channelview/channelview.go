package channelview

import (
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/domain"
	"github.com/zeromicro/go-zero/core/logx"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// Chat builds a channel the session encoder can write. Photo is always non-nil;
// a nil photo panics inside TLChannel.Encode.
func Chat(ch domain.Channel, creator bool) *mtproto.Chat {
	out := &mtproto.Chat{
		Id:                          ch.ID,
		Title:                       ch.Title,
		Broadcast:                   ch.Broadcast,
		Megagroup:                   ch.Megagroup,
		Signatures:                  ch.Signatures,
		SignatureProfiles:           ch.SignatureProfiles,
		SlowmodeEnabled:             ch.SlowmodeSeconds > 0,
		Creator:                     creator,
		Date:                        int32(ch.CreatedAt),
		Photo:                       channelPhoto(ch),
		ParticipantsCount_FLAGINT32: mtproto.MakeFlagsInt32(1),
		Color_FLAGPEERCOLOR:         peerColor(ch.Color, ch.BackgroundEmojiID),
		ProfileColor:                peerColor(ch.ProfileColor, ch.ProfileBackgroundEmojiID),
	}
	if ch.AccessHash != 0 {
		out.AccessHash_FLAGINT64 = mtproto.MakeFlagsInt64(ch.AccessHash)
	}
	if ch.Username != "" {
		out.Username = mtproto.MakeFlagsString(ch.Username)
	}
	return mtproto.MakeTLChannel(out).To_Chat()
}

func channelPhoto(ch domain.Channel) *mtproto.ChatPhoto {
	if ch.PhotoID <= 0 || ch.PhotoDCID <= 0 {
		return mtproto.MakeTLChatPhotoEmpty(nil).To_ChatPhoto()
	}
	return mtproto.MakeTLChatPhoto(&mtproto.ChatPhoto{
		HasVideo: ch.PhotoHasVideo,
		PhotoId:  ch.PhotoID,
		DcId:     ch.PhotoDCID,
	}).To_ChatPhoto()
}

func peerColor(color *int32, backgroundEmojiID *int64) *mtproto.PeerColor {
	if color == nil && backgroundEmojiID == nil {
		return nil
	}
	data := &mtproto.PeerColor{}
	if color != nil {
		data.Color = wrapperspb.Int32(*color)
	}
	if backgroundEmojiID != nil {
		data.BackgroundEmojiId_FLAGINT64 = wrapperspb.Int64(*backgroundEmojiID)
	}
	return mtproto.MakeTLPeerColor(data).To_PeerColor()
}

// ChatsByID loads the channels this process stored. A closed database returns no
// chats and does not fail the user or basic-group dialogs around them.
func ChatsByID(userID int64, ids []int64) []*mtproto.Chat {
	if !domain.Ready() {
		if len(ids) > 0 {
			logx.Errorf("channel chats: domain mysql is not open")
		}
		return []*mtproto.Chat{}
	}
	out := make([]*mtproto.Chat, 0, len(ids))
	for _, id := range ids {
		ch, ok, err := domain.LoadChannel(id)
		if err != nil {
			logx.Errorf("channel chats: %v", err)
			continue
		}
		if !ok {
			continue
		}
		out = append(out, Chat(ch, ch.Creator == userID))
	}
	return out
}

func ChatForUpdates(userID, channelID int64) (*mtproto.Chat, error) {
	ch, ok, err := domain.LoadChannel(channelID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, domain.ErrChannelMissing
	}
	return Chat(ch, ch.Creator == userID), nil
}

func UpdateRecipientIDs(channelID int64) ([]int64, error) {
	members, err := domain.ListChannelMembers(channelID)
	if err != nil {
		return nil, err
	}
	ids := make([]int64, 0, len(members))
	now := time.Now().Unix()
	for _, member := range members {
		if member.BannedRights != nil && member.BannedRights.Active(now) && member.BannedRights.Kicks(now) {
			continue
		}
		ids = append(ids, member.UserID)
	}
	return ids, nil
}

// SendAsChannels returns persisted channels from which the caller can post.
// Ownership and administrator rights are read from the same channel/member
// tables used by channel RPCs; caller-local preferences are not treated as
// authorization.
func SendAsChannels(userID int64) ([]*mtproto.Chat, error) {
	if !domain.Ready() {
		return []*mtproto.Chat{}, nil
	}
	ids, err := domain.ListChannelIDsForUser(userID)
	if err != nil {
		return nil, err
	}
	out := make([]*mtproto.Chat, 0, len(ids))
	for _, id := range ids {
		ch, ok, err := domain.LoadChannel(id)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		if ch.Creator != userID {
			member, found, memberErr := domain.LoadChannelMember(id, userID)
			if memberErr != nil {
				return nil, memberErr
			}
			if !found || (member.BannedRights.Active(time.Now().Unix()) &&
				(member.BannedRights.Kicks(time.Now().Unix()) || member.BannedRights.SendMessages)) || member.AdminRights == nil ||
				(!member.AdminRights.PostMessages && !member.AdminRights.Anonymous) {
				continue
			}
		}
		out = append(out, Chat(ch, ch.Creator == userID))
	}
	return out, nil
}

// UpdateChannelUsername stores the public username after its owner service has
// authorized and committed the username change.
func UpdateChannelUsername(channelID int64, username string) error {
	return domain.UpdateChannelUsername(channelID, username)
}

// CommonChannelIDs returns APIFull channels shared by both users. The chat
// service owns basic groups, so this supplements its common-chat lookup.
func CommonChannelIDs(firstUserID, secondUserID int64) []int64 {
	if !domain.Ready() {
		return []int64{}
	}
	ids, err := domain.CommonChannelIDs(firstUserID, secondUserID)
	if err != nil {
		logx.Errorf("common channel lookup: %v", err)
		return []int64{}
	}
	return ids
}

// AppendCreatorDialogs adds channels this user created that the biz dialog list
// does not already contain. alreadyCounted holds channel ids that were in the
// pre-offset biz list, so Count is not raised twice for those.
func AppendCreatorDialogs(out *mtproto.Messages_Dialogs, userID int64, alreadyCounted map[int64]struct{}) {
	if out == nil {
		return
	}
	if !domain.Ready() {
		logx.Errorf("messages.getDialogs channel list: domain mysql is not open")
		return
	}
	rows, err := domain.ListByCreator(userID)
	if err != nil {
		logx.Errorf("messages.getDialogs channel list: %v", err)
		return
	}
	present := channelIDs(out.Dialogs)
	for _, ch := range rows {
		if _, ok := present[ch.ID]; ok {
			continue
		}
		out.Dialogs = append(out.Dialogs, newChannelDialog(ch.ID))
		out.Chats = append(out.Chats, Chat(ch, true))
		if _, counted := alreadyCounted[ch.ID]; !counted {
			out.Count++
		}
	}
}

// AppendRequested fills peer-dialog lookups for channels this caller created.
// Another user's channel is not synthesized.
func AppendRequested(out *mtproto.Messages_PeerDialogs, userID int64, channelIDs []int64) {
	if out == nil || len(channelIDs) == 0 {
		return
	}
	if !domain.Ready() {
		logx.Errorf("messages.getPeerDialogs channel: domain mysql is not open")
		return
	}
	present := channelIDsIn(out.Dialogs)
	for _, id := range channelIDs {
		if _, ok := present[id]; ok {
			continue
		}
		ch, ok, err := domain.LoadChannel(id)
		if err != nil {
			logx.Errorf("messages.getPeerDialogs channel: %v", err)
			continue
		}
		if !ok || ch.Creator != userID {
			continue
		}
		out.Dialogs = append(out.Dialogs, newChannelDialog(id))
		out.Chats = append(out.Chats, Chat(ch, true))
	}
}

func newChannelDialog(id int64) *mtproto.Dialog {
	top, err := domain.TopChannelMessage(id)
	if err != nil {
		logx.Errorf("channel dialog top message: %v", err)
		top = 0
	}
	return mtproto.MakeTLDialog(&mtproto.Dialog{
		Peer:           mtproto.MakePeerChannel(id),
		TopMessage:     top,
		NotifySettings: mtproto.MakeTLPeerNotifySettings(&mtproto.PeerNotifySettings{}).To_PeerNotifySettings(),
	}).To_Dialog()
}

func channelIDs(dialogs []*mtproto.Dialog) map[int64]struct{} {
	return channelIDsIn(dialogs)
}

func channelIDsIn(dialogs []*mtproto.Dialog) map[int64]struct{} {
	out := make(map[int64]struct{}, len(dialogs))
	for _, d := range dialogs {
		if d == nil || d.Peer == nil || d.Peer.PredicateName != mtproto.Predicate_peerChannel || d.Peer.ChannelId == 0 {
			continue
		}
		out[d.Peer.ChannelId] = struct{}{}
	}
	return out
}
