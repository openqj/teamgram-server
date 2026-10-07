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
	"bytes"
	"encoding/json"
	"fmt"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/persist"
	chatpb "github.com/teamgram/teamgram-server/app/service/biz/chat/chat"
	messagepb "github.com/teamgram/teamgram-server/app/service/biz/message/message"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// RPCPollsServer: Layer 229 methods previously returned ERR_ENTERPRISE_IS_BLOCKED.

type pollBallot struct {
	userId  int64
	options [][]byte
	date    int32
}

// pollBallotJSON is the persisted form. Byte options encode as base64.
type pollBallotJSON struct {
	UserId  int64    `json:"userId"`
	Options [][]byte `json:"options"`
	Date    int32    `json:"date"`
}

func pollStoreKey(peer *mtproto.InputPeer, msgId int32) string {
	if peer == nil {
		return fmt.Sprintf(":::%d", msgId)
	}
	return fmt.Sprintf("%d:%d:%d:%s:%d", peer.UserId, peer.ChatId, peer.ChannelId, peer.Username, msgId)
}

func copyOptions(opts [][]byte) [][]byte {
	if len(opts) == 0 {
		return nil
	}
	out := make([][]byte, len(opts))
	for i, o := range opts {
		out[i] = append([]byte(nil), o...)
	}
	return out
}

func loadBallots(key string) ([]pollBallot, error) {
	raw, err := persist.Default.Get(key)
	if err != nil || raw == "" {
		return nil, err
	}
	var stored []pollBallotJSON
	if err := json.Unmarshal([]byte(raw), &stored); err != nil {
		return nil, err
	}
	out := make([]pollBallot, len(stored))
	for i, v := range stored {
		out[i] = pollBallot{userId: v.UserId, date: v.Date, options: copyOptions(v.Options)}
	}
	return out, nil
}

func saveBallots(key string, list []pollBallot) error {
	stored := make([]pollBallotJSON, len(list))
	for i, v := range list {
		stored[i] = pollBallotJSON{UserId: v.userId, Options: copyOptions(v.options), Date: v.date}
	}
	b, err := json.Marshal(stored)
	if err != nil {
		return err
	}
	return persist.Default.Set(key, string(b))
}

func castPoll(key string, userId int64, options [][]byte) error {
	list, err := loadBallots(key)
	if err != nil {
		return err
	}
	copied := copyOptions(options)
	now := int32(time.Now().Unix())
	for i, v := range list {
		if v.userId != userId {
			continue
		}
		if len(copied) == 0 {
			next := make([]pollBallot, 0, len(list)-1)
			next = append(next, list[:i]...)
			next = append(next, list[i+1:]...)
			return saveBallots(key, next)
		}
		list[i].options = copied
		list[i].date = now
		return saveBallots(key, list)
	}
	if len(copied) == 0 {
		return nil
	}
	return saveBallots(key, append(list, pollBallot{userId: userId, options: copied, date: now}))
}

func pollPeer(p *mtproto.InputPeer) *mtproto.Peer {
	if p == nil {
		return nil
	}
	switch {
	case p.ChannelId != 0:
		return mtproto.MakePeerChannel(p.ChannelId)
	case p.ChatId != 0:
		return mtproto.MakePeerChat(p.ChatId)
	case p.UserId != 0:
		return mtproto.MakePeerUser(p.UserId)
	default:
		return mtproto.MakeTLPeerUser(&mtproto.Peer{}).To_Peer()
	}
}

