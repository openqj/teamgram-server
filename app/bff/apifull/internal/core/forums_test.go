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

package core

import (
	"errors"
	"strconv"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/domain"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/persist"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func TestForumTopicsCreateGetDelete(t *testing.T) {
	_ = domain.DeleteChannel(1, 7)
	_ = domain.DeleteChannel(1, 9)
	t.Cleanup(func() {
		_ = domain.DeleteChannel(1, 7)
		_ = domain.DeleteChannel(1, 9)
	})
	for _, k := range []string{"forum:seq", "forum:1", "forum:channel:7", "forum:channel:9", "forum:channel:9:settings"} {
		if err := persist.Default.Set(k, ""); err != nil {
			t.Fatal(err)
		}
	}
	if err := domain.SaveChannel(domain.Channel{ID: 7, AccessHash: 7007, Creator: 1, Title: "forum-peer", Megagroup: true}); err != nil {
		t.Fatal(err)
	}
	if err := domain.JoinChannel(7, 2); err != nil {
		t.Fatal(err)
	}
	if err := domain.SaveChannel(domain.Channel{ID: 9, AccessHash: 9009, Creator: 1, Title: "forum-channel", Megagroup: true}); err != nil {
		t.Fatal(err)
	}

	anon := &ApiFullCore{}
	if _, err := anon.MessagesCreateForumTopic(&mtproto.TLMessagesCreateForumTopic{
		Title: "x",
		Peer:  mtproto.MakeInputPeerChannel(7),
	}); err != mtproto.ErrAuthKeyUnregistered {
		t.Fatalf("auth: %v", err)
	}

	c := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: 1}}
	peer := mtproto.MakeTLInputPeerChannel(&mtproto.InputPeer{ChannelId: 7, AccessHash: 7007}).To_InputPeer()
	if _, err := c.MessagesCreateForumTopic(&mtproto.TLMessagesCreateForumTopic{
		Title: "topic-a",
		Peer:  peer,
	}); err != nil {
		t.Fatal(err)
	}

	got, err := c.MessagesGetForumTopics(&mtproto.TLMessagesGetForumTopics{Peer: peer})
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || len(got.Topics) != 1 || got.Topics[0].GetTitle() != "topic-a" {
		t.Fatalf("get: %+v", got)
	}
	if got.Topics[0].GetPeer() == nil || got.Topics[0].GetFromId() == nil || got.Topics[0].GetNotifySettings() == nil {
		t.Fatalf("topic is missing required TL fields: %+v", got.Topics[0])
	}
	if err := got.To_MessagesForumTopics().Encode(mtproto.NewEncodeBuf(512), 229); err != nil {
		t.Fatalf("forum topics did not encode: %v", err)
	}
	id := got.Topics[0].GetId()

	if _, err := c.MessagesCreateForumTopic(&mtproto.TLMessagesCreateForumTopic{Title: "topic-b", Peer: peer}); err != nil {
		t.Fatal(err)
	}
	got, err = c.MessagesGetForumTopics(&mtproto.TLMessagesGetForumTopics{Peer: peer})
	if err != nil || got == nil || len(got.Topics) != 2 {
		t.Fatalf("second topic: %+v %v", got, err)
	}
	secondID := got.Topics[1].GetId()

	if _, err := c.MessagesEditForumTopic(&mtproto.TLMessagesEditForumTopic{
		Peer:        peer,
		TopicId:     id,
		Title:       wrapperspb.String("topic-a-edited"),
		IconEmojiId: wrapperspb.Int64(42),
		Closed:      mtproto.BoolTrue,
		Hidden:      mtproto.BoolTrue,
	}); err != nil {
		t.Fatal(err)
	}
	pinned, err := c.MessagesUpdatePinnedForumTopic(&mtproto.TLMessagesUpdatePinnedForumTopic{
		Peer:    peer,
		TopicId: id,
		Pinned:  mtproto.BoolTrue,
	})
	if err != nil || pinned == nil || len(pinned.GetUpdates()) != 1 ||
		pinned.GetUpdates()[0].To_UpdatePinnedForumTopic().GetTopicId_INT32() != id ||
		!pinned.GetUpdates()[0].To_UpdatePinnedForumTopic().GetPinned() {
		t.Fatalf("pin update: %+v %v", pinned, err)
	}
	if _, err := c.MessagesReorderPinnedForumTopics(&mtproto.TLMessagesReorderPinnedForumTopics{
		Peer:  peer,
		Order: []int32{secondID, id},
	}); err != nil {
		t.Fatal(err)
	}
	got, err = c.MessagesGetForumTopics(&mtproto.TLMessagesGetForumTopics{Peer: peer})
	if err != nil || got == nil || len(got.Topics) != 2 || got.Topics[0].GetId() != secondID ||
		got.Topics[1].GetTitle() != "topic-a-edited" || !got.Topics[1].GetClosed() ||
		!got.Topics[1].GetHidden() || !got.Topics[1].GetPinned() || got.Topics[1].GetIconEmojiId().GetValue() != 42 {
		t.Fatalf("edited, pinned, reordered topics: %+v %v", got, err)
	}
	if err := got.To_MessagesForumTopics().Encode(mtproto.NewEncodeBuf(512), 229); err != nil {
		t.Fatalf("edited forum topics did not encode: %v", err)
	}
	other := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: 2}}
	otherGot, err := other.MessagesGetForumTopics(&mtproto.TLMessagesGetForumTopics{Peer: peer})
	if err != nil || otherGot == nil || len(otherGot.Topics) != 2 || otherGot.Topics[1].GetMy() {
		t.Fatalf("topic creator flag leaked to another user: %+v %v", otherGot, err)
	}

	byID, err := c.MessagesGetForumTopicsByID(&mtproto.TLMessagesGetForumTopicsByID{
		Peer:   peer,
		Topics: []int32{id},
	})
	if err != nil {
		t.Fatal(err)
	}
	if byID == nil || len(byID.Topics) != 1 || byID.Topics[0].GetId() != id {
		t.Fatalf("getByID: %+v", byID)
	}

	aff, err := c.MessagesDeleteTopicHistory(&mtproto.TLMessagesDeleteTopicHistory{
		Peer:     peer,
		TopMsgId: id,
	})
	if err != nil {
		t.Fatal(err)
	}
	if aff == nil || aff.GetPtsCount() != 1 {
		t.Fatalf("delete: %+v", aff)
	}

	got, err = c.MessagesGetForumTopics(&mtproto.TLMessagesGetForumTopics{Peer: peer})
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || len(got.Topics) != 1 || got.Topics[0].GetId() != secondID {
		t.Fatalf("after delete: %+v", got)
	}
	if _, err := c.MessagesDeleteTopicHistory(&mtproto.TLMessagesDeleteTopicHistory{Peer: peer, TopMsgId: secondID}); err != nil {
		t.Fatal(err)
	}
	got, err = c.MessagesGetForumTopics(&mtproto.TLMessagesGetForumTopics{Peer: peer})
	if err != nil || got == nil || len(got.Topics) != 0 {
		t.Fatalf("after final delete: %+v %v", got, err)
	}

	ch := mtproto.MakeTLInputChannel(&mtproto.InputChannel{ChannelId: 9, AccessHash: 9009}).To_InputChannel()
	if _, err := c.ChannelsCreateForumTopic(&mtproto.TLChannelsCreateForumTopic{
		Title:   "topic-b",
		Channel: ch,
	}); err != nil {
		t.Fatal(err)
	}
	got, err = c.ChannelsGetForumTopics(&mtproto.TLChannelsGetForumTopics{Channel: ch})
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || len(got.Topics) != 1 || got.Topics[0].GetTitle() != "topic-b" {
		t.Fatalf("channels get: %+v", got)
	}
	channelTopicID := got.Topics[0].GetId()
	if _, err := c.ChannelsEditForumTopic(&mtproto.TLChannelsEditForumTopic{
		Channel: ch,
		TopicId: channelTopicID,
		Title:   wrapperspb.String("channel-topic-edited"),
		Closed:  mtproto.BoolTrue,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := c.ChannelsUpdatePinnedForumTopic(&mtproto.TLChannelsUpdatePinnedForumTopic{
		Channel: ch,
		TopicId: channelTopicID,
		Pinned:  mtproto.BoolTrue,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := c.ChannelsToggleForum(&mtproto.TLChannelsToggleForum{
		Channel: ch,
		Enabled: mtproto.BoolTrue,
		Tabs:    mtproto.BoolTrue,
	}); err != nil {
		t.Fatal(err)
	}
	viewUpdate, err := c.ChannelsToggleViewForumAsMessages(&mtproto.TLChannelsToggleViewForumAsMessages{
		Channel: ch,
		Enabled: mtproto.BoolTrue,
	})
	if err != nil || viewUpdate == nil || len(viewUpdate.GetUpdates()) != 1 ||
		viewUpdate.GetUpdates()[0].GetPredicateName() != mtproto.Predicate_updateChannelViewForumAsMessages {
		t.Fatalf("view-as-messages update: %+v %v", viewUpdate, err)
	}
	settings, err := loadForumSettings("channel:9")
	if err != nil || !settings.Enabled || !settings.Tabs || !settings.ViewAsMessages {
		t.Fatalf("forum settings: %+v %v", settings, err)
	}
	got, err = c.ChannelsGetForumTopics(&mtproto.TLChannelsGetForumTopics{Channel: ch})
	if err != nil || got == nil || len(got.Topics) != 1 || got.Topics[0].GetTitle() != "channel-topic-edited" ||
		!got.Topics[0].GetClosed() || !got.Topics[0].GetPinned() {
		t.Fatalf("channels edited topic: %+v %v", got, err)
	}
	if err := got.To_MessagesForumTopics().Encode(mtproto.NewEncodeBuf(512), 229); err != nil {
		t.Fatalf("channel forum topics did not encode: %v", err)
	}
	if _, err := c.ChannelsDeleteTopicHistory(&mtproto.TLChannelsDeleteTopicHistory{
		Channel:  ch,
		TopMsgId: channelTopicID,
	}); err != nil {
		t.Fatal(err)
	}
	got, err = c.ChannelsGetForumTopics(&mtproto.TLChannelsGetForumTopics{Channel: ch})
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || len(got.Topics) != 0 {
		t.Fatalf("channels after delete: %+v", got)
	}

	if _, err := c.MessagesEditForumTopic(&mtproto.TLMessagesEditForumTopic{}); err != mtproto.ErrPeerIdInvalid {
		t.Fatalf("edit without peer: %v", err)
	}
}

func TestForumProdTopicTitle(t *testing.T) {
	_ = domain.DeleteChannel(1, 3)
	t.Cleanup(func() { _ = domain.DeleteChannel(1, 3) })
	if err := persist.Default.Set("forum:1", ""); err != nil {
		t.Fatal(err)
	}
	if err := persist.Default.Set("forum:channel:3", ""); err != nil {
		t.Fatal(err)
	}
	if err := domain.SaveChannel(domain.Channel{ID: 3, AccessHash: 3003, Creator: 1, Title: "forum-prod", Megagroup: true}); err != nil {
		t.Fatal(err)
	}
	c := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: 1}}
	peer := mtproto.MakeTLInputPeerChannel(&mtproto.InputPeer{ChannelId: 3, AccessHash: 3003}).To_InputPeer()
	if _, err := c.MessagesCreateForumTopic(&mtproto.TLMessagesCreateForumTopic{
		Title: "prod-topic",
		Peer:  peer,
	}); err != nil {
		t.Fatal(err)
	}
	raw, err := persist.Default.Get("forum:1")
	if err != nil || raw != "prod-topic" {
		t.Fatalf("store: %q %v", raw, err)
	}
	got, err := c.MessagesGetForumTopics(&mtproto.TLMessagesGetForumTopics{Peer: peer})
	if err != nil || got == nil || len(got.Topics) == 0 || got.Topics[0].GetTitle() != "prod-topic" {
		t.Fatalf("title: %+v %v", got, err)
	}
}

func TestForumMySQL(t *testing.T) {
	_ = domain.DeleteChannel(12, 12)
	t.Cleanup(func() { _ = domain.DeleteChannel(12, 12) })
	if err := persist.Default.Set("forum:12", ""); err != nil {
		t.Fatal(err)
	}
	if err := persist.Default.Set("forum:channel:12", ""); err != nil {
		t.Fatal(err)
	}
	if err := domain.SaveChannel(domain.Channel{ID: 12, AccessHash: 12012, Creator: 12, Title: "forum-mysql", Megagroup: true}); err != nil {
		t.Fatal(err)
	}
	anon := &ApiFullCore{}
	ch := mtproto.MakeTLInputChannel(&mtproto.InputChannel{ChannelId: 12, AccessHash: 12012}).To_InputChannel()
	if _, err := anon.ChannelsCreateForumTopic(&mtproto.TLChannelsCreateForumTopic{
		Title:   "mysql-topic",
		Channel: ch,
	}); err != mtproto.ErrAuthKeyUnregistered {
		t.Fatalf("auth: %v", err)
	}
	c := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: 12}}
	if _, err := c.ChannelsCreateForumTopic(&mtproto.TLChannelsCreateForumTopic{
		Title:   "mysql-topic",
		Channel: ch,
	}); err != nil {
		t.Fatal(err)
	}
	raw, err := persist.Default.Get("forum:12")
	if err != nil || raw != "mysql-topic" {
		t.Fatalf("store: %q %v", raw, err)
	}
	got, err := c.ChannelsGetForumTopics(&mtproto.TLChannelsGetForumTopics{Channel: ch})
	if err != nil || got == nil || len(got.Topics) == 0 || got.Topics[0].GetTitle() != "mysql-topic" {
		t.Fatalf("title: %+v %v", got, err)
	}
}

