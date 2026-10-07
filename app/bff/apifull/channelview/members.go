package channelview

import (
	"math"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/domain"
	"github.com/zeromicro/go-zero/core/logx"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// stored keeps a database failure off the wire. The driver text can carry the DSN.
func stored(err error) error {
	switch mapDomain(err) {
	case nil:
		return nil
	case mtproto.ErrChannelInvalid, mtproto.ErrChatAdminRequired, mtproto.ErrMessageIdInvalid:
		return mapDomain(err)
	default:
		logx.Errorf("channel record read failed")
		return mtproto.ErrInternalServerError
	}
}

func channelUser(userID, caller int64) *mtproto.User {
	return mtproto.MakeTLUser(&mtproto.User{
		Id:   userID,
		Self: caller == userID,
	}).To_User()
}

func creatorParticipant(ch domain.Channel) *mtproto.ChannelParticipant {
	return mtproto.MakeTLChannelParticipantCreator(&mtproto.ChannelParticipant{
		UserId: ch.Creator,
		AdminRights: mtproto.MakeTLChatAdminRights(&mtproto.ChatAdminRights{
			ChangeInfo:     true,
			PostMessages:   true,
			EditMessages:   true,
			DeleteMessages: true,
			BanUsers:       true,
			InviteUsers:    true,
			PinMessages:    true,
			AddAdmins:      true,
			ManageCall:     true,
		}).To_ChatAdminRights(),
	}).To_ChannelParticipant()
}

func adminRights(r *domain.ChannelAdminRights) *mtproto.ChatAdminRights {
	if r == nil || r.Empty() {
		return nil
	}
	return mtproto.MakeTLChatAdminRights(&mtproto.ChatAdminRights{
		ChangeInfo:           r.ChangeInfo,
		PostMessages:         r.PostMessages,
		EditMessages:         r.EditMessages,
		DeleteMessages:       r.DeleteMessages,
		BanUsers:             r.BanUsers,
		InviteUsers:          r.InviteUsers,
		PinMessages:          r.PinMessages,
		AddAdmins:            r.AddAdmins,
		Anonymous:            r.Anonymous,
		ManageCall:           r.ManageCall,
		Other:                r.Other,
		ManageTopics:         r.ManageTopics,
		PostStories:          r.PostStories,
		EditStories:          r.EditStories,
		DeleteStories:        r.DeleteStories,
		ManageDirectMessages: r.ManageDirectMessages,
		ManageRanks:          r.ManageRanks,
		ManageLinkedPeers:    r.ManageLinkedPeers,
	}).To_ChatAdminRights()
}

func bannedRights(r *domain.ChannelBannedRights) *mtproto.ChatBannedRights {
	if !r.Active(time.Now().Unix()) {
		return nil
	}
	return mtproto.MakeTLChatBannedRights(&mtproto.ChatBannedRights{
		ViewMessages:    r.ViewMessages,
		SendMessages:    r.SendMessages,
		SendMedia:       r.SendMedia,
		SendStickers:    r.SendStickers,
		SendGifs:        r.SendGifs,
		SendGames:       r.SendGames,
		SendInline:      r.SendInline,
		EmbedLinks:      r.EmbedLinks,
		SendPolls:       r.SendPolls,
		ChangeInfo:      r.ChangeInfo,
		InviteUsers:     r.InviteUsers,
		PinMessages:     r.PinMessages,
		ManageTopics:    r.ManageTopics,
		SendPhotos:      r.SendPhotos,
		SendVideos:      r.SendVideos,
		SendRoundvideos: r.SendRoundvideos,
		SendAudios:      r.SendAudios,
		SendVoices:      r.SendVoices,
		SendDocs:        r.SendDocs,
		SendPlain:       r.SendPlain,
		EditRank:        r.EditRank,
		SendReactions:   r.SendReactions,
		UntilDate: func() int32 {
			if r.UntilDate == 0 {
				return math.MaxInt32
			}
			return r.UntilDate
		}(),
	}).To_ChatBannedRights()
}

// Full is the channelFull the encoder can write. Photo and notify settings are
// empty but non-nil; a nil value panics inside TLChannelFull.Encode.
func Full(caller int64, ch domain.Channel) (*mtproto.Messages_ChatFull, error) {
	members, err := domain.ListChannelMembers(ch.ID)
	if err != nil {
		return nil, stored(err)
	}
	participantsCount, adminsCount := int32(0), int32(0)
	for _, member := range members {
		if member.BannedRights.Kicks(time.Now().Unix()) {
			continue
		}
		participantsCount++
		if member.Creator || !member.AdminRights.Empty() {
			adminsCount++
		}
	}
	pinned, err := domain.ListPinnedChannelMessages(caller, ch.ID, 1)
	if err != nil {
		return nil, stored(err)
	}
	pts, err := domain.ChannelMessagePTS(ch.ID)
	if err != nil {
		return nil, stored(err)
	}
	readMax, err := domain.ChannelReadMaxID(caller, ch.ID)
	if err != nil {
		return nil, stored(err)
	}
	outboxMax, err := domain.ChannelOutboxMaxID(caller, ch.ID)
	if err != nil {
		return nil, stored(err)
	}
	full := &mtproto.ChatFull{
		Id:                  ch.ID,
		About:               ch.About,
		ParticipantsCount:   wrapperspb.Int32(participantsCount),
		AdminsCount:         wrapperspb.Int32(adminsCount),
		CanViewParticipants: !ch.ParticipantsHidden || caller == ch.Creator,
		Antispam:            ch.Antispam,
		HiddenPrehistory:    ch.HiddenPrehistory,
		ParticipantsHidden:  ch.ParticipantsHidden,
		ReadInboxMaxId:      readMax,
		ReadOutboxMaxId:     outboxMax,
		Pts:                 pts,
		ChatPhoto:           mtproto.MakeTLPhotoEmpty(nil).To_Photo(),
		NotifySettings:      mtproto.MakeTLPeerNotifySettings(&mtproto.PeerNotifySettings{}).To_PeerNotifySettings(),
		BotInfo:             []*mtproto.BotInfo{},
	}
	if ch.DiscussionGroupID > 0 {
		full.LinkedChatId = wrapperspb.Int64(ch.DiscussionGroupID)
	}
	if ch.LocationLat != nil && ch.LocationLong != nil {
		full.Location = mtproto.MakeTLChannelLocation(&mtproto.ChannelLocation{
			GeoPoint: mtproto.MakeTLGeoPoint(&mtproto.GeoPoint{
				Lat:  *ch.LocationLat,
				Long: *ch.LocationLong,
			}).To_GeoPoint(),
			Address: ch.LocationAddress,
		}).To_ChannelLocation()
	} else {
		full.Location = mtproto.MakeTLChannelLocationEmpty(nil).To_ChannelLocation()
	}
	if ch.SlowmodeSeconds > 0 {
		full.SlowmodeSeconds = wrapperspb.Int32(ch.SlowmodeSeconds)
	}
	if caller == ch.Creator {
		full.CanSetUsername = true
		full.CanSetStickers = true
	}
	if len(pinned) > 0 {
		full.PinnedMsgId = wrapperspb.Int32(pinned[0].MessageID)
	}
	users := []*mtproto.User{}
	if ch.Creator != 0 {
		users = []*mtproto.User{channelUser(ch.Creator, caller)}
	}
	return mtproto.MakeTLMessagesChatFull(&mtproto.Messages_ChatFull{
		FullChat: mtproto.MakeTLChannelFull(full).To_ChatFull(),
		Chats:    []*mtproto.Chat{Chat(ch, ch.Creator == caller)},
		Users:    users,
	}).To_Messages_ChatFull(), nil
}

func memberParticipant(caller int64, ch domain.Channel, member domain.ChannelMember) *mtproto.ChannelParticipant {
	if member.Creator {
		return creatorParticipant(ch)
	}
	date := int32(member.JoinedAt)
	if member.BannedRights != nil && member.BannedRights.ViewMessages && !member.BannedRights.Active(time.Now().Unix()) {
		return LeftParticipant(member.UserID)
	}
	if rights := bannedRights(member.BannedRights); rights != nil {
		return mtproto.MakeTLChannelParticipantBanned(&mtproto.ChannelParticipant{
			Left:         rights.GetViewMessages(),
			Peer:         mtproto.MakeTLPeerUser(&mtproto.Peer{UserId: member.UserID}).To_Peer(),
			KickedBy:     member.BannedBy,
			Date:         int32(member.BannedAt),
			BannedRights: rights,
			Rank:         rankValue(member.Rank),
		}).To_ChannelParticipant()
	}
	if member.UserID == caller {
		return mtproto.MakeTLChannelParticipantSelf(&mtproto.ChannelParticipant{
			UserId:          member.UserID,
			InviterId_INT64: member.InvitedBy,
			Date:            date,
			Rank:            rankValue(member.Rank),
			AdminRights:     adminRights(member.AdminRights),
		}).To_ChannelParticipant()
	}
	if rights := adminRights(member.AdminRights); rights != nil {
		return mtproto.MakeTLChannelParticipantAdmin(&mtproto.ChannelParticipant{
			UserId:          member.UserID,
			InviterId_INT64: member.InvitedBy,
			Date:            date,
			Rank:            rankValue(member.Rank),
			AdminRights:     rights,
			CanEdit:         caller == ch.Creator,
		}).To_ChannelParticipant()
	}
	return mtproto.MakeTLChannelParticipant(&mtproto.ChannelParticipant{
		UserId: member.UserID,
		Date:   date,
	}).To_ChannelParticipant()
}

// LeftParticipant is used for participant updates when a kicked member is
// unbanned and consequently has no persisted member row.
func LeftParticipant(userID int64) *mtproto.ChannelParticipant {
	return mtproto.MakeTLChannelParticipantLeft(&mtproto.ChannelParticipant{UserId: userID}).To_ChannelParticipant()
}

// MemberParticipant exposes the wire participant conversion to handlers that
// need to publish a participant update after changing persisted rights.
func MemberParticipant(caller int64, ch domain.Channel, member domain.ChannelMember) *mtproto.ChannelParticipant {
	return memberParticipant(caller, ch, member)
}

func rankValue(rank string) *wrapperspb.StringValue {
	if rank == "" {
		return nil
	}
	return wrapperspb.String(rank)
}

func memberListed(filter *mtproto.ChannelParticipantsFilter, member domain.ChannelMember) bool {
	if filter == nil || filter.GetPredicateName() == "" {
		return true
	}
	switch filter.GetPredicateName() {
	case mtproto.Predicate_channelParticipantsRecent:
		return !member.BannedRights.Kicks(time.Now().Unix())
	case mtproto.Predicate_channelParticipantsAdmins:
		return member.Creator || adminRights(member.AdminRights) != nil
	case mtproto.Predicate_channelParticipantsBanned:
		return member.BannedRights.Active(time.Now().Unix()) && !member.BannedRights.Kicks(time.Now().Unix())
	case mtproto.Predicate_channelParticipantsKicked:
		return member.BannedRights.Active(time.Now().Unix()) && member.BannedRights.Kicks(time.Now().Unix())
	case mtproto.Predicate_channelParticipantsSearch:
		return filter.GetQ_STRING() == "" && !member.BannedRights.Kicks(time.Now().Unix())
	default:
		return false
	}
}

// Participants lists persisted channel members for filters backed by the local member store.
func Participants(caller int64, ch domain.Channel, filter *mtproto.ChannelParticipantsFilter, offset, limit int32, members []domain.ChannelMember) *mtproto.Channels_ChannelParticipants {
	filtered := make([]domain.ChannelMember, 0, len(members))
	if ch.Creator != 0 && (!ch.ParticipantsHidden || caller == ch.Creator) {
		for _, member := range members {
			if memberListed(filter, member) {
				filtered = append(filtered, member)
			}
		}
	}
	page := []*mtproto.ChannelParticipant{}
	users := []*mtproto.User{}
	count := int32(len(filtered))
	if offset < 0 {
		offset = 0
	}
	if limit > 0 && int(offset) < len(filtered) {
		end := int(offset) + int(limit)
		if end > len(filtered) {
			end = len(filtered)
		}
		for _, member := range filtered[int(offset):end] {
			page = append(page, memberParticipant(caller, ch, member))
			users = append(users, channelUser(member.UserID, caller))
		}
	}
	return mtproto.MakeTLChannelsChannelParticipants(&mtproto.Channels_ChannelParticipants{
		Count:        count,
		Participants: page,
		Chats:        []*mtproto.Chat{},
		Users:        users,
	}).To_Channels_ChannelParticipants()
}

// TypingRecipients validates a channel peer and returns the active members
// that can receive a typing update. Broadcast channels do not have a typing
// audience; callers should keep those requests fail-closed.
func TypingRecipients(userID int64, peer *mtproto.InputPeer) ([]int64, error) {
	channelID, err := ValidateInputPeer(userID, peer)
	if err != nil {
		return nil, err
	}
	ch, ok, err := domain.LoadChannel(channelID)
	if err != nil {
		return nil, stored(err)
	}
	if !ok {
		return nil, mtproto.ErrChannelInvalid
	}
	if ch.Broadcast && !ch.Megagroup {
		return nil, mtproto.ErrMethodNotImpl
	}
	members, err := domain.ListChannelMembers(channelID)
	if err != nil {
		return nil, stored(err)
	}
	recipients := make([]int64, 0, len(members))
	seen := make(map[int64]struct{}, len(members))
	now := time.Now().Unix()
	for _, member := range members {
		if member.UserID <= 0 || member.UserID == userID || member.BannedRights.Kicks(now) {
			continue
		}
		if _, exists := seen[member.UserID]; exists {
			continue
		}
		seen[member.UserID] = struct{}{}
		recipients = append(recipients, member.UserID)
	}
	return recipients, nil
}

// Participant resolves inputPeerSelf and inputPeerUser against the stored member record.
func Participant(caller int64, ch domain.Channel, peer *mtproto.InputPeer, member domain.ChannelMember, found bool) (*mtproto.Channels_ChannelParticipant, error) {
	if ch.Creator == 0 {
		return nil, mtproto.ErrChannelInvalid
	}
	if ch.ParticipantsHidden && caller != ch.Creator {
		return nil, mtproto.ErrUserNotParticipant
	}
	if peer == nil {
		return nil, mtproto.ErrUserIdInvalid
	}
	var uid int64
	switch peer.GetPredicateName() {
	case mtproto.Predicate_inputPeerSelf:
		uid = caller
	case mtproto.Predicate_inputPeerUser:
		uid = peer.GetUserId()
		if uid == 0 {
			return nil, mtproto.ErrUserIdInvalid
		}
	default:
		return nil, mtproto.ErrUserIdInvalid
	}
	if !found || uid != member.UserID {
		return nil, mtproto.ErrUserNotParticipant
	}
	return mtproto.MakeTLChannelsChannelParticipant(&mtproto.Channels_ChannelParticipant{
		Participant: memberParticipant(caller, ch, member),
		Chats:       []*mtproto.Chat{},
		Users:       []*mtproto.User{channelUser(member.UserID, caller)},
	}).To_Channels_ChannelParticipant(), nil
}
