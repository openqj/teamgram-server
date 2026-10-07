package core

import (
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

func (c *UserCore) UserExportBotToken(in *user.TLUserExportBotToken) (*mtproto.String, error) {
	if in == nil || in.GetBotId() <= 0 {
		return nil, mtproto.ErrInputRequestInvalid
	}
	if c.MD == nil || c.MD.GetUserId() <= 0 {
		return nil, mtproto.ErrForbiddenUserBotInvalid
	}
	if c.svcCtx == nil || c.svcCtx.Dao == nil {
		return nil, mtproto.ErrMethodNotImpl
	}
	botUser, err := c.svcCtx.Dao.UsersDAO.SelectById(c.ctx, in.GetBotId())
	if err != nil {
		return nil, err
	}
	if botUser == nil || botUser.UserType != user.UserTypeBot {
		return nil, mtproto.ErrBotInvalid
	}
	if in.GetAccessHash() != 0 && in.GetAccessHash() != botUser.AccessHash {
		return nil, mtproto.ErrUserIdInvalid
	}

	token, err := c.svcCtx.Dao.ExportBotToken(c.ctx, in.GetBotId(), c.MD.GetUserId(), in.GetRevoke())
	if err != nil {
		return nil, err
	}
	return mtproto.MakeTLString(&mtproto.String{V: token}).To_String(), nil
}