func ballotsToResults(ballots []pollBallot, viewer int64) *mtproto.PollResults {
	type acc struct {
		opt          []byte
		voters       int32
		chosen       bool
		recentVoters []*mtproto.Peer
	}
	var order []string
	counts := map[string]*acc{}
	for _, b := range ballots {
		seen := map[string]struct{}{}
		for _, opt := range b.options {
			k := string(opt)
			if _, dup := seen[k]; dup {
				continue
			}
			seen[k] = struct{}{}
			a, ok := counts[k]
			if !ok {
				a = &acc{opt: append([]byte(nil), opt...)}
				counts[k] = a
				order = append(order, k)
			}
			a.voters++
			if b.userId == viewer {
				a.chosen = true
			}
			if len(a.recentVoters) < 3 {
				a.recentVoters = append(a.recentVoters, mtproto.MakePeerUser(b.userId))
			}
		}
	}
	results := make([]*mtproto.PollAnswerVoters, 0, len(order))
	for _, k := range order {
		a := counts[k]
		result := &mtproto.PollAnswerVoters{Chosen: a.chosen, Option: a.opt}
		if a.voters > 0 {
			result.Voters_FLAGINT32 = wrapperspb.Int32(a.voters)
			result.RecentVoters = a.recentVoters
		}
		results = append(results, mtproto.MakeTLPollAnswerVoters(result).To_PollAnswerVoters())
	}
	return mtproto.MakeTLPollResults(&mtproto.PollResults{
		Results:     results,
		TotalVoters: wrapperspb.Int32(int32(len(ballots))),
	}).To_PollResults()
}

func pollQuestionKey(userId int64) string {
	return fmt.Sprintf("poll:%d", userId)
}

func savePollQuestion(userId int64, question string) error {
	if question == "" {
		return nil
	}
	return persist.Default.Set(pollQuestionKey(userId), question)
}

func loadPollQuestion(userId int64) (string, error) {
	q, err := persist.Default.Get(pollQuestionKey(userId))
	if err != nil || q == "" {
		return "", err
	}
	return q, nil
}

func firstOptionText(opts [][]byte) string {
	for _, o := range opts {
		if len(o) > 0 {
			return string(o)
		}
	}
	return ""
}

func answerQuestion(a *mtproto.PollAnswer) string {
	if a == nil {
		return ""
	}
	if t := a.GetText(); t != "" {
		return t
	}
	if t := a.GetText_STRING(); t != "" {
		return t
	}
	if t := a.GetText_TEXTWITHENTITIES(); t != nil {
		return t.GetText()
	}
	return ""
}

func pollWithQuestion(q string) *mtproto.Poll {
	if q == "" {
		return nil
	}
	return mtproto.MakeTLPoll(&mtproto.Poll{Question: q}).To_Poll().FixData()
}

func pollUpdates(msgId int32, peer *mtproto.InputPeer, ballots []pollBallot, viewer int64, question string) *mtproto.Updates {
	if len(ballots) == 0 && question == "" {
		return mtproto.MakeEmptyUpdates()
	}
	return mtproto.MakeUpdatesByUpdates(mtproto.MakeTLUpdateMessagePoll(&mtproto.Update{
		PollId:          int64(msgId),
		Poll:            pollWithQuestion(question),
		Peer_PEER:       pollPeer(peer),
		MsgId_FLAGINT32: wrapperspb.Int32(msgId),
		Results:         ballotsToResults(ballots, viewer),
	}).To_Update())
}

func hasOption(opts [][]byte, want []byte) bool {
	for _, o := range opts {
		if bytes.Equal(o, want) {
			return true
		}
	}
	return false
}

func userVote(b pollBallot) *mtproto.MessageUserVote {
	if len(b.options) > 1 {
		return mtproto.MakeTLMessageUserVoteMultiple(&mtproto.MessageUserVote{
			UserId:  b.userId,
			Options: b.options,
			Date:    b.date,
		}).To_MessageUserVote()
	}
	var opt []byte
	if len(b.options) == 1 {
		opt = b.options[0]
	}
	return mtproto.MakeTLMessageUserVote(&mtproto.MessageUserVote{
		UserId: b.userId,
		Option: opt,
		Date:   b.date,
	}).To_MessageUserVote()
}

func peerVote(b pollBallot) *mtproto.MessagePeerVote {
	if len(b.options) > 1 {
		return mtproto.MakeTLMessagePeerVoteMultiple(&mtproto.MessagePeerVote{
			Peer:    mtproto.MakePeerUser(b.userId),
			Options: b.options,
			Date:    b.date,
		}).To_MessagePeerVote()
	}
	var opt []byte
	if len(b.options) == 1 {
		opt = b.options[0]
	}
	return mtproto.MakeTLMessagePeerVote(&mtproto.MessagePeerVote{
		Peer:   mtproto.MakePeerUser(b.userId),
		Option: opt,
		Date:   b.date,
	}).To_MessagePeerVote()
}

