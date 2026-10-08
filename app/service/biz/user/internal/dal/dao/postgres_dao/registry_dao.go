package postgres_dao

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/teamgram/teamgram-server/app/service/biz/user/internal/dal/dataobject"
)

const botColumns = `id, bot_id, bot_type, creator_user_id, manager_bot_id,
bot_can_manage_bots, token, description, bot_chat_history, bot_nochats,
bot_inline_geo, bot_info_version, bot_inline_placeholder, attach_menu_enabled,
bot_attach_menu, bot_business, bot_has_main_app, bot_active_users,
has_menu_button, menu_button_text, menu_button_url, bot_can_edit,
has_preview_medias, description_photo_id, description_document_id, main_app_url,
has_app_settings, placeholder_path, background_color, background_dark_color,
header_color, header_dark_color, privacy_policy_url, mode`

type BotsDAO struct{ db DB }

func NewBotsDAO(db DB) *BotsDAO { return &BotsDAO{db: db} }

func scanBot(row interface{ Scan(...any) error }) (*dataobject.BotsDO, error) {
	do := new(dataobject.BotsDO)
	err := row.Scan(&do.Id, &do.BotId, &do.BotType, &do.CreatorUserId, &do.ManagerBotId,
		&do.BotCanManageBots, &do.Token, &do.Description, &do.BotChatHistory, &do.BotNochats,
		&do.BotInlineGeo, &do.BotInfoVersion, &do.BotInlinePlaceholder, &do.AttachMenuEnabled,
		&do.BotAttachMenu, &do.BotBusiness, &do.BotHasMainApp, &do.BotActiveUsers,
		&do.HasMenuButton, &do.MenuButtonText, &do.MenuButtonUrl, &do.BotCanEdit,
		&do.HasPreviewMedias, &do.DescriptionPhotoId, &do.DescriptionDocumentId,
		&do.MainAppUrl, &do.HasAppSettings, &do.PlaceholderPath, &do.BackgroundColor,
		&do.BackgroundDarkColor, &do.HeaderColor, &do.HeaderDarkColor, &do.PrivacyPolicyUrl,
		&do.Mode)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return do, nil
}

func scanBots(rows pgx.Rows) ([]dataobject.BotsDO, error) {
	defer rows.Close()
	result := make([]dataobject.BotsDO, 0)
	for rows.Next() {
		var do dataobject.BotsDO
		if _, err := scanBotRow(rows, &do); err != nil {
			return nil, err
		}
		result = append(result, do)
	}
	return result, rows.Err()
}

func scanBotRow(row interface{ Scan(...any) error }, do *dataobject.BotsDO) (struct{}, error) {
	err := row.Scan(&do.Id, &do.BotId, &do.BotType, &do.CreatorUserId, &do.ManagerBotId,
		&do.BotCanManageBots, &do.Token, &do.Description, &do.BotChatHistory, &do.BotNochats,
		&do.BotInlineGeo, &do.BotInfoVersion, &do.BotInlinePlaceholder, &do.AttachMenuEnabled,
		&do.BotAttachMenu, &do.BotBusiness, &do.BotHasMainApp, &do.BotActiveUsers,
		&do.HasMenuButton, &do.MenuButtonText, &do.MenuButtonUrl, &do.BotCanEdit,
		&do.HasPreviewMedias, &do.DescriptionPhotoId, &do.DescriptionDocumentId,
		&do.MainAppUrl, &do.HasAppSettings, &do.PlaceholderPath, &do.BackgroundColor,
		&do.BackgroundDarkColor, &do.HeaderColor, &do.HeaderDarkColor, &do.PrivacyPolicyUrl,
		&do.Mode)
	return struct{}{}, err
}

func (d *BotsDAO) Select(ctx context.Context, botID int64) (*dataobject.BotsDO, error) {
	return scanBot(d.db.QueryRow(ctx, `SELECT `+botColumns+` FROM bots WHERE bot_id = $1`, botID))
}

func (d *BotsDAO) SelectByToken(ctx context.Context, token string) (int64, error) {
	var botID int64
	err := d.db.QueryRow(ctx, `SELECT bot_id FROM bots WHERE token = $1`, token).Scan(&botID)
	if err == pgx.ErrNoRows {
		return 0, nil
	}
	return botID, err
}

func (d *BotsDAO) SelectByIdList(ctx context.Context, ids []int32) ([]dataobject.BotsDO, error) {
	if len(ids) == 0 {
		return []dataobject.BotsDO{}, nil
	}
	values := make([]int64, len(ids))
	for i, id := range ids {
		values[i] = int64(id)
	}
	rows, err := d.db.Query(ctx, `SELECT `+botColumns+` FROM bots WHERE bot_id = ANY($1::bigint[]) ORDER BY bot_id`, values)
	if err != nil {
		return nil, err
	}
	return scanBots(rows)
}

