package core

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/apifull/channelview"
	apifullDao "github.com/teamgram/teamgram-server/app/bff/apifull/internal/dao"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/domain"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/svc"
	user_client "github.com/teamgram/teamgram-server/app/service/biz/user/client"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
	"github.com/zeromicro/go-zero/core/logx"
)

type personalChannelUserClient struct {
	user_client.UserClient
	user *mtproto.ImmutableUser
}

func (c *personalChannelUserClient) UserGetImmutableUserV2(context.Context, *userpb.TLUserGetImmutableUserV2) (*mtproto.ImmutableUser, error) {
	return c.user, nil
}

type channelMemberUserClient struct {
	user_client.UserClient
}

func (c *channelMemberUserClient) UserGetImmutableUserV2(_ context.Context, in *userpb.TLUserGetImmutableUserV2) (*mtproto.ImmutableUser, error) {
	if in == nil || in.GetId() == 0 {
		return nil, nil
	}
	return &mtproto.ImmutableUser{User: &mtproto.UserData{Id: in.GetId(), FirstName: "user"}}, nil
}

func (c *channelMemberUserClient) UserGetMutableUsersV2(_ context.Context, in *userpb.TLUserGetMutableUsersV2) (*mtproto.MutableUsers, error) {
	users := make([]*mtproto.ImmutableUser, 0, len(in.GetId()))
	for _, id := range in.GetId() {
		if id > 0 {
			users = append(users, &mtproto.ImmutableUser{User: &mtproto.UserData{Id: id, FirstName: "hydrated"}})
		}
	}
	return &mtproto.MutableUsers{Users: users}, nil
}

func TestChannelsUnauthed(t *testing.T) {
	calls := []struct {
		name string
		call func(*ApiFullCore) error
	}{
		{"MessagesGetPersonalChannelHistory", func(c *ApiFullCore) error {
			_, err := c.MessagesGetPersonalChannelHistory(nil)
			return err
		}},
		{"ChannelsReadHistory", func(c *ApiFullCore) error {
			_, err := c.ChannelsReadHistory(nil)
			return err
		}},
		{"ChannelsDeleteMessages", func(c *ApiFullCore) error {
			_, err := c.ChannelsDeleteMessages(nil)
			return err
		}},
		{"ChannelsGetMessages", func(c *ApiFullCore) error {
			_, err := c.ChannelsGetMessages(nil)
			return err
		}},
		{"ChannelsGetParticipants", func(c *ApiFullCore) error {
			_, err := c.ChannelsGetParticipants(nil)
			return err
		}},
		{"ChannelsGetParticipant", func(c *ApiFullCore) error {
			_, err := c.ChannelsGetParticipant(nil)
			return err
		}},
		{"ChannelsGetChannels", func(c *ApiFullCore) error {
			_, err := c.ChannelsGetChannels(nil)
			return err
		}},
		{"ChannelsGetFullChannel", func(c *ApiFullCore) error {
			_, err := c.ChannelsGetFullChannel(nil)
			return err
		}},
		{"ChannelsCreateChannel", func(c *ApiFullCore) error {
			_, err := c.ChannelsCreateChannel(nil)
			return err
		}},
		{"ChannelsEditAdmin", func(c *ApiFullCore) error {
			_, err := c.ChannelsEditAdmin(nil)
			return err
		}},
		{"ChannelsEditTitle", func(c *ApiFullCore) error {
			_, err := c.ChannelsEditTitle(nil)
			return err
		}},
		{"ChannelsEditPhoto", func(c *ApiFullCore) error {
			_, err := c.ChannelsEditPhoto(nil)
			return err
		}},
		{"ChannelsJoinChannel7F6A1E22", func(c *ApiFullCore) error {
			_, err := c.ChannelsJoinChannel7F6A1E22(nil)
			return err
		}},
		{"ChannelsLeaveChannel", func(c *ApiFullCore) error {
			_, err := c.ChannelsLeaveChannel(nil)
			return err
		}},
		{"ChannelsInviteToChannelC9E33D54", func(c *ApiFullCore) error {
			_, err := c.ChannelsInviteToChannelC9E33D54(nil)
			return err
		}},
		{"ChannelsDeleteChannel", func(c *ApiFullCore) error {
			_, err := c.ChannelsDeleteChannel(nil)
			return err
		}},
		{"ChannelsExportMessageLink", func(c *ApiFullCore) error {
			_, err := c.ChannelsExportMessageLink(nil)
			return err
		}},
		{"ChannelsToggleSignatures", func(c *ApiFullCore) error {
			_, err := c.ChannelsToggleSignatures(nil)
			return err
		}},
		{"ChannelsGetAdminedPublicChannels", func(c *ApiFullCore) error {
			_, err := c.ChannelsGetAdminedPublicChannels(nil)
			return err
		}},
		{"ChannelsEditBanned", func(c *ApiFullCore) error {
			_, err := c.ChannelsEditBanned(nil)
			return err
		}},
		{"ChannelsGetAdminLog", func(c *ApiFullCore) error {
			_, err := c.ChannelsGetAdminLog(nil)
			return err
		}},
		{"ChannelsSetStickers", func(c *ApiFullCore) error {
			_, err := c.ChannelsSetStickers(nil)
			return err
		}},
		{"ChannelsReadMessageContents", func(c *ApiFullCore) error {
			_, err := c.ChannelsReadMessageContents(nil)
			return err
		}},
		{"ChannelsDeleteHistory9BAA9647", func(c *ApiFullCore) error {
			_, err := c.ChannelsDeleteHistory9BAA9647(nil)
			return err
		}},
		{"ChannelsTogglePreHistoryHidden", func(c *ApiFullCore) error {
			_, err := c.ChannelsTogglePreHistoryHidden(nil)
			return err
		}},
		{"ChannelsGetGroupsForDiscussion", func(c *ApiFullCore) error {
			_, err := c.ChannelsGetGroupsForDiscussion(nil)
			return err
		}},
		{"ChannelsSetDiscussionGroup", func(c *ApiFullCore) error {
			_, err := c.ChannelsSetDiscussionGroup(nil)
			return err
		}},
		{"ChannelsEditLocation", func(c *ApiFullCore) error {
			_, err := c.ChannelsEditLocation(nil)
			return err
		}},
		{"ChannelsToggleSlowMode", func(c *ApiFullCore) error {
			_, err := c.ChannelsToggleSlowMode(nil)
			return err
		}},
		{"ChannelsGetInactiveChannels", func(c *ApiFullCore) error {
			_, err := c.ChannelsGetInactiveChannels(nil)
			return err
		}},
		{"ChannelsDeleteParticipantHistory", func(c *ApiFullCore) error {
			_, err := c.ChannelsDeleteParticipantHistory(nil)
			return err
		}},
		{"ChannelsToggleParticipantsHidden", func(c *ApiFullCore) error {
			_, err := c.ChannelsToggleParticipantsHidden(nil)
			return err
		}},
		{"ChannelsJoinChannel24B524C5", func(c *ApiFullCore) error {
			_, err := c.ChannelsJoinChannel24B524C5(nil)
			return err
		}},
		{"ChannelsEditCreator", func(c *ApiFullCore) error {
			_, err := c.ChannelsEditCreator(nil)
			return err
		}},
		{"ChannelsGetFutureCreatorAfterLeave", func(c *ApiFullCore) error {
			_, err := c.ChannelsGetFutureCreatorAfterLeave(nil)
			return err
		}},
		{"ChannelsInviteToChannel199F3A6C", func(c *ApiFullCore) error {
			_, err := c.ChannelsInviteToChannel199F3A6C(nil)
			return err
		}},
		{"ChannelsDeleteHistoryAF369D42", func(c *ApiFullCore) error {
			_, err := c.ChannelsDeleteHistoryAF369D42(nil)
			return err
		}},
	}

	cores := []*ApiFullCore{
		{},
		{MD: &metadata.RpcMetadata{UserId: 0}},
	}
	for _, tc := range calls {
		for _, c := range cores {
			if err := tc.call(c); !errors.Is(err, mtproto.ErrAuthKeyUnregistered) {
				t.Fatalf("%s unauthed: got %v", tc.name, err)
			}
		}
	}
}