type resolvedPoll struct {
	poll  *mtproto.Poll
	box   *mtproto.MessageBox
	peer  *mtproto.InputPeer
	msgID int32
}

func pollBallotKey(pollID int64) string {
	return fmt.Sprintf("poll:ballot:%d", pollID)
}

func pollQuestionText(poll *mtproto.Poll) string {
	if poll == nil {
		return ""
	}
	if question := poll.GetQuestion_TEXTWITHENTITIES(); question != nil {
		return question.GetText()
	}
	if question := poll.GetQuestion(); question != "" {
		return question
	}
	return poll.GetQuestion_STRING()
}

func (c *ApiFullCore) authorizePollPeer(userID int64, peer *mtproto.InputPeer) (int32, int64, error) {
	if peer == nil {
		return 0, 0, mtproto.ErrPeerIdInvalid
	}
	peerType, peerID := apifullPeerTypeID(userID, peer)
	if peerID <= 0 {
		return 0, 0, mtproto.ErrPeerIdInvalid
	}
	d := c.apifullDao()
	switch peerType {
	case mtproto.PEER_SELF:
		return peerType, peerID, nil
	case mtproto.PEER_USER:
		if peer.GetPredicateName() != mtproto.Predicate_inputPeerUser || peer.GetAccessHash() == 0 || d == nil || d.UserClient == nil {
			return 0, 0, mtproto.ErrPeerIdInvalid
		}
		users, err := d.UserClient.UserGetMutableUsers(c.ctx, &userpb.TLUserGetMutableUsers{
			Id: []int64{peerID},
			To: []int64{userID},
		})
		if err != nil {
			return 0, 0, err
		}
		if users == nil {
			return 0, 0, mtproto.ErrInternalServerError
		}
		for _, user := range users.GetDatas() {
			if user != nil && user.GetUser() != nil && !user.GetUser().GetDeleted() &&
				user.GetUser().GetId() == peerID && user.GetUser().GetAccessHash() == peer.GetAccessHash() {
				return peerType, peerID, nil
			}
		}
		return 0, 0, mtproto.ErrPeerIdInvalid
	case mtproto.PEER_CHAT:
		if peer.GetPredicateName() != mtproto.Predicate_inputPeerChat || d == nil || d.ChatClient == nil {
			return 0, 0, mtproto.ErrPeerIdInvalid
		}
		chat, err := d.ChatClient.ChatGetMutableChat(c.ctx, &chatpb.TLChatGetMutableChat{ChatId: peerID})
		if err != nil {
			return 0, 0, err
		}
		if chat == nil || chat.GetChat() == nil || chat.GetChat().GetId() != peerID {
			return 0, 0, mtproto.ErrPeerIdInvalid
		}
		member, ok := chat.GetImmutableChatParticipant(userID)
		if !ok || member == nil || !member.IsChatMemberStateNormal() {
			return 0, 0, mtproto.ErrUserNotParticipant
		}
		return peerType, peerID, nil
	case mtproto.PEER_CHANNEL:
		if err := c.authorizeReactionPeer(userID, peer); err != nil {
			return 0, 0, err
		}
		return peerType, peerID, nil
	default:
		return 0, 0, mtproto.ErrPeerIdInvalid
	}
}

func pollBoxMatches(peerType int32, peerID int64, box *mtproto.MessageBox) bool {
	if box == nil {
		return false
	}
	if peerType == mtproto.PEER_SELF {
		return box.GetPeerId() == peerID && (box.GetPeerType() == mtproto.PEER_SELF || box.GetPeerType() == mtproto.PEER_USER)
	}
	return box.GetPeerType() == peerType && box.GetPeerId() == peerID
}

