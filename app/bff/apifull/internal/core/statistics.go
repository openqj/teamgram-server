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
	"encoding/json"
	"fmt"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/apifull/channelview"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/domain"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/persist"
)

// RPCStatisticsServer: Layer 229 methods previously returned ERR_ENTERPRISE_IS_BLOCKED.

func statsInputPeerID(p *mtproto.InputPeer) int64 {
	if p == nil {
		return 0
	}
	if id := p.GetChannelId(); id != 0 {
		return id
	}
	if id := p.GetChatId(); id != 0 {
		return id
	}
	return p.GetUserId()
}

func inputChannelID(ch *mtproto.InputChannel) int64 {
	if ch == nil {
		return 0
	}
	return ch.GetChannelId()
}

func msgViewsKey(userID, peerID int64) string {
	return fmt.Sprintf("mv:%d:%d", userID, peerID)
}

type msgViewStore struct {
	Total int64           `json:"total"`
	Msgs  map[int32]int64 `json:"msgs"`
}

func loadMsgViews(userID, peerID int64) (msgViewStore, error) {
	empty := msgViewStore{Msgs: map[int32]int64{}}
	raw, err := persist.Default.Get(msgViewsKey(userID, peerID))
	if err != nil || raw == "" {
		return empty, err
	}
	var stored msgViewStore
	if json.Unmarshal([]byte(raw), &stored) != nil || stored.Msgs == nil {
		return empty, nil
	}
	return stored, nil
}

func storedStoryViews(userID int64, id int32) (int64, error) {
	stories, err := loadUserStories(userID)
	if err != nil || stories == nil {
		return 0, err
	}
	var total int64
	var one int64
	seen := false
	for sid, item := range stories.Items {
		n := int64(0)
		if item != nil && item.GetViews() != nil {
			n = int64(item.GetViews().GetViewsCount())
		}
		if n == 0 {
			n = int64(len(stories.Viewers[sid]))
		}
		total += n
		if id != 0 && sid == id {
			one = n
			seen = true
		}
	}
	if id != 0 {
		if !seen {
			return int64(len(stories.Viewers[id])), nil
		}
		return one, nil
	}
	return total, nil
}

func storedChannelMemberCount(channelID int64) (int64, error) {
	if channelID == 0 {
		return 0, nil
	}
	members, err := domain.ListChannelMembers(channelID)
	if err != nil {
		return 0, err
	}
	return int64(len(members)), nil
}

func absStat(n int64) *mtproto.StatsAbsValueAndPrev {
	return mtproto.MakeTLStatsAbsValueAndPrev(&mtproto.StatsAbsValueAndPrev{
		Current: float64(n),
	}).To_StatsAbsValueAndPrev()
}

func zeroStat() *mtproto.StatsAbsValueAndPrev {
	return absStat(0)
}

func statPeriod() *mtproto.StatsDateRangeDays {
	return mtproto.MakeTLStatsDateRangeDays(&mtproto.StatsDateRangeDays{}).To_StatsDateRangeDays()
}

func countGraph(n int64) *mtproto.StatsGraph {
	return mtproto.MakeTLStatsGraph(&mtproto.StatsGraph{
		Json: mtproto.MakeTLDataJSON(&mtproto.DataJSON{
			Data: fmt.Sprintf(`{"count":%d}`, n),
		}).To_DataJSON(),
	}).To_StatsGraph()
}

// resolveStatsChannel keeps statistics scoped to a real channel and to an
// owner or persisted administrator. Returning synthetic stats for an
// arbitrary InputChannel would bypass both access-hash and membership checks.
func (c *ApiFullCore) resolveStatsChannel(userID int64, input *mtproto.InputChannel) (domain.Channel, error) {
	channel, err := c.resolveMemberChannel(userID, input, inputChannelID(input))
	if err != nil {
		return domain.Channel{}, err
	}
	if channel.Creator == userID {
		return channel, nil
	}
	if err = c.requireChannelMember(userID, channel.ID); err != nil {
		return domain.Channel{}, err
	}
	member, found, err := domain.LoadChannelMember(channel.ID, userID)
	if err != nil {
		return domain.Channel{}, c.mapChannelMemberError(err)
	}
	if !found || member.AdminRights == nil || member.AdminRights.Empty() {
		return domain.Channel{}, mtproto.ErrChatAdminRequired
	}
	return channel, nil
}

func storyOwner(userID int64, peer *mtproto.InputPeer) int64 {
	if peer == nil || storyPeerOwned(userID, peer) {
		return userID
	}
	if id := peer.GetUserId(); id != 0 {
		return id
	}
	return userID
}