func TestChannelsCreateChannel(t *testing.T) {
	const id int64 = 1
	c := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: id}}
	_, err := c.ChannelsCreateChannel(nil)
	if err != nil {
		t.Fatalf("got %v", err)
	}
}

func TestChannelsCreateThenGetTitle(t *testing.T) {
	c := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: 1}}
	up, err := c.ChannelsCreateChannel(&mtproto.TLChannelsCreateChannel{Title: "prod-chan"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if up == nil || len(up.GetChats()) != 1 || up.GetChats()[0].GetId() == 0 {
		t.Fatalf("create channel: %+v", up)
	}
	id := up.GetChats()[0].GetId()
	got, err := c.ChannelsGetChannels(&mtproto.TLChannelsGetChannels{
		Id: []*mtproto.InputChannel{
			mtproto.MakeTLInputChannel(&mtproto.InputChannel{ChannelId: id, AccessHash: id}).To_InputChannel(),
		},
	})
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got == nil || len(got.GetChats()) != 1 || got.GetChats()[0].GetTitle() != "prod-chan" {
		t.Fatalf("Title: %+v", got)
	}
}

func TestChannelsEditTitleRoundTrip(t *testing.T) {
	const owner, outsider int64 = 81401, 81402
	ownerCore := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: owner}}
	created, err := ownerCore.ChannelsCreateChannel(&mtproto.TLChannelsCreateChannel{Title: "before-edit"})
	if err != nil || created == nil || len(created.GetChats()) != 1 {
		t.Fatalf("create channel: result=%+v err=%v", created, err)
	}
	channelID := created.GetChats()[0].GetId()
	channel := mtproto.MakeTLInputChannel(&mtproto.InputChannel{
		ChannelId: channelID, AccessHash: channelID,
	}).To_InputChannel()
	updated, err := ownerCore.ChannelsEditTitle(&mtproto.TLChannelsEditTitle{Channel: channel, Title: "after-edit"})
	if err != nil || updated == nil || len(updated.GetChats()) != 1 || updated.GetChats()[0].GetTitle() != "after-edit" {
		t.Fatalf("edit title: result=%+v err=%v", updated, err)
	}
	if err = updated.Encode(mtproto.NewEncodeBuf(4096), 229); err != nil {
		t.Fatalf("encode edited channel response: %v", err)
	}
	got, err := ownerCore.ChannelsGetChannels(&mtproto.TLChannelsGetChannels{Id: []*mtproto.InputChannel{channel}})
	if err != nil || got == nil || len(got.GetChats()) != 1 || got.GetChats()[0].GetTitle() != "after-edit" {
		t.Fatalf("read edited title: result=%+v err=%v", got, err)
	}
	outsiderCore := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: outsider}}
	if _, err = outsiderCore.ChannelsEditTitle(&mtproto.TLChannelsEditTitle{Channel: channel, Title: "unauthorized"}); !errors.Is(err, mtproto.ErrChatAdminRequired) {
		t.Fatalf("outsider edit: got %v", err)
	}
	got, err = ownerCore.ChannelsGetChannels(&mtproto.TLChannelsGetChannels{Id: []*mtproto.InputChannel{channel}})
	if err != nil || got == nil || len(got.GetChats()) != 1 || got.GetChats()[0].GetTitle() != "after-edit" {
		t.Fatalf("title after rejected edit: result=%+v err=%v", got, err)
	}
}

