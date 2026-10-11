package dao

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/user/internal/dal/dataobject"
)

var errPostgresUserStoreUnavailable = errors.New("biz/user: postgres store is not configured")

// SelectUserByID reads a user from the authoritative PostgreSQL store.
func (d *Dao) SelectUserByID(ctx context.Context, id int64) (*dataobject.UsersDO, error) {
	if d != nil && d.Postgres != nil && d.Postgres.Store != nil && d.Postgres.Store.Users != nil {
		return d.Postgres.Store.Users.SelectByID(ctx, id)
	}
	return nil, errPostgresUserStoreUnavailable
}

func (d *Dao) SelectUserByPhone(ctx context.Context, phone string) (*dataobject.UsersDO, error) {
	if d != nil && d.Postgres != nil && d.Postgres.Store != nil && d.Postgres.Store.Users != nil {
		return d.Postgres.Store.Users.SelectByPhoneNumber(ctx, phone)
	}
	return nil, errPostgresUserStoreUnavailable
}

func (d *Dao) UpdateUserFields(ctx context.Context, id int64, values map[string]any) (int64, error) {
	if d != nil && d.Postgres != nil && d.Postgres.Store != nil && d.Postgres.Store.Users != nil {
		var rows int64
		err := d.Postgres.InTx(ctx, func(tx pgx.Tx) error {
			var err error
			rows, err = d.Postgres.Store.Users.UpdateTx(ctx, tx, values, id)
			if err != nil {
				return err
			}
			if rows == 0 && len(values) != 0 {
				return mtproto.ErrUserIdInvalid
			}
			return nil
		})
		return rows, err
	}
	return 0, errPostgresUserStoreUnavailable
}

func (d *Dao) SearchUserIDs(ctx context.Context, q, q2 string, excluded []int64, limit int32) ([]int64, error) {
	if d != nil && d.Postgres != nil && d.Postgres.Store != nil && d.Postgres.Store.Users != nil {
		return d.Postgres.Store.Users.SearchByQueryString(ctx, q, q2, excluded, limit)
	}
	return nil, errPostgresUserStoreUnavailable
}

func (d *Dao) SelectUserContactIDs(ctx context.Context, ownerID int64) ([]int64, error) {
	if d != nil && d.Postgres != nil && d.Postgres.Store != nil && d.Postgres.Store.Contacts != nil {
		return d.Postgres.Store.Contacts.SelectUserContactIdList(ctx, ownerID)
	}
	return nil, errPostgresUserStoreUnavailable
}

func (d *Dao) SelectReverseContactIDs(ctx context.Context, contactID int64) ([]int64, error) {
	if d != nil && d.Postgres != nil && d.Postgres.Store != nil && d.Postgres.Store.Contacts != nil {
		return d.Postgres.Store.Contacts.SelectUserReverseContactIdList(ctx, contactID)
	}
	return nil, errPostgresUserStoreUnavailable
}

func (d *Dao) SelectUsersByPhones(ctx context.Context, phones []string) ([]dataobject.UsersDO, error) {
	if d != nil && d.Postgres != nil && d.Postgres.Store != nil && d.Postgres.Store.Users != nil {
		return d.Postgres.Store.Users.SelectUsersByPhoneList(ctx, phones)
	}
	return nil, errPostgresUserStoreUnavailable
}

func (d *Dao) UpdateContactName(ctx context.Context, firstName, lastName string, ownerID, contactID int64) (int64, error) {
	if d != nil && d.Postgres != nil && d.Postgres.Store != nil && d.Postgres.Store.Contacts != nil {
		return d.Postgres.Store.Contacts.UpdateContactName(ctx, firstName, lastName, ownerID, contactID)
	}
	return 0, errPostgresUserStoreUnavailable
}

func (d *Dao) UpdateContactMutual(ctx context.Context, mutual bool, ownerID, contactID int64) (int64, error) {
	if d != nil && d.Postgres != nil && d.Postgres.Store != nil && d.Postgres.Store.Contacts != nil {
		return d.Postgres.Store.Contacts.UpdateMutual(ctx, mutual, ownerID, contactID)
	}
	return 0, errPostgresUserStoreUnavailable
}

func (d *Dao) InsertOrUpdateContact(ctx context.Context, value *dataobject.UserContactsDO) (int64, int64, error) {
	if d != nil && d.Postgres != nil && d.Postgres.Store != nil && d.Postgres.Store.Contacts != nil {
		return d.Postgres.Store.Contacts.InsertOrUpdate(ctx, value)
	}
	return 0, 0, errPostgresUserStoreUnavailable
}

func (d *Dao) SelectUsernames(ctx context.Context, userID int64, cb func(*dataobject.UsernameDO)) ([]dataobject.UsernameDO, error) {
	if d != nil && d.Postgres != nil && d.Postgres.Store != nil && d.Postgres.Store.Username != nil {
		list, err := d.Postgres.Store.Username.SelectAllByPeer(ctx, 2, userID)
		if cb != nil {
			for i := range list {
				cb(&list[i])
			}
		}
		return list, err
	}
	return nil, errPostgresUserStoreUnavailable
}

