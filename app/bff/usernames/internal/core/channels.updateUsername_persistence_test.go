package core

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	mysql "github.com/go-sql-driver/mysql"
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/apifull/channelview"
	"github.com/teamgram/teamgram-server/app/bff/usernames/internal/dao"
	"github.com/teamgram/teamgram-server/app/bff/usernames/internal/svc"
	user_client "github.com/teamgram/teamgram-server/app/service/biz/user/client"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

type channelUsernamePersistenceClient struct {
	user_client.UserClient
	updated *userpb.TLUserUpdateUsernameByUsername
}

func (c *channelUsernamePersistenceClient) UserGetChannelUsername(context.Context, *userpb.TLUserGetChannelUsername) (*userpb.UsernameData, error) {
	return &userpb.UsernameData{}, nil
}

func (c *channelUsernamePersistenceClient) UserUpdateUsernameByUsername(_ context.Context, in *userpb.TLUserUpdateUsernameByUsername) (*mtproto.Bool, error) {
	c.updated = in
	return mtproto.BoolTrue, nil
}

func isolatedChannelUsernameDSN(t *testing.T) string {
	t.Helper()
	dsn := os.Getenv("APIFULL_MYSQL_DSN")
	if dsn == "" {
		t.Skip("APIFULL_MYSQL_DSN is required for channel username persistence")
	}
	cfg, err := mysql.ParseDSN(dsn)
	if err != nil || cfg.DBName != "teamgram_audit" || cfg.Net != "tcp" || cfg.Addr != "127.0.0.1:13306" {
		t.Fatal("channel username persistence requires the isolated audit database")
	}
	return cfg.FormatDSN()
}

func TestChannelsUpdateUsernamePersistsAPIFullChannel(t *testing.T) {
	dsn := isolatedChannelUsernameDSN(t)
	if err := channelview.Open(dsn); err != nil {
		t.Fatal("open channel store")
	}
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Fatal("open audit database")
	}
	t.Cleanup(func() { _ = db.Close() })

	channelID := time.Now().UnixNano()
	const owner int64 = 92001
	if _, err = db.Exec(`INSERT INTO apifull_channel
		(id, access_hash, creator_user_id, title, about, broadcast, megagroup, created_at)
		VALUES (?,?,?,?,?,?,?,?)`, channelID, channelID, owner, "username-persistence", "", 1, 0, time.Now().Unix()); err != nil {
		t.Fatal("insert channel fixture")
	}
	t.Cleanup(func() {
		if _, err := db.Exec(`DELETE FROM apifull_channel WHERE id=?`, channelID); err != nil {
			t.Errorf("clean up channel fixture: %v", err)
		}
	})

	client := &channelUsernamePersistenceClient{}
	core := New(context.Background(), &svc.ServiceContext{Dao: &dao.Dao{UserClient: client}})
	core.MD = &metadata.RpcMetadata{UserId: owner}
	core.channelChatsByID = func(userID int64, ids []int64) []*mtproto.Chat {
		if userID != owner || len(ids) != 1 || ids[0] != channelID {
			t.Fatalf("channel admin lookup got user ID %d and channel IDs %v", userID, ids)
		}
		return []*mtproto.Chat{mtproto.MakeTLChannel(&mtproto.Chat{Id: channelID, Creator: true}).To_Chat()}
	}

	const username = "stored_channel_name"
	result, err := core.ChannelsUpdateUsername(&mtproto.TLChannelsUpdateUsername{
		Channel:  mtproto.MakeTLInputChannel(&mtproto.InputChannel{ChannelId: channelID, AccessHash: channelID}).To_InputChannel(),
		Username: username,
	})
	if err != nil || !mtproto.FromBool(result) {
		t.Fatalf("update username = (%v, %v), want BoolTrue and nil", result, err)
	}
	if client.updated == nil || client.updated.GetPeerType() != mtproto.PEER_CHANNEL || client.updated.GetPeerId() != channelID || client.updated.GetUsername() != username {
		t.Fatalf("username service update = %+v, want channel %d username %q", client.updated, channelID, username)
	}

	var stored string
	if err = db.QueryRow(`SELECT username FROM apifull_channel WHERE id=?`, channelID).Scan(&stored); err != nil {
		t.Fatal("read persisted channel username")
	}
	if stored != username {
		t.Fatalf("persisted username = %q, want %q", stored, username)
	}
}