func TestChannelMySQL(t *testing.T) {
	c := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: 1}}
	up, err := c.ChannelsCreateChannel(&mtproto.TLChannelsCreateChannel{Title: "mysql-chan"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if up == nil || len(up.Chats) == 0 || up.Chats[0].GetId() == 0 {
		t.Fatalf("create channel: %+v", up)
	}
	id := up.Chats[0].GetId()
	got, ok, err := domain.LoadChannel(id)
	if err != nil || !ok || got.Title != "mysql-chan" {
		t.Fatalf("channel %+v ok %v err %v", got, ok, err)
	}
}

func TestChannelsGetAdminedPublicChannelsUsesCanonicalRows(t *testing.T) {
	const owner int64 = 81205
	publicID := time.Now().UnixNano()
	privateID := publicID + 1
	if err := domain.SaveChannel(domain.Channel{ID: publicID, AccessHash: publicID, Creator: owner, Title: "public-admined", Username: "public_admined", Broadcast: true}); err != nil {
		t.Fatal("save public channel:", err)
	}
	if err := domain.SaveChannel(domain.Channel{ID: privateID, AccessHash: privateID, Creator: owner, Title: "private-admined", Broadcast: true}); err != nil {
		t.Fatal("save private channel:", err)
	}
	t.Cleanup(func() {
		_ = domain.DeleteChannel(owner, publicID)
		_ = domain.DeleteChannel(owner, privateID)
	})

	core := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: owner}}
	result, err := core.ChannelsGetAdminedPublicChannels(&mtproto.TLChannelsGetAdminedPublicChannels{})
	if err != nil {
		t.Fatal("get public channels:", err)
	}
	if result == nil || len(result.GetChats()) != 1 || result.GetChats()[0].GetId() != publicID || result.GetChats()[0].GetUsername() == nil || result.GetChats()[0].GetUsername().GetValue() != "public_admined" {
		t.Fatalf("public channel rows = %+v", result)
	}
}

func TestChannelSettingsRoundTrip(t *testing.T) {
	const owner, reader int64 = 81201, 81202
	c := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: owner}}
	created, err := c.ChannelsCreateChannel(&mtproto.TLChannelsCreateChannel{Title: "settings-test"})
	if err != nil || created == nil || len(created.GetChats()) != 1 {
		t.Fatalf("create channel: result=%+v err=%v", created, err)
	}
	channelID := created.GetChats()[0].GetId()
	input := mtproto.MakeTLInputChannel(&mtproto.InputChannel{ChannelId: channelID, AccessHash: channelID}).To_InputChannel()

	updated, err := c.ChannelsToggleSignatures(&mtproto.TLChannelsToggleSignatures{
		Channel: input, SignaturesEnabled: true, ProfilesEnabled: true,
	})
	if err != nil || updated == nil || len(updated.GetChats()) != 1 || !updated.GetChats()[0].GetSignatures() || !updated.GetChats()[0].GetSignatureProfiles() {
		t.Fatalf("toggle signatures: result=%+v err=%v", updated, err)
	}
	updated, err = c.ChannelsTogglePreHistoryHidden(&mtproto.TLChannelsTogglePreHistoryHidden{Channel: input, Enabled: mtproto.BoolTrue})
	if err != nil || updated == nil || len(updated.GetChats()) != 1 {
		t.Fatalf("toggle prehistory: result=%+v err=%v", updated, err)
	}
	updated, err = c.ChannelsToggleSlowMode(&mtproto.TLChannelsToggleSlowMode{Channel: input, Seconds: 30})
	if err != nil || updated == nil || len(updated.GetChats()) != 1 || !updated.GetChats()[0].GetSlowmodeEnabled() {
		t.Fatalf("toggle slow mode: result=%+v err=%v", updated, err)
	}
	updated, err = c.ChannelsToggleParticipantsHidden(&mtproto.TLChannelsToggleParticipantsHidden{Channel: input, Enabled: mtproto.BoolTrue})
	if err != nil || updated == nil || len(updated.GetChats()) != 1 {
		t.Fatalf("toggle hidden participants: result=%+v err=%v", updated, err)
	}

	loaded, ok, err := domain.LoadChannel(channelID)
	if err != nil || !ok || !loaded.Signatures || !loaded.SignatureProfiles || !loaded.HiddenPrehistory || !loaded.ParticipantsHidden || loaded.SlowmodeSeconds != 30 {
		t.Fatalf("stored channel settings: channel=%+v ok=%v err=%v", loaded, ok, err)
	}
	chats, err := c.ChannelsGetChannels(&mtproto.TLChannelsGetChannels{Id: []*mtproto.InputChannel{input}})
	if err != nil || chats == nil || len(chats.GetChats()) != 1 || !chats.GetChats()[0].GetSignatures() || !chats.GetChats()[0].GetSignatureProfiles() {
		t.Fatalf("get channels settings: result=%+v err=%v", chats, err)
	}
	full, err := c.ChannelsGetFullChannel(&mtproto.TLChannelsGetFullChannel{Channel: input})
	if err != nil {
		t.Fatalf("get full channel: %v", err)
	}
	fullChannel := full.GetFullChat().To_ChannelFull()
	if !fullChannel.GetHiddenPrehistory() || !fullChannel.GetParticipantsHidden() || fullChannel.GetSlowmodeSeconds().GetValue() != 30 {
		t.Fatalf("full channel settings: %+v", fullChannel)
	}

	readerCore := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: reader}, Logger: logx.WithContext(context.Background())}
	participants, err := readerCore.ChannelsGetParticipants(&mtproto.TLChannelsGetParticipants{Channel: input})
	if err != nil || participants.GetCount() != 0 || len(participants.GetParticipants()) != 0 {
		t.Fatalf("hidden participants: result=%+v err=%v", participants, err)
	}
	if _, err = readerCore.ChannelsGetParticipant(&mtproto.TLChannelsGetParticipant{
		Channel:     input,
		Participant: mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: owner, AccessHash: 1}).To_InputPeer(),
	}); !errors.Is(err, mtproto.ErrUserNotParticipant) {
		t.Fatalf("hidden participant lookup: got %v", err)
	}
	if _, err = readerCore.ChannelsToggleSlowMode(&mtproto.TLChannelsToggleSlowMode{Channel: input, Seconds: 60}); !errors.Is(err, mtproto.ErrChatAdminRequired) {
		t.Fatalf("non-owner setting update: got %v", err)
	}
}

