// Copyright 2022 Teamgram Authors
//  All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//   http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//
// Author: teamgramio (teamgram.io@gmail.com)
//

package core

import (
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/chat/chat"
	"github.com/teamgram/teamgram-server/app/service/biz/chat/internal/dal/dataobject"
)

// ChatHideChatJoinRequests
// chat.hideChatJoinRequests flags:# self_id:long chat_id:long approved:flags.0?true link:flags.1?string user_id:flags.2?long = RecentChatInviteRequesters;
func (c *ChatCore) ChatHideChatJoinRequests(in *chat.TLChatHideChatJoinRequests) (*chat.RecentChatInviteRequesters, error) {
	selfID, err := c.requireInviteSelf(in.GetSelfId())
	if err != nil {
		return nil, err
	}
	if in.GetChatId() <= 0 {
		return nil, mtproto.ErrChatIdInvalid
	}
	if link := in.GetLink(); link != nil && link.GetValue() == "" {
		return nil, mtproto.ErrInviteHashInvalid
	}
	if in.GetUserId() == nil {
		c.Logger.Errorf("chat.hideChatJoinRequests - error: method ChatHideChatJoinRequests not impl")
		return nil, mtproto.ErrMethodNotImpl
	}

	joinId := in.GetUserId().GetValue()
	if joinId <= 0 {
		return nil, mtproto.ErrUserIdInvalid
	}
	linkHash := ""
	var requests []dataobject.ChatInviteParticipantsDO
	if link := in.GetLink(); link != nil {
		linkHash = chat.GetInviteHashByLink(link.GetValue())
		if linkHash == "" {
			return nil, mtproto.ErrInviteHashInvalid
		}
		if _, err = c.requireInviteLinkPermission(in.GetChatId(), selfID, link.GetValue()); err != nil {
			return nil, err
		}
		if c.svcCtx.Dao.Postgres != nil && c.svcCtx.Dao.Postgres.Store != nil {
			requests, err = c.svcCtx.Dao.Postgres.Store.InviteParticipants.SelectListByLink(c.ctx, linkHash, 1)
		} else {
			requests, err = c.svcCtx.Dao.ChatInviteParticipantsDAO.SelectListByLink(c.ctx, linkHash, 1)
		}
		if err != nil {
			return nil, err
		}
	} else {
		if _, err = c.requireInvitePermission(in.GetChatId(), selfID, 0); err != nil {
			return nil, err
		}
		if c.svcCtx.Dao.Postgres != nil && c.svcCtx.Dao.Postgres.Store != nil {
			requests, err = c.svcCtx.Dao.Postgres.Store.InviteParticipants.SelectRecentRequestedList(c.ctx, in.GetChatId())
		} else {
			requests, err = c.svcCtx.Dao.ChatInviteParticipantsDAO.SelectRecentRequestedList(c.ctx, in.GetChatId())
		}
		if err != nil {
			return nil, err
		}
	}

	matched := false
	for i := range requests {
		if requests[i].ChatId == in.ChatId && requests[i].UserId == joinId && requests[i].Requested {
			matched = true
			break
		}
	}
	if !matched {
		return c.recentChatInviteRequesters(in.ChatId, linkHash)
	}

	if in.GetApproved() {
		_, err = c.ChatAddChatUser(&chat.TLChatAddChatUser{
			ChatId:    in.ChatId,
			InviterId: selfID,
			UserId:    joinId,
		})
		if err != nil {
			c.Logger.Errorf("chat.importChatInvite - error: %v", err)
			return nil, err
		}
		if linkHash == "" {
			var updateErr error
			if c.svcCtx.Dao.Postgres != nil && c.svcCtx.Dao.Postgres.Store != nil {
				tx, txErr := c.svcCtx.Dao.Postgres.Pool.Begin(c.ctx)
				if txErr == nil {
					defer tx.Rollback(c.ctx)
					_, txErr = c.svcCtx.Dao.Postgres.Store.InviteParticipants.UpdateApprovedByOn(c.ctx, tx, selfID, in.ChatId, joinId)
					if txErr == nil {
						txErr = tx.Commit(c.ctx)
					}
				}
				updateErr = txErr
			} else {
				_, updateErr = c.svcCtx.Dao.ChatInviteParticipantsDAO.UpdateApprovedBy(c.ctx, selfID, in.ChatId, joinId)
			}
			if updateErr != nil {
				return nil, updateErr
			}
		} else if c.svcCtx.Dao.Postgres != nil && c.svcCtx.Dao.Postgres.Store != nil {
			tx, txErr := c.svcCtx.Dao.Postgres.Pool.Begin(c.ctx)
			if txErr == nil {
				defer tx.Rollback(c.ctx)
				_, txErr = c.svcCtx.Dao.Postgres.Store.InviteParticipants.UpdateApprovedByLinkOn(c.ctx, tx, selfID, in.ChatId, joinId, linkHash)
				if txErr == nil {
					txErr = tx.Commit(c.ctx)
				}
			}
			if err = txErr; err != nil {
				return nil, err
			}
		} else if _, err := c.svcCtx.Dao.ChatInviteParticipantsDAO.UpdateApprovedByLink(c.ctx, selfID, in.ChatId, joinId, linkHash); err != nil {
			return nil, err
		}
	} else {
		if linkHash == "" {
			var deleteErr error
			if c.svcCtx.Dao.Postgres != nil && c.svcCtx.Dao.Postgres.Store != nil {
				tx, txErr := c.svcCtx.Dao.Postgres.Pool.Begin(c.ctx)
				if txErr == nil {
					defer tx.Rollback(c.ctx)
					_, txErr = c.svcCtx.Dao.Postgres.Store.InviteParticipants.DeleteOn(c.ctx, tx, in.ChatId, joinId)
					if txErr == nil {
						txErr = tx.Commit(c.ctx)
					}
				}
				deleteErr = txErr
			} else {
				_, deleteErr = c.svcCtx.Dao.ChatInviteParticipantsDAO.Delete(c.ctx, in.ChatId, joinId)
			}
			if deleteErr != nil {
				return nil, deleteErr
			}
		} else if c.svcCtx.Dao.Postgres != nil && c.svcCtx.Dao.Postgres.Store != nil {
			tx, txErr := c.svcCtx.Dao.Postgres.Pool.Begin(c.ctx)
			if txErr == nil {
				defer tx.Rollback(c.ctx)
				_, txErr = c.svcCtx.Dao.Postgres.Store.InviteParticipants.DeleteByLinkOn(c.ctx, tx, in.ChatId, joinId, linkHash)
				if txErr == nil {
					txErr = tx.Commit(c.ctx)
				}
			}
			if err = txErr; err != nil {
				return nil, err
			}
		} else if _, err := c.svcCtx.Dao.ChatInviteParticipantsDAO.DeleteByLink(c.ctx, in.ChatId, joinId, linkHash); err != nil {
			return nil, err
		}
	}

	return c.recentChatInviteRequesters(in.ChatId, linkHash)
}

