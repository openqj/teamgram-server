package core

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/teamgram/proto/mtproto"
	msgpb "github.com/teamgram/teamgram-server/app/messenger/msg/msg/msg"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

const bingHelpText = `I can search Bing for you.

Send any keywords to search images.
/search <keywords> - search Bing images
/help - show this help`

// handleBuiltinBotUpdates handles the private-chat surface of the built-in
// Bing bot. Inline queries are handled by apifull; this path only supplies the
// ordinary bot chat menu and its reply keyboard.
func (c *SyncCore) handleBuiltinBotUpdates(ctx context.Context, botID int64, updates *mtproto.Updates) error {
	if botID != userpb.BotBingId || updates == nil {
		return nil
	}
	var firstErr error
	mtproto.VisitUpdates(botID, updates, map[string]mtproto.UpdateVisitedFunc{
		mtproto.Predicate_updateNewMessage: func(_ int64, update *mtproto.Update, _ []*mtproto.User, _ []*mtproto.Chat, _ int32) {
			if firstErr != nil || update == nil {
				return
			}
			if err := c.handleBingMessage(ctx, update.GetMessage_MESSAGE()); err != nil {
				firstErr = err
			}
		},
	})
	return firstErr
}

func (c *SyncCore) handleBingMessage(ctx context.Context, message *mtproto.Message) error {
	if c == nil || c.svcCtx == nil || c.svcCtx.Dao == nil || message == nil || message.GetOut() || message.GetId() <= 0 {
		return nil
	}
	peerUserID := message.GetPeerId().GetUserId()
	recipientID := message.GetFromId().GetUserId()
	// Inbox delivery to a bot represents the private peer as the sender and
	// may omit from_id. Accept that shape as well as peer=@bing/from_id=user.
	if recipientID <= 0 && peerUserID > 0 && peerUserID != userpb.BotBingId {
		recipientID = peerUserID
	}
	if recipientID <= 0 || (peerUserID != userpb.BotBingId && peerUserID != recipientID) {
		return nil
	}
	return c.sendBingReply(ctx, recipientID, message.GetId(), bingReplyText(message.GetMessage()))
}

func bingReplyText(raw string) string {
	text := strings.TrimSpace(raw)
	command, args := parseBingCommand(text)
	switch command {
	case "start", "help":
		return bingHelpText
	case "search", "images":
		if args == "" {
			return "Send the image keywords you want to search on Bing."
		}
		return bingSearchReply("images", args)
	case "videos":
		if args == "" {
			return "Send the video keywords you want to search on Bing."
		}
		return bingSearchReply("videos", args)
	case "news":
		if args == "" {
			return "Send the news keywords you want to search on Bing."
		}
		return bingSearchReply("news", args)
	}

	switch strings.ToLower(text) {
	case "search images":
		return "Send the image keywords you want to search on Bing."
	case "search videos":
		return "Send the video keywords you want to search on Bing."
	case "search news":
		return "Send the news keywords you want to search on Bing."
	case "bing home":
		return "Open Bing: https://www.bing.com/"
	case "help":
		return bingHelpText
	}
	if text == "" {
		return bingHelpText
	}
	return bingSearchReply("images", text)
}

func parseBingCommand(text string) (string, string) {
	if !strings.HasPrefix(text, "/") {
		return "", text
	}
	parts := strings.Fields(text)
	if len(parts) == 0 {
		return "", ""
	}
	command := strings.TrimPrefix(parts[0], "/")
	if at := strings.IndexByte(command, '@'); at >= 0 {
		command = command[:at]
	}
	return strings.ToLower(command), strings.TrimSpace(strings.TrimPrefix(text, parts[0]))
}

func bingSearchReply(kind, term string) string {
	base := "https://www.bing.com/images/search?q="
	switch kind {
	case "videos":
		base = "https://www.bing.com/videos/search?q="
	case "news":
		base = "https://www.bing.com/news/search?q="
	}
	return fmt.Sprintf("Bing %s search for %q:\n%s", kind, term, base+url.QueryEscape(term))
}

func (c *SyncCore) sendBingReply(ctx context.Context, recipientID int64, messageID int32, text string) error {
	if c.svcCtx.Dao.MsgClient == nil {
		return mtproto.ErrMethodNotImpl
	}
	replyCtx, err := botFatherRPCContext(ctx, recipientID)
	if err != nil {
		return err
	}
	raw := sha256.Sum256([]byte(fmt.Sprintf("bing:%d:%d", recipientID, messageID)))
	randomID := int64(binary.LittleEndian.Uint64(raw[:]) & 0x7fffffffffffffff)
	if randomID == 0 {
		randomID = 1
	}
	_, err = c.svcCtx.Dao.MsgSendMessageV2(replyCtx, &msgpb.TLMsgSendMessageV2{
		UserId: userpb.BotBingId, PeerType: mtproto.PEER_USER, PeerId: recipientID,
		Message: []*msgpb.OutboxMessage{msgpb.MakeTLOutboxMessage(&msgpb.OutboxMessage{
			RandomId: randomID,
			Message: mtproto.MakeTLMessage(&mtproto.Message{
				Out:         true,
				FromId:      mtproto.MakePeerUser(userpb.BotBingId),
				PeerId:      mtproto.MakePeerUser(recipientID),
				Date:        int32(time.Now().Unix()),
				Message:     text,
				ReplyMarkup: bingReplyKeyboard(),
			}).To_Message(),
		}).To_OutboxMessage()},
	})
	return err
}

func bingReplyKeyboard() *mtproto.ReplyMarkup {
	rows := []*mtproto.KeyboardButtonRow{
		mtproto.MakeTLKeyboardButtonRow(&mtproto.KeyboardButtonRow{Buttons: []*mtproto.KeyboardButton{
			mtproto.MakeTLKeyboardButton(&mtproto.KeyboardButton{Text: "Search images"}).To_KeyboardButton(),
			mtproto.MakeTLKeyboardButton(&mtproto.KeyboardButton{Text: "Search videos"}).To_KeyboardButton(),
		}}).To_KeyboardButtonRow(),
		mtproto.MakeTLKeyboardButtonRow(&mtproto.KeyboardButtonRow{Buttons: []*mtproto.KeyboardButton{
			mtproto.MakeTLKeyboardButton(&mtproto.KeyboardButton{Text: "Search news"}).To_KeyboardButton(),
			mtproto.MakeTLKeyboardButton(&mtproto.KeyboardButton{Text: "Help"}).To_KeyboardButton(),
		}}).To_KeyboardButtonRow(),
	}
	return mtproto.MakeTLReplyKeyboardMarkup(&mtproto.ReplyMarkup{
		Resize:     true,
		Persistent: true,
		Rows:       rows,
	}).To_ReplyMarkup()
}
