package channelview

import (
	"errors"
	"strings"
	"time"

	"github.com/teamgram/marmota/pkg/random2"
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/domain"
	"github.com/teamgram/teamgram-server/pkg/env2"
)

type InviteOptions struct {
	RequestNeeded bool
	ExpireDate    int32
	UsageLimit    int32
	Title         string
}

type InviteUpdate struct {
	Revoked       bool
	ExpireDate    *int32
	UsageLimit    *int32
	RequestNeeded *bool
	Title         *string
}

// ValidateInputChannelInviteAdmin reports whether input names an APIFull
// channel. When it does, it validates both the access hash and invite-admin
// rights; callers can retain their basic-chat path when handled is false.
func ValidateInputChannelInviteAdmin(userID int64, input *mtproto.InputChannel) (handled bool, err error) {
	if input == nil || input.GetPredicateName() == mtproto.Predicate_inputChannelEmpty || input.GetChannelId() == 0 {
		return true, mtproto.ErrChannelInvalid
	}
	channel, ok, err := domain.LoadChannel(input.GetChannelId())
	if err != nil {
		return true, stored(err)
	}
	if !ok {
		return false, nil
	}
	if channel.AccessHash != input.GetAccessHash() {
		return true, mtproto.ErrChannelInvalid
	}
	_, err = requireInviteAdmin(userID, channel.ID, 0)
	return true, err
}

// ValidateInputPeerInviteAdmin is the InputPeer counterpart used by the
// messages.* join-request methods. It reports handled=false for a basic chat
// so callers can retain their existing service path.
func ValidateInputPeerInviteAdmin(userID int64, input *mtproto.InputPeer) (handled bool, err error) {
	if input == nil || input.GetPredicateName() != mtproto.Predicate_inputPeerChannel || input.GetChannelId() == 0 {
		return false, nil
	}
	channel, ok, err := domain.LoadChannel(input.GetChannelId())
	if err != nil {
		return true, stored(err)
	}
	if !ok {
		return false, nil
	}
	if channel.AccessHash != input.GetAccessHash() {
		return true, mtproto.ErrChannelInvalid
	}
	_, err = requireInviteAdmin(userID, channel.ID, 0)
	return true, err
}

func ExportInvite(userID, channelID int64, options InviteOptions) (*mtproto.ExportedChatInvite, error) {
	if _, err := requireInviteAdmin(userID, channelID, 0); err != nil {
		return nil, err
	}
	invite, err := domain.CreateChannelInvite(domain.ChannelInvite{
		ChannelID:     channelID,
		AdminID:       userID,
		Link:          generateChannelInviteHash(),
		RequestNeeded: options.RequestNeeded,
		ExpireDate:    int64(options.ExpireDate),
		UsageLimit:    options.UsageLimit,
		Title:         options.Title,
		Date:          time.Now().Unix(),
	})
	if err != nil {
		return nil, inviteError(err)
	}
	return exportedInvite(invite)
}

func ExportedInvites(userID, channelID, adminID int64, revoked bool, offsetDate int32, offsetLink string, limit int32) ([]*mtproto.ExportedChatInvite, error) {
	if _, err := requireInviteAdmin(userID, channelID, adminID); err != nil {
		return nil, err
	}
	if limit == 0 {
		limit = 50
	}
	if limit < 0 {
		return nil, mtproto.ErrLimitInvalid
	}

	invites, err := domain.ListChannelInvites(channelID, adminID)
	if err != nil {
		return nil, inviteError(err)
	}
	result := make([]*mtproto.ExportedChatInvite, 0, len(invites))
	for _, invite := range invites {
		if invite.Revoked != revoked {
			continue
		}
		exported, err := exportedInvite(invite)
		if err != nil {
			return nil, err
		}
		result = append(result, exported)
	}

	offset := 0
	if offsetLink != "" && offsetDate != 0 {
		offset = -1
		for i, invite := range result {
			if (invite.GetLink() == offsetLink || inviteHash(offsetLink) == inviteHash(invite.GetLink())) && invite.GetDate() == offsetDate {
				offset = i
				break
			}
		}
	}
	if offset < 0 {
		return result[:0], nil
	}
	end := offset + int(limit)
	if end > len(result) {
		end = len(result)
	}
	return result[offset:end], nil
}

