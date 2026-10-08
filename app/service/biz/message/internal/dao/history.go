// Copyright (c) 2021-present,  Teamgram Studio (https://teamgram.io).
//  All rights reserved.
//
// Author: teamgramio (teamgram.io@gmail.com)
//

package dao

import (
	"context"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/message/internal/dal/dataobject"

	"github.com/zeromicro/go-zero/core/logx"
)

// GetOffsetIdBackwardHistoryMessages offset
func (d *Dao) GetOffsetIdBackwardHistoryMessages(ctx context.Context, userId int64, peer *mtproto.PeerUtil, offsetId, minId, maxId, limit int32, hash int64) (messages []*mtproto.MessageBox, err error) {
	if peer == nil {
		return nil, mtproto.ErrPeerIdInvalid
	}
	switch peer.PeerType {
	case mtproto.PEER_SELF, mtproto.PEER_USER, mtproto.PEER_CHAT:
		var (
			did = mtproto.MakeDialogId(userId, peer.PeerType, peer.PeerId)
		)

		if d.Postgres != nil && d.Postgres.Store != nil && d.Postgres.Store.Messages != nil {
			var list []dataobject.MessagesDO
			list, err = d.SelectBackwardHistory(ctx, userId, did.A, did.B, offsetId, limit)
			for i := range list {
				messages = append(messages, d.MakeMessageBox(ctx, userId, &list[i]))
			}
		} else {
			_, err = d.MessagesDAO.SelectBackwardByOffsetIdLimitWithCB(ctx, userId, did.A, did.B, offsetId, limit, func(sz, i int, v *dataobject.MessagesDO) {
				messages = append(messages, d.MakeMessageBox(ctx, userId, v))
			})
		}
	case mtproto.PEER_CHANNEL:
		logx.Errorf("blocked, License key from https://teamgram.net required to unlock enterprise features.")
		return nil, mtproto.ErrEnterpriseIsBlocked
	default:
		return nil, mtproto.ErrPeerIdInvalid
	}

	if err != nil {
		return nil, err
	}
	return mtproto.ToSafeMessageBoxList(messages), nil
}

func (d *Dao) GetOffsetIdForwardHistoryMessages(ctx context.Context, userId int64, peer *mtproto.PeerUtil, offsetId, minId, maxId, limit int32, hash int64) (messages []*mtproto.MessageBox, err error) {
	if peer == nil {
		return nil, mtproto.ErrPeerIdInvalid
	}
	switch peer.PeerType {
	case mtproto.PEER_SELF, mtproto.PEER_USER, mtproto.PEER_CHAT:
		var (
			did = mtproto.MakeDialogId(userId, peer.PeerType, peer.PeerId)
		)

		if d.Postgres != nil && d.Postgres.Store != nil && d.Postgres.Store.Messages != nil {
			var list []dataobject.MessagesDO
			list, err = d.SelectForwardHistory(ctx, userId, did.A, did.B, offsetId, limit)
			for i := range list {
				messages = append(messages, d.MakeMessageBox(ctx, userId, &list[i]))
			}
		} else {
			_, err = d.MessagesDAO.SelectForwardByOffsetIdLimitWithCB(ctx, userId, did.A, did.B, offsetId, limit, func(sz, i int, v *dataobject.MessagesDO) {
				messages = append(messages, d.MakeMessageBox(ctx, userId, v))
			})
		}
	case mtproto.PEER_CHANNEL:
		logx.Errorf("blocked, License key from https://teamgram.net required to unlock enterprise features.")
		return nil, mtproto.ErrEnterpriseIsBlocked
	default:
		return nil, mtproto.ErrPeerIdInvalid
	}

	if err != nil {
		return nil, err
	}
	return mtproto.ToSafeMessageBoxList(messages), nil
}

func (d *Dao) GetOffsetDateBackwardHistoryMessages(ctx context.Context, userId int64, peer *mtproto.PeerUtil, offsetDate, minId, maxId, limit int32, hash int64) (messages []*mtproto.MessageBox, err error) {
	if peer == nil {
		return nil, mtproto.ErrPeerIdInvalid
	}
	switch peer.PeerType {
	case mtproto.PEER_SELF, mtproto.PEER_USER, mtproto.PEER_CHAT:
		var (
			did = mtproto.MakeDialogId(userId, peer.PeerType, peer.PeerId)
		)

		if d.Postgres != nil && d.Postgres.Store != nil && d.Postgres.Store.Messages != nil {
			var list []dataobject.MessagesDO
			list, err = d.SelectBackwardHistoryByDate(ctx, userId, did.A, did.B, int64(offsetDate), limit)
			for i := range list {
				messages = append(messages, d.MakeMessageBox(ctx, userId, &list[i]))
			}
		} else {
			_, err = d.MessagesDAO.SelectBackwardByOffsetDateLimitWithCB(ctx, userId, did.A, did.B, int64(offsetDate), limit, func(sz, i int, v *dataobject.MessagesDO) {
				messages = append(messages, d.MakeMessageBox(ctx, userId, v))
			})
		}
	case mtproto.PEER_CHANNEL:
		logx.Errorf("blocked, License key from https://teamgram.net required to unlock enterprise features.")
		return nil, mtproto.ErrEnterpriseIsBlocked
	default:
		return nil, mtproto.ErrPeerIdInvalid
	}

	if err != nil {
		return nil, err
	}
	return mtproto.ToSafeMessageBoxList(messages), nil
}

