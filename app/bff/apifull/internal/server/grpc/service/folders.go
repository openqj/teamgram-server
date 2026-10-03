// Copyright 2026 Teamgram Authors
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

package service

import (
	"context"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/core"
)

func (s *Service) MessagesGetDialogFiltersEFD48C89(ctx context.Context, request *mtproto.TLMessagesGetDialogFiltersEFD48C89) (*mtproto.Messages_DialogFilters, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("MessagesGetDialogFiltersEFD48C89 - request: %s", request)
	r, err := c.MessagesGetDialogFiltersEFD48C89(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("MessagesGetDialogFiltersEFD48C89 - reply: %s", r)
	return r, nil
}

func (s *Service) MessagesGetSuggestedDialogFilters(ctx context.Context, request *mtproto.TLMessagesGetSuggestedDialogFilters) (*mtproto.Vector_DialogFilterSuggested, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("MessagesGetSuggestedDialogFilters - request: %s", request)
	r, err := c.MessagesGetSuggestedDialogFilters(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("MessagesGetSuggestedDialogFilters - reply: %s", r)
	return r, nil
}

func (s *Service) MessagesUpdateDialogFilter(ctx context.Context, request *mtproto.TLMessagesUpdateDialogFilter) (*mtproto.Bool, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("MessagesUpdateDialogFilter - request: %s", request)
	r, err := c.MessagesUpdateDialogFilter(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("MessagesUpdateDialogFilter - reply: %s", r)
	return r, nil
}

func (s *Service) MessagesUpdateDialogFiltersOrder(ctx context.Context, request *mtproto.TLMessagesUpdateDialogFiltersOrder) (*mtproto.Bool, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("MessagesUpdateDialogFiltersOrder - request: %s", request)
	r, err := c.MessagesUpdateDialogFiltersOrder(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("MessagesUpdateDialogFiltersOrder - reply: %s", r)
	return r, nil
}

func (s *Service) FoldersEditPeerFolders(ctx context.Context, request *mtproto.TLFoldersEditPeerFolders) (*mtproto.Updates, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("FoldersEditPeerFolders - request: %s", request)
	r, err := c.FoldersEditPeerFolders(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("FoldersEditPeerFolders - reply: %s", r)
	return r, nil
}

func (s *Service) ChatlistsExportChatlistInvite(ctx context.Context, request *mtproto.TLChatlistsExportChatlistInvite) (*mtproto.Chatlists_ExportedChatlistInvite, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("ChatlistsExportChatlistInvite - request: %s", request)
	r, err := c.ChatlistsExportChatlistInvite(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("ChatlistsExportChatlistInvite - reply: %s", r)
	return r, nil
}

func (s *Service) ChatlistsDeleteExportedInvite(ctx context.Context, request *mtproto.TLChatlistsDeleteExportedInvite) (*mtproto.Bool, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("ChatlistsDeleteExportedInvite - request: %s", request)
	r, err := c.ChatlistsDeleteExportedInvite(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("ChatlistsDeleteExportedInvite - reply: %s", r)
	return r, nil
}

func (s *Service) ChatlistsEditExportedInvite(ctx context.Context, request *mtproto.TLChatlistsEditExportedInvite) (*mtproto.ExportedChatlistInvite, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("ChatlistsEditExportedInvite - request: %s", request)
	r, err := c.ChatlistsEditExportedInvite(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("ChatlistsEditExportedInvite - reply: %s", r)
	return r, nil
}

func (s *Service) ChatlistsGetExportedInvites(ctx context.Context, request *mtproto.TLChatlistsGetExportedInvites) (*mtproto.Chatlists_ExportedInvites, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("ChatlistsGetExportedInvites - request: %s", request)
	r, err := c.ChatlistsGetExportedInvites(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("ChatlistsGetExportedInvites - reply: %s", r)
	return r, nil
}

func (s *Service) ChatlistsCheckChatlistInvite(ctx context.Context, request *mtproto.TLChatlistsCheckChatlistInvite) (*mtproto.Chatlists_ChatlistInvite, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("ChatlistsCheckChatlistInvite - request: %s", request)
	r, err := c.ChatlistsCheckChatlistInvite(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("ChatlistsCheckChatlistInvite - reply: %s", r)
	return r, nil
}

func (s *Service) ChatlistsJoinChatlistInvite(ctx context.Context, request *mtproto.TLChatlistsJoinChatlistInvite) (*mtproto.Updates, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("ChatlistsJoinChatlistInvite - request: %s", request)
	r, err := c.ChatlistsJoinChatlistInvite(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("ChatlistsJoinChatlistInvite - reply: %s", r)
	return r, nil
}

func (s *Service) ChatlistsGetChatlistUpdates(ctx context.Context, request *mtproto.TLChatlistsGetChatlistUpdates) (*mtproto.Chatlists_ChatlistUpdates, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("ChatlistsGetChatlistUpdates - request: %s", request)
	r, err := c.ChatlistsGetChatlistUpdates(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("ChatlistsGetChatlistUpdates - reply: %s", r)
	return r, nil
}

func (s *Service) ChatlistsJoinChatlistUpdates(ctx context.Context, request *mtproto.TLChatlistsJoinChatlistUpdates) (*mtproto.Updates, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("ChatlistsJoinChatlistUpdates - request: %s", request)
	r, err := c.ChatlistsJoinChatlistUpdates(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("ChatlistsJoinChatlistUpdates - reply: %s", r)
	return r, nil
}

func (s *Service) ChatlistsHideChatlistUpdates(ctx context.Context, request *mtproto.TLChatlistsHideChatlistUpdates) (*mtproto.Bool, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("ChatlistsHideChatlistUpdates - request: %s", request)
	r, err := c.ChatlistsHideChatlistUpdates(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("ChatlistsHideChatlistUpdates - reply: %s", r)
	return r, nil
}

func (s *Service) ChatlistsGetLeaveChatlistSuggestions(ctx context.Context, request *mtproto.TLChatlistsGetLeaveChatlistSuggestions) (*mtproto.Vector_Peer, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("ChatlistsGetLeaveChatlistSuggestions - request: %s", request)
	r, err := c.ChatlistsGetLeaveChatlistSuggestions(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("ChatlistsGetLeaveChatlistSuggestions - reply: %s", r)
	return r, nil
}

func (s *Service) ChatlistsLeaveChatlist(ctx context.Context, request *mtproto.TLChatlistsLeaveChatlist) (*mtproto.Updates, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("ChatlistsLeaveChatlist - request: %s", request)
	r, err := c.ChatlistsLeaveChatlist(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("ChatlistsLeaveChatlist - reply: %s", r)
	return r, nil
}

func (s *Service) MessagesGetDialogFiltersF19ED96D(ctx context.Context, request *mtproto.TLMessagesGetDialogFiltersF19ED96D) (*mtproto.Vector_DialogFilter, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("MessagesGetDialogFiltersF19ED96D - request: %s", request)
	r, err := c.MessagesGetDialogFiltersF19ED96D(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("MessagesGetDialogFiltersF19ED96D - reply: %s", r)
	return r, nil
}

func (s *Service) FoldersDeleteFolder(ctx context.Context, request *mtproto.TLFoldersDeleteFolder) (*mtproto.Updates, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("FoldersDeleteFolder - request: %s", request)
	r, err := c.FoldersDeleteFolder(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("FoldersDeleteFolder - reply: %s", r)
	return r, nil
}