func TestChannelMutationRequiresAccessHash(t *testing.T) {
	const owner int64 = 81211
	channelID := time.Now().UnixNano()
	if err := domain.SaveChannel(domain.Channel{
		ID: channelID, AccessHash: channelID, Creator: owner, Title: "hash-guard-test", Broadcast: true,
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = domain.DeleteChannel(owner, channelID)
	})
	if _, err := channelview.Post(owner, channelID, "keep-on-rejected-write", 0); err != nil {
		t.Fatalf("seed channel message: %v", err)
	}
	badChannel := mtproto.MakeTLInputChannel(&mtproto.InputChannel{
		ChannelId: channelID, AccessHash: channelID + 1,
	}).To_InputChannel()
	core := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: owner}}

	if _, err := core.ChannelsEditTitle(&mtproto.TLChannelsEditTitle{Channel: badChannel, Title: "changed"}); !errors.Is(err, mtproto.ErrChannelInvalid) {
		t.Fatalf("edit title with bad hash: got %v", err)
	}
	if _, err := core.ChannelsToggleSlowMode(&mtproto.TLChannelsToggleSlowMode{Channel: badChannel, Seconds: 30}); !errors.Is(err, mtproto.ErrChannelInvalid) {
		t.Fatalf("change settings with bad hash: got %v", err)
	}
	if _, err := core.ChannelsDeleteMessages(&mtproto.TLChannelsDeleteMessages{Channel: badChannel, Id: []int32{1}}); !errors.Is(err, mtproto.ErrChannelInvalid) {
		t.Fatalf("delete messages with bad hash: got %v", err)
	}
	if _, err := core.ChannelsDeleteHistory9BAA9647(&mtproto.TLChannelsDeleteHistory9BAA9647{
		Channel: badChannel, MaxId: 1, ForEveryone: true,
	}); !errors.Is(err, mtproto.ErrChannelInvalid) {
		t.Fatalf("delete history with bad hash: got %v", err)
	}
	if _, err := core.ChannelsDeleteHistoryAF369D42(&mtproto.TLChannelsDeleteHistoryAF369D42{Channel: badChannel, MaxId: 1}); !errors.Is(err, mtproto.ErrChannelInvalid) {
		t.Fatalf("hide history with bad hash: got %v", err)
	}

	stored, ok, err := domain.LoadChannel(channelID)
	if err != nil || !ok || stored.Title != "hash-guard-test" || stored.SlowmodeSeconds != 0 {
		t.Fatalf("channel after rejected writes: channel=%+v ok=%v err=%v", stored, ok, err)
	}
	box, err := channelview.MessagesBox(owner, channelID, []int32{1})
	if err != nil || box == nil || len(box.GetMessages()) != 1 || box.GetMessages()[0].GetMessage() != "keep-on-rejected-write" {
		t.Fatalf("message after rejected writes: box=%+v err=%v", box, err)
	}
}

func TestChannelsEditLocationRoundTrip(t *testing.T) {
	const owner, outsider int64 = 81221, 81222
	ownerCore := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: owner}}
	created, err := ownerCore.ChannelsCreateChannel(&mtproto.TLChannelsCreateChannel{Title: "location-test"})
	if err != nil || created == nil || len(created.GetChats()) != 1 {
		t.Fatalf("create channel: result=%+v err=%v", created, err)
	}
	channelID := created.GetChats()[0].GetId()
	t.Cleanup(func() { _ = domain.DeleteChannel(owner, channelID) })
	input := mtproto.MakeTLInputChannel(&mtproto.InputChannel{ChannelId: channelID, AccessHash: channelID}).To_InputChannel()
	geo := mtproto.MakeTLInputGeoPoint(&mtproto.InputGeoPoint{Lat: 31.2304, Long: 121.4737}).To_InputGeoPoint()
	updated, err := ownerCore.ChannelsEditLocation(&mtproto.TLChannelsEditLocation{Channel: input, GeoPoint: geo, Address: "Shanghai"})
	if err != nil || !mtproto.FromBool(updated) {
		t.Fatalf("edit location: result=%v err=%v", updated, err)
	}
	stored, ok, err := domain.LoadChannel(channelID)
	if err != nil || !ok || stored.LocationLat == nil || stored.LocationLong == nil || *stored.LocationLat != 31.2304 || *stored.LocationLong != 121.4737 || stored.LocationAddress != "Shanghai" {
		t.Fatalf("stored location: channel=%+v ok=%v err=%v", stored, ok, err)
	}
	full, err := ownerCore.ChannelsGetFullChannel(&mtproto.TLChannelsGetFullChannel{Channel: input})
	if err != nil || full == nil {
		t.Fatalf("read full channel: result=%+v err=%v", full, err)
	}
	location := full.GetFullChat().To_ChannelFull().GetLocation()
	if location == nil || location.GetPredicateName() != mtproto.Predicate_channelLocation || location.GetGeoPoint() == nil ||
		location.GetGeoPoint().GetLat() != 31.2304 || location.GetGeoPoint().GetLong() != 121.4737 || location.GetAddress() != "Shanghai" {
		t.Fatalf("read location: %+v", location)
	}
	if err = full.Encode(mtproto.NewEncodeBuf(8192), 229); err != nil {
		t.Fatalf("encode full channel with location: %v", err)
	}

	outsiderCore := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: outsider}}
	if _, err = outsiderCore.ChannelsEditLocation(&mtproto.TLChannelsEditLocation{Channel: input, GeoPoint: geo, Address: "blocked"}); !errors.Is(err, mtproto.ErrChatAdminRequired) {
		t.Fatalf("outsider edit: got %v", err)
	}
	badHash := mtproto.MakeTLInputChannel(&mtproto.InputChannel{ChannelId: channelID, AccessHash: channelID + 1}).To_InputChannel()
	if _, err = ownerCore.ChannelsEditLocation(&mtproto.TLChannelsEditLocation{Channel: badHash, GeoPoint: geo, Address: "blocked"}); !errors.Is(err, mtproto.ErrChannelInvalid) {
		t.Fatalf("bad-hash edit: got %v", err)
	}

	clear := mtproto.MakeTLInputGeoPointEmpty(nil).To_InputGeoPoint()
	if _, err = ownerCore.ChannelsEditLocation(&mtproto.TLChannelsEditLocation{Channel: input, GeoPoint: clear, Address: "ignored"}); err != nil {
		t.Fatalf("clear location: %v", err)
	}
	stored, ok, err = domain.LoadChannel(channelID)
	if err != nil || !ok || stored.LocationLat != nil || stored.LocationLong != nil || stored.LocationAddress != "" {
		t.Fatalf("cleared location: channel=%+v ok=%v err=%v", stored, ok, err)
	}
}