func (c *ApiFullCore) resolvePoll(userID int64, peer *mtproto.InputPeer, msgID int32) (*resolvedPoll, error) {
	if msgID <= 0 {
		return nil, mtproto.ErrMessageIdInvalid
	}
	peerType, peerID, err := c.authorizePollPeer(userID, peer)
	if err != nil {
		return nil, err
	}
	d := c.apifullDao()
	if d == nil || d.PollMessageReader == nil {
		return nil, mtproto.ErrInternalServerError
	}
	box, err := d.PollMessageReader.MessageGetUserMessage(c.ctx, &messagepb.TLMessageGetUserMessage{
		UserId: userID,
		Id:     msgID,
	})
	if err != nil {
		return nil, err
	}
	if box == nil || box.GetMessageId() != msgID || box.GetMessage() == nil || box.GetMessage().GetId() != msgID || !pollBoxMatches(peerType, peerID, box) {
		return nil, mtproto.ErrMessageIdInvalid
	}
	media := box.GetMessage().GetMedia()
	if media == nil || media.GetPredicateName() != mtproto.Predicate_messageMediaPoll || media.GetPoll() == nil || media.GetPoll().GetId() == 0 {
		return nil, mtproto.ErrMessageIdInvalid
	}
	return &resolvedPoll{poll: media.GetPoll(), box: box, peer: peer, msgID: msgID}, nil
}

func (r *resolvedPoll) closed(now int32) bool {
	if r == nil || r.poll == nil || r.poll.GetClosed() {
		return true
	}
	if closeDate := r.poll.GetCloseDate(); closeDate != nil && closeDate.GetValue() <= now {
		return true
	}
	if closePeriod := r.poll.GetClosePeriod(); closePeriod != nil && r.box != nil && r.box.GetMessage().GetDate() > 0 && r.box.GetMessage().GetDate()+closePeriod.GetValue() <= now {
		return true
	}
	return false
}

func pollResultsFor(poll *mtproto.Poll, ballots []pollBallot, viewer int64) *mtproto.PollResults {
	type count struct {
		option       []byte
		voters       int32
		chosen       bool
		recentVoters []*mtproto.Peer
	}
	answers := poll.GetAnswers()
	counts := make(map[string]*count, len(answers))
	order := make([]string, 0, len(answers))
	for _, answer := range answers {
		if answer == nil || len(answer.GetOption()) == 0 {
			continue
		}
		key := string(answer.GetOption())
		if _, exists := counts[key]; exists {
			continue
		}
		counts[key] = &count{option: append([]byte(nil), answer.GetOption()...)}
		order = append(order, key)
	}
	var total int32
	for _, ballot := range ballots {
		voted := false
		seen := make(map[string]struct{}, len(ballot.options))
		for _, option := range ballot.options {
			key := string(option)
			if _, duplicate := seen[key]; duplicate {
				continue
			}
			seen[key] = struct{}{}
			entry := counts[key]
			if entry == nil {
				continue
			}
			voted = true
			entry.voters++
			if ballot.userId == viewer {
				entry.chosen = true
			}
			if len(entry.recentVoters) < 3 {
				entry.recentVoters = append(entry.recentVoters, mtproto.MakePeerUser(ballot.userId))
			}
		}
		if voted {
			total++
		}
	}
	results := make([]*mtproto.PollAnswerVoters, 0, len(order))
	for _, key := range order {
		entry := counts[key]
		result := &mtproto.PollAnswerVoters{Chosen: entry.chosen, Option: entry.option}
		if entry.voters > 0 {
			result.Voters_FLAGINT32 = wrapperspb.Int32(entry.voters)
			result.RecentVoters = entry.recentVoters
		}
		results = append(results, mtproto.MakeTLPollAnswerVoters(result).To_PollAnswerVoters())
	}
	return mtproto.MakeTLPollResults(&mtproto.PollResults{
		Results:     results,
		TotalVoters: wrapperspb.Int32(total),
	}).To_PollResults()
}

func pollUpdatesFor(resolved *resolvedPoll, ballots []pollBallot, viewer int64) *mtproto.Updates {
	return mtproto.MakeUpdatesByUpdates(mtproto.MakeTLUpdateMessagePoll(&mtproto.Update{
		PollId:          resolved.poll.GetId(),
		Poll:            resolved.poll,
		Peer_PEER:       pollPeer(resolved.peer),
		MsgId_FLAGINT32: wrapperspb.Int32(resolved.msgID),
		Results:         pollResultsFor(resolved.poll, ballots, viewer),
	}).To_Update())
}