func (d *BotsDAO) SelectByIdListWithCB(ctx context.Context, ids []int32, cb func(int, int, *dataobject.BotsDO)) ([]dataobject.BotsDO, error) {
	list, err := d.SelectByIdList(ctx, ids)
	if cb != nil {
		for i := range list {
			cb(len(list), i, &list[i])
		}
	}
	return list, err
}

func (d *BotsDAO) Update(ctx context.Context, values map[string]any, botID int64) (int64, error) {
	return d.update(ctx, d.db, values, botID)
}

func (d *BotsDAO) UpdateTx(ctx context.Context, tx DB, values map[string]any, botID int64) (int64, error) {
	return d.update(ctx, tx, values, botID)
}

func (d *BotsDAO) update(ctx context.Context, db DB, values map[string]any, botID int64) (int64, error) {
	allowed := map[string]bool{
		"bot_type": true, "creator_user_id": true, "manager_bot_id": true,
		"bot_can_manage_bots": true, "token": true, "description": true,
		"bot_chat_history": true, "bot_nochats": true, "verified": true,
		"bot_inline_geo": true, "bot_info_version": true, "bot_inline_placeholder": true,
		"attach_menu_enabled": true, "bot_attach_menu": true, "bot_business": true,
		"bot_has_main_app": true, "bot_active_users": true, "has_menu_button": true,
		"menu_button_text": true, "menu_button_url": true, "bot_can_edit": true,
		"has_preview_medias": true, "description_photo_id": true, "description_document_id": true,
		"main_app_url": true, "has_app_settings": true, "placeholder_path": true,
		"background_color": true, "background_dark_color": true, "header_color": true,
		"header_dark_color": true, "privacy_policy_url": true, "mode": true,
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		if !allowed[key] {
			return 0, fmt.Errorf("unsupported bots column %q", key)
		}
		keys = append(keys, key)
	}
	if len(keys) == 0 {
		return 0, nil
	}
	sort.Strings(keys)
	args := make([]any, 0, len(keys)+1)
	sets := make([]string, 0, len(keys))
	for i, key := range keys {
		args = append(args, values[key])
		sets = append(sets, fmt.Sprintf("%s = $%d", key, i+1))
	}
	args = append(args, botID)
	tag, err := db.Exec(ctx, `UPDATE bots SET `+strings.Join(sets, ", ")+fmt.Sprintf(" WHERE bot_id = $%d", len(args)), args...)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

type BotCommandsDAO struct{ db DB }

func NewBotCommandsDAO(db DB) *BotCommandsDAO { return &BotCommandsDAO{db: db} }

func (d *BotCommandsDAO) InsertOrUpdate(ctx context.Context, do *dataobject.BotCommandsDO) (int64, int64, error) {
	var id int64
	err := d.db.QueryRow(ctx, `INSERT INTO bot_commands(bot_id,command,description) VALUES($1,$2,$3)
 ON CONFLICT (bot_id, command) DO UPDATE SET description = EXCLUDED.description RETURNING id`, do.BotId, do.Command, do.Description).Scan(&id)
	if err != nil {
		return 0, 0, err
	}
	do.Id = id
	return id, 1, nil
}

func (d *BotCommandsDAO) InsertOrUpdateTx(ctx context.Context, tx DB, do *dataobject.BotCommandsDO) (int64, int64, error) {
	var id int64
	err := tx.QueryRow(ctx, `INSERT INTO bot_commands(bot_id,command,description) VALUES($1,$2,$3)
 ON CONFLICT (bot_id, command) DO UPDATE SET description = EXCLUDED.description RETURNING id`, do.BotId, do.Command, do.Description).Scan(&id)
	if err != nil {
		return 0, 0, err
	}
	do.Id = id
	return id, 1, nil
}

func (d *BotCommandsDAO) InsertBulk(ctx context.Context, list []*dataobject.BotCommandsDO) (int64, int64, error) {
	return d.insertBulk(ctx, d.db, list)
}

func (d *BotCommandsDAO) InsertBulkTx(ctx context.Context, tx DB, list []*dataobject.BotCommandsDO) (int64, int64, error) {
	return d.insertBulk(ctx, tx, list)
}

func (d *BotCommandsDAO) insertBulk(ctx context.Context, db DB, list []*dataobject.BotCommandsDO) (int64, int64, error) {
	var affected int64
	for _, item := range list {
		if _, _, err := d.insertOrUpdateOn(ctx, db, item); err != nil {
			return 0, affected, err
		}
		affected++
	}
	return 0, affected, nil
}

func (d *BotCommandsDAO) insertOrUpdateOn(ctx context.Context, db DB, do *dataobject.BotCommandsDO) (int64, int64, error) {
	var id int64
	err := db.QueryRow(ctx, `INSERT INTO bot_commands(bot_id,command,description) VALUES($1,$2,$3)
 ON CONFLICT (bot_id, command) DO UPDATE SET description = EXCLUDED.description RETURNING id`, do.BotId, do.Command, do.Description).Scan(&id)
	if err != nil {
		return 0, 0, err
	}
	do.Id = id
	return id, 1, nil
}

func (d *BotCommandsDAO) Delete(ctx context.Context, botID int64) (int64, error) {
	return d.delete(ctx, d.db, botID)
}

func (d *BotCommandsDAO) DeleteTx(ctx context.Context, tx DB, botID int64) (int64, error) {
	return d.delete(ctx, tx, botID)
}

func (d *BotCommandsDAO) delete(ctx context.Context, db DB, botID int64) (int64, error) {
	tag, err := db.Exec(ctx, `DELETE FROM bot_commands WHERE bot_id = $1`, botID)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

func (d *BotCommandsDAO) SelectList(ctx context.Context, botID int64) ([]dataobject.BotCommandsDO, error) {
	return d.selectList(ctx, `SELECT id,bot_id,command,description FROM bot_commands WHERE bot_id = $1 ORDER BY id`, botID)
}

func (d *BotCommandsDAO) SelectListByIdList(ctx context.Context, ids []int32) ([]dataobject.BotCommandsDO, error) {
	if len(ids) == 0 {
		return []dataobject.BotCommandsDO{}, nil
	}
	values := make([]int64, len(ids))
	for i, id := range ids {
		values[i] = int64(id)
	}
	return d.selectList(ctx, `SELECT id,bot_id,command,description FROM bot_commands WHERE bot_id = ANY($1::bigint[]) ORDER BY id`, values)
}

func (d *BotCommandsDAO) selectList(ctx context.Context, query string, args ...any) ([]dataobject.BotCommandsDO, error) {
	rows, err := d.db.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]dataobject.BotCommandsDO, 0)
	for rows.Next() {
		var do dataobject.BotCommandsDO
		if err := rows.Scan(&do.Id, &do.BotId, &do.Command, &do.Description); err != nil {
			return nil, err
		}
		result = append(result, do)
	}
	return result, rows.Err()
}

