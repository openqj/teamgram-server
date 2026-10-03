package mysql_dao

import (
	"context"
	"database/sql"

	"github.com/zeromicro/go-zero/core/logx"
)

// DeleteByLink removes only the pending request made with the given invite.
func (dao *ChatInviteParticipantsDAO) DeleteByLink(ctx context.Context, chatId, userId int64, link string) (rowsAffected int64, err error) {
	var result sql.Result
	result, err = dao.db.Exec(ctx,
		"delete from chat_invite_participants where chat_id = ? and user_id = ? and link = ? and requested = 1",
		chatId, userId, link)
	if err != nil {
		logx.WithContext(ctx).Errorf("exec in DeleteByLink(_), error: %v", err)
		return
	}
	rowsAffected, err = result.RowsAffected()
	if err != nil {
		logx.WithContext(ctx).Errorf("rowsAffected in DeleteByLink(_), error: %v", err)
	}
	return
}

// UpdateApprovedByLink approves only the pending request made with the given invite.
func (dao *ChatInviteParticipantsDAO) UpdateApprovedByLink(ctx context.Context, approvedBy, chatId, userId int64, link string) (rowsAffected int64, err error) {
	var result sql.Result
	result, err = dao.db.Exec(ctx,
		"update chat_invite_participants set requested = 0, approved_by = ? where chat_id = ? and user_id = ? and link = ? and requested = 1",
		approvedBy, chatId, userId, link)
	if err != nil {
		logx.WithContext(ctx).Errorf("exec in UpdateApprovedByLink(_), error: %v", err)
		return
	}
	rowsAffected, err = result.RowsAffected()
	if err != nil {
		logx.WithContext(ctx).Errorf("rowsAffected in UpdateApprovedByLink(_), error: %v", err)
	}
	return
}