func ExportedInvite(userID, channelID int64, link string) (*mtproto.ExportedChatInvite, error) {
	invite, err := domain.GetChannelInvite(channelID, inviteHash(link))
	if err != nil {
		return nil, inviteError(err)
	}
	if _, err = requireInviteAdmin(userID, channelID, invite.AdminID); err != nil {
		return nil, err
	}
	return exportedInvite(invite)
}

func EditExportedInvite(userID, channelID int64, link string, update InviteUpdate) ([]*mtproto.ExportedChatInvite, error) {
	hash := inviteHash(link)
	invite, err := domain.GetChannelInvite(channelID, hash)
	if err != nil {
		return nil, inviteError(err)
	}
	if _, err = requireInviteAdmin(userID, channelID, invite.AdminID); err != nil {
		return nil, err
	}
	replacement := ""
	if update.Revoked && invite.Permanent {
		replacement = generateChannelInviteHash()
	}
	updated, err := domain.UpdateChannelInvite(channelID, hash, replacement, domain.ChannelInviteUpdate{
		Revoked:       update.Revoked,
		ExpireDate:    update.ExpireDate,
		UsageLimit:    update.UsageLimit,
		RequestNeeded: update.RequestNeeded,
		Title:         update.Title,
	}, time.Now().Unix())
	if err != nil {
		return nil, inviteError(err)
	}
	result := make([]*mtproto.ExportedChatInvite, 0, len(updated))
	for _, item := range updated {
		exported, err := exportedInvite(item)
		if err != nil {
			return nil, err
		}
		result = append(result, exported)
	}
	return result, nil
}

func DeleteExportedInvite(userID, channelID int64, link string) error {
	invite, err := domain.GetChannelInvite(channelID, inviteHash(link))
	if err != nil {
		return inviteError(err)
	}
	if _, err = requireInviteAdmin(userID, channelID, invite.AdminID); err != nil {
		return err
	}
	deleted, err := domain.DeleteChannelInvite(channelID, invite.Link)
	if err != nil {
		return inviteError(err)
	}
	if !deleted {
		return mtproto.ErrInviteHashInvalid
	}
	return nil
}

func DeleteRevokedExportedInvites(userID, channelID, adminID int64) error {
	if _, err := requireInviteAdmin(userID, channelID, adminID); err != nil {
		return err
	}
	return inviteError(domain.DeleteRevokedChannelInvites(channelID, adminID))
}

func ResolveInviteRequest(userID, channelID, requesterID int64, link string, approved bool) ([]*mtproto.ChatInviteImporter, error) {
	if _, err := requireInviteAdmin(userID, channelID, 0); err != nil {
		return nil, err
	}
	if err := domain.ResolveChannelInviteRequest(channelID, requesterID, userID, inviteHash(link), approved, time.Now().Unix()); err != nil {
		return nil, inviteError(err)
	}
	return PendingInviteRequesters(userID, channelID, link)
}

func PendingInviteRequesters(userID, channelID int64, link string) ([]*mtproto.ChatInviteImporter, error) {
	hash := inviteHash(link)
	if hash != "" {
		invite, err := domain.GetChannelInvite(channelID, hash)
		if err != nil {
			return nil, inviteError(err)
		}
		if _, err = requireInviteAdmin(userID, channelID, invite.AdminID); err != nil {
			return nil, err
		}
	} else if _, err := requireInviteAdmin(userID, channelID, 0); err != nil {
		return nil, err
	}
	importers, err := domain.ListChannelInviteImporters(channelID, hash, true, "")
	if err != nil {
		return nil, inviteError(err)
	}
	return inviteImporterValues(importers, true, hash), nil
}