func (d *BotCommandsDAO) SelectListWithCB(ctx context.Context, botID int64, cb func(int, int, *dataobject.BotCommandsDO)) ([]dataobject.BotCommandsDO, error) {
	list, err := d.SelectList(ctx, botID)
	if cb != nil {
		for i := range list {
			cb(len(list), i, &list[i])
		}
	}
	return list, err
}

func (d *BotCommandsDAO) SelectListByIdListWithCB(ctx context.Context, ids []int32, cb func(int, int, *dataobject.BotCommandsDO)) ([]dataobject.BotCommandsDO, error) {
	list, err := d.SelectListByIdList(ctx, ids)
	if cb != nil {
		for i := range list {
			cb(len(list), i, &list[i])
		}
	}
	return list, err
}

type ImportedContactsDAO struct{ db DB }

func NewImportedContactsDAO(db DB) *ImportedContactsDAO { return &ImportedContactsDAO{db: db} }

func (d *ImportedContactsDAO) InsertOrUpdate(ctx context.Context, do *dataobject.ImportedContactsDO) (int64, int64, error) {
	return d.insertOrUpdate(ctx, d.db, do)
}

func (d *ImportedContactsDAO) InsertOrUpdateTx(ctx context.Context, tx DB, do *dataobject.ImportedContactsDO) (int64, int64, error) {
	return d.insertOrUpdate(ctx, tx, do)
}

func (d *ImportedContactsDAO) insertOrUpdate(ctx context.Context, db DB, do *dataobject.ImportedContactsDO) (int64, int64, error) {
	var id int64
	err := db.QueryRow(ctx, `INSERT INTO imported_contacts(user_id,imported_user_id,deleted) VALUES($1,$2,FALSE)
 ON CONFLICT (user_id,imported_user_id) DO UPDATE SET deleted=FALSE RETURNING id`, do.UserId, do.ImportedUserId).Scan(&id)
	if err != nil {
		return 0, 0, err
	}
	do.Id = id
	return id, 1, nil
}

func (d *ImportedContactsDAO) SelectList(ctx context.Context, userID int64) ([]dataobject.ImportedContactsDO, error) {
	return d.selectList(ctx, `SELECT id,user_id,imported_user_id,deleted FROM imported_contacts WHERE user_id=$1 AND deleted=FALSE ORDER BY id`, userID)
}