func TestChannelsReadHistoryRoundTrip(t *testing.T) {
	const owner, reader, outsider int64 = 81101, 81102, 81103
	channelID := time.Now().UnixNano()
	if err := domain.SaveChannel(domain.Channel{
		ID:         channelID,
		AccessHash: channelID,
		Creator:    owner,
		Title:      "read-history-test",
		Broadcast:  true,
		CreatedAt:  time.Now().Unix(),
	}); err != nil {
		t.Fatal(err)
	}
	if err := domain.JoinChannel(channelID, reader); err != nil {
		t.Fatalf("join reader: %v", err)
	}
	for _, text := range []string{"first", "second", "third"} {
		if _, err := channelview.Post(owner, channelID, text, 0); err != nil {
			t.Fatal(err)
		}
	}
	input := mtproto.MakeTLInputChannel(&mtproto.InputChannel{
		ChannelId:  channelID,
		AccessHash: channelID,
	}).To_InputChannel()
	readerCore := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: reader}}
	outsiderCore := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: outsider}}
	if _, err := outsiderCore.ChannelsReadHistory(&mtproto.TLChannelsReadHistory{Channel: input, MaxId: 2}); !errors.Is(err, mtproto.ErrUserNotParticipant) {
		t.Fatalf("outsider read: got %v", err)
	}
	if readMax, err := domain.ChannelReadMaxID(outsider, channelID); err != nil || readMax != 0 {
		t.Fatalf("outsider cursor after rejected read: cursor=%d err=%v", readMax, err)
	}
	badHash := mtproto.MakeTLInputChannel(&mtproto.InputChannel{
		ChannelId: channelID, AccessHash: channelID + 1,
	}).To_InputChannel()
	if _, err := readerCore.ChannelsReadHistory(&mtproto.TLChannelsReadHistory{Channel: badHash, MaxId: 3}); !errors.Is(err, mtproto.ErrChannelInvalid) {
		t.Fatalf("bad-hash read: got %v", err)
	}
	if readMax, err := domain.ChannelReadMaxID(reader, channelID); err != nil || readMax != 0 {
		t.Fatalf("reader cursor after rejected bad-hash read: cursor=%d err=%v", readMax, err)
	}
	read, err := readerCore.ChannelsReadHistory(&mtproto.TLChannelsReadHistory{Channel: input, MaxId: 2})
	if err != nil || !mtproto.FromBool(read) {
		t.Fatalf("read history: result=%v err=%v", read, err)
	}
	full, err := readerCore.ChannelsGetFullChannel(&mtproto.TLChannelsGetFullChannel{Channel: input})
	if err != nil {
		t.Fatalf("get full channel for reader: %v", err)
	}
	readerFull := full.GetFullChat().To_ChannelFull()
	if readerFull.GetReadInboxMaxId() != 2 || readerFull.GetReadOutboxMaxId() != 0 {
		t.Fatalf("reader cursors: inbox=%d outbox=%d", readerFull.GetReadInboxMaxId(), readerFull.GetReadOutboxMaxId())
	}
	if _, err = readerCore.ChannelsReadHistory(&mtproto.TLChannelsReadHistory{Channel: input, MaxId: 1}); err != nil {
		t.Fatalf("read older history: %v", err)
	}
	full, err = readerCore.ChannelsGetFullChannel(&mtproto.TLChannelsGetFullChannel{Channel: input})
	if err != nil || full.GetFullChat().To_ChannelFull().GetReadInboxMaxId() != 2 {
		t.Fatalf("read cursor moved backwards: result=%+v err=%v", full, err)
	}

	ownerCore := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: owner}}
	if _, err = ownerCore.ChannelsReadHistory(&mtproto.TLChannelsReadHistory{Channel: input, MaxId: 99}); err != nil {
		t.Fatalf("read through latest message: %v", err)
	}
	full, err = ownerCore.ChannelsGetFullChannel(&mtproto.TLChannelsGetFullChannel{Channel: input})
	if err != nil {
		t.Fatalf("get full channel for owner: %v", err)
	}
	ownerFull := full.GetFullChat().To_ChannelFull()
	if ownerFull.GetReadInboxMaxId() != 3 || ownerFull.GetReadOutboxMaxId() != 3 {
		t.Fatalf("owner cursors: inbox=%d outbox=%d", ownerFull.GetReadInboxMaxId(), ownerFull.GetReadOutboxMaxId())
	}
	if _, err = readerCore.ChannelsReadHistory(&mtproto.TLChannelsReadHistory{Channel: input}); !errors.Is(err, mtproto.ErrMessageIdInvalid) {
		t.Fatalf("empty max id: got %v", err)
	}
}

