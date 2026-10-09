package dao

import (
	"context"

	"github.com/teamgram/teamgram-server/app/service/biz/chat/internal/dal/dataobject"
)

func (d *Dao) SelectChatInviteByLink(ctx context.Context, link string) (*dataobject.ChatInvitesDO, error) {
	if d.Postgres != nil {
		return d.Postgres.Store.Invites.SelectByLink(ctx, link)
	}
	return d.ChatInvitesDAO.SelectByLink(ctx, link)
}

func (d *Dao) SelectChatInvitesByAdminIdWithCB(ctx context.Context, chatID, adminID int64, cb func(int, int, *dataobject.ChatInvitesDO)) ([]dataobject.ChatInvitesDO, error) {
	var list []dataobject.ChatInvitesDO
	var err error
	if d.Postgres != nil {
		list, err = d.Postgres.Store.Invites.SelectListByAdminId(ctx, chatID, adminID)
	} else {
		return d.ChatInvitesDAO.SelectListByAdminIdWithCB(ctx, chatID, adminID, cb)
	}
	if err != nil {
		return nil, err
	}
	if cb != nil {
		for i := range list {
			cb(len(list), i, &list[i])
		}
	}
	return list, nil
}

func (d *Dao) SelectChatInvitesByChatIdWithCB(ctx context.Context, chatID int64, cb func(int, int, *dataobject.ChatInvitesDO)) ([]dataobject.ChatInvitesDO, error) {
	var list []dataobject.ChatInvitesDO
	var err error
	if d.Postgres != nil {
		list, err = d.Postgres.Store.Invites.SelectListByChatId(ctx, chatID)
	} else {
		return d.ChatInvitesDAO.SelectListByChatIdWithCB(ctx, chatID, cb)
	}
	if err != nil {
		return nil, err
	}
	if cb != nil {
		for i := range list {
			cb(len(list), i, &list[i])
		}
	}
	return list, nil
}

func (d *Dao) InsertChatInvite(ctx context.Context, value *dataobject.ChatInvitesDO) (int64, int64, error) {
	if d.Postgres != nil {
		return d.Postgres.Store.Invites.Insert(ctx, value)
	}
	return d.ChatInvitesDAO.Insert(ctx, value)
}

func (d *Dao) UpdateChatInvite(ctx context.Context, values map[string]interface{}, chatID int64, link string) (int64, error) {
	if d.Postgres != nil {
		return d.Postgres.Store.Invites.Update(ctx, values, chatID, link)
	}
	return d.ChatInvitesDAO.Update(ctx, values, chatID, link)
}

func (d *Dao) DeleteChatInviteByLink(ctx context.Context, chatID int64, link string) (int64, error) {
	if d.Postgres != nil {
		return d.Postgres.Store.Invites.DeleteByLink(ctx, chatID, link)
	}
	return d.ChatInvitesDAO.DeleteByLink(ctx, chatID, link)
}

func (d *Dao) DeleteRevokedChatInvites(ctx context.Context, chatID, adminID int64) (int64, error) {
	if d.Postgres != nil {
		return d.Postgres.Store.Invites.DeleteByRevoked(ctx, chatID, adminID)
	}
	return d.ChatInvitesDAO.DeleteByRevoked(ctx, chatID, adminID)
}

func (d *Dao) SelectInviteParticipantsByLink(ctx context.Context, link string, requested int32) ([]dataobject.ChatInviteParticipantsDO, error) {
	if d.Postgres != nil {
		return d.Postgres.Store.InviteParticipants.SelectListByLink(ctx, link, requested)
	}
	return d.ChatInviteParticipantsDAO.SelectListByLink(ctx, link, requested)
}

func (d *Dao) SelectInviteParticipantsByLinkWithCB(ctx context.Context, link string, requested int32, cb func(int, int, *dataobject.ChatInviteParticipantsDO)) ([]dataobject.ChatInviteParticipantsDO, error) {
	var list []dataobject.ChatInviteParticipantsDO
	var err error
	if d.Postgres != nil {
		list, err = d.Postgres.Store.InviteParticipants.SelectListByLink(ctx, link, requested)
	} else {
		return d.ChatInviteParticipantsDAO.SelectListByLinkWithCB(ctx, link, requested, cb)
	}
	if err != nil {
		return nil, err
	}
	if cb != nil {
		for i := range list {
			cb(len(list), i, &list[i])
		}
	}
	return list, nil
}

func (d *Dao) SelectRecentInviteParticipants(ctx context.Context, chatID int64) ([]dataobject.ChatInviteParticipantsDO, error) {
	if d.Postgres != nil {
		return d.Postgres.Store.InviteParticipants.SelectRecentRequestedList(ctx, chatID)
	}
	return d.ChatInviteParticipantsDAO.SelectRecentRequestedList(ctx, chatID)
}

func (d *Dao) SelectRecentInviteParticipantsWithCB(ctx context.Context, chatID int64, cb func(int, int, *dataobject.ChatInviteParticipantsDO)) ([]dataobject.ChatInviteParticipantsDO, error) {
	var list []dataobject.ChatInviteParticipantsDO
	var err error
	if d.Postgres != nil {
		list, err = d.Postgres.Store.InviteParticipants.SelectRecentRequestedList(ctx, chatID)
	} else {
		return d.ChatInviteParticipantsDAO.SelectRecentRequestedListWithCB(ctx, chatID, cb)
	}
	if err != nil {
		return nil, err
	}
	if cb != nil {
		for i := range list {
			cb(len(list), i, &list[i])
		}
	}
	return list, nil
}