func validPollOptions(poll *mtproto.Poll, options [][]byte) error {
	seen := make(map[string]struct{}, len(options))
	for _, option := range options {
		if len(option) == 0 || !hasPollOption(poll, option) {
			return mtproto.ErrPollOptionInvalid
		}
		key := string(option)
		if _, duplicate := seen[key]; duplicate {
			return mtproto.ErrPollOptionDuplicate
		}
		seen[key] = struct{}{}
	}
	if !poll.GetMultipleChoice() && len(options) > 1 {
		return mtproto.ErrPollAnswerInvalid
	}
	return nil
}

func hasPollOption(poll *mtproto.Poll, option []byte) bool {
	for _, answer := range poll.GetAnswers() {
		if answer != nil && bytes.Equal(answer.GetOption(), option) {
			return true
		}
	}
	return false
}

func (c *ApiFullCore) MessagesSendVote(in *mtproto.TLMessagesSendVote) (*mtproto.Updates, error) {
	userId, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	resolved, err := c.resolvePoll(userId, in.GetPeer(), in.GetMsgId())
	if err != nil {
		return nil, err
	}
	if resolved.closed(int32(time.Now().Unix())) {
		return nil, mtproto.ErrMessagePollClosed
	}
	if err := validPollOptions(resolved.poll, in.GetOptions()); err != nil {
		return nil, err
	}
	key := pollBallotKey(resolved.poll.GetId())
	if err := castPoll(key, userId, in.Options); err != nil {
		return nil, err
	}
	ballots, err := loadBallots(key)
	if err != nil {
		return nil, err
	}
	if err := rememberUnreadPoll(userId, resolved.peer, resolved.msgID, pollQuestionText(resolved.poll), len(in.Options) > 0); err != nil {
		return nil, err
	}
	return pollUpdatesFor(resolved, ballots, userId), nil
}

func (c *ApiFullCore) MessagesGetPollResults(in *mtproto.TLMessagesGetPollResults) (*mtproto.Updates, error) {
	userId, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	resolved, err := c.resolvePoll(userId, in.GetPeer(), in.GetMsgId())
	if err != nil {
		return nil, err
	}
	ballots, err := loadBallots(pollBallotKey(resolved.poll.GetId()))
	if err != nil {
		return nil, err
	}
	return pollUpdatesFor(resolved, ballots, userId), nil
}

func (c *ApiFullCore) MessagesGetPollVotes(in *mtproto.TLMessagesGetPollVotes) (*mtproto.Messages_VotesList, error) {
	userId, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	resolved, err := c.resolvePoll(userId, in.GetPeer(), in.GetId())
	if err != nil {
		return nil, err
	}
	if !resolved.poll.GetPublicVoters() {
		return nil, mtproto.ErrPollVoteRequired
	}
	if len(in.GetOption()) > 0 && !hasPollOption(resolved.poll, in.GetOption()) {
		return nil, mtproto.ErrPollOptionInvalid
	}
	ballots, err := loadBallots(pollBallotKey(resolved.poll.GetId()))
	if err != nil {
		return nil, err
	}
	peerVotes := make([]*mtproto.MessagePeerVote, 0)
	for _, b := range ballots {
		if len(in.GetOption()) > 0 && !hasOption(b.options, in.GetOption()) {
			continue
		}
		peerVotes = append(peerVotes, peerVote(b))
	}
	count := int32(len(peerVotes))
	if limit := in.GetLimit(); limit > 0 && int(limit) < len(peerVotes) {
		peerVotes = peerVotes[:limit]
	}
	return mtproto.MakeTLMessagesVotesList(&mtproto.Messages_VotesList{
		Count:                       count,
		Votes_VECTORMESSAGEPEERVOTE: peerVotes,
		Chats:                       []*mtproto.Chat{},
		Users:                       []*mtproto.User{},
	}).To_Messages_VotesList(), nil
}

type pollAnswerJSON struct {
	Text   string `json:"text"`
	Option []byte `json:"option"`
}

func pollAnswerKey(peer *mtproto.InputPeer, msgId int32) string {
	return "poll:ans:" + pollStoreKey(peer, msgId)
}

func loadPollAnswers(key string) ([]pollAnswerJSON, error) {
	raw, err := persist.Default.Get(key)
	if err != nil || raw == "" {
		return nil, err
	}
	var list []pollAnswerJSON
	if err := json.Unmarshal([]byte(raw), &list); err != nil {
		return nil, err
	}
	return list, nil
}