func InviteAdmins(userID, channelID int64) ([]*mtproto.ChatAdminWithInvites, error) {
	channel, err := requireInviteAdmin(userID, channelID, 0)
	if err != nil {
		return nil, err
	}

	invites, err := domain.ListChannelInvitesForChannel(channelID)
	if err != nil {
		return nil, inviteError(err)
	}

	admins := make([]*mtproto.ChatAdminWithInvites, 0)
	byID := make(map[int64]*mtproto.ChatAdminWithInvites)
	for _, invite := range invites {
		admin := byID[invite.AdminID]
		if admin == nil {
			canInvite, err := domain.ChannelCanInvite(channel, invite.AdminID, time.Now().Unix())
			if err != nil {
				return nil, inviteError(err)
			}
			if !canInvite {
				continue
			}
			admin = mtproto.MakeTLChatAdminWithInvites(&mtproto.ChatAdminWithInvites{
				AdminId: invite.AdminID,
			}).To_ChatAdminWithInvites()
			byID[invite.AdminID] = admin
			admins = append(admins, admin)
		}
		if invite.Revoked {
			admin.RevokedInvitesCount++
		} else {
			admin.InvitesCount++
		}
	}
	return admins, nil
}

func InviteImporters(userID, channelID int64, link string, requested bool, query string, offsetDate int32, offsetUser int64, limit int32) ([]*mtproto.ChatInviteImporter, error) {
	hash := inviteHash(link)
	if hash != "" {
		invite, err := domain.GetChannelInvite(channelID, hash)
		if err != nil {
			return nil, inviteError(err)
		}
		if _, err = requireInviteAdmin(userID, channelID, invite.AdminID); err != nil {
			return nil, err
		}
	} else if _, err := requireInviteAdmin(userID, channelID, 0); err != nil {
		return nil, err
	}
	if limit == 0 {
		limit = 50
	}
	if limit < 0 {
		return nil, mtproto.ErrLimitInvalid
	}
	if query != "" && len(query) < 3 {
		return []*mtproto.ChatInviteImporter{}, nil
	}

	importers, err := domain.ListChannelInviteImporters(channelID, hash, requested, query)
	if err != nil {
		return nil, inviteError(err)
	}
	result := inviteImporterValues(importers, requested, hash)

	offset := 0
	for i, importer := range result {
		if importer.GetUserId() == offsetUser && importer.GetDate() == offsetDate {
			offset = i + 1
			break
		}
	}
	end := offset + int(limit)
	if end > len(result) {
		end = len(result)
	}
	return result[offset:end], nil
}

func inviteImporterValues(importers []domain.ChannelInviteImporter, requested bool, hash string) []*mtproto.ChatInviteImporter {
	result := make([]*mtproto.ChatInviteImporter, 0, len(importers))
	for _, importer := range importers {
		approvedBy := mtproto.MakeFlagsInt64(importer.ApprovedBy)
		if requested && hash == "" {
			approvedBy = nil
		}
		result = append(result, mtproto.MakeTLChatInviteImporter(&mtproto.ChatInviteImporter{
			Requested:  importer.Requested,
			UserId:     importer.UserID,
			Date:       int32(importer.Date),
			ApprovedBy: approvedBy,
		}).To_ChatInviteImporter())
	}
	return result
}

func CheckInvite(userID int64, hash string) (*mtproto.ChatInvite, error) {
	invite, channel, alreadyMember, err := domain.CheckChannelInvite(hash, userID, time.Now().Unix())
	if err != nil {
		return nil, inviteError(err)
	}
	if alreadyMember {
		return mtproto.MakeTLChatInviteAlready(&mtproto.ChatInvite{
			Chat: Chat(channel, channel.Creator == userID),
		}).To_ChatInvite(), nil
	}
	members, err := domain.ListChannelMembers(channel.ID)
	if err != nil {
		return nil, inviteError(err)
	}
	count := int32(0)
	for _, member := range members {
		if !member.BannedRights.Kicks(time.Now().Unix()) {
			count++
		}
	}
	return mtproto.MakeTLChatInvite(&mtproto.ChatInvite{
		Channel:           true,
		Broadcast:         channel.Broadcast,
		Public:            channel.Username != "",
		Megagroup:         channel.Megagroup,
		RequestNeeded:     invite.RequestNeeded,
		Title:             channel.Title,
		About:             mtproto.MakeFlagsString(channel.About),
		Photo:             mtproto.MakeTLPhotoEmpty(nil).To_Photo(),
		ParticipantsCount: count,
		Participants:      []*mtproto.User{},
	}).To_ChatInvite(), nil
}

