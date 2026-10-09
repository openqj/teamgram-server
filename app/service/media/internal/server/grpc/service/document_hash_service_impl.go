package service

import (
	"context"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/media/internal/core"
	"github.com/teamgram/teamgram-server/app/service/media/media/hashrpc"
)

func (s *Service) GetDocumentByHash(ctx context.Context, request *hashrpc.DocumentHashRequest) (*mtproto.Document, error) {
	return core.New(ctx, s.svcCtx).GetDocumentByHash(request)
}