func savePollAnswers(key string, list []pollAnswerJSON) error {
	if list == nil {
		list = []pollAnswerJSON{}
	}
	b, err := json.Marshal(list)
	if err != nil {
		return err
	}
	return persist.Default.Set(key, string(b))
}

func (c *ApiFullCore) MessagesAddPollAnswer(in *mtproto.TLMessagesAddPollAnswer) (*mtproto.Updates, error) {
	userId, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil {
		return mtproto.MakeEmptyUpdates(), nil
	}
	key := pollAnswerKey(in.GetPeer(), in.GetMsgId())
	list, err := loadPollAnswers(key)
	if err != nil {
		return nil, err
	}
	var text string
	var opt []byte
	if a := in.GetAnswer(); a != nil {
		opt = append([]byte(nil), a.GetOption()...)
		text = answerQuestion(a)
	}
	if err := savePollQuestion(userId, text); err != nil {
		return nil, err
	}
	if text == "" {
		text, err = loadPollQuestion(userId)
		if err != nil {
			return nil, err
		}
	}
	list = append(list, pollAnswerJSON{Text: text, Option: opt})
	if err := savePollAnswers(key, list); err != nil {
		return nil, err
	}
	ballots, err := loadBallots("poll:" + pollStoreKey(in.GetPeer(), in.GetMsgId()))
	if err != nil {
		return nil, err
	}
	return pollUpdates(in.GetMsgId(), in.GetPeer(), ballots, userId, text), nil
}

func (c *ApiFullCore) MessagesDeletePollAnswer(in *mtproto.TLMessagesDeletePollAnswer) (*mtproto.Updates, error) {
	userId, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil {
		return mtproto.MakeEmptyUpdates(), nil
	}
	key := pollAnswerKey(in.GetPeer(), in.GetMsgId())
	list, err := loadPollAnswers(key)
	if err != nil {
		return nil, err
	}
	next := list[:0]
	for _, a := range list {
		if !bytes.Equal(a.Option, in.GetOption()) {
			next = append(next, a)
		}
	}
	if err := savePollAnswers(key, next); err != nil {
		return nil, err
	}
	ballots, err := loadBallots("poll:" + pollStoreKey(in.GetPeer(), in.GetMsgId()))
	if err != nil {
		return nil, err
	}
	question, err := loadPollQuestion(userId)
	if err != nil {
		return nil, err
	}
	return pollUpdates(in.GetMsgId(), in.GetPeer(), ballots, userId, question), nil
}

type unreadPollJSON struct {
	U        int64  `json:"u,omitempty"`
	C        int64  `json:"c,omitempty"`
	Ch       int64  `json:"ch,omitempty"`
	Name     string `json:"n,omitempty"`
	MsgId    int32  `json:"msgId"`
	Question string `json:"q,omitempty"`
}

type pollReadMark struct {
	All  bool   `json:"all,omitempty"`
	U    int64  `json:"u,omitempty"`
	C    int64  `json:"c,omitempty"`
	Ch   int64  `json:"ch,omitempty"`
	Name string `json:"n,omitempty"`
}

func unreadPollKey(userId int64) string {
	return fmt.Sprintf("poll:unread:%d", userId)
}

func pollReadMarksKey(userId int64) string {
	return fmt.Sprintf("poll:readmarks:%d", userId)
}

func loadUnreadPolls(userId int64) ([]unreadPollJSON, error) {
	raw, err := persist.Default.Get(unreadPollKey(userId))
	if err != nil || raw == "" {
		return nil, err
	}
	var list []unreadPollJSON
	if err := json.Unmarshal([]byte(raw), &list); err != nil {
		return nil, err
	}
	return list, nil
}

func saveUnreadPolls(userId int64, list []unreadPollJSON) error {
	if list == nil {
		list = []unreadPollJSON{}
	}
	b, err := json.Marshal(list)
	if err != nil {
		return err
	}
	return persist.Default.Set(unreadPollKey(userId), string(b))
}

func loadPollReadMarks(userId int64) ([]pollReadMark, error) {
	raw, err := persist.Default.Get(pollReadMarksKey(userId))
	if err != nil || raw == "" {
		return nil, err
	}
	var list []pollReadMark
	if err := json.Unmarshal([]byte(raw), &list); err != nil {
		return nil, err
	}
	return list, nil
}

