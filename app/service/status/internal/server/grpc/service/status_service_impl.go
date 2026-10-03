/*
 * WARNING! All changes made in this file will be lost!
 * Created from 'scheme.tl' by 'mtprotoc'
 *
 * Copyright 2022 Teamgram Authors.
 *  All rights reserved.
 *
 * Author: teamgramio (teamgram.io@gmail.com)
 */

package service

import (
	"context"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/status/internal/core"
	"github.com/teamgram/teamgram-server/app/service/status/status"
)

func validateStatusChannelID(channelID int64) error {
	if channelID <= 0 {
		return mtproto.ErrChannelInvalid
	}
	return nil
}

func validateStatusUserID(userID int64) error {
	if userID <= 0 {
		return mtproto.ErrUserIdInvalid
	}
	return nil
}

func validateStatusChannelIDs(channelIDs []int64) error {
	for _, channelID := range channelIDs {
		if err := validateStatusChannelID(channelID); err != nil {
			return err
		}
	}
	return nil
}

func validateStatusUserIDs(userIDs []int64) error {
	for _, userID := range userIDs {
		if err := validateStatusUserID(userID); err != nil {
			return err
		}
	}
	return nil
}

// StatusSetSessionOnline
// status.setSessionOnline user_id:long session:SessionEntry = Bool;
func (s *Service) StatusSetSessionOnline(ctx context.Context, request *status.TLStatusSetSessionOnline) (*mtproto.Bool, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("status.setSessionOnline - metadata: %s, request: %s", c.MD, request)

	r, err := c.StatusSetSessionOnline(request)
	if err != nil {
		return nil, err
	}

	c.Logger.Debugf("status.setSessionOnline - reply: %s", r)
	return r, err
}

// StatusSetSessionOffline
// status.setSessionOffline user_id:long auth_key_id:long = Bool;
func (s *Service) StatusSetSessionOffline(ctx context.Context, request *status.TLStatusSetSessionOffline) (*mtproto.Bool, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("status.setSessionOffline - metadata: %s, request: %s", c.MD, request)

	r, err := c.StatusSetSessionOffline(request)
	if err != nil {
		return nil, err
	}

	c.Logger.Debugf("status.setSessionOffline - reply: %s", r)
	return r, err
}

// StatusGetUserOnlineSessions
// status.getUserOnlineSessions user_id:long = UserSessionEntryList;
func (s *Service) StatusGetUserOnlineSessions(ctx context.Context, request *status.TLStatusGetUserOnlineSessions) (*status.UserSessionEntryList, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("status.getUserOnlineSessions - metadata: %s, request: %s", c.MD, request)

	r, err := c.StatusGetUserOnlineSessions(request)
	if err != nil {
		return nil, err
	}

	c.Logger.Debugf("status.getUserOnlineSessions - reply: %s", r)
	return r, err
}

// StatusGetUsersOnlineSessionsList
// status.getUsersOnlineSessionsList users:Vector<long> = Vector<UserSessionEntryList>;
func (s *Service) StatusGetUsersOnlineSessionsList(ctx context.Context, request *status.TLStatusGetUsersOnlineSessionsList) (*status.Vector_UserSessionEntryList, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("status.getUsersOnlineSessionsList - metadata: %s, request: %s", c.MD, request)

	r, err := c.StatusGetUsersOnlineSessionsList(request)
	if err != nil {
		return nil, err
	}

	c.Logger.Debugf("status.getUsersOnlineSessionsList - reply: %s", r)
	return r, err
}

// StatusGetChannelOnlineUsers
// status.getChannelOnlineUsers channel_id:long = Vector<long>;
func (s *Service) StatusGetChannelOnlineUsers(ctx context.Context, request *status.TLStatusGetChannelOnlineUsers) (*status.Vector_Long, error) {
	if err := validateStatusChannelID(request.GetChannelId()); err != nil {
		return nil, err
	}

	return nil, mtproto.ErrMethodNotImpl
}

// StatusSetUserChannelsOnline
// status.setUserChannelsOnline user_id:long channels:Vector<long> = Bool;
func (s *Service) StatusSetUserChannelsOnline(ctx context.Context, request *status.TLStatusSetUserChannelsOnline) (*mtproto.Bool, error) {
	if err := validateStatusUserID(request.GetUserId()); err != nil {
		return nil, err
	}
	if err := validateStatusChannelIDs(request.GetChannels()); err != nil {
		return nil, err
	}

	return nil, mtproto.ErrMethodNotImpl
}

// StatusSetUserChannelsOffline
// status.setUserChannelsOffline user_id:long channels:Vector<long> = Bool;
func (s *Service) StatusSetUserChannelsOffline(ctx context.Context, request *status.TLStatusSetUserChannelsOffline) (*mtproto.Bool, error) {
	if err := validateStatusUserID(request.GetUserId()); err != nil {
		return nil, err
	}
	if err := validateStatusChannelIDs(request.GetChannels()); err != nil {
		return nil, err
	}

	return nil, mtproto.ErrMethodNotImpl
}

// StatusSetChannelUserOffline
// status.setChannelUserOffline channel_id:long user_id:long = Bool;
func (s *Service) StatusSetChannelUserOffline(ctx context.Context, request *status.TLStatusSetChannelUserOffline) (*mtproto.Bool, error) {
	if err := validateStatusChannelID(request.GetChannelId()); err != nil {
		return nil, err
	}
	if err := validateStatusUserID(request.GetUserId()); err != nil {
		return nil, err
	}

	return nil, mtproto.ErrMethodNotImpl
}

// StatusSetChannelUsersOnline
// status.setChannelUsersOnline channel_id:long user_id:long = Bool;
func (s *Service) StatusSetChannelUsersOnline(ctx context.Context, request *status.TLStatusSetChannelUsersOnline) (*mtproto.Bool, error) {
	if err := validateStatusChannelID(request.GetChannelId()); err != nil {
		return nil, err
	}
	if err := validateStatusUserIDs(request.GetId()); err != nil {
		return nil, err
	}

	return nil, mtproto.ErrMethodNotImpl
}

// StatusSetChannelOffline
// status.setChannelOffline channel_id:long = Bool;
func (s *Service) StatusSetChannelOffline(ctx context.Context, request *status.TLStatusSetChannelOffline) (*mtproto.Bool, error) {
	if err := validateStatusChannelID(request.GetChannelId()); err != nil {
		return nil, err
	}

	return nil, mtproto.ErrMethodNotImpl
}