func (c *ApiFullCore) StatsGetBroadcastStats(in *mtproto.TLStatsGetBroadcastStats) (*mtproto.Stats_BroadcastStats, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil || in.GetChannel() == nil {
		return nil, mtproto.ErrChannelInvalid
	}
	channel, err := c.resolveStatsChannel(uid, in.GetChannel())
	if err != nil {
		return nil, err
	}
	channelID := channel.ID
	views, err := loadMsgViews(uid, channelID)
	if err != nil {
		return nil, err
	}
	storyViews, err := storedStoryViews(uid, 0)
	if err != nil {
		return nil, err
	}
	members, err := storedChannelMemberCount(channelID)
	if err != nil {
		return nil, err
	}
	posts := []*mtproto.PostInteractionCounters{}
	messages := []*mtproto.MessageInteractionCounters{}
	for id, n := range views.Msgs {
		posts = append(posts, mtproto.MakeTLPostInteractionCountersMessage(&mtproto.PostInteractionCounters{
			MsgId: id,
			Views: int32(n),
		}).To_PostInteractionCounters())
		messages = append(messages, mtproto.MakeTLMessageInteractionCounters(&mtproto.MessageInteractionCounters{
			MsgId: id,
			Views: int32(n),
		}).To_MessageInteractionCounters())
	}
	out := &mtproto.Stats_BroadcastStats{
		Period:                       statPeriod(),
		Followers:                    absStat(members),
		ViewsPerPost:                 absStat(views.Total),
		SharesPerPost:                zeroStat(),
		ReactionsPerPost:             zeroStat(),
		ViewsPerStory:                absStat(storyViews),
		SharesPerStory:               zeroStat(),
		ReactionsPerStory:            zeroStat(),
		EnabledNotifications:         mtproto.MakeTLStatsPercentValue(&mtproto.StatsPercentValue{}).To_StatsPercentValue(),
		GrowthGraph:                  countGraph(views.Total),
		FollowersGraph:               countGraph(members),
		MuteGraph:                    countGraph(0),
		TopHoursGraph:                countGraph(0),
		InteractionsGraph:            countGraph(views.Total),
		IvInteractionsGraph:          countGraph(0),
		ViewsBySourceGraph:           countGraph(views.Total),
		NewFollowersBySourceGraph:    countGraph(members),
		LanguagesGraph:               countGraph(0),
		ReactionsByEmotionGraph:      countGraph(0),
		StoryInteractionsGraph:       countGraph(storyViews),
		StoryReactionsByEmotionGraph: countGraph(0),
		RecentPostsInteractions:      posts,
		RecentMessageInteractions:    messages,
	}
	return mtproto.MakeTLStatsBroadcastStats(out).To_Stats_BroadcastStats(), nil
}

func (c *ApiFullCore) StatsLoadAsyncGraph(in *mtproto.TLStatsLoadAsyncGraph) (*mtproto.StatsGraph, error) {
	if _, err := c.requireUserId(); err != nil {
		return nil, err
	}
	if in == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	token := in.GetToken()
	if token == "" {
		return nil, mtproto.ErrTokenEmpty
	}
	return nil, mtproto.ErrMethodNotImpl
}

func (c *ApiFullCore) StatsGetMegagroupStats(in *mtproto.TLStatsGetMegagroupStats) (*mtproto.Stats_MegagroupStats, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil || in.GetChannel() == nil {
		return nil, mtproto.ErrChannelInvalid
	}
	channel, err := c.resolveStatsChannel(uid, in.GetChannel())
	if err != nil {
		return nil, err
	}
	channelID := channel.ID
	views, err := loadMsgViews(uid, channelID)
	if err != nil {
		return nil, err
	}
	members, err := storedChannelMemberCount(channelID)
	if err != nil {
		return nil, err
	}
	out := &mtproto.Stats_MegagroupStats{
		Period:                  statPeriod(),
		Members:                 absStat(members),
		Messages:                absStat(views.Total),
		Viewers:                 absStat(views.Total),
		Posters:                 zeroStat(),
		GrowthGraph:             countGraph(members),
		MembersGraph:            countGraph(members),
		NewMembersBySourceGraph: countGraph(members),
		LanguagesGraph:          countGraph(0),
		MessagesGraph:           countGraph(views.Total),
		ActionsGraph:            countGraph(0),
		TopHoursGraph:           countGraph(0),
		WeekdaysGraph:           countGraph(0),
		TopPosters:              []*mtproto.StatsGroupTopPoster{},
		TopAdmins:               []*mtproto.StatsGroupTopAdmin{},
		TopInviters:             []*mtproto.StatsGroupTopInviter{},
		Users:                   []*mtproto.User{},
	}
	return mtproto.MakeTLStatsMegagroupStats(out).To_Stats_MegagroupStats(), nil
}