func (d *Dao) SelectInviteParticipantsByQueryWithCB(ctx context.Context, chatID int64, link string, requested int32, q string, cb func(int, int, *dataobject.ChatInviteParticipantsDO)) ([]dataobject.ChatInviteParticipantsDO, error) {
	var list []dataobject.ChatInviteParticipantsDO
	var err error
	if d.Postgres != nil {
		list, err = d.Postgres.Store.InviteParticipants.SelectListByQuery(ctx, chatID, link, requested, q)
	} else {
		return d.ChatInviteParticipantsDAO.SelectListByQueryWithCB(ctx, chatID, link, requested, q, cb)
	}
	if err != nil {
		return nil, err
	}
	if cb != nil {
		for i := range list {
			cb(len(list), i, &list[i])
		}
	}
	return list, nil
}

func (d *Dao) InsertInviteParticipant(ctx context.Context, value *dataobject.ChatInviteParticipantsDO) (int64, int64, error) {
	if d.Postgres != nil {
		return d.Postgres.Store.InviteParticipants.Insert(ctx, value)
	}
	return d.ChatInviteParticipantsDAO.Insert(ctx, value)
}

func (d *Dao) UpdateInviteApproved(ctx context.Context, approvedBy, chatID, userID int64) (int64, error) {
	if d.Postgres != nil {
		return d.Postgres.Store.InviteParticipants.UpdateApprovedBy(ctx, approvedBy, chatID, userID)
	}
	return d.ChatInviteParticipantsDAO.UpdateApprovedBy(ctx, approvedBy, chatID, userID)
}

func (d *Dao) UpdateInviteApprovedByLink(ctx context.Context, approvedBy, chatID, userID int64, link string) (int64, error) {
	if d.Postgres != nil {
		return d.Postgres.Store.InviteParticipants.UpdateApprovedByLink(ctx, approvedBy, chatID, userID, link)
	}
	return d.ChatInviteParticipantsDAO.UpdateApprovedByLink(ctx, approvedBy, chatID, userID, link)
}

func (d *Dao) DeleteInviteParticipant(ctx context.Context, chatID, userID int64) (int64, error) {
	if d.Postgres != nil {
		return d.Postgres.Store.InviteParticipants.Delete(ctx, chatID, userID)
	}
	return d.ChatInviteParticipantsDAO.Delete(ctx, chatID, userID)
}

func (d *Dao) DeleteInviteParticipantByLink(ctx context.Context, chatID, userID int64, link string) (int64, error) {
	if d.Postgres != nil {
		return d.Postgres.Store.InviteParticipants.DeleteByLink(ctx, chatID, userID, link)
	}
	return d.ChatInviteParticipantsDAO.DeleteByLink(ctx, chatID, userID, link)
}

func (d *Dao) UpdateChatParticipantLink(ctx context.Context, link string, chatID, userID int64) (int64, error) {
	if d.Postgres != nil {
		return d.Postgres.Store.Participants.UpdateLink(ctx, link, chatID, userID)
	}
	return d.ChatParticipantsDAO.UpdateLink(ctx, link, chatID, userID)
}

func (d *Dao) SelectChatParticipantsWithCB(ctx context.Context, chatID int64, cb func(int, int, *dataobject.ChatParticipantsDO)) ([]dataobject.ChatParticipantsDO, error) {
	var list []dataobject.ChatParticipantsDO
	var err error
	if d.Postgres != nil {
		list, err = d.Postgres.Store.Participants.SelectList(ctx, chatID)
	} else {
		return d.ChatParticipantsDAO.SelectListWithCB(ctx, chatID, cb)
	}
	if err != nil {
		return nil, err
	}
	if cb != nil {
		for i := range list {
			cb(len(list), i, &list[i])
		}
	}
	return list, nil
}

func (d *Dao) SelectUsersChatIdListWithCB(ctx context.Context, userIDs []int64, cb func(int, int, *dataobject.ChatParticipantsDO)) ([]dataobject.ChatParticipantsDO, error) {
	var list []dataobject.ChatParticipantsDO
	var err error
	if d.Postgres != nil {
		list, err = d.Postgres.Store.Participants.SelectUsersChatIdList(ctx, userIDs)
	} else {
		return d.ChatParticipantsDAO.SelectUsersChatIdListWithCB(ctx, userIDs, cb)
	}
	if err != nil {
		return nil, err
	}
	if cb != nil {
		for i := range list {
			cb(len(list), i, &list[i])
		}
	}
	return list, nil
}

func (d *Dao) SelectMyAdminListWithCB(ctx context.Context, userID int64, cb func(int, int, int64)) ([]int64, error) {
	var list []int64
	var err error
	if d.Postgres != nil {
		list, err = d.Postgres.Store.Participants.SelectMyAdminList(ctx, userID)
	} else {
		return d.ChatParticipantsDAO.SelectMyAdminListWithCB(ctx, userID, cb)
	}
	if err != nil {
		return nil, err
	}
	if cb != nil {
		for i := range list {
			cb(len(list), i, list[i])
		}
	}
	return list, nil
}

func (d *Dao) SearchChatsForUser(ctx context.Context, userID int64, query string, offset int64, limit int32) ([]int64, error) {
	if d.Postgres != nil {
		return d.Postgres.Store.Chats.SearchByQueryStringForUserOffset(ctx, userID, query, offset, limit)
	}
	return d.ChatsDAO.SearchByQueryStringForUserOffset(ctx, userID, query, offset, limit)
}