func (d *Dao) GetOffsetDateForwardHistoryMessages(ctx context.Context, userId int64, peer *mtproto.PeerUtil, offsetDate, minId, maxId, limit int32, hash int64) (messages []*mtproto.MessageBox, err error) {
	if peer == nil {
		return nil, mtproto.ErrPeerIdInvalid
	}
	switch peer.PeerType {
	case mtproto.PEER_SELF, mtproto.PEER_USER, mtproto.PEER_CHAT:
		var (
			did = mtproto.MakeDialogId(userId, peer.PeerType, peer.PeerId)
		)

		if d.Postgres != nil && d.Postgres.Store != nil && d.Postgres.Store.Messages != nil {
			var list []dataobject.MessagesDO
			list, err = d.SelectForwardHistoryByDate(ctx, userId, did.A, did.B, int64(offsetDate), limit)
			for i := range list {
				messages = append(messages, d.MakeMessageBox(ctx, userId, &list[i]))
			}
		} else {
			_, err = d.MessagesDAO.SelectForwardByOffsetDateLimitWithCB(ctx, userId, did.A, did.B, int64(offsetDate), limit, func(sz, i int, v *dataobject.MessagesDO) {
				messages = append(messages, d.MakeMessageBox(ctx, userId, v))
			})
		}
	case mtproto.PEER_CHANNEL:
		logx.Errorf("blocked, License key from https://teamgram.net required to unlock enterprise features.")
		return nil, mtproto.ErrEnterpriseIsBlocked
	default:
		return nil, mtproto.ErrPeerIdInvalid
	}

	if err != nil {
		return nil, err
	}
	return mtproto.ToSafeMessageBoxList(messages), nil
}

// GetOffsetIdBackwardUnreadMentions GetOffsetIdBackwardUnreadMentions
func (d *Dao) GetOffsetIdBackwardUnreadMentions(ctx context.Context, userId int64, peer *mtproto.PeerUtil, offsetId, minId, maxId, limit int32) (messages []*mtproto.MessageBox, err error) {
	if peer == nil {
		return nil, mtproto.ErrPeerIdInvalid
	}
	switch peer.PeerType {
	case mtproto.PEER_CHAT:
		var (
			did = mtproto.MakeDialogId(userId, peer.PeerType, peer.PeerId)
		)

		if d.Postgres != nil && d.Postgres.Store != nil && d.Postgres.Store.Messages != nil {
			var list []dataobject.MessagesDO
			list, err = d.Postgres.Store.Messages.SelectBackwardUnreadMentionsByOffsetIdLimit(ctx, userId, did.A, did.B, offsetId, minId, maxId, limit)
			for i := range list {
				messages = append(messages, d.MakeMessageBox(ctx, userId, &list[i]))
			}
		} else {
			_, err = d.MessagesDAO.SelectBackwardUnreadMentionsByOffsetIdLimitWithCB(ctx, userId, did.A, did.B, offsetId, minId, maxId, limit, func(sz, i int, v *dataobject.MessagesDO) {
				messages = append(messages, d.MakeMessageBox(ctx, userId, v))
			})
		}
	case mtproto.PEER_CHANNEL:
		return nil, mtproto.ErrEnterpriseIsBlocked
	default:
		return nil, mtproto.ErrPeerIdInvalid
	}
	if err != nil {
		return nil, err
	}
	return mtproto.ToSafeMessageBoxList(messages), nil
}

func (d *Dao) GetOffsetIdForwardUnreadMentions(ctx context.Context, userId int64, peer *mtproto.PeerUtil, offsetId, minId, maxId, limit int32) (messages []*mtproto.MessageBox, err error) {
	if peer == nil {
		return nil, mtproto.ErrPeerIdInvalid
	}
	switch peer.PeerType {
	case mtproto.PEER_CHAT:
		var (
			did = mtproto.MakeDialogId(userId, peer.PeerType, peer.PeerId)
		)

		if d.Postgres != nil && d.Postgres.Store != nil && d.Postgres.Store.Messages != nil {
			var list []dataobject.MessagesDO
			list, err = d.Postgres.Store.Messages.SelectForwardUnreadMentionsByOffsetIdLimit(ctx, userId, did.A, did.B, offsetId, minId, maxId, limit)
			for i := range list {
				messages = append(messages, d.MakeMessageBox(ctx, userId, &list[i]))
			}
		} else {
			_, err = d.MessagesDAO.SelectForwardUnreadMentionsByOffsetIdLimitWithCB(ctx, userId, did.A, did.B, offsetId, minId, maxId, limit, func(sz, i int, v *dataobject.MessagesDO) {
				messages = append(messages, d.MakeMessageBox(ctx, userId, v))
			})
		}
	case mtproto.PEER_CHANNEL:
		return nil, mtproto.ErrEnterpriseIsBlocked
	default:
		return nil, mtproto.ErrPeerIdInvalid
	}
	if err != nil {
		return nil, err
	}
	return mtproto.ToSafeMessageBoxList(messages), nil
}