func (c *ApiFullCore) validatePublicForwardsMessage(userID int64, input *mtproto.InputChannel, msgID int32) error {
	ch, err := c.resolveMemberChannel(userID, input, inputChannelID(input))
	if err != nil {
		return err
	}
	if ch.Creator != userID {
		return mtproto.ErrChatAdminRequired
	}
	_, err = channelview.MessagesBox(userID, ch.ID, []int32{msgID})
	return err
}

func (c *ApiFullCore) StatsGetMessagePublicForwards5F150144(in *mtproto.TLStatsGetMessagePublicForwards5F150144) (*mtproto.Stats_PublicForwards, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil {
		return nil, mtproto.ErrChannelInvalid
	}
	if err = c.validatePublicForwardsMessage(uid, in.GetChannel(), in.GetMsgId()); err != nil {
		return nil, err
	}
	return nil, mtproto.ErrMethodNotImpl
}

func (c *ApiFullCore) StatsGetMessageStats(in *mtproto.TLStatsGetMessageStats) (*mtproto.Stats_MessageStats, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil || in.GetChannel() == nil {
		return nil, mtproto.ErrChannelInvalid
	}
	if in.GetMsgId() <= 0 {
		return nil, mtproto.ErrMessageIdInvalid
	}
	channel, err := c.resolveStatsChannel(uid, in.GetChannel())
	if err != nil {
		return nil, err
	}
	counts, err := domain.ChannelMessageViewCounts(channel.ID, []int32{in.GetMsgId()})
	if err != nil {
		return nil, err
	}
	views, ok := counts[in.GetMsgId()]
	if !ok {
		return nil, mtproto.ErrMessageIdInvalid
	}
	out := &mtproto.Stats_MessageStats{
		ViewsGraph:              countGraph(int64(views)),
		ReactionsByEmotionGraph: countGraph(0),
	}
	return mtproto.MakeTLStatsMessageStats(out).To_Stats_MessageStats(), nil
}

func (c *ApiFullCore) StatsGetStoryStats(in *mtproto.TLStatsGetStoryStats) (*mtproto.Stats_StoryStats, error) {
	_ = in
	if _, err := c.requireUserId(); err != nil {
		return nil, err
	}
	return nil, mtproto.ErrMethodNotImpl
}

func (c *ApiFullCore) StatsGetStoryPublicForwards(in *mtproto.TLStatsGetStoryPublicForwards) (*mtproto.Stats_PublicForwards, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil || in.GetPeer() == nil {
		return nil, mtproto.ErrPeerIdInvalid
	}
	peer := in.GetPeer()
	switch peer.GetPredicateName() {
	case mtproto.Predicate_inputPeerSelf:
	case mtproto.Predicate_inputPeerUser:
		if peer.GetUserId() == 0 {
			return nil, mtproto.ErrPeerIdInvalid
		}
		if peer.GetUserId() != uid {
			return nil, mtproto.ErrChatAdminRequired
		}
	default:
		return nil, mtproto.ErrPeerIdInvalid
	}
	if in.GetId() == 0 {
		return nil, mtproto.ErrStoryIdEmpty
	}
	stories, err := loadUserStories(uid)
	if err != nil {
		return nil, err
	}
	if stories.Items[in.GetId()] == nil {
		return nil, mtproto.ErrStoryIdEmpty
	}
	return nil, mtproto.ErrMethodNotImpl
}

func (c *ApiFullCore) StatsGetPollStats(in *mtproto.TLStatsGetPollStats) (*mtproto.Stats_PollStats, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	if in.GetPeer() == nil {
		return nil, mtproto.ErrPeerIdInvalid
	}
	if in.GetMsgId() <= 0 {
		return nil, mtproto.ErrMessageIdInvalid
	}
	d := c.apifullDao()
	if d == nil || d.PollMessageReader == nil {
		return nil, mtproto.ErrMethodNotImpl
	}
	resolved, err := c.resolvePoll(uid, in.GetPeer(), in.GetMsgId())
	if err != nil {
		return nil, err
	}
	ballots, err := loadBallots(pollBallotKey(resolved.poll.GetId()))
	if err != nil {
		return nil, err
	}
	out := &mtproto.Stats_PollStats{
		VotesGraph: countGraph(int64(len(ballots))),
	}
	return mtproto.MakeTLStatsPollStats(out).To_Stats_PollStats(), nil
}

func (c *ApiFullCore) StatsGetMessagePublicForwards5630281B(in *mtproto.TLStatsGetMessagePublicForwards5630281B) (*mtproto.Messages_Messages, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil {
		return nil, mtproto.ErrChannelInvalid
	}
	if err = c.validatePublicForwardsMessage(uid, in.GetChannel(), in.GetMsgId()); err != nil {
		return nil, err
	}
	return nil, mtproto.ErrMethodNotImpl
}