func savePollReadMarks(userId int64, list []pollReadMark) error {
	if list == nil {
		list = []pollReadMark{}
	}
	b, err := json.Marshal(list)
	if err != nil {
		return err
	}
	return persist.Default.Set(pollReadMarksKey(userId), string(b))
}

func unreadFromPeer(peer *mtproto.InputPeer, msgId int32, question string) unreadPollJSON {
	row := unreadPollJSON{MsgId: msgId, Question: question}
	if peer != nil {
		row.U, row.C, row.Ch, row.Name = peer.UserId, peer.ChatId, peer.ChannelId, peer.Username
	}
	return row
}

func sameUnreadPoll(a, b unreadPollJSON) bool {
	return a.U == b.U && a.C == b.C && a.Ch == b.Ch && a.Name == b.Name && a.MsgId == b.MsgId
}

func rememberUnreadPoll(userId int64, peer *mtproto.InputPeer, msgId int32, question string, voted bool) error {
	list, err := loadUnreadPolls(userId)
	if err != nil {
		return err
	}
	row := unreadFromPeer(peer, msgId, question)
	next := list[:0]
	for _, it := range list {
		if sameUnreadPoll(it, row) {
			continue
		}
		next = append(next, it)
	}
	if voted {
		next = append(next, row)
	}
	return saveUnreadPolls(userId, next)
}

func unreadPollPeerMatch(p *mtproto.InputPeer, e unreadPollJSON) bool {
	if p == nil || (p.UserId == 0 && p.ChatId == 0 && p.ChannelId == 0 && p.Username == "") {
		return true
	}
	return p.UserId == e.U && p.ChatId == e.C && p.ChannelId == e.Ch && p.Username == e.Name
}

func markFromPeer(peer *mtproto.InputPeer) pollReadMark {
	if peer == nil || (peer.UserId == 0 && peer.ChatId == 0 && peer.ChannelId == 0 && peer.Username == "") {
		return pollReadMark{All: true}
	}
	return pollReadMark{U: peer.UserId, C: peer.ChatId, Ch: peer.ChannelId, Name: peer.Username}
}

func pollMessage(e unreadPollJSON) *mtproto.Message {
	// The unread-poll response is decoded by regular Telegram clients, so it
	// must carry the complete Poll constructor rather than only its question.
	// Older unread rows do not persist the original answer labels; stable
	// synthetic answers keep the response structurally valid without changing
	// the ballot ledger.
	poll := mtproto.MakeTLPoll(&mtproto.Poll{
		Id:           int64(e.MsgId),
		Hash:         int64(e.MsgId),
		PublicVoters: true,
		Question_TEXTWITHENTITIES: mtproto.MakeTLTextWithEntities(&mtproto.TextWithEntities{
			Text: e.Question,
		}).To_TextWithEntities(),
		Answers: []*mtproto.PollAnswer{
			mtproto.MakeTLPollAnswer(&mtproto.PollAnswer{
				Text_TEXTWITHENTITIES: mtproto.MakeTLTextWithEntities(&mtproto.TextWithEntities{Text: "One"}).To_TextWithEntities(),
				Option:                []byte{0},
			}).To_PollAnswer(),
			mtproto.MakeTLPollAnswer(&mtproto.PollAnswer{
				Text_TEXTWITHENTITIES: mtproto.MakeTLTextWithEntities(&mtproto.TextWithEntities{Text: "Two"}).To_TextWithEntities(),
				Option:                []byte{1},
			}).To_PollAnswer(),
		},
	}).To_Poll()
	results := make([]*mtproto.PollAnswerVoters, 0, len(poll.GetAnswers()))
	for _, answer := range poll.GetAnswers() {
		results = append(results, mtproto.MakeTLPollAnswerVoters(&mtproto.PollAnswerVoters{
			Option: append([]byte(nil), answer.GetOption()...),
		}).To_PollAnswerVoters())
	}
	return mtproto.MakeTLMessage(&mtproto.Message{
		Id:     e.MsgId,
		PeerId: pollPeer(&mtproto.InputPeer{UserId: e.U, ChatId: e.C, ChannelId: e.Ch, Username: e.Name}),
		Media: mtproto.MakeTLMessageMediaPoll(&mtproto.MessageMedia{
			Poll: poll,
			Results: mtproto.MakeTLPollResults(&mtproto.PollResults{
				Results:     results,
				TotalVoters: wrapperspb.Int32(0),
			}).To_PollResults(),
		}).To_MessageMedia(),
	}).To_Message()
}

