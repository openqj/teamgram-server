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

func (s *Service) StoriesCanSendStory30EB63F0(ctx context.Context, request *mtproto.TLStoriesCanSendStory30EB63F0) (*mtproto.Stories_CanSendStoryCount, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("StoriesCanSendStory30EB63F0 - request: %s", request)
	r, err := c.StoriesCanSendStory30EB63F0(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("StoriesCanSendStory30EB63F0 - reply: %s", r)
	return r, nil
}

func (s *Service) StoriesSendStory(ctx context.Context, request *mtproto.TLStoriesSendStory) (*mtproto.Updates, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("StoriesSendStory - request: %s", request)
	r, err := c.StoriesSendStory(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("StoriesSendStory - reply: %s", r)
	return r, nil
}

func (s *Service) StoriesEditStory(ctx context.Context, request *mtproto.TLStoriesEditStory) (*mtproto.Updates, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("StoriesEditStory - request: %s", request)
	r, err := c.StoriesEditStory(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("StoriesEditStory - reply: %s", r)
	return r, nil
}

func (s *Service) StoriesDeleteStories(ctx context.Context, request *mtproto.TLStoriesDeleteStories) (*mtproto.Vector_Int, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("StoriesDeleteStories - request: %s", request)
	r, err := c.StoriesDeleteStories(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("StoriesDeleteStories - reply: %s", r)
	return r, nil
}

func (s *Service) StoriesTogglePinned(ctx context.Context, request *mtproto.TLStoriesTogglePinned) (*mtproto.Vector_Int, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("StoriesTogglePinned - request: %s", request)
	r, err := c.StoriesTogglePinned(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("StoriesTogglePinned - reply: %s", r)
	return r, nil
}

func (s *Service) StoriesGetAllStories(ctx context.Context, request *mtproto.TLStoriesGetAllStories) (*mtproto.Stories_AllStories, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("StoriesGetAllStories - request: %s", request)
	r, err := c.StoriesGetAllStories(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("StoriesGetAllStories - reply: %s", r)
	return r, nil
}

func (s *Service) StoriesGetPinnedStories(ctx context.Context, request *mtproto.TLStoriesGetPinnedStories) (*mtproto.Stories_Stories, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("StoriesGetPinnedStories - request: %s", request)
	r, err := c.StoriesGetPinnedStories(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("StoriesGetPinnedStories - reply: %s", r)
	return r, nil
}

func (s *Service) StoriesGetStoriesArchive(ctx context.Context, request *mtproto.TLStoriesGetStoriesArchive) (*mtproto.Stories_Stories, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("StoriesGetStoriesArchive - request: %s", request)
	r, err := c.StoriesGetStoriesArchive(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("StoriesGetStoriesArchive - reply: %s", r)
	return r, nil
}

func (s *Service) StoriesGetStoriesByID(ctx context.Context, request *mtproto.TLStoriesGetStoriesByID) (*mtproto.Stories_Stories, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("StoriesGetStoriesByID - request: %s", request)
	r, err := c.StoriesGetStoriesByID(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("StoriesGetStoriesByID - reply: %s", r)
	return r, nil
}

func (s *Service) StoriesToggleAllStoriesHidden(ctx context.Context, request *mtproto.TLStoriesToggleAllStoriesHidden) (*mtproto.Bool, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("StoriesToggleAllStoriesHidden - request: %s", request)
	r, err := c.StoriesToggleAllStoriesHidden(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("StoriesToggleAllStoriesHidden - reply: %s", r)
	return r, nil
}

func (s *Service) StoriesReadStories(ctx context.Context, request *mtproto.TLStoriesReadStories) (*mtproto.Vector_Int, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("StoriesReadStories - request: %s", request)
	r, err := c.StoriesReadStories(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("StoriesReadStories - reply: %s", r)
	return r, nil
}

func (s *Service) StoriesIncrementStoryViews(ctx context.Context, request *mtproto.TLStoriesIncrementStoryViews) (*mtproto.Bool, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("StoriesIncrementStoryViews - request: %s", request)
	r, err := c.StoriesIncrementStoryViews(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("StoriesIncrementStoryViews - reply: %s", r)
	return r, nil
}

func (s *Service) StoriesGetStoryViewsList(ctx context.Context, request *mtproto.TLStoriesGetStoryViewsList) (*mtproto.Stories_StoryViewsList, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("StoriesGetStoryViewsList - request: %s", request)
	r, err := c.StoriesGetStoryViewsList(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("StoriesGetStoryViewsList - reply: %s", r)
	return r, nil
}

func (s *Service) StoriesGetStoriesViews(ctx context.Context, request *mtproto.TLStoriesGetStoriesViews) (*mtproto.Stories_StoryViews, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("StoriesGetStoriesViews - request: %s", request)
	r, err := c.StoriesGetStoriesViews(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("StoriesGetStoriesViews - reply: %s", r)
	return r, nil
}

func (s *Service) StoriesExportStoryLink(ctx context.Context, request *mtproto.TLStoriesExportStoryLink) (*mtproto.ExportedStoryLink, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("StoriesExportStoryLink - request: %s", request)
	r, err := c.StoriesExportStoryLink(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("StoriesExportStoryLink - reply: %s", r)
	return r, nil
}

func (s *Service) StoriesReport19D8EB45(ctx context.Context, request *mtproto.TLStoriesReport19D8EB45) (*mtproto.ReportResult, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("StoriesReport19D8EB45 - request: %s", request)
	r, err := c.StoriesReport19D8EB45(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("StoriesReport19D8EB45 - reply: %s", r)
	return r, nil
}

func (s *Service) StoriesActivateStealthMode(ctx context.Context, request *mtproto.TLStoriesActivateStealthMode) (*mtproto.Updates, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("StoriesActivateStealthMode - request: %s", request)
	r, err := c.StoriesActivateStealthMode(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("StoriesActivateStealthMode - reply: %s", r)
	return r, nil
}

func (s *Service) StoriesSendReaction(ctx context.Context, request *mtproto.TLStoriesSendReaction) (*mtproto.Updates, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("StoriesSendReaction - request: %s", request)
	r, err := c.StoriesSendReaction(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("StoriesSendReaction - reply: %s", r)
	return r, nil
}

func (s *Service) StoriesGetPeerStories(ctx context.Context, request *mtproto.TLStoriesGetPeerStories) (*mtproto.Stories_PeerStories, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("StoriesGetPeerStories - request: %s", request)
	r, err := c.StoriesGetPeerStories(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("StoriesGetPeerStories - reply: %s", r)
	return r, nil
}

func (s *Service) StoriesGetAllReadPeerStories(ctx context.Context, request *mtproto.TLStoriesGetAllReadPeerStories) (*mtproto.Updates, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("StoriesGetAllReadPeerStories - request: %s", request)
	r, err := c.StoriesGetAllReadPeerStories(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("StoriesGetAllReadPeerStories - reply: %s", r)
	return r, nil
}

func (s *Service) StoriesGetPeerMaxIDs78499170(ctx context.Context, request *mtproto.TLStoriesGetPeerMaxIDs78499170) (*mtproto.Vector_RecentStory, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("StoriesGetPeerMaxIDs78499170 - request: %s", request)
	r, err := c.StoriesGetPeerMaxIDs78499170(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("StoriesGetPeerMaxIDs78499170 - reply: %s", r)
	return r, nil
}

func (s *Service) StoriesGetChatsToSend(ctx context.Context, request *mtproto.TLStoriesGetChatsToSend) (*mtproto.Messages_Chats, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("StoriesGetChatsToSend - request: %s", request)
	r, err := c.StoriesGetChatsToSend(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("StoriesGetChatsToSend - reply: %s", r)
	return r, nil
}

func (s *Service) StoriesTogglePeerStoriesHidden(ctx context.Context, request *mtproto.TLStoriesTogglePeerStoriesHidden) (*mtproto.Bool, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("StoriesTogglePeerStoriesHidden - request: %s", request)
	r, err := c.StoriesTogglePeerStoriesHidden(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("StoriesTogglePeerStoriesHidden - reply: %s", r)
	return r, nil
}

func (s *Service) StoriesGetStoryReactionsList(ctx context.Context, request *mtproto.TLStoriesGetStoryReactionsList) (*mtproto.Stories_StoryReactionsList, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("StoriesGetStoryReactionsList - request: %s", request)
	r, err := c.StoriesGetStoryReactionsList(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("StoriesGetStoryReactionsList - reply: %s", r)
	return r, nil
}

func (s *Service) StoriesTogglePinnedToTop(ctx context.Context, request *mtproto.TLStoriesTogglePinnedToTop) (*mtproto.Bool, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("StoriesTogglePinnedToTop - request: %s", request)
	r, err := c.StoriesTogglePinnedToTop(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("StoriesTogglePinnedToTop - reply: %s", r)
	return r, nil
}

func (s *Service) StoriesSearchPosts(ctx context.Context, request *mtproto.TLStoriesSearchPosts) (*mtproto.Stories_FoundStories, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("StoriesSearchPosts - request: %s", request)
	r, err := c.StoriesSearchPosts(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("StoriesSearchPosts - reply: %s", r)
	return r, nil
}

func (s *Service) StoriesCreateAlbum(ctx context.Context, request *mtproto.TLStoriesCreateAlbum) (*mtproto.StoryAlbum, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("StoriesCreateAlbum - request: %s", request)
	r, err := c.StoriesCreateAlbum(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("StoriesCreateAlbum - reply: %s", r)
	return r, nil
}

func (s *Service) StoriesUpdateAlbum(ctx context.Context, request *mtproto.TLStoriesUpdateAlbum) (*mtproto.StoryAlbum, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("StoriesUpdateAlbum - request: %s", request)
	r, err := c.StoriesUpdateAlbum(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("StoriesUpdateAlbum - reply: %s", r)
	return r, nil
}

func (s *Service) StoriesReorderAlbums(ctx context.Context, request *mtproto.TLStoriesReorderAlbums) (*mtproto.Bool, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("StoriesReorderAlbums - request: %s", request)
	r, err := c.StoriesReorderAlbums(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("StoriesReorderAlbums - reply: %s", r)
	return r, nil
}

func (s *Service) StoriesDeleteAlbum(ctx context.Context, request *mtproto.TLStoriesDeleteAlbum) (*mtproto.Bool, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("StoriesDeleteAlbum - request: %s", request)
	r, err := c.StoriesDeleteAlbum(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("StoriesDeleteAlbum - reply: %s", r)
	return r, nil
}

func (s *Service) StoriesGetAlbums(ctx context.Context, request *mtproto.TLStoriesGetAlbums) (*mtproto.Stories_Albums, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("StoriesGetAlbums - request: %s", request)
	r, err := c.StoriesGetAlbums(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("StoriesGetAlbums - reply: %s", r)
	return r, nil
}

func (s *Service) StoriesGetAlbumStories(ctx context.Context, request *mtproto.TLStoriesGetAlbumStories) (*mtproto.Stories_Stories, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("StoriesGetAlbumStories - request: %s", request)
	r, err := c.StoriesGetAlbumStories(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("StoriesGetAlbumStories - reply: %s", r)
	return r, nil
}

func (s *Service) StoriesStartLive(ctx context.Context, request *mtproto.TLStoriesStartLive) (*mtproto.Updates, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("StoriesStartLive - request: %s", request)
	r, err := c.StoriesStartLive(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("StoriesStartLive - reply: %s", r)
	return r, nil
}

func (s *Service) StoriesGetPeerMaxIDs535983C3(ctx context.Context, request *mtproto.TLStoriesGetPeerMaxIDs535983C3) (*mtproto.Vector_Int, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("StoriesGetPeerMaxIDs535983C3 - request: %s", request)
	r, err := c.StoriesGetPeerMaxIDs535983C3(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("StoriesGetPeerMaxIDs535983C3 - reply: %s", r)
	return r, nil
}

func (s *Service) StoriesCanSendStoryC7DFDFDD(ctx context.Context, request *mtproto.TLStoriesCanSendStoryC7DFDFDD) (*mtproto.Bool, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("StoriesCanSendStoryC7DFDFDD - request: %s", request)
	r, err := c.StoriesCanSendStoryC7DFDFDD(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("StoriesCanSendStoryC7DFDFDD - reply: %s", r)
	return r, nil
}

func (s *Service) StoriesReport1923FA8C(ctx context.Context, request *mtproto.TLStoriesReport1923FA8C) (*mtproto.Bool, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("StoriesReport1923FA8C - request: %s", request)
	r, err := c.StoriesReport1923FA8C(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("StoriesReport1923FA8C - reply: %s", r)
	return r, nil
}

func (s *Service) UsersGetStoriesMaxIDs(ctx context.Context, request *mtproto.TLUsersGetStoriesMaxIDs) (*mtproto.Vector_Int, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("UsersGetStoriesMaxIDs - request: %s", request)
	r, err := c.UsersGetStoriesMaxIDs(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("UsersGetStoriesMaxIDs - reply: %s", r)
	return r, nil
}

func (s *Service) ContactsToggleStoriesHidden(ctx context.Context, request *mtproto.TLContactsToggleStoriesHidden) (*mtproto.Bool, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("ContactsToggleStoriesHidden - request: %s", request)
	r, err := c.ContactsToggleStoriesHidden(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("ContactsToggleStoriesHidden - reply: %s", r)
	return r, nil
}

func (s *Service) StoriesCanSendStoryB100D45D(ctx context.Context, request *mtproto.TLStoriesCanSendStoryB100D45D) (*mtproto.Bool, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("StoriesCanSendStoryB100D45D - request: %s", request)
	r, err := c.StoriesCanSendStoryB100D45D(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("StoriesCanSendStoryB100D45D - reply: %s", r)
	return r, nil
}

func (s *Service) StoriesGetUserStories(ctx context.Context, request *mtproto.TLStoriesGetUserStories) (*mtproto.Stories_UserStories, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("StoriesGetUserStories - request: %s", request)
	r, err := c.StoriesGetUserStories(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("StoriesGetUserStories - reply: %s", r)
	return r, nil
}

func (s *Service) StoriesGetAllReadUserStories(ctx context.Context, request *mtproto.TLStoriesGetAllReadUserStories) (*mtproto.Updates, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("StoriesGetAllReadUserStories - request: %s", request)
	r, err := c.StoriesGetAllReadUserStories(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("StoriesGetAllReadUserStories - reply: %s", r)
	return r, nil
}

func (s *Service) StoriesReportC95BE06A(ctx context.Context, request *mtproto.TLStoriesReportC95BE06A) (*mtproto.Bool, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("StoriesReportC95BE06A - request: %s", request)
	r, err := c.StoriesReportC95BE06A(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("StoriesReportC95BE06A - reply: %s", r)
	return r, nil
}
