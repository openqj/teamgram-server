package dao

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/teamgram/marmota/pkg/net/rpcx"
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/apifull/layer229"
	"github.com/teamgram/teamgram-server/app/interface/session/internal/config"
	"github.com/teamgram/teamgram-server/app/service/authsession/authsession"
	authsession_client "github.com/teamgram/teamgram-server/app/service/authsession/client"
	user_client "github.com/teamgram/teamgram-server/app/service/biz/user/client"
	"github.com/teamgram/teamgram-server/app/service/biz/user/user"
	"github.com/zeromicro/go-zero/core/logx"
)

type rpcUsers struct {
	users user_client.UserClient
	auth  authsession_client.AuthsessionClient
}

func (r rpcUsers) CreateNewUser(ctx context.Context, authKeyID int64, phone, country, first, last string) (*mtproto.User, error) {
	created, err := r.users.UserCreateNewUser(ctx, &user.TLUserCreateNewUser{
		SecretKeyId: authKeyID,
		Phone:       phone,
		CountryCode: country,
		FirstName:   first,
		LastName:    last,
	})
	if err != nil && strings.Contains(err.Error(), "PHONE_NUMBER_OCCUPIED") {
		found, ferr := r.users.UserGetUserIdByPhone(ctx, &user.TLUserGetUserIdByPhone{Phone: phone})
		if ferr != nil {
			return nil, ferr
		}
		created, err = r.users.UserGetImmutableUser(ctx, &user.TLUserGetImmutableUser{Id: found.GetV()})
	}
	if err != nil {
		return nil, err
	}
	if created == nil || created.Id() == 0 {
		return nil, errors.New("user.createNewUser returned an empty user")
	}
	if _, err = r.auth.AuthsessionBindAuthKeyUser(ctx, &authsession.TLAuthsessionBindAuthKeyUser{
		AuthKeyId: authKeyID,
		UserId:    created.Id(),
	}); err != nil {
		return nil, err
	}
	return created.ToSelfUser(), nil
}

func (r rpcUsers) UpdateName(ctx context.Context, userID int64, first, last string) error {
	_, err := r.users.UserUpdateFirstAndLastName(ctx, &user.TLUserUpdateFirstAndLastName{
		UserId:    userID,
		FirstName: first,
		LastName:  last,
	})
	return err
}

func wireLayer229(c config.Config, auth authsession_client.AuthsessionClient) error {
	dsn := c.PostgresDSN
	if !strings.HasPrefix(strings.ToLower(strings.TrimSpace(dsn)), "postgres") {
		return errors.New("session: PostgresDSN is required for Layer 229")
	}
	if err := layer229.UsePostgres(dsn); err != nil {
		return fmt.Errorf("session: open Layer 229 PostgreSQL store: %w", err)
	}
	logx.Info("layer229 postgres open")
	layer229.SetProjectID(c.FirebaseProjectID)
	if c.UserClient.Etcd.Key == "" && len(c.UserClient.Endpoints) == 0 && c.UserClient.Target == "" {
		return nil
	}
	layer229.SetDirectory(rpcUsers{
		users: user_client.NewUserClient(rpcx.GetCachedRpcClient(c.UserClient)),
		auth:  auth,
	})
	return nil
}
