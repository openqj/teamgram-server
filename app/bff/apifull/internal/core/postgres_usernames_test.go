package core

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/dao"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/domain"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/svc"
	userclient "github.com/teamgram/teamgram-server/app/service/biz/user/client"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

type usernameTestClient struct {
	userclient.UserClient
	t         *testing.T
	actorID   int64
	bots      []*mtproto.ImmutableUser
	peerType  int32
	peerID    int64
	name      string
	active    bool
	order     []string
	mutations int
	result    *mtproto.Bool
	err       error
}

func (c *usernameTestClient) checkActor(ctx context.Context) {
	c.t.Helper()
	md, err := metadata.RpcMetadataFromMD(metadata.MD(metadata.ExtractOutgoing(ctx)))
	if err != nil || md.GetUserId() != c.actorID {
		c.t.Fatalf("username mutation metadata: actor=%d want=%d err=%v", md.GetUserId(), c.actorID, err)
	}
}

func (c *usernameTestClient) UserToggleUsername(ctx context.Context, in *userpb.TLUserToggleUsername) (*mtproto.Bool, error) {
	c.checkActor(ctx)
	c.peerType, c.peerID, c.name, c.active = in.GetPeerType(), in.GetPeerId(), in.GetUsername(), mtproto.FromBool(in.GetActive())
	c.mutations++
	return c.result, c.err
}

func (c *usernameTestClient) UserReorderUsernames(ctx context.Context, in *userpb.TLUserReorderUsernames) (*mtproto.Bool, error) {
	c.checkActor(ctx)
	c.peerType, c.peerID, c.order = in.GetPeerType(), in.GetPeerId(), append([]string(nil), in.GetUsernameList()...)
	c.mutations++
	return c.result, c.err
}

func (c *usernameTestClient) UserDeactivateAllChannelUsernames(ctx context.Context, in *userpb.TLUserDeactivateAllChannelUsernames) (*mtproto.Bool, error) {
	c.checkActor(ctx)
	c.peerType, c.peerID = mtproto.PEER_CHANNEL, in.GetChannelId()
	c.mutations++
	return c.result, c.err
}

func (c *usernameTestClient) UserGetCreatedBots(ctx context.Context) (*userpb.Vector_ImmutableUser, error) {
	c.checkActor(ctx)
	return &userpb.Vector_ImmutableUser{Datas: c.bots}, nil
}

type usernameWireRequest interface {
	Encode(*mtproto.EncodeBuf, int32) error
}

func invokeUsernameRequest(c *ApiFullCore, request any) (*mtproto.Bool, error) {
	switch in := request.(type) {
	case *mtproto.TLAccountToggleUsername:
		return c.AccountToggleUsername(in)
	case *mtproto.TLAccountReorderUsernames:
		return c.AccountReorderUsernames(in)
	case *mtproto.TLChannelsToggleUsername:
		return c.ChannelsToggleUsername(in)
	case *mtproto.TLChannelsReorderUsernames:
		return c.ChannelsReorderUsernames(in)
	case *mtproto.TLChannelsDeactivateAllUsernames:
		return c.ChannelsDeactivateAllUsernames(in)
	case *mtproto.TLBotsToggleUsername:
		return c.BotsToggleUsername(in)
	case *mtproto.TLBotsReorderUsernames:
		return c.BotsReorderUsernames(in)
	default:
		return nil, mtproto.ErrInputConstructorInvalid
	}
}

