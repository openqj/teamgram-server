package core

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"strings"
	"time"

	"github.com/teamgram/proto/mtproto"
	msgpb "github.com/teamgram/teamgram-server/app/messenger/msg/msg/msg"
	"github.com/teamgram/teamgram-server/app/messenger/sync/internal/dao"
	"github.com/teamgram/teamgram-server/app/messenger/sync/sync"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

const (
	volunteerSupportAwaitProblem     = "await_problem"
	volunteerSupportAwaitCategory    = "await_category"
	volunteerSupportAwaitEnvironment = "await_environment"
	volunteerSupportSubmitted        = "submitted"
)

const (
	volunteerSupportWelcome             = "Hello. I'm Volunteer Support.\n\nPlease describe the problem you're experiencing. Include any error message and the steps that led to it, or choose a topic below."
	volunteerSupportCategoryQuestion    = "Thanks. Which area is affected? Choose an option below."
	volunteerSupportEnvironmentQuestion = "Got it. Which device and app version are you using?"
	volunteerSupportSubmittedReply      = "Thanks. Your report has been recorded for support review. Please wait for a reply here."
	volunteerSupportSubmittedReminder   = "Your report is already recorded for support review. Send /new to start another report."
)

const (
	volunteerSupportTopicDevices  = "How to use Telegram on many devices?"
	volunteerSupportTopicGroups   = "Learn more about groups"
	volunteerSupportTopicChannels = "Learn more about channels"
	volunteerSupportTopicStickers = "Learn more about stickers"
	volunteerSupportTopicBots     = "Learn more about bots"
)

// HandleVolunteerSupportUpdates consumes messages addressed to the built-in
// support peer and keeps the conversation state in PostgreSQL.
func (c *SyncCore) HandleVolunteerSupportUpdates(in *sync.TLSyncPushUpdates) (*mtproto.Void, error) {
	if in == nil || in.GetUserId() != userpb.VolunteerSupportId || in.GetUpdates() == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	var firstErr error
	mtproto.VisitUpdates(in.GetUserId(), in.GetUpdates(), map[string]mtproto.UpdateVisitedFunc{
		mtproto.Predicate_updateNewMessage: func(_ int64, update *mtproto.Update, _ []*mtproto.User, _ []*mtproto.Chat, _ int32) {
			if firstErr != nil || update == nil || update.GetMessage_MESSAGE() == nil {
				return
			}
			if err := c.handleVolunteerSupportMessage(update.GetMessage_MESSAGE()); err != nil {
				firstErr = err
			}
		},
	})
	if firstErr != nil {
		return nil, firstErr
	}
	return mtproto.EmptyVoid, nil
}

func (c *SyncCore) handleVolunteerSupportMessage(message *mtproto.Message) error {
	if c == nil || c.svcCtx == nil || c.svcCtx.Dao == nil || message == nil || message.GetOut() || message.GetId() <= 0 {
		return nil
	}
	peerUserID := message.GetPeerId().GetUserId()
	sender := message.GetFromId().GetUserId()
	if sender <= 0 && peerUserID > 0 && peerUserID != userpb.VolunteerSupportId {
		sender = peerUserID
	}
	if sender <= 0 || (peerUserID != userpb.VolunteerSupportId && peerUserID != sender) {
		return nil
	}
	ctx, err := botFatherRPCContext(c.ctx, sender)
	if err != nil {
		return err
	}
	session, err := c.svcCtx.Dao.LoadVolunteerSupportSession(ctx, sender)
	if err != nil {
		return err
	}
	if message.GetId() < session.LastMessageID {
		return nil
	}
	if message.GetId() == session.LastMessageID {
		if session.LastReply == "" {
			return nil
		}
		return c.sendVolunteerSupportReply(ctx, sender, message.GetId(), session.LastReply, volunteerSupportReplyMarkup(session.State))
	}

	reply := processVolunteerSupportText(session, message.GetMessage())
	session.LastMessageID = message.GetId()
	session.LastReply = reply
	if err := c.svcCtx.Dao.SaveVolunteerSupportSession(ctx, session); err != nil {
		return err
	}
	return c.sendVolunteerSupportReply(ctx, sender, message.GetId(), reply, volunteerSupportReplyMarkup(session.State))
}

