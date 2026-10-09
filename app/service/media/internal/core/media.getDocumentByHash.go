package core

import (
	"crypto/sha256"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/media/media/hashrpc"
)

func (c *MediaCore) GetDocumentByHash(in *hashrpc.DocumentHashRequest) (*mtproto.Document, error) {
	if in == nil || len(in.GetSha256()) != sha256.Size || in.GetSize() < 0 || in.GetMimeType() == "" {
		return nil, mtproto.ErrDocumentInvalid
	}
	document, err := c.svcCtx.Dao.LookupDocumentByHash(c.ctx, in.GetSha256(), in.GetSize(), in.GetMimeType())
	if err != nil {
		return nil, err
	}
	if document == nil {
		return mtproto.MakeTLDocumentEmpty(&mtproto.Document{}).To_Document(), nil
	}
	return document, nil
}
