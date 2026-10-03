package core

import (
	"context"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	apifullDao "github.com/teamgram/teamgram-server/app/bff/apifull/internal/dao"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/svc"
	user_client "github.com/teamgram/teamgram-server/app/service/biz/user/client"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

type accentColorUserClient struct {
	user_client.UserClient
	request *userpb.TLUserSetColor
	result  *mtproto.Bool
	err     error
}

func (c *accentColorUserClient) UserSetColor(_ context.Context, in *userpb.TLUserSetColor) (*mtproto.Bool, error) {
	c.request = in
	return c.result, c.err
}

func newAccentColorCore(userID int64) (*ApiFullCore, *accentColorUserClient) {
	client := &accentColorUserClient{result: mtproto.BoolTrue}
	return &ApiFullCore{
		ctx: context.Background(),
		MD:  &metadata.RpcMetadata{UserId: userID},
		svcCtx: &svc.ServiceContext{Dao: &apifullDao.Dao{
			UserClient: client,
		}},
	}, client
}