func TestChannelsMemberLifecycleRoundTrip(t *testing.T) {
	channelID := time.Now().UnixNano()
	owner := channelID%1_000_000_000 + 1_000_000_000
	memberID, joinerID, inviteeID, outsiderID := owner+1, owner+2, owner+3, owner+4
	if err := domain.SaveChannel(domain.Channel{
		ID: channelID, AccessHash: channelID, Creator: owner, Title: "member-lifecycle-test", Username: "member-lifecycle-test", Megagroup: true,
	}); err != nil {
		t.Fatal(err)
	}
	inputChannel := mtproto.MakeTLInputChannel(&mtproto.InputChannel{ChannelId: channelID, AccessHash: channelID}).To_InputChannel()
	userClient := &channelMemberUserClient{}
	ownerCore := &ApiFullCore{
		ctx:    context.Background(),
		svcCtx: &svc.ServiceContext{Dao: &apifullDao.Dao{UserClient: userClient}},
		MD:     &metadata.RpcMetadata{UserId: owner},
		Logger: logx.WithContext(context.Background()),
	}
	coreFor := func(uid int64) *ApiFullCore {
		return &ApiFullCore{ctx: context.Background(), MD: &metadata.RpcMetadata{UserId: uid}, Logger: logx.WithContext(context.Background())}
	}
	inputUser := func(uid int64) *mtproto.InputUser {
		return mtproto.MakeTLInputUser(&mtproto.InputUser{UserId: uid, AccessHash: uid}).To_InputUser()
	}
	inputPeer := func(uid int64) *mtproto.InputPeer {
		return mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: uid, AccessHash: uid}).To_InputPeer()
	}
	if _, err := ownerCore.ChannelsInviteToChannelC9E33D54(&mtproto.TLChannelsInviteToChannelC9E33D54{
		Channel: inputChannel, Users: []*mtproto.InputUser{inputUser(memberID)},
	}); err != nil {
		t.Fatalf("invite member: %v", err)
	}
	member, found, err := domain.LoadChannelMember(channelID, memberID)
	if err != nil || !found || member.InvitedBy != owner {
		t.Fatalf("invited member was not persisted: member=%+v found=%v err=%v", member, found, err)
	}
	memberCore := coreFor(memberID)
	participants, err := memberCore.ChannelsGetParticipants(&mtproto.TLChannelsGetParticipants{Channel: inputChannel, Limit: 50})
	if err != nil || participants.GetCount() != 2 || len(participants.GetParticipants()) != 2 {
		t.Fatalf("member listing: result=%+v err=%v", participants, err)
	}
	lookup, err := ownerCore.ChannelsGetParticipant(&mtproto.TLChannelsGetParticipant{Channel: inputChannel, Participant: inputPeer(memberID)})
	if err != nil || lookup.GetParticipant().GetUserId() != memberID {
		t.Fatalf("member lookup: result=%+v err=%v", lookup, err)
	}
	if _, err = coreFor(outsiderID).ChannelsGetParticipants(&mtproto.TLChannelsGetParticipants{Channel: inputChannel, Limit: 50}); !errors.Is(err, mtproto.ErrUserNotParticipant) {
		t.Fatalf("nonmember listing: got %v", err)
	}
	if _, err = coreFor(outsiderID).ChannelsGetParticipant(&mtproto.TLChannelsGetParticipant{Channel: inputChannel, Participant: inputPeer(memberID)}); !errors.Is(err, mtproto.ErrUserNotParticipant) {
		t.Fatalf("nonmember lookup: got %v", err)
	}
	if _, err = memberCore.ChannelsInviteToChannel199F3A6C(&mtproto.TLChannelsInviteToChannel199F3A6C{
		Channel: inputChannel, Users: []*mtproto.InputUser{inputUser(inviteeID)},
	}); !errors.Is(err, mtproto.ErrChatAdminRequired) {
		t.Fatalf("noncreator invite: got %v", err)
	}
	joinerCore := coreFor(joinerID)
	if _, err = joinerCore.ChannelsJoinChannel24B524C5(&mtproto.TLChannelsJoinChannel24B524C5{Channel: inputChannel}); err != nil {
		t.Fatalf("public channel join: %v", err)
	}
	if _, found, err = domain.LoadChannelMember(channelID, joinerID); err != nil || !found {
		t.Fatalf("joined member was not persisted: found=%v err=%v", found, err)
	}
	if _, err = ownerCore.ChannelsInviteToChannel199F3A6C(&mtproto.TLChannelsInviteToChannel199F3A6C{
		Channel: inputChannel, Users: []*mtproto.InputUser{inputUser(outsiderID)},
	}); err != nil {
		t.Fatalf("invite through layer-229 constructor: %v", err)
	}
	participants, err = joinerCore.ChannelsGetParticipants(&mtproto.TLChannelsGetParticipants{Channel: inputChannel, Limit: 50})
	if err != nil || participants.GetCount() != 4 || len(participants.GetParticipants()) != 4 {
		t.Fatalf("membership list after join/invite: result=%+v err=%v", participants, err)
	}
	if _, err = joinerCore.ChannelsJoinChannel24B524C5(&mtproto.TLChannelsJoinChannel24B524C5{Channel: inputChannel}); !errors.Is(err, mtproto.ErrUserAlreadyParticipant) {
		t.Fatalf("repeat join: got %v", err)
	}
	if _, err = memberCore.ChannelsLeaveChannel(&mtproto.TLChannelsLeaveChannel{Channel: inputChannel}); err != nil {
		t.Fatalf("leave channel: %v", err)
	}
	if _, found, err = domain.LoadChannelMember(channelID, memberID); err != nil || found {
		t.Fatalf("left member remains stored: found=%v err=%v", found, err)
	}
	if _, err = memberCore.ChannelsGetParticipants(&mtproto.TLChannelsGetParticipants{Channel: inputChannel, Limit: 50}); !errors.Is(err, mtproto.ErrUserNotParticipant) {
		t.Fatalf("left member listing: got %v", err)
	}
	joinResult, err := memberCore.ChannelsJoinChannel7F6A1E22(&mtproto.TLChannelsJoinChannel7F6A1E22{Channel: inputChannel})
	if err != nil || joinResult == nil || joinResult.GetUpdates() == nil {
		t.Fatalf("legacy join constructor: result=%+v err=%v", joinResult, err)
	}
	if _, err = memberCore.ChannelsLeaveChannel(&mtproto.TLChannelsLeaveChannel{Channel: inputChannel}); err != nil {
		t.Fatalf("leave after legacy join: %v", err)
	}
	if _, err = coreFor(owner).ChannelsLeaveChannel(&mtproto.TLChannelsLeaveChannel{Channel: inputChannel}); !errors.Is(err, mtproto.ErrUserCreator) {
		t.Fatalf("creator leave: got %v", err)
	}
	badHash := mtproto.MakeTLInputChannel(&mtproto.InputChannel{ChannelId: channelID, AccessHash: channelID + 1}).To_InputChannel()
	if _, err = joinerCore.ChannelsGetParticipants(&mtproto.TLChannelsGetParticipants{Channel: badHash, Limit: 50}); !errors.Is(err, mtproto.ErrChannelInvalid) {
		t.Fatalf("invalid channel access hash: got %v", err)
	}
	privateID := channelID + 1
	if err = domain.SaveChannel(domain.Channel{ID: privateID, AccessHash: privateID, Creator: owner, Title: "private-member-test"}); err != nil {
		t.Fatal(err)
	}
	privateChannel := mtproto.MakeTLInputChannel(&mtproto.InputChannel{ChannelId: privateID, AccessHash: privateID}).To_InputChannel()
	if _, err = joinerCore.ChannelsJoinChannel7F6A1E22(&mtproto.TLChannelsJoinChannel7F6A1E22{Channel: privateChannel}); !errors.Is(err, mtproto.ErrChannelPrivate) {
		t.Fatalf("private channel join: got %v", err)
	}
}