// ImportInvite persists the membership or request and returns a channel chat
// in Updates, which lets clients add the newly joined channel immediately.
func ImportInvite(userID int64, hash string) (*mtproto.Updates, bool, error) {
	result, err := domain.ImportChannelInvite(hash, userID, time.Now().Unix())
	if err != nil {
		return nil, false, inviteError(err)
	}
	if result.RequestNeeded {
		return nil, true, nil
	}
	return mtproto.MakeUpdatesByUpdatesChats([]*mtproto.Chat{Chat(result.Channel, result.Channel.Creator == userID)}), false, nil
}

func requireInviteAdmin(userID, channelID, targetAdminID int64) (domain.Channel, error) {
	if userID <= 0 {
		return domain.Channel{}, mtproto.ErrUserIdInvalid
	}
	channel, ok, err := domain.LoadChannel(channelID)
	if err != nil {
		return domain.Channel{}, stored(err)
	}
	if !ok {
		return domain.Channel{}, mtproto.ErrChannelInvalid
	}
	if channel.Creator == userID {
		return channel, nil
	}

	member, ok, err := domain.LoadChannelMember(channelID, userID)
	if err != nil {
		return domain.Channel{}, stored(err)
	}
	if !ok || member.BannedRights.Kicks(time.Now().Unix()) {
		return domain.Channel{}, mtproto.ErrUserNotParticipant
	}
	if member.BannedRights.Active(time.Now().Unix()) && member.BannedRights.InviteUsers {
		return domain.Channel{}, mtproto.ErrChatAdminRequired
	}
	if member.AdminRights == nil || !member.AdminRights.InviteUsers {
		return domain.Channel{}, mtproto.ErrChatAdminRequired
	}
	if targetAdminID != 0 && targetAdminID != userID && !member.AdminRights.AddAdmins {
		return domain.Channel{}, mtproto.ErrChatAdminRequired
	}
	return channel, nil
}

func exportedInvite(invite domain.ChannelInvite) (*mtproto.ExportedChatInvite, error) {
	usage, err := domain.CountChannelInviteParticipants(invite.ChannelID, invite.Link, false)
	if err != nil {
		return nil, inviteError(err)
	}
	requested, err := domain.CountChannelInviteParticipants(invite.ChannelID, invite.Link, true)
	if err != nil {
		return nil, inviteError(err)
	}
	return mtproto.MakeTLChatInviteExported(&mtproto.ExportedChatInvite{
		Revoked:       invite.Revoked,
		Permanent:     invite.Permanent,
		RequestNeeded: invite.RequestNeeded,
		Link:          "https://" + env2.TDotMe + "/+" + invite.Link,
		AdminId:       invite.AdminID,
		Date:          int32(invite.Date),
		StartDate:     mtproto.MakeFlagsInt32(int32(invite.StartDate)),
		ExpireDate:    mtproto.MakeFlagsInt32(int32(invite.ExpireDate)),
		UsageLimit:    mtproto.MakeFlagsInt32(invite.UsageLimit),
		Usage:         mtproto.MakeFlagsInt32(usage),
		Requested:     mtproto.MakeFlagsInt32(requested),
		Title:         mtproto.MakeFlagsString(invite.Title),
	}).To_ExportedChatInvite(), nil
}

func inviteHash(link string) string {
	prefix := "https://" + env2.TDotMe + "/+"
	return strings.TrimPrefix(link, prefix)
}

func generateChannelInviteHash() string {
	return random2.RandomAlphabetic(1) + random2.RandomAlphanumeric(19)
}

func inviteError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, domain.ErrChannelInviteMissing):
		return mtproto.ErrInviteHashInvalid
	case errors.Is(err, domain.ErrChannelInviteExpired):
		return mtproto.ErrInviteHashExpired
	case errors.Is(err, domain.ErrChannelInviteBanned):
		return mtproto.ErrUserBannedInChannel
	case errors.Is(err, domain.ErrChannelMemberExists):
		return mtproto.ErrUserAlreadyParticipant
	case errors.Is(err, domain.ErrInvalidChannelMember):
		return mtproto.ErrUserIdInvalid
	default:
		return stored(err)
	}
}