func TestForumChannelAuthorization(t *testing.T) {
	channelID := time.Now().UnixNano()
	accessHash := channelID + 1
	owner := channelID%1_000_000_000 + 7_000_000_000
	member, admin, outsider := owner+1, owner+2, owner+3
	if err := domain.SaveChannel(domain.Channel{
		ID: channelID, AccessHash: accessHash, Creator: owner, Title: "forum-authorization", Megagroup: true,
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := domain.DeleteChannel(owner, channelID); err != nil {
			t.Errorf("cleanup channel: %v", err)
		}
	})
	for _, userID := range []int64{member, admin} {
		if err := domain.JoinChannel(channelID, userID); err != nil {
			t.Fatalf("join %d: %v", userID, err)
		}
	}
	if err := domain.EditChannelAdmin(channelID, owner, admin, &domain.ChannelAdminRights{ManageTopics: true}, "forum-admin"); err != nil {
		t.Fatal(err)
	}

	key := "channel:" + strconv.FormatInt(channelID, 10)
	for _, storageKey := range []string{forumStoreKey(key), forumStoreKey(key) + ":settings", forumUserKey(owner), forumUserKey(member), forumUserKey(admin)} {
		if err := persist.Default.Set(storageKey, ""); err != nil {
			t.Fatal(err)
		}
	}
	coreFor := func(userID int64) *ApiFullCore {
		return &ApiFullCore{MD: &metadata.RpcMetadata{UserId: userID}}
	}
	channel := mtproto.MakeTLInputChannel(&mtproto.InputChannel{ChannelId: channelID, AccessHash: accessHash}).To_InputChannel()
	peer := mtproto.MakeTLInputPeerChannel(&mtproto.InputPeer{ChannelId: channelID, AccessHash: accessHash}).To_InputPeer()
	badChannel := mtproto.MakeTLInputChannel(&mtproto.InputChannel{ChannelId: channelID, AccessHash: accessHash + 1}).To_InputChannel()
	badPeer := mtproto.MakeTLInputPeerChannel(&mtproto.InputPeer{ChannelId: channelID, AccessHash: accessHash + 1}).To_InputPeer()

	if _, err := coreFor(owner).ChannelsCreateForumTopic(&mtproto.TLChannelsCreateForumTopic{Channel: channel, Title: "owner topic"}); err != nil {
		t.Fatalf("owner create: %v", err)
	}
	topics, err := coreFor(owner).ChannelsGetForumTopics(&mtproto.TLChannelsGetForumTopics{Channel: channel})
	if err != nil || len(topics.GetTopics()) != 1 {
		t.Fatalf("owner topics: %+v %v", topics, err)
	}
	topicID := topics.GetTopics()[0].GetId()

	for _, call := range []struct {
		name string
		call func(*ApiFullCore) error
	}{
		{"channels.getForumTopics", func(c *ApiFullCore) error {
			_, err := c.ChannelsGetForumTopics(&mtproto.TLChannelsGetForumTopics{Channel: channel})
			return err
		}},
		{"channels.getForumTopicsByID", func(c *ApiFullCore) error {
			_, err := c.ChannelsGetForumTopicsByID(&mtproto.TLChannelsGetForumTopicsByID{Channel: channel, Topics: []int32{topicID}})
			return err
		}},
		{"messages.getForumTopics", func(c *ApiFullCore) error {
			_, err := c.MessagesGetForumTopics(&mtproto.TLMessagesGetForumTopics{Peer: peer})
			return err
		}},
		{"messages.getForumTopicsByID", func(c *ApiFullCore) error {
			_, err := c.MessagesGetForumTopicsByID(&mtproto.TLMessagesGetForumTopicsByID{Peer: peer, Topics: []int32{topicID}})
			return err
		}},
	} {
		if err := call.call(coreFor(member)); err != nil {
			t.Errorf("member %s: %v", call.name, err)
		}
		if err := call.call(coreFor(outsider)); !errors.Is(err, mtproto.ErrUserNotParticipant) {
			t.Errorf("outsider %s: got %v, want USER_NOT_PARTICIPANT", call.name, err)
		}
	}

	for _, call := range []struct {
		name string
		call func(*ApiFullCore) error
	}{
		{"channels.toggleForum", func(c *ApiFullCore) error {
			_, err := c.ChannelsToggleForum(&mtproto.TLChannelsToggleForum{Channel: channel, Enabled: mtproto.BoolTrue, Tabs: mtproto.BoolTrue})
			return err
		}},
		{"channels.toggleViewForumAsMessages", func(c *ApiFullCore) error {
			_, err := c.ChannelsToggleViewForumAsMessages(&mtproto.TLChannelsToggleViewForumAsMessages{Channel: channel, Enabled: mtproto.BoolTrue})
			return err
		}},
		{"channels.createForumTopic", func(c *ApiFullCore) error {
			_, err := c.ChannelsCreateForumTopic(&mtproto.TLChannelsCreateForumTopic{Channel: channel, Title: "blocked"})
			return err
		}},
		{"channels.editForumTopic", func(c *ApiFullCore) error {
			_, err := c.ChannelsEditForumTopic(&mtproto.TLChannelsEditForumTopic{Channel: channel, TopicId: topicID, Title: wrapperspb.String("blocked")})
			return err
		}},
		{"channels.updatePinnedForumTopic", func(c *ApiFullCore) error {
			_, err := c.ChannelsUpdatePinnedForumTopic(&mtproto.TLChannelsUpdatePinnedForumTopic{Channel: channel, TopicId: topicID, Pinned: mtproto.BoolTrue})
			return err
		}},
		{"channels.deleteTopicHistory", func(c *ApiFullCore) error {
			_, err := c.ChannelsDeleteTopicHistory(&mtproto.TLChannelsDeleteTopicHistory{Channel: channel, TopMsgId: topicID})
			return err
		}},
		{"channels.reorderPinnedForumTopics", func(c *ApiFullCore) error {
			_, err := c.ChannelsReorderPinnedForumTopics(&mtproto.TLChannelsReorderPinnedForumTopics{Channel: channel, Order: []int32{topicID}})
			return err
		}},
		{"messages.createForumTopic", func(c *ApiFullCore) error {
			_, err := c.MessagesCreateForumTopic(&mtproto.TLMessagesCreateForumTopic{Peer: peer, Title: "blocked"})
			return err
		}},
		{"messages.editForumTopic", func(c *ApiFullCore) error {
			_, err := c.MessagesEditForumTopic(&mtproto.TLMessagesEditForumTopic{Peer: peer, TopicId: topicID, Title: wrapperspb.String("blocked")})
			return err
		}},
		{"messages.updatePinnedForumTopic", func(c *ApiFullCore) error {
			_, err := c.MessagesUpdatePinnedForumTopic(&mtproto.TLMessagesUpdatePinnedForumTopic{Peer: peer, TopicId: topicID, Pinned: mtproto.BoolTrue})
			return err
		}},
		{"messages.deleteTopicHistory", func(c *ApiFullCore) error {
			_, err := c.MessagesDeleteTopicHistory(&mtproto.TLMessagesDeleteTopicHistory{Peer: peer, TopMsgId: topicID})
			return err
		}},
		{"messages.reorderPinnedForumTopics", func(c *ApiFullCore) error {
			_, err := c.MessagesReorderPinnedForumTopics(&mtproto.TLMessagesReorderPinnedForumTopics{Peer: peer, Order: []int32{topicID}})
			return err
		}},
	} {
		if err := call.call(coreFor(member)); !errors.Is(err, mtproto.ErrChatAdminRequired) {
			t.Errorf("member %s: got %v, want CHAT_ADMIN_REQUIRED", call.name, err)
		}
		if err := call.call(coreFor(outsider)); !errors.Is(err, mtproto.ErrUserNotParticipant) {
			t.Errorf("outsider %s: got %v, want USER_NOT_PARTICIPANT", call.name, err)
		}
	}

	if _, err := coreFor(admin).MessagesEditForumTopic(&mtproto.TLMessagesEditForumTopic{
		Peer: peer, TopicId: topicID, Title: wrapperspb.String("admin topic"),
	}); err != nil {
		t.Fatalf("topic admin write: %v", err)
	}
	if _, err := coreFor(admin).ChannelsToggleForum(&mtproto.TLChannelsToggleForum{
		Channel: channel, Enabled: mtproto.BoolTrue, Tabs: mtproto.BoolTrue,
	}); err != nil {
		t.Fatalf("topic admin toggle: %v", err)
	}

	for _, call := range []struct {
		name string
		call func() error
	}{
		{"channels bad hash", func() error {
			_, err := coreFor(owner).ChannelsGetForumTopics(&mtproto.TLChannelsGetForumTopics{Channel: badChannel})
			return err
		}},
		{"messages bad hash", func() error {
			_, err := coreFor(owner).MessagesGetForumTopics(&mtproto.TLMessagesGetForumTopics{Peer: badPeer})
			return err
		}},
		{"channels missing", func() error {
			_, err := coreFor(owner).ChannelsGetForumTopics(&mtproto.TLChannelsGetForumTopics{Channel: mtproto.MakeTLInputChannel(&mtproto.InputChannel{ChannelId: channelID + 1, AccessHash: accessHash}).To_InputChannel()})
			return err
		}},
		{"messages missing", func() error {
			_, err := coreFor(owner).MessagesGetForumTopics(&mtproto.TLMessagesGetForumTopics{Peer: mtproto.MakeTLInputPeerChannel(&mtproto.InputPeer{ChannelId: channelID + 1, AccessHash: accessHash}).To_InputPeer()})
			return err
		}},
	} {
		if err := call.call(); !errors.Is(err, mtproto.ErrChannelInvalid) {
			t.Errorf("%s: got %v, want CHANNEL_INVALID", call.name, err)
		}
	}
}