func TestMessagesGetPersonalChannelHistoryRoundTrip(t *testing.T) {
	const viewer, owner int64 = 81301, 81302
	channelID := time.Now().UnixNano()
	if err := domain.SaveChannel(domain.Channel{
		ID: channelID, AccessHash: channelID, Creator: owner, Title: "personal-history-test", Broadcast: true,
	}); err != nil {
		t.Fatal(err)
	}
	for _, text := range []string{"first", "second", "third"} {
		if _, err := channelview.Post(owner, channelID, text, 0); err != nil {
			t.Fatal(err)
		}
	}
	lookup := &personalChannelUserClient{user: &mtproto.ImmutableUser{
		User: &mtproto.UserData{Id: owner, PersonalChannelId: channelID},
	}}
	c := &ApiFullCore{
		ctx: context.Background(),
		MD:  &metadata.RpcMetadata{UserId: viewer},
		svcCtx: &svc.ServiceContext{Dao: &apifullDao.Dao{
			UserClient: lookup,
		}},
	}
	result, err := c.MessagesGetPersonalChannelHistory(&mtproto.TLMessagesGetPersonalChannelHistory{
		UserId: mtproto.MakeTLInputUser(&mtproto.InputUser{UserId: owner, AccessHash: 123}).To_InputUser(),
		Limit:  10,
		MinId:  1,
		MaxId:  4,
	})
	if err != nil {
		t.Fatalf("personal channel history: %v", err)
	}
	if got := result.GetMessages(); len(got) != 2 || got[0].GetMessage() != "third" || got[1].GetMessage() != "second" {
		t.Fatalf("personal channel history: %+v", result)
	}
	if err = result.Encode(mtproto.NewEncodeBuf(4096), 229); err != nil {
		t.Fatalf("encode personal channel history: %v", err)
	}
	page, err := c.MessagesGetPersonalChannelHistory(&mtproto.TLMessagesGetPersonalChannelHistory{
		UserId: mtproto.MakeTLInputUser(&mtproto.InputUser{UserId: owner, AccessHash: 123}).To_InputUser(),
		Limit:  2,
		MaxId:  4,
	})
	if err != nil || page.GetCount() != 3 || len(page.GetMessages()) != 2 {
		t.Fatalf("personal channel page: result=%+v err=%v", page, err)
	}
	if err = page.Encode(mtproto.NewEncodeBuf(4096), 229); err != nil {
		t.Fatalf("encode personal channel page: %v", err)
	}
}