func (d *ImportedContactsDAO) SelectListWithCB(ctx context.Context, userID int64, cb func(int, int, *dataobject.ImportedContactsDO)) ([]dataobject.ImportedContactsDO, error) {
	list, err := d.SelectList(ctx, userID)
	if cb != nil {
		for i := range list {
			cb(len(list), i, &list[i])
		}
	}
	return list, err
}

func (d *ImportedContactsDAO) SelectAllList(ctx context.Context, userID int64) ([]dataobject.ImportedContactsDO, error) {
	return d.selectList(ctx, `SELECT id,user_id,imported_user_id,deleted FROM imported_contacts WHERE user_id=$1 ORDER BY id`, userID)
}

func (d *ImportedContactsDAO) SelectAllListWithCB(ctx context.Context, userID int64, cb func(int, int, *dataobject.ImportedContactsDO)) ([]dataobject.ImportedContactsDO, error) {
	list, err := d.SelectAllList(ctx, userID)
	if cb != nil {
		for i := range list {
			cb(len(list), i, &list[i])
		}
	}
	return list, err
}

func (d *ImportedContactsDAO) SelectListByImportedList(ctx context.Context, userID int64, ids []int64) ([]dataobject.ImportedContactsDO, error) {
	if len(ids) == 0 {
		return []dataobject.ImportedContactsDO{}, nil
	}
	return d.selectList(ctx, `SELECT id,user_id,imported_user_id,deleted FROM imported_contacts WHERE user_id=$1 AND deleted=FALSE AND imported_user_id=ANY($2::bigint[])`, userID, ids)
}

func (d *ImportedContactsDAO) SelectListByImportedListWithCB(ctx context.Context, userID int64, ids []int64, cb func(int, int, *dataobject.ImportedContactsDO)) ([]dataobject.ImportedContactsDO, error) {
	list, err := d.SelectListByImportedList(ctx, userID, ids)
	if cb != nil {
		for i := range list {
			cb(len(list), i, &list[i])
		}
	}
	return list, err
}

func (d *ImportedContactsDAO) selectList(ctx context.Context, query string, args ...any) ([]dataobject.ImportedContactsDO, error) {
	rows, err := d.db.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]dataobject.ImportedContactsDO, 0)
	for rows.Next() {
		var do dataobject.ImportedContactsDO
		if err := rows.Scan(&do.Id, &do.UserId, &do.ImportedUserId, &do.Deleted); err != nil {
			return nil, err
		}
		result = append(result, do)
	}
	return result, rows.Err()
}

func (d *ImportedContactsDAO) Delete(ctx context.Context, userID, importedUserID int64) (int64, error) {
	return d.delete(ctx, d.db, userID, importedUserID)
}

func (d *ImportedContactsDAO) DeleteTx(ctx context.Context, tx DB, userID, importedUserID int64) (int64, error) {
	return d.delete(ctx, tx, userID, importedUserID)
}

func (d *ImportedContactsDAO) delete(ctx context.Context, db DB, userID, importedUserID int64) (int64, error) {
	tag, err := db.Exec(ctx, `UPDATE imported_contacts SET deleted=TRUE WHERE user_id=$1 AND imported_user_id=$2`, userID, importedUserID)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

type DefaultHistoryTtlDAO struct{ db DB }

func NewDefaultHistoryTtlDAO(db DB) *DefaultHistoryTtlDAO { return &DefaultHistoryTtlDAO{db: db} }

func (d *DefaultHistoryTtlDAO) InsertOrUpdate(ctx context.Context, do *dataobject.DefaultHistoryTtlDO) (int64, int64, error) {
	return d.insertOrUpdate(ctx, d.db, do)
}

func (d *DefaultHistoryTtlDAO) InsertOrUpdateTx(ctx context.Context, tx DB, do *dataobject.DefaultHistoryTtlDO) (int64, int64, error) {
	return d.insertOrUpdate(ctx, tx, do)
}

func (d *DefaultHistoryTtlDAO) insertOrUpdate(ctx context.Context, db DB, do *dataobject.DefaultHistoryTtlDO) (int64, int64, error) {
	var id int64
	err := db.QueryRow(ctx, `INSERT INTO default_history_ttl(user_id,period) VALUES($1,$2)
 ON CONFLICT (user_id) DO UPDATE SET period=EXCLUDED.period RETURNING id`, do.UserId, do.Period).Scan(&id)
	if err != nil {
		return 0, 0, err
	}
	do.Id = id
	return id, 1, nil
}

func (d *DefaultHistoryTtlDAO) Select(ctx context.Context, userID int64) (*dataobject.DefaultHistoryTtlDO, error) {
	var do dataobject.DefaultHistoryTtlDO
	err := d.db.QueryRow(ctx, `SELECT id,user_id,period FROM default_history_ttl WHERE user_id=$1`, userID).Scan(&do.Id, &do.UserId, &do.Period)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	return &do, err
}
