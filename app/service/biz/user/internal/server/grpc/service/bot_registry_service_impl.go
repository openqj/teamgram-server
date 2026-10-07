package service

import (
	"context"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/user/internal/core"
	"github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

func (s *Service) GetCreatedBots(ctx context.Context, request *user.BotRegistryGetCreatedBotsRequest) (*user.Vector_ImmutableUser, error) {
	c := core.New(ctx, s.svcCtx)
	return c.UserGetCreatedBots(request)
}

func (s *Service) GetAdminedBots(ctx context.Context, request *user.BotRegistryGetAdminedBotsRequest) (*user.Vector_ImmutableUser, error) {
	c := core.New(ctx, s.svcCtx)
	return c.UserGetAdminedBots(request)
}

func (s *Service) CreateBot(ctx context.Context, request *user.BotRegistryCreateBotRequest) (*mtproto.ImmutableUser, error) {
	c := core.New(ctx, s.svcCtx)
	return c.UserCreateManagedBot(request)
}