func TestChannelDeleteMethods(t *testing.T) {
	const owner, reader, outsider int64 = 81001, 81002, 81003
	channelID := time.Now().UnixNano()
	if err := domain.SaveChannel(domain.Channel{
		ID:         channelID,
		AccessHash: channelID,
		Creator:    owner,
		Title:      "delete-test",
		Broadcast:  true,
		CreatedAt:  time.Now().Unix(),
	}); err != nil {
		t.Fatal(err)
	}
	if err := domain.JoinChannel(channelID, reader); err != nil {
		t.Fatal(err)
	}
	for i, text := range []string{"first", "second", "third"} {
		updates, err := channelview.Post(owner, channelID, text, 0)
		if err != nil {
			t.Fatal(err)
		}
		if len(updates.GetUpdates()) != 1 || updates.GetUpdates()[0].Pts_INT32 != int32(i+1) || updates.GetUpdates()[0].PtsCount != 1 {
			t.Fatalf("initial sequence update: %+v", updates)
		}
		if err = updates.To_Updates().Encode(mtproto.NewEncodeBuf(4096), 229); err != nil {
			t.Fatalf("encode new channel message: %v", err)
		}
	}
	edited, err := channelview.Edit(owner, channelID, 1, "first edited")
	if err != nil || len(edited.GetUpdates()) != 1 || edited.GetUpdates()[0].Pts_INT32 != 4 {
		t.Fatalf("edit channel message: result=%+v err=%v", edited, err)
	}
	if err = edited.To_Updates().Encode(mtproto.NewEncodeBuf(4096), 229); err != nil {
		t.Fatalf("encode edit channel message: %v", err)
	}
	if _, err := domain.SetChannelMessagePinned(channelID, owner, 1, true); err != nil {
		t.Fatal(err)
	}
	input := mtproto.MakeTLInputChannel(&mtproto.InputChannel{
		ChannelId:  channelID,
		AccessHash: channelID,
	}).To_InputChannel()
	ownerCore := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: owner}}
	readerCore := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: reader}}
	outsiderCore := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: outsider}}

	if _, err := outsiderCore.ChannelsGetFullChannel(&mtproto.TLChannelsGetFullChannel{Channel: input}); !errors.Is(err, mtproto.ErrUserNotParticipant) {
		t.Fatalf("outsider full channel: got %v", err)
	}
	if _, err := outsiderCore.ChannelsDeleteHistory9BAA9647(&mtproto.TLChannelsDeleteHistory9BAA9647{
		Channel: input,
		MaxId:   1,
	}); !errors.Is(err, mtproto.ErrUserNotParticipant) {
		t.Fatalf("outsider private history delete: got %v", err)
	}
	if _, err := outsiderCore.ChannelsDeleteHistoryAF369D42(&mtproto.TLChannelsDeleteHistoryAF369D42{
		Channel: input,
		MaxId:   1,
	}); !errors.Is(err, mtproto.ErrUserNotParticipant) {
		t.Fatalf("outsider legacy history delete: got %v", err)
	}
	outsiderHistory, err := channelview.History(outsider, channelID, 0, 20)
	if err != nil || len(outsiderHistory.GetMessages()) != 3 {
		t.Fatalf("outsider history after rejected deletes: result=%+v err=%v", outsiderHistory, err)
	}

	empty, err := ownerCore.ChannelsDeleteMessages(&mtproto.TLChannelsDeleteMessages{Channel: input})
	if err != nil || empty.GetPtsCount() != 0 {
		t.Fatalf("empty delete: result=%+v err=%v", empty, err)
	}
	ownerHistory, err := channelview.History(owner, channelID, 0, 20)
	if err != nil || len(ownerHistory.GetMessages()) != 3 {
		t.Fatalf("empty delete removed messages: result=%+v err=%v", ownerHistory, err)
	}

	_, err = readerCore.ChannelsDeleteMessages(&mtproto.TLChannelsDeleteMessages{Channel: input, Id: []int32{1}})
	if !errors.Is(err, mtproto.ErrChatAdminRequired) {
		t.Fatalf("non-creator delete: got %v", err)
	}
	deleted, err := ownerCore.ChannelsDeleteMessages(&mtproto.TLChannelsDeleteMessages{Channel: input, Id: []int32{2}})
	if err != nil || deleted.GetPtsCount() != 1 || deleted.GetPts() != 5 {
		t.Fatalf("single delete: result=%+v err=%v", deleted, err)
	}
	if err = deleted.To_MessagesAffectedMessages().Encode(mtproto.NewEncodeBuf(512), 229); err != nil {
		t.Fatalf("encode affected messages: %v", err)
	}

	privateDelete, err := readerCore.ChannelsDeleteHistory9BAA9647(&mtproto.TLChannelsDeleteHistory9BAA9647{
		Channel: input,
		MaxId:   1,
	})
	if err != nil || privateDelete == nil {
		t.Fatalf("private history delete: result=%+v err=%v", privateDelete, err)
	}
	legacyDelete, err := readerCore.ChannelsDeleteHistoryAF369D42(&mtproto.TLChannelsDeleteHistoryAF369D42{
		Channel: input,
		MaxId:   3,
	})
	if err != nil || !mtproto.FromBool(legacyDelete) {
		t.Fatalf("legacy history delete: result=%v err=%v", legacyDelete, err)
	}
	readerHistory, err := channelview.History(reader, channelID, 0, 20)
	if err != nil || len(readerHistory.GetMessages()) != 0 {
		t.Fatalf("reader history after private delete: result=%+v err=%v", readerHistory, err)
	}
	readerByID, err := channelview.MessagesBox(reader, channelID, []int32{1})
	if !errors.Is(err, mtproto.ErrMessageIdInvalid) || readerByID != nil {
		t.Fatalf("reader by id after private delete: result=%+v err=%v", readerByID, err)
	}
	readerSearch, err := channelview.Search(reader, channelID, "", 0, 0, 0, 0, 0, 0, 0, 20)
	if err != nil || len(readerSearch.GetMessages()) != 0 {
		t.Fatalf("reader search after private delete: result=%+v err=%v", readerSearch, err)
	}
	readerPinned, err := channelview.Pinned(reader, channelID, 20)
	if err != nil || len(readerPinned.GetMessages()) != 0 {
		t.Fatalf("reader pinned after private delete: result=%+v err=%v", readerPinned, err)
	}
	if _, err = channelview.Texts(reader, channelID, []int32{1}); !errors.Is(err, mtproto.ErrMessageIdInvalid) {
		t.Fatalf("reader text lookup after private delete: got %v", err)
	}
	readerPresent, err := channelview.PresentIDs(reader, channelID, []int32{1, 3})
	if err != nil || len(readerPresent) != 0 {
		t.Fatalf("reader present ids after private delete: result=%v err=%v", readerPresent, err)
	}
	ownerPinned, err := channelview.Pinned(owner, channelID, 20)
	if err != nil || len(ownerPinned.GetMessages()) != 1 {
		t.Fatalf("private delete affected pinned owner view: result=%+v err=%v", ownerPinned, err)
	}
	ownerHistory, err = channelview.History(owner, channelID, 0, 20)
	if err != nil || len(ownerHistory.GetMessages()) != 2 {
		t.Fatalf("private delete affected owner: result=%+v err=%v", ownerHistory, err)
	}

	globalDelete, err := ownerCore.ChannelsDeleteHistory9BAA9647(&mtproto.TLChannelsDeleteHistory9BAA9647{
		ForEveryone: true,
		Channel:     input,
		MaxId:       3,
	})
	if err != nil || globalDelete == nil || len(globalDelete.GetUpdates()) != 1 {
		t.Fatalf("global history delete: result=%+v err=%v", globalDelete, err)
	}
	update := globalDelete.GetUpdates()[0]
	if update.GetPredicateName() != mtproto.Predicate_updateDeleteChannelMessages || len(update.GetMessages()) != 2 || update.Pts_INT32 != 7 || update.PtsCount != 2 {
		t.Fatalf("global delete update: %+v", update)
	}
	if err = globalDelete.To_Updates().Encode(mtproto.NewEncodeBuf(2048), 229); err != nil {
		t.Fatalf("encode global delete updates: %v", err)
	}
	ownerHistory, err = channelview.History(owner, channelID, 0, 20)
	if err != nil || len(ownerHistory.GetMessages()) != 0 {
		t.Fatalf("owner history after global delete: result=%+v err=%v", ownerHistory, err)
	}
	row, err := domain.InsertChannelMessage(channelID, owner, 0, "after-delete")
	if err != nil || row.MessageID != 4 || row.Pts != 8 {
		t.Fatalf("sequence after deleting history: row=%+v err=%v", row, err)
	}
}