func (d *Dao) SelectUsername(ctx context.Context, username string) (*dataobject.UsernameDO, error) {
	if d != nil && d.Postgres != nil && d.Postgres.Store != nil && d.Postgres.Store.Username != nil {
		return d.Postgres.Store.Username.SelectByUsername(ctx, username)
	}
	return nil, errPostgresUserStoreUnavailable
}

func (d *Dao) SelectUsernamesByPeer(ctx context.Context, peerType int32, peerID int64) ([]dataobject.UsernameDO, error) {
	if d != nil && d.Postgres != nil && d.Postgres.Store != nil && d.Postgres.Store.Username != nil {
		return d.Postgres.Store.Username.SelectByPeer(ctx, peerType, peerID)
	}
	return nil, errPostgresUserStoreUnavailable
}

func (d *Dao) SelectUsernameList(ctx context.Context, names []string) ([]dataobject.UsernameDO, error) {
	if d != nil && d.Postgres != nil && d.Postgres.Store != nil && d.Postgres.Store.Username != nil {
		return d.Postgres.Store.Username.SelectList(ctx, names)
	}
	return nil, errPostgresUserStoreUnavailable
}

func (d *Dao) SearchUsernames(ctx context.Context, q string, excluded []int64, limit int32) ([]dataobject.UsernameDO, error) {
	if d != nil && d.Postgres != nil && d.Postgres.Store != nil && d.Postgres.Store.Username != nil {
		return d.Postgres.Store.Username.SearchByQueryNotIdList(ctx, q, excluded, limit)
	}
	return nil, errPostgresUserStoreUnavailable
}

func (d *Dao) InsertUsername(ctx context.Context, value *dataobject.UsernameDO) (int64, int64, error) {
	if d != nil && d.Postgres != nil && d.Postgres.Store != nil && d.Postgres.Store.Username != nil {
		return d.Postgres.Store.Username.Insert(ctx, value)
	}
	return 0, 0, errPostgresUserStoreUnavailable
}

func (d *Dao) DeleteUsername(ctx context.Context, username string) (int64, error) {
	if d != nil && d.Postgres != nil && d.Postgres.Store != nil && d.Postgres.Store.Username != nil {
		return d.Postgres.Store.Username.Delete(ctx, username)
	}
	return 0, errPostgresUserStoreUnavailable
}

func (d *Dao) DeleteUsernameByPeer(ctx context.Context, peerType int32, peerID int64) (int64, error) {
	if d != nil && d.Postgres != nil && d.Postgres.Store != nil && d.Postgres.Store.Username != nil {
		return d.Postgres.Store.Username.DeleteByPeer(ctx, peerType, peerID)
	}
	return 0, errPostgresUserStoreUnavailable
}

func (d *Dao) SelectBot(ctx context.Context, botID int64) (*dataobject.BotsDO, error) {
	if d != nil && d.Postgres != nil && d.Postgres.Store != nil && d.Postgres.Store.Bots != nil {
		return d.Postgres.Store.Bots.Select(ctx, botID)
	}
	return nil, errPostgresUserStoreUnavailable
}

func (d *Dao) SelectBotByToken(ctx context.Context, token string) (int64, error) {
	if d != nil && d.Postgres != nil && d.Postgres.Store != nil && d.Postgres.Store.Bots != nil {
		return d.Postgres.Store.Bots.SelectByToken(ctx, token)
	}
	return 0, errPostgresUserStoreUnavailable
}

func (d *Dao) SelectBotIdsByCreatorUserID(ctx context.Context, creatorUserID int64) ([]int64, error) {
	if d != nil && d.Postgres != nil && d.Postgres.Store != nil && d.Postgres.Store.Bots != nil {
		return d.Postgres.Store.Bots.SelectBotIdsByCreatorUserId(ctx, creatorUserID)
	}
	return nil, errPostgresUserStoreUnavailable
}

func (d *Dao) SelectBotIdsByManagerBotID(ctx context.Context, managerBotID int64) ([]int64, error) {
	if d != nil && d.Postgres != nil && d.Postgres.Store != nil && d.Postgres.Store.Bots != nil {
		return d.Postgres.Store.Bots.SelectBotIdsByManagerBotId(ctx, managerBotID)
	}
	return nil, errPostgresUserStoreUnavailable
}

func (d *Dao) SelectBotCommands(ctx context.Context, botID int64) ([]dataobject.BotCommandsDO, error) {
	if d != nil && d.Postgres != nil && d.Postgres.Store != nil && d.Postgres.Store.BotCommands != nil {
		return d.Postgres.Store.BotCommands.SelectList(ctx, botID)
	}
	return nil, errPostgresUserStoreUnavailable
}