func (c *ChatCore) recentChatInviteRequesters(chatId int64, linkHash string) (*chat.RecentChatInviteRequesters, error) {
	var (
		requestList []dataobject.ChatInviteParticipantsDO
		err         error
	)
	if linkHash != "" {
		if c.svcCtx.Dao.Postgres != nil && c.svcCtx.Dao.Postgres.Store != nil {
			requestList, err = c.svcCtx.Dao.Postgres.Store.InviteParticipants.SelectListByLink(c.ctx, linkHash, 1)
		} else {
			requestList, err = c.svcCtx.Dao.ChatInviteParticipantsDAO.SelectListByLink(c.ctx, linkHash, 1)
		}
	} else {
		if c.svcCtx.Dao.Postgres != nil && c.svcCtx.Dao.Postgres.Store != nil {
			requestList, err = c.svcCtx.Dao.Postgres.Store.InviteParticipants.SelectRecentRequestedList(c.ctx, chatId)
		} else {
			requestList, err = c.svcCtx.Dao.ChatInviteParticipantsDAO.SelectRecentRequestedList(c.ctx, chatId)
		}
	}
	if err != nil {
		return nil, err
	}
	if linkHash != "" {
		filtered := requestList[:0]
		for _, request := range requestList {
			if request.ChatId == chatId && request.Requested {
				filtered = append(filtered, request)
			}
		}
		requestList = filtered
	}
	requesters := chat.MakeTLRecentChatInviteRequesters(&chat.RecentChatInviteRequesters{
		RequestsPending:  int32(len(requestList)),
		RecentRequesters: make([]int64, 0, len(requestList)),
	}).To_RecentChatInviteRequesters()
	for _, request := range requestList {
		requesters.RecentRequesters = append(requesters.RecentRequesters, request.UserId)
	}

	return requesters, nil
}