func (c *ApiFullCore) MessagesGetUnreadPollVotes(in *mtproto.TLMessagesGetUnreadPollVotes) (*mtproto.Messages_Messages, error) {
	msgs := []*mtproto.Message{}
	if c != nil && c.MD != nil && c.MD.UserId != 0 {
		list, err := loadUnreadPolls(c.MD.UserId)
		if err != nil {
			return nil, err
		}
		var peer *mtproto.InputPeer
		var offset, add, limit, maxID, minID int32
		if in != nil {
			peer = in.GetPeer()
			offset, add, limit = in.GetOffsetId(), in.GetAddOffset(), in.GetLimit()
			maxID, minID = in.GetMaxId(), in.GetMinId()
		}
		var ids []int32
		byID := map[int32]unreadPollJSON{}
		for _, e := range list {
			if !unreadPollPeerMatch(peer, e) {
				continue
			}
			if _, ok := byID[e.MsgId]; ok {
				continue
			}
			byID[e.MsgId] = e
			ids = append(ids, e.MsgId)
		}
		for _, id := range pageIDs(ids, offset, add, limit, maxID, minID) {
			msgs = append(msgs, pollMessage(byID[id]))
		}
		if len(msgs) == 0 {
			marks, err := loadPollReadMarks(c.MD.UserId)
			if err != nil {
				return nil, err
			}
			q, err := loadPollQuestion(c.MD.UserId)
			if err != nil {
				return nil, err
			}
			if q != "" && len(marks) == 0 {
				msgs = append(msgs, mtproto.MakeTLMessage(&mtproto.Message{
					Media: mtproto.MakeTLMessageMediaPoll(&mtproto.MessageMedia{
						Poll: pollWithQuestion(q),
					}).To_MessageMedia(),
				}).To_Message())
			}
		}
	}
	return mtproto.MakeTLMessagesMessages(&mtproto.Messages_Messages{
		Messages: msgs,
		Topics:   []*mtproto.ForumTopic{},
		Chats:    []*mtproto.Chat{},
		Users:    []*mtproto.User{},
	}).To_Messages_Messages(), nil
}

func (c *ApiFullCore) MessagesReadPollVotes(in *mtproto.TLMessagesReadPollVotes) (*mtproto.Messages_AffectedHistory, error) {
	userId, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	var peer *mtproto.InputPeer
	var top int32
	if in != nil {
		peer = in.GetPeer()
		if v := in.GetTopMsgId(); v != nil {
			top = v.GetValue()
		}
	}
	b, err := json.Marshal(map[string]int32{"top_msg_id": top})
	if err != nil {
		return nil, err
	}
	if err := persist.Default.Set(fmt.Sprintf("poll:read:%d:%s", userId, pollStoreKey(peer, top)), string(b)); err != nil {
		return nil, err
	}
	list, err := loadUnreadPolls(userId)
	if err != nil {
		return nil, err
	}
	marks, err := loadPollReadMarks(userId)
	if err != nil {
		return nil, err
	}
	q, err := loadPollQuestion(userId)
	if err != nil {
		return nil, err
	}
	legacy := q != "" && len(marks) == 0
	next := list[:0]
	var removed int32
	for _, e := range list {
		if unreadPollPeerMatch(peer, e) && (top == 0 || e.MsgId == top || e.MsgId == 0) {
			removed++
			continue
		}
		next = append(next, e)
	}
	if err := saveUnreadPolls(userId, next); err != nil {
		return nil, err
	}
	marks = append(marks, markFromPeer(peer))
	if err := savePollReadMarks(userId, marks); err != nil {
		return nil, err
	}
	if removed == 0 && legacy {
		removed = 1
	}
	return mtproto.MakeTLMessagesAffectedHistory(&mtproto.Messages_AffectedHistory{
		Pts:      removed,
		PtsCount: removed,
	}).To_Messages_AffectedHistory(), nil
}
