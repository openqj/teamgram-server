/*
 * Created from 'scheme.tl' by 'mtprotoc'
 *
 * Copyright (c) 2021-present,  Teamgram Studio (https://teamgram.io).
 *  All rights reserved.
 *
 * Author: teamgramio (teamgram.io@gmail.com)
 */

package core

import (
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/messenger/msg/inbox/inbox"
	"github.com/teamgram/teamgram-server/app/messenger/msg/msg/msg"
)

// MsgReadMessageContents
// msg.readMessageContents user_id:long auth_key_id:long peer_type:int peer_id:long id:Vector<ContentMessage> = messages.AffectedMessage;
func (c *MsgCore) MsgReadMessageContents(in *msg.TLMsgReadMessageContents) (*mtproto.Messages_AffectedMessages, error) {
	if in == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	if c.svcCtx.Dao.Postgres != nil {
		ids := make([]int32, 0, len(in.Id))
		for _, content := range in.Id {
			if content == nil || content.Id <= 0 {
				return nil, mtproto.ErrMessageIdInvalid
			}
			ids = append(ids, content.Id)
		}
		updates, pts, err := c.svcCtx.Dao.ReadMessageContentsState(c.ctx, in.UserId, mtproto.MakePeerUtil(in.PeerType, in.PeerId), ids)
		if err != nil {
			return nil, err
		}
		var count int32
		for _, update := range updates {
			count += update.PtsCount
		}
		return mtproto.MakeTLMessagesAffectedMessages(&mtproto.Messages_AffectedMessages{Pts: pts, PtsCount: count}).To_Messages_AffectedMessages(), nil
	}
	var (
		pts, ptsCount int32
	)

	affected, err := c.readMentionedMessageContents(in)
	if err != nil {
		return nil, err
	}
	ptsCount += affected
	affected, err = c.readMediaUnreadMessageContents(in)
	if err != nil {
		return nil, err
	}
	ptsCount += affected
	affected, err = c.readReactionUnreadMessageContents(in)
	if err != nil {
		return nil, err
	}
	ptsCount += affected

	if ptsCount > 0 {
		pts = c.svcCtx.Dao.IDGenClient2.NextNPtsId(c.ctx, in.UserId, int(ptsCount))
	} else {
		ptsCount = 0
		pts = c.svcCtx.Dao.IDGenClient2.CurrentPtsId(c.ctx, in.UserId)
	}

	return mtproto.MakeTLMessagesAffectedMessages(&mtproto.Messages_AffectedMessages{
		Pts:      pts,
		PtsCount: ptsCount,
	}).To_Messages_AffectedMessages(), nil
}

func (c *MsgCore) readMentionedMessageContents(in *msg.TLMsgReadMessageContents) (int32, error) {
	var (
		ptsCount int32 = 0
	)

	switch in.PeerType {
	case mtproto.PEER_USER:
		return 0, nil
	case mtproto.PEER_CHAT:
		for _, m := range in.Id {
			if m.Mentioned {
				ptsCount++
			}
		}
		if ptsCount > 0 {
			// CommonDAO.CalcSize converts query errors to a zero count. This path
			// mutates unread state, so keep the underlying database error visible.
			unreadMentions, err := c.svcCtx.Dao.CountMentionedMessages(c.ctx, in.UserId, mtproto.PEER_CHAT, in.PeerId)
			if err != nil {
				return 0, err
			}
			for _, m := range in.Id {
				if m.Mentioned {
					if _, err := c.svcCtx.Dao.UpdateMentionedAndMediaUnread(c.ctx, in.UserId, m.Id); err != nil {
						return 0, err
					}
				}
			}

			sz := int(unreadMentions) - int(ptsCount)
			if sz < 0 {
				sz = 0
			}

			if _, err := c.svcCtx.Dao.UpdateDialogCustomMap(c.ctx, map[string]interface{}{
				"unread_mentions_count": sz,
			}, in.UserId, mtproto.PEER_CHAT, in.PeerId); err != nil {
				return 0, err
			}
		}

		return ptsCount, nil
	default:
		err := mtproto.ErrPeerIdInvalid
		c.Logger.Errorf("DeleteMessages - error: %v", err)

		return 0, err
	}
}

func (c *MsgCore) readMediaUnreadMessageContents(in *msg.TLMsgReadMessageContents) (int32, error) {
	var (
		ptsCount int32 = 0
	)

	switch in.PeerType {
	case mtproto.PEER_USER:
		// id := make([]*inbox.InboxMessageId, 0, len(in.Id))
		for _, m := range in.Id {
			if m.MediaUnread {
				ptsCount++
				if _, err := c.svcCtx.Dao.UpdateMessageMediaUnread(c.ctx, in.UserId, m.Id); err != nil {
					return 0, err
				}
				if in.UserId != in.PeerId {
					if _, err := c.svcCtx.Dao.InboxClient.InboxReadMediaUnreadToInboxV2(
						c.ctx, &inbox.TLInboxReadMediaUnreadToInboxV2{
							UserId:          in.PeerId,
							PeerType:        mtproto.PEER_USER,
							PeerId:          in.UserId,
							DialogMessageId: m.DialogMessageId,
						}); err != nil {
						return 0, err
					}
				}
			}
		}

		return ptsCount, nil
	case mtproto.PEER_CHAT:
		// TODO: update sender
		for _, m := range in.Id {
			if m.MediaUnread {
				ptsCount++
				if _, err := c.svcCtx.Dao.UpdateMessageMediaUnread(c.ctx, in.UserId, m.Id); err != nil {
					return 0, err
				}
				if _, err := c.svcCtx.Dao.InboxClient.InboxReadMediaUnreadToInboxV2(
					c.ctx, &inbox.TLInboxReadMediaUnreadToInboxV2{
						UserId:          m.SendUserId,
						PeerType:        mtproto.PEER_CHAT,
						PeerId:          in.PeerId,
						DialogMessageId: m.DialogMessageId,
					}); err != nil {
					return 0, err
				}
			}
		}

		return ptsCount, nil
	default:
		err := mtproto.ErrPeerIdInvalid
		c.Logger.Errorf("DeleteMessages - error: %v", err)

		return 0, err
	}
}

func (c *MsgCore) readReactionUnreadMessageContents(in *msg.TLMsgReadMessageContents) (int32, error) {
	var (
		unreadReactionsCount int32
	)

	for _, m := range in.Id {
		if m.Reaction {
			unreadReactionsCount++
		}
	}
	for _, m := range in.Id {
		if m.Reaction {
			if c.svcCtx.MsgPlugin != nil {
				if err := c.svcCtx.MsgPlugin.ReadReactionUnreadMessage(c.ctx, in.UserId, m.Id); err != nil {
					return 0, err
				}
			} else {
				return 0, mtproto.ErrMethodNotImpl
			}
		}
	}

	if unreadReactionsCount > 0 {
		if _, err := c.svcCtx.Dao.UpdateDialogUnreadCount(c.ctx, 0, 0, -unreadReactionsCount,
			in.UserId, in.PeerType, in.PeerId); err != nil {
			return 0, err
		}
	}
	return unreadReactionsCount, nil
}
