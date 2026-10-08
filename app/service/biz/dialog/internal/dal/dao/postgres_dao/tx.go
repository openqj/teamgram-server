package postgres_dao

import (
	"context"

	"github.com/teamgram/teamgram-server/app/service/biz/dialog/internal/dal/dataobject"
)

// The generated MySQL layer exposes transaction-suffixed methods. These
// wrappers retain that call shape while accepting the pgx transaction boundary
// used by the PostgreSQL service path.
func (d *DialogsDAO) UpdateOutboxDialogTx(ctx context.Context, tx DB, topMessage int32, date2, userID int64, peerType int32, peerID int64) (int64, error) {
	return (&DialogsDAO{db: tx}).UpdateOutboxDialog(ctx, topMessage, date2, userID, peerType, peerID)
}
func (d *DialogsDAO) UpdateInboxDialogTx(ctx context.Context, tx DB, values map[string]any, userID int64, peerType int32, peerID int64) (int64, error) {
	return (&DialogsDAO{db: tx}).UpdateInboxDialog(ctx, values, userID, peerType, peerID)
}
func (d *DialogsDAO) UpdateReadInboxMaxIdTx(ctx context.Context, tx DB, unreadCount, readInboxMaxID int32, userID, peerDialogID int64) (int64, error) {
	return (&DialogsDAO{db: tx}).UpdateReadInboxMaxId(ctx, unreadCount, readInboxMaxID, userID, peerDialogID)
}
func (d *DialogsDAO) UpdateReadOutboxMaxIdTx(ctx context.Context, tx DB, value int32, userID, peerDialogID int64) (int64, error) {
	return (&DialogsDAO{db: tx}).UpdateReadOutboxMaxId(ctx, value, userID, peerDialogID)
}
func (d *DialogsDAO) UpdateTopMessageTx(ctx context.Context, tx DB, value int32, userID, peerDialogID int64) (int64, error) {
	return (&DialogsDAO{db: tx}).UpdateTopMessage(ctx, value, userID, peerDialogID)
}
func (d *DialogsDAO) UpdatePinnedMsgIdTx(ctx context.Context, tx DB, value int32, userID, peerDialogID int64) (int64, error) {
	return (&DialogsDAO{db: tx}).UpdatePinnedMsgId(ctx, value, userID, peerDialogID)
}
func (d *DialogsDAO) DeleteTx(ctx context.Context, tx DB, userID int64, peerType int32, peerID int64) (int64, error) {
	return (&DialogsDAO{db: tx}).Delete(ctx, userID, peerType, peerID)
}
func (d *DialogsDAO) SaveDraftTx(ctx context.Context, tx DB, draftType int32, messageData string, userID int64, peerType int32, peerID int64) (int64, error) {
	return (&DialogsDAO{db: tx}).SaveDraft(ctx, draftType, messageData, userID, peerType, peerID)
}
func (d *DialogsDAO) ClearAllDraftsTx(ctx context.Context, tx DB, userID int64) (int64, error) {
	return (&DialogsDAO{db: tx}).ClearAllDrafts(ctx, userID)
}
func (d *DialogsDAO) UpdatePeerFolderIdTx(ctx context.Context, tx DB, folderID int32, userID int64, peerType int32, peerID int64) (int64, error) {
	return (&DialogsDAO{db: tx}).UpdatePeerFolderId(ctx, folderID, userID, peerType, peerID)
}
func (d *DialogsDAO) UpdatePeerDialogListFolderIdTx(ctx context.Context, tx DB, folderID int32, userID int64, ids []int64) (int64, error) {
	return (&DialogsDAO{db: tx}).UpdatePeerDialogListFolderId(ctx, folderID, userID, ids)
}
func (d *DialogsDAO) UpdatePeerDialogListPinnedTx(ctx context.Context, tx DB, pinned int64, userID int64, ids []int64) (int64, error) {
	return (&DialogsDAO{db: tx}).UpdatePeerDialogListPinned(ctx, pinned, userID, ids)
}
func (d *DialogsDAO) UpdateFolderPeerDialogListPinnedTx(ctx context.Context, tx DB, pinned int64, userID int64, ids []int64) (int64, error) {
	return (&DialogsDAO{db: tx}).UpdateFolderPeerDialogListPinned(ctx, pinned, userID, ids)
}
func (d *DialogsDAO) UpdateUnPinnedNotIdListTx(ctx context.Context, tx DB, userID int64, ids []int64) (int64, error) {
	return (&DialogsDAO{db: tx}).UpdateUnPinnedNotIdList(ctx, userID, ids)
}
func (d *DialogsDAO) UpdateFolderUnPinnedNotIdListTx(ctx context.Context, tx DB, userID int64, ids []int64) (int64, error) {
	return (&DialogsDAO{db: tx}).UpdateFolderUnPinnedNotIdList(ctx, userID, ids)
}
func (d *DialogsDAO) UpdateUnreadCountTx(ctx context.Context, tx DB, unreadCount, unreadMentionsCount, unreadReactionsCount int32, userID int64, peerType int32, peerID int64) (int64, error) {
	return (&DialogsDAO{db: tx}).UpdateUnreadCount(ctx, unreadCount, unreadMentionsCount, unreadReactionsCount, userID, peerType, peerID)
}

func (d *DialogFiltersDAO) InsertOrUpdateTx(ctx context.Context, tx DB, do *dataobject.DialogFiltersDO) (int64, int64, error) {
	return (&DialogFiltersDAO{db: tx}).InsertOrUpdate(ctx, do)
}
func (d *DialogFiltersDAO) UpdateOrderTx(ctx context.Context, tx DB, orderValue, userID int64, filterID int32) (int64, error) {
	return (&DialogFiltersDAO{db: tx}).UpdateOrder(ctx, orderValue, userID, filterID)
}
func (d *DialogFiltersDAO) ClearTx(ctx context.Context, tx DB, userID int64, filterID int32) (int64, error) {
	return (&DialogFiltersDAO{db: tx}).Clear(ctx, userID, filterID)
}

func (d *DraftsDAO) InsertOrUpdateTx(ctx context.Context, tx DB, do *dataobject.DraftsDO) (int64, int64, error) {
	return (&DraftsDAO{db: tx}).InsertOrUpdate(ctx, do)
}
func (d *DraftsDAO) ClearByIdListTx(ctx context.Context, tx DB, userID int32, ids []int64) (int64, error) {
	return (&DraftsDAO{db: tx}).ClearByIdList(ctx, userID, ids)
}

func (d *SavedDialogsDAO) InsertOrUpdateTx(ctx context.Context, tx DB, do *dataobject.SavedDialogsDO) (int64, int64, error) {
	return (&SavedDialogsDAO{db: tx}).InsertOrUpdate(ctx, do)
}
func (d *SavedDialogsDAO) UpdateUserUnPinnedTx(ctx context.Context, tx DB, userID int64) (int64, error) {
	return (&SavedDialogsDAO{db: tx}).UpdateUserUnPinned(ctx, userID)
}
func (d *SavedDialogsDAO) UpdateUserPeerPinnedTx(ctx context.Context, tx DB, pinned, userID int64, peerType int32, peerID int64) (int64, error) {
	return (&SavedDialogsDAO{db: tx}).UpdateUserPeerPinned(ctx, pinned, userID, peerType, peerID)
}