func TestPostgresUsernameRPCWireForwarding(t *testing.T) {
	uid := time.Now().UnixNano()
	channelID, botID := uid+1, uid+2
	if err := domain.SaveChannel(domain.Channel{ID: channelID, AccessHash: channelID, Creator: uid, Megagroup: true}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = domain.DeleteChannel(uid, channelID) })
	channel := mtproto.MakeTLInputChannel(&mtproto.InputChannel{ChannelId: channelID, AccessHash: channelID}).To_InputChannel()
	bot := mtproto.MakeTLInputUser(&mtproto.InputUser{UserId: botID, AccessHash: botID}).To_InputUser()
	order := []string{"PurchasedB", "PurchasedA"}
	client := &usernameTestClient{t: t, actorID: uid, result: mtproto.BoolTrue, bots: []*mtproto.ImmutableUser{{User: &mtproto.UserData{Id: botID, AccessHash: botID, Bot: &mtproto.BotData{}}}}}
	c := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: uid}, svcCtx: &svc.ServiceContext{Dao: &dao.Dao{UserClient: client}}}
	checks := []struct {
		name     string
		request  usernameWireRequest
		peerType int32
		peerID   int64
	}{
		{"account.toggleUsername", &mtproto.TLAccountToggleUsername{Constructor: mtproto.TLConstructor_CRC32_account_toggleUsername, Username: "PurchasedA", Active: mtproto.BoolFalse}, mtproto.PEER_USER, uid},
		{"account.reorderUsernames", &mtproto.TLAccountReorderUsernames{Constructor: mtproto.TLConstructor_CRC32_account_reorderUsernames, Order: order}, mtproto.PEER_USER, uid},
		{"channels.toggleUsername", &mtproto.TLChannelsToggleUsername{Constructor: mtproto.TLConstructor_CRC32_channels_toggleUsername, Channel: channel, Username: "PurchasedA", Active: mtproto.BoolFalse}, mtproto.PEER_CHANNEL, channelID},
		{"channels.reorderUsernames", &mtproto.TLChannelsReorderUsernames{Constructor: mtproto.TLConstructor_CRC32_channels_reorderUsernames, Channel: channel, Order: order}, mtproto.PEER_CHANNEL, channelID},
		{"channels.deactivateAllUsernames", &mtproto.TLChannelsDeactivateAllUsernames{Constructor: mtproto.TLConstructor_CRC32_channels_deactivateAllUsernames, Channel: channel}, mtproto.PEER_CHANNEL, channelID},
		{"bots.toggleUsername", &mtproto.TLBotsToggleUsername{Constructor: mtproto.TLConstructor_CRC32_bots_toggleUsername, Bot: bot, Username: "PurchasedA", Active: mtproto.BoolFalse}, mtproto.PEER_USER, botID},
		{"bots.reorderUsernames", &mtproto.TLBotsReorderUsernames{Constructor: mtproto.TLConstructor_CRC32_bots_reorderUsernames, Bot: bot, Order: order}, mtproto.PEER_USER, botID},
	}
	for _, check := range checks {
		t.Run(check.name, func(t *testing.T) {
			if result, err := invokeUsernameRequest(&ApiFullCore{}, check.request); result != nil || !errors.Is(err, mtproto.ErrAuthKeyUnregistered) {
				t.Fatalf("unauthenticated: result=%v err=%v", result, err)
			}
			buf := mtproto.NewEncodeBuf(256)
			if err := check.request.Encode(buf, 229); err != nil {
				t.Fatal(err)
			}
			decode := mtproto.NewDecodeBuf(buf.GetBuf())
			request := decode.Object()
			if err := decode.GetError(); err != nil || decode.GetOffset() != decode.GetSize() || reflect.TypeOf(request) != reflect.TypeOf(check.request) {
				t.Fatalf("Layer 229 request: type=%T err=%v", request, err)
			}
			client.name, client.order = "", nil
			before := client.mutations
			result, err := invokeUsernameRequest(c, request)
			if err != nil || !mtproto.FromBool(result) || client.mutations != before+1 || client.peerType != check.peerType || client.peerID != check.peerID {
				t.Fatalf("forwarding: result=%v peer=%d/%d mutations=%d err=%v", result, client.peerType, client.peerID, client.mutations-before, err)
			}
			if client.name != "" && (client.name != "PurchasedA" || client.active) {
				t.Fatalf("toggle changed wire parameters: name=%q active=%v", client.name, client.active)
			}
			if client.order != nil && !reflect.DeepEqual(client.order, order) {
				t.Fatalf("reorder changed wire vector: %v", client.order)
			}
			if err = result.Encode(mtproto.NewEncodeBuf(16), 229); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestPostgresUsernameRPCPermissionsAndErrors(t *testing.T) {
	channelID := time.Now().UnixNano()
	owner, member, outsider := channelID+1, channelID+2, channelID+3
	if err := domain.SaveChannel(domain.Channel{ID: channelID, AccessHash: channelID, Creator: owner, Megagroup: true}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = domain.DeleteChannel(owner, channelID) })
	if err := domain.JoinChannel(channelID, member); err != nil {
		t.Fatal(err)
	}
	client := &usernameTestClient{t: t, actorID: member, result: mtproto.BoolTrue}
	c := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: member}, svcCtx: &svc.ServiceContext{Dao: &dao.Dao{UserClient: client}}}
	input := mtproto.MakeTLInputChannel(&mtproto.InputChannel{ChannelId: channelID, AccessHash: channelID}).To_InputChannel()
	if _, err := c.ChannelsDeactivateAllUsernames(&mtproto.TLChannelsDeactivateAllUsernames{Channel: input}); !errors.Is(err, mtproto.ErrChatAdminRequired) || client.mutations != 0 {
		t.Fatalf("member mutation: err=%v writes=%d", err, client.mutations)
	}
	c.MD.UserId, client.actorID = outsider, outsider
	if _, err := c.ChannelsReorderUsernames(&mtproto.TLChannelsReorderUsernames{Channel: input}); !errors.Is(err, mtproto.ErrUserNotParticipant) || client.mutations != 0 {
		t.Fatalf("outsider mutation: err=%v writes=%d", err, client.mutations)
	}
	c.MD.UserId, client.actorID = owner, owner
	badHash := mtproto.MakeTLInputChannel(&mtproto.InputChannel{ChannelId: channelID, AccessHash: channelID + 1}).To_InputChannel()
	if _, err := c.ChannelsToggleUsername(&mtproto.TLChannelsToggleUsername{Channel: badHash, Username: "owned", Active: mtproto.BoolTrue}); !errors.Is(err, mtproto.ErrChannelInvalid) || client.mutations != 0 {
		t.Fatalf("wrong hash mutation: err=%v writes=%d", err, client.mutations)
	}
	bot := mtproto.MakeTLInputUser(&mtproto.InputUser{UserId: outsider, AccessHash: outsider}).To_InputUser()
	if _, err := c.BotsToggleUsername(&mtproto.TLBotsToggleUsername{Bot: bot, Username: "owned", Active: mtproto.BoolTrue}); !errors.Is(err, mtproto.ErrBotInvalid) || client.mutations != 0 {
		t.Fatalf("unowned bot mutation: err=%v writes=%d", err, client.mutations)
	}
	client.bots = []*mtproto.ImmutableUser{{User: &mtproto.UserData{Id: outsider, AccessHash: outsider + 1, Bot: &mtproto.BotData{}}}}
	if _, err := c.BotsReorderUsernames(&mtproto.TLBotsReorderUsernames{Bot: bot}); !errors.Is(err, mtproto.ErrUserIdInvalid) || client.mutations != 0 {
		t.Fatalf("wrong bot hash: err=%v writes=%d", err, client.mutations)
	}
	if _, err := c.AccountToggleUsername(&mtproto.TLAccountToggleUsername{Username: "owned"}); !errors.Is(err, mtproto.ErrInputRequestInvalid) || client.mutations != 0 {
		t.Fatalf("missing Bool mutation: err=%v writes=%d", err, client.mutations)
	}
	if err := domain.EditChannelAdmin(channelID, owner, member, &domain.ChannelAdminRights{ChangeInfo: true}, "username-admin"); err != nil {
		t.Fatal(err)
	}
	c.MD.UserId, client.actorID = member, member
	if result, err := c.ChannelsDeactivateAllUsernames(&mtproto.TLChannelsDeactivateAllUsernames{Channel: input}); err != nil || !mtproto.FromBool(result) {
		t.Fatalf("authorized admin mutation: result=%v err=%v", result, err)
	}
	client.err = mtproto.ErrOrderInvalid
	if result, err := c.AccountReorderUsernames(&mtproto.TLAccountReorderUsernames{Order: []string{"unknown"}}); result != nil || !errors.Is(err, mtproto.ErrOrderInvalid) {
		t.Fatalf("canonical User error: result=%v err=%v", result, err)
	}
	client.err, client.result = nil, nil
	if result, err := c.AccountToggleUsername(&mtproto.TLAccountToggleUsername{Username: "owned", Active: mtproto.BoolTrue}); result != nil || !errors.Is(err, mtproto.ErrInternalServerError) {
		t.Fatalf("empty User response: result=%v err=%v", result, err)
	}
}