func processVolunteerSupportText(session *dao.VolunteerSupportSession, rawText string) string {
	text := strings.TrimSpace(rawText)
	command := volunteerSupportCommand(text)
	switch command {
	case "start", "new", "reset":
		clearVolunteerSupportSession(session)
		session.State = volunteerSupportAwaitProblem
		return volunteerSupportWelcome
	case "cancel":
		clearVolunteerSupportSession(session)
		return "Canceled. Send /start when you are ready to contact support."
	}

	if text == "" {
		if session.State == volunteerSupportSubmitted {
			return volunteerSupportSubmittedReminder
		}
		session.State = volunteerSupportAwaitProblem
		return volunteerSupportWelcome
	}

	switch session.State {
	case "", "idle":
		session.Problem = text
		session.State = volunteerSupportAwaitCategory
		return volunteerSupportCategoryQuestion
	case volunteerSupportAwaitProblem:
		session.Problem = text
		session.State = volunteerSupportAwaitCategory
		return volunteerSupportCategoryQuestion
	case volunteerSupportAwaitCategory:
		session.Category = text
		session.State = volunteerSupportAwaitEnvironment
		return volunteerSupportEnvironmentQuestion
	case volunteerSupportAwaitEnvironment:
		session.Environment = text
		session.State = volunteerSupportSubmitted
		now := time.Now()
		session.SubmittedAt = &now
		return volunteerSupportSubmittedReply
	case volunteerSupportSubmitted:
		return volunteerSupportSubmittedReminder
	default:
		clearVolunteerSupportSession(session)
		session.State = volunteerSupportAwaitProblem
		return volunteerSupportWelcome
	}
}

func volunteerSupportCommand(text string) string {
	if !strings.HasPrefix(text, "/") {
		return ""
	}
	parts := strings.Fields(text)
	if len(parts) == 0 {
		return ""
	}
	command := strings.TrimPrefix(parts[0], "/")
	if at := strings.IndexByte(command, '@'); at >= 0 {
		command = command[:at]
	}
	return strings.ToLower(command)
}

func (c *SyncCore) sendVolunteerSupportReply(ctx context.Context, recipientID int64, messageID int32, text string, replyMarkup *mtproto.ReplyMarkup) error {
	if c.svcCtx.Dao.MsgClient == nil {
		return mtproto.ErrMethodNotImpl
	}
	raw := sha256.Sum256([]byte(fmt.Sprintf("volunteer-support:%d:%d", recipientID, messageID)))
	randomID := int64(binary.LittleEndian.Uint64(raw[:]) & 0x7fffffffffffffff)
	if randomID == 0 {
		randomID = 1
	}
	_, err := c.svcCtx.Dao.MsgSendMessageV2(ctx, &msgpb.TLMsgSendMessageV2{
		UserId: userpb.VolunteerSupportId, PeerType: mtproto.PEER_USER, PeerId: recipientID,
		Message: []*msgpb.OutboxMessage{msgpb.MakeTLOutboxMessage(&msgpb.OutboxMessage{
			RandomId: randomID,
			Message: mtproto.MakeTLMessage(&mtproto.Message{
				Out: true, FromId: mtproto.MakePeerUser(userpb.VolunteerSupportId), PeerId: mtproto.MakePeerUser(recipientID),
				Date: int32(time.Now().Unix()), Message: text, ReplyMarkup: replyMarkup,
			}).To_Message(),
		}).To_OutboxMessage()},
	})
	return err
}

func volunteerSupportReplyMarkup(state string) *mtproto.ReplyMarkup {
	switch state {
	case volunteerSupportAwaitProblem:
		return volunteerSupportKeyboard(
			volunteerSupportTopicDevices,
			volunteerSupportTopicGroups,
			volunteerSupportTopicChannels,
			volunteerSupportTopicStickers,
			volunteerSupportTopicBots,
		)
	case volunteerSupportAwaitCategory:
		return volunteerSupportKeyboard("Login", "Messages and chats", "Calls", "Payments", "Other")
	default:
		return mtproto.MakeTLReplyKeyboardHide(&mtproto.ReplyMarkup{}).To_ReplyMarkup()
	}
}

func volunteerSupportKeyboard(labels ...string) *mtproto.ReplyMarkup {
	rows := make([]*mtproto.KeyboardButtonRow, 0, len(labels))
	for _, label := range labels {
		rows = append(rows, mtproto.MakeTLKeyboardButtonRow(&mtproto.KeyboardButtonRow{
			Buttons: []*mtproto.KeyboardButton{
				mtproto.MakeTLKeyboardButton(&mtproto.KeyboardButton{Text: label}).To_KeyboardButton(),
			},
		}).To_KeyboardButtonRow())
	}
	return mtproto.MakeTLReplyKeyboardMarkup(&mtproto.ReplyMarkup{
		Resize:     true,
		Persistent: true,
		Rows:       rows,
	}).To_ReplyMarkup()
}

func clearVolunteerSupportSession(session *dao.VolunteerSupportSession) {
	session.State = "idle"
	session.Problem = ""
	session.Category = ""
	session.Environment = ""
	session.SubmittedAt = nil
}
