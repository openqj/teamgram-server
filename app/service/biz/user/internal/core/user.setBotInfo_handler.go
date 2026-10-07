package core

import (
	"context"
	"errors"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/teamgram/marmota/pkg/stores/sqlx"
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/user/internal/dao"
	"github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

func (c *UserCore) UserSetBotInfo(in *user.BotRegistrySetBotInfoRequest) (*mtproto.Bool, error) {
	if in == nil || in.GetBotId() <= 0 {
		return nil, mtproto.ErrInputRequestInvalid
	}
	if c == nil || c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.DB == nil {
		return nil, mtproto.ErrMethodNotImpl
	}
	if c.MD == nil || c.MD.GetUserId() <= 0 {
		return nil, mtproto.ErrAuthKeyUnregistered
	}
	if c.MD.GetUserId() != in.GetBotId() {
		return nil, mtproto.ErrUserIdInvalid
	}
	if in.GetName() != nil && (strings.TrimSpace(in.GetName().GetValue()) == "" || !validBotProfileText(in.GetName().GetValue(), 64, false)) {
		return nil, mtproto.ErrInputRequestInvalid
	}
	if in.GetAbout() != nil && !validBotProfileText(in.GetAbout().GetValue(), 128, true) {
		return nil, mtproto.ErrInputRequestInvalid
	}
	if in.GetDescription() != nil && !validBotProfileText(in.GetDescription().GetValue(), 10240, true) {
		return nil, mtproto.ErrInputRequestInvalid
	}

	_, _, err := c.svcCtx.Dao.CachedConn.Exec(c.ctx, func(ctx context.Context, conn *sqlx.DB) (int64, int64, error) {
		result := sqlx.TxWrapper(ctx, conn, func(tx *sqlx.Tx, storeResult *sqlx.StoreResult) {
			var bot struct {
				BotID    int64 `db:"bot_id"`
				UserType int32 `db:"user_type"`
				Deleted  bool  `db:"deleted"`
			}
			storeResult.Err = tx.QueryRowPartial(&bot, `SELECT b.bot_id, u.user_type, u.deleted
				FROM bots b JOIN users u ON u.id=b.bot_id WHERE b.bot_id=? FOR UPDATE`, in.GetBotId())
			if errors.Is(storeResult.Err, sqlx.ErrNotFound) {
				storeResult.Err = mtproto.ErrBotInvalid
				return
			}
			if storeResult.Err != nil {
				return
			}
			if bot.BotID != in.GetBotId() || bot.UserType != user.UserTypeBot || bot.Deleted {
				storeResult.Err = mtproto.ErrBotInvalid
				return
			}

			sets := make([]string, 0, 2)
			args := make([]any, 0, 3)
			if in.GetName() != nil {
				sets = append(sets, "first_name=?")
				args = append(args, in.GetName().GetValue())
			}
			if in.GetAbout() != nil {
				sets = append(sets, "about=?")
				args = append(args, in.GetAbout().GetValue())
			}
			if len(sets) > 0 {
				args = append(args, in.GetBotId())
				_, storeResult.Err = tx.Exec("UPDATE users SET "+strings.Join(sets, ",")+" WHERE id=?", args...)
				if storeResult.Err != nil {
					return
				}
			}
			if in.GetDescription() != nil {
				_, storeResult.Err = tx.Exec("UPDATE bots SET description=? WHERE bot_id=?", in.GetDescription().GetValue(), in.GetBotId())
			}
		})
		return 0, 1, result.Err
	}, dao.GenCacheUserDataCacheKey(in.GetBotId()))
	if err != nil {
		return nil, err
	}
	return mtproto.BoolTrue, nil
}

func validBotProfileText(value string, maxRunes int, allowLineBreaks bool) bool {
	if !utf8.ValidString(value) || utf8.RuneCountInString(value) > maxRunes {
		return false
	}
	for _, r := range value {
		if unicode.IsControl(r) && !(allowLineBreaks && (r == '\n' || r == '\r' || r == '\t')) {
			return false
		}
	}
	return true
}
