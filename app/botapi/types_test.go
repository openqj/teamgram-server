package botapi

import (
	"encoding/json"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/pkg/mtprotoext"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func TestAllowedUpdateTypesIncludeWebAppQuery(t *testing.T) {
	if !IsValidAllowedUpdateType("web_app_query") {
		t.Fatal("web_app_query must be accepted by allowed_updates")
	}
	if !IsValidAllowedUpdateType("guest_message") {
		t.Fatal("guest_message must be accepted by allowed_updates")
	}
	if IsValidAllowedUpdateType("unknown_update") {
		t.Fatal("unknown update type must be rejected")
	}
	for _, name := range []string{"managed_bot", "subscription", "stopped_message_generation"} {
		if !IsValidAllowedUpdateType(name) {
			t.Fatalf("%s must be accepted by allowed_updates", name)
		}
	}
}

func TestUpdateFromMTProtoManagedBotAndSubscription(t *testing.T) {
	users := []*mtproto.User{{Id: 10, FirstName: wrapperspb.String("Owner")}, {Id: 20, Bot: true, FirstName: wrapperspb.String("Managed")}}
	managed, ok := UpdateFromMTProto(&mtproto.Update{
		PredicateName: mtproto.Predicate_updateManagedBot,
		UserId:        10,
		BotId:         20,
	}, users, nil)
	if !ok || managed == nil || managed.ManagedBot == nil || managed.ManagedBot.User.ID != 10 || managed.ManagedBot.Bot.ID != 20 {
		t.Fatalf("managed_bot update = %#v", managed)
	}

	subscription, ok := UpdateFromMTProto(&mtproto.Update{
		PredicateName: mtproto.Predicate_updateBotStarsSubscription,
		UserId:        10,
		Canceled:      true,
		Payload_BYTES: []byte("monthly"),
	}, users, nil)
	if !ok || subscription == nil || subscription.Subscription == nil {
		t.Fatalf("subscription update = %#v", subscription)
	}
	if subscription.Subscription.User.ID != 10 || subscription.Subscription.InvoicePayload != "monthly" || subscription.Subscription.State != "canceled" {
		t.Fatalf("unexpected subscription conversion: %#v", subscription.Subscription)
	}
}

func TestMessageFromMTProtoSenderFields(t *testing.T) {
	message := MessageFromMTProto(&mtproto.Message{
		FromBoostsApplied: wrapperspb.Int32(3),
		FromRank:          wrapperspb.String("moderator"),
		ViaBusinessBotId:  wrapperspb.Int64(20),
		InvertMedia:       true,
	}, []*mtproto.User{{Id: 20, Bot: true}}, nil)
	if message == nil || message.SenderBoostCount != 3 || message.SenderTag != "moderator" || message.SenderBusinessBot == nil || message.SenderBusinessBot.ID != 20 || !message.ShowCaptionAboveMedia {
		t.Fatalf("unexpected sender fields: %#v", message)
	}
}

func TestReplyMarkupFromMTProtoOmitsNonInlineMessageMarkup(t *testing.T) {
	reply := mtproto.MakeTLReplyKeyboardMarkup(&mtproto.ReplyMarkup{
		Rows: []*mtproto.KeyboardButtonRow{
			mtproto.MakeTLKeyboardButtonRow(&mtproto.KeyboardButtonRow{
				Buttons: []*mtproto.KeyboardButton{
					mtproto.MakeTLKeyboardButton(&mtproto.KeyboardButton{Text: "Keyboard ping"}).To_KeyboardButton(),
				},
			}).To_KeyboardButtonRow(),
		},
	}).To_ReplyMarkup()
	if got := replyMarkupFromMTProto(reply); got != nil {
		t.Fatalf("reply keyboard must not be serialized as Message.reply_markup: %#v", got)
	}

	inline := mtproto.MakeTLReplyInlineMarkup(&mtproto.ReplyMarkup{
		Rows: []*mtproto.KeyboardButtonRow{
			mtproto.MakeTLKeyboardButtonRow(&mtproto.KeyboardButtonRow{
				Buttons: []*mtproto.KeyboardButton{
					mtproto.MakeTLKeyboardButtonCallback(&mtproto.KeyboardButton{Text: "Ping", Data: []byte("ping")}).To_KeyboardButton(),
				},
			}).To_KeyboardButtonRow(),
		},
	}).To_ReplyMarkup()
	if got := replyMarkupFromMTProto(inline); got == nil {
		t.Fatal("inline keyboard must remain in Message.reply_markup")
	}
}

func TestMessageFromMTProtoManagedBotCreated(t *testing.T) {
	message := MessageFromMTProto(&mtproto.Message{
		Action: &mtproto.MessageAction{
			PredicateName: mtproto.Predicate_messageActionManagedBotCreated,
			BotId:         20,
		},
	}, []*mtproto.User{{Id: 20, Bot: true}}, nil)
	if message == nil || message.ManagedBotCreated == nil || message.ManagedBotCreated.Bot == nil || message.ManagedBotCreated.Bot.ID != 20 {
		t.Fatalf("managed_bot_created = %#v", message)
	}
}

func TestMessageFromMTProtoWebAppData(t *testing.T) {
	message := MessageFromMTProto(mtproto.MakeTLMessageService(&mtproto.Message{
		Action: mtproto.MakeMessageActionWebViewDataSentMe("Open app", `{"choice":"confirm"}`),
	}).To_Message(), nil, nil)
	if message == nil || message.WebAppData == nil {
		t.Fatal("web view data action should map to Message.web_app_data")
	}
	if message.WebAppData.Data != `{"choice":"confirm"}` || message.WebAppData.ButtonText != "Open app" {
		t.Fatalf("unexpected web app data: %#v", message.WebAppData)
	}
}

func TestUpdateFromMTProtoEphemeralMessage(t *testing.T) {
	ephemeral := &mtproto.EphemeralMessage{
		Id:          42,
		ReceiverId:  7,
		Date:        123,
		Message:     "ephemeral",
		ReplyMarkup: nil,
	}
	if err := mtprotoext.SetEphemeralRichMessage(ephemeral, mtproto.MakeTLRichMessage(&mtproto.RichMessage{Rtl: true}).To_RichMessage()); err != nil {
		t.Fatal(err)
	}
	converted, ok := UpdateFromMTProto(&mtproto.Update{
		PredicateName:            mtproto.Predicate_updateNewEphemeralMessage,
		Message_EPHEMERALMESSAGE: ephemeral,
	}, []*mtproto.User{{Id: 7}}, nil)
	if !ok || converted == nil || converted.Message == nil {
		t.Fatal("ephemeral update should convert to a message update")
	}
	message := converted.Message
	if message.MessageID != 0 || message.EphemeralMessageID != 42 || message.Date != 123 {
		t.Fatalf("unexpected ephemeral message identity: %+v", message)
	}
	if message.ReceiverUser == nil || message.ReceiverUser.ID != 7 {
		t.Fatalf("receiver_user was not converted: %+v", message.ReceiverUser)
	}
	if message.Text != "ephemeral" {
		t.Fatalf("message text = %q, want ephemeral", message.Text)
	}
	if message.RichMessage == nil || !message.RichMessage.GetRtl() {
		t.Fatalf("rich message was not converted: %+v", message.RichMessage)
	}

	data, err := EncodeUpdate(converted)
	if err != nil {
		t.Fatalf("encode ephemeral update: %v", err)
	}
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("decode encoded ephemeral update: %v", err)
	}
	if _, present := payload["message"]; !present {
		t.Fatal("ephemeral update must use the Bot API message field")
	}
}

func TestMessageFromMTProtoGiftAndChecklist(t *testing.T) {
	giftMessage := MessageFromMTProto(&mtproto.Message{
		Action: &mtproto.MessageAction{
			PredicateName: mtproto.Predicate_messageActionStarGift,
			Gift: &mtproto.StarGift{
				Id:    101,
				Stars: 25,
			},
		},
	}, nil, nil)
	if giftMessage == nil || giftMessage.Gift == nil || giftMessage.Gift.Gift == nil {
		t.Fatal("star gift action should map to Message.gift")
	}
	if giftMessage.Gift.Gift.ID != "101" || giftMessage.Gift.Gift.StarCount != 25 {
		t.Fatalf("unexpected gift conversion: %+v", giftMessage.Gift.Gift)
	}

	checklistMessage := MessageFromMTProto(&mtproto.Message{
		Media: &mtproto.MessageMedia{
			PredicateName: mtproto.Predicate_messageMediaToDo,
			Todo: &mtproto.TodoList{
				OthersCanAppend:   true,
				OthersCanComplete: true,
				Title:             &mtproto.TextWithEntities{Text: "Tasks"},
				List: []*mtproto.TodoItem{{
					Id:    3,
					Title: &mtproto.TextWithEntities{Text: "First task"},
				}},
			},
		},
	}, nil, nil)
	if checklistMessage == nil || checklistMessage.Checklist == nil {
		t.Fatal("todo media should map to Message.checklist")
	}
	checklist := checklistMessage.Checklist
	if checklist.Title != "Tasks" || !checklist.OthersCanAddTasks || !checklist.OthersCanMarkTasksDone || len(checklist.Tasks) != 1 || checklist.Tasks[0].ID != 3 {
		t.Fatalf("unexpected checklist conversion: %+v", checklist)
	}

	doneMessage := MessageFromMTProto(&mtproto.Message{
		Action: &mtproto.MessageAction{
			PredicateName: mtproto.Predicate_messageActionTodoCompletions,
			Completed:     []int32{3},
			Incompleted:   []int32{4},
		},
	}, nil, nil)
	if doneMessage == nil || doneMessage.ChecklistTasksDone == nil {
		t.Fatal("todo completion action should map to checklist_tasks_done")
	}
	if len(doneMessage.ChecklistTasksDone.MarkedAsDoneTaskIDs) != 1 || doneMessage.ChecklistTasksDone.MarkedAsDoneTaskIDs[0] != 3 || len(doneMessage.ChecklistTasksDone.MarkedAsNotDoneTaskIDs) != 1 || doneMessage.ChecklistTasksDone.MarkedAsNotDoneTaskIDs[0] != 4 {
		t.Fatalf("unexpected checklist completion conversion: %+v", doneMessage.ChecklistTasksDone)
	}
}

func TestOwnedGiftInfoIncludesBotAPIUnionType(t *testing.T) {
	owned := ownedGiftInfoFromSaved(&mtproto.SavedStarGift{
		Gift:    &mtproto.StarGift{Id: 66, Stars: 25},
		SavedId: wrapperspb.Int64(66),
	}, nil, nil)
	if owned == nil || owned.Type != "regular" {
		t.Fatalf("owned gift type = %#v, want regular", owned)
	}
	encoded, err := json.Marshal(owned)
	if err != nil {
		t.Fatalf("marshal owned gift: %v", err)
	}
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &payload); err != nil {
		t.Fatalf("decode owned gift: %v", err)
	}
	if string(payload["type"]) != `"regular"` {
		t.Fatalf("owned gift JSON type = %s, want regular", payload["type"])
	}
}

func TestMessageFromMTProtoPaidMediaPreview(t *testing.T) {
	message := MessageFromMTProto(&mtproto.Message{
		Media: &mtproto.MessageMedia{
			PredicateName: mtproto.Predicate_messageMediaPaidMedia,
			StarsAmount:   17,
			ExtendedMedia_VECTORMESSAGEEXTENDEDMEDIA: []*mtproto.MessageExtendedMedia{{
				PredicateName: mtproto.Predicate_messageExtendedMediaPreview,
				W:             wrapperspb.Int32(640),
				H:             wrapperspb.Int32(360),
				VideoDuration: wrapperspb.Int32(8),
			}},
		},
	}, nil, nil)
	if message == nil || message.PaidMedia == nil {
		t.Fatal("paid media should map to Message.paid_media")
	}
	if message.PaidMedia.StarCount != 17 || len(message.PaidMedia.PaidMedia) != 1 {
		t.Fatalf("unexpected paid media conversion: %+v", message.PaidMedia)
	}
	preview, ok := message.PaidMedia.PaidMedia[0].(*PaidMediaPreview)
	if !ok || preview.Type != "preview" || preview.Width != 640 || preview.Height != 360 || preview.Duration != 8 {
		t.Fatalf("unexpected paid media preview: %#v", message.PaidMedia.PaidMedia[0])
	}
}

func TestEntityTypeUsesBotAPINames(t *testing.T) {
	for predicate, want := range map[string]string{
		mtproto.Predicate_messageEntityBotCommand:  "bot_command",
		mtproto.Predicate_messageEntityStrike:      "strikethrough",
		mtproto.Predicate_messageEntityTextUrl:     "text_link",
		mtproto.Predicate_messageEntityMentionName: "text_mention",
		mtproto.Predicate_messageEntityCustomEmoji: "custom_emoji",
		mtproto.Predicate_messageEntityPhone:       "phone_number",
		mtproto.Predicate_messageEntityBlockquote:  "blockquote",
	} {
		if got := entityType(predicate); got != want {
			t.Fatalf("entityType(%q) = %q, want %q", predicate, got, want)
		}
	}
}

func TestUpdateFromMTProtoDoesNotFabricateUnsupportedUpdates(t *testing.T) {
	unsupported := []string{
		mtproto.Predicate_updateDeleteEphemeralMessages,
		mtproto.Predicate_updateBotStopped,
	}
	for _, predicate := range unsupported {
		if update, ok := UpdateFromMTProto(&mtproto.Update{PredicateName: predicate}, nil, nil); ok || update != nil {
			t.Errorf("unsupported MTProto update %q was fabricated as Bot API update: %#v", predicate, update)
		}
	}
}

func TestUpdateFromMTProtoChatMemberUpdates(t *testing.T) {
	users := []*mtproto.User{
		{Id: 9, Bot: true},
		{Id: 10},
		{Id: 11},
	}
	chats := []*mtproto.Chat{{Id: 55, Title: "Compat group"}}

	myChatMember, ok := UpdateFromMTProtoForBot(&mtproto.Update{
		PredicateName: mtproto.Predicate_updateChannelParticipant,
		ChannelId:     55,
		ActorId:       11,
		UserId:        9,
		Date_INT32:    123,
		PrevParticipant_FLAGCHANNELPARTICIPANT: &mtproto.ChannelParticipant{
			PredicateName: mtproto.Predicate_channelParticipantLeft,
			UserId:        9,
			Left:          true,
		},
		NewParticipant_FLAGCHANNELPARTICIPANT: &mtproto.ChannelParticipant{
			PredicateName: mtproto.Predicate_channelParticipantSelf,
			UserId:        9,
			Self:          true,
		},
	}, users, chats, 9)
	if !ok || myChatMember == nil || myChatMember.MyChatMember == nil {
		t.Fatalf("bot membership update was not classified as my_chat_member: %#v", myChatMember)
	}
	if got := myChatMember.MyChatMember.Chat.ID; got != -1000000000055 {
		t.Fatalf("chat id = %d, want -1000000000055", got)
	}
	if myChatMember.MyChatMember.OldChatMember.Status != "left" || myChatMember.MyChatMember.NewChatMember.Status != "member" {
		t.Fatalf("unexpected member transition: %#v", myChatMember.MyChatMember)
	}

	chatMember, ok := UpdateFromMTProtoForBot(&mtproto.Update{
		PredicateName: mtproto.Predicate_updateChatParticipant,
		ChatId_INT64:  55,
		ActorId:       11,
		UserId:        10,
		Date_INT32:    124,
		PrevParticipant_FLAGCHATPARTICIPANT: &mtproto.ChatParticipant{
			PredicateName: mtproto.Predicate_chatParticipant,
			UserId:        10,
		},
		NewParticipant_FLAGCHATPARTICIPANT: &mtproto.ChatParticipant{
			PredicateName: mtproto.Predicate_chatParticipantAdmin,
			UserId:        10,
			Rank:          wrapperspb.String("moderator"),
		},
	}, users, chats, 9)
	if !ok || chatMember == nil || chatMember.ChatMember == nil {
		t.Fatalf("member update was not classified as chat_member: %#v", chatMember)
	}
	if chatMember.ChatMember.Chat.ID != -55 || chatMember.ChatMember.NewChatMember.Status != "administrator" {
		t.Fatalf("unexpected group member update: %#v", chatMember.ChatMember)
	}
}

func TestCallbackMessageUsesClickerChatForBotPrivatePeer(t *testing.T) {
	converted, ok := UpdateFromMTProtoForBot(&mtproto.Update{
		PredicateName:  mtproto.Predicate_updateBotCallbackQuery,
		QueryId_INT64:  99,
		UserId:         7,
		Peer_PEER:      mtproto.MakePeerUser(900000000001),
		MsgId_INT32:    636,
		Data_FLAGBYTES: []byte("ping"),
	}, []*mtproto.User{{Id: 7, FirstName: wrapperspb.String("Clicker")}, {Id: 900000000001, Bot: true}}, nil, 900000000001)
	if !ok || converted == nil || converted.CallbackQuery == nil || converted.CallbackQuery.Message == nil {
		t.Fatalf("callback update = %#v", converted)
	}
	if got := converted.CallbackQuery.Message.Chat.ID; got != 7 {
		t.Fatalf("callback chat id = %d, want clicker id 7", got)
	}
	if got := converted.CallbackQuery.Message.MessageID; got != 636 {
		t.Fatalf("callback message id = %d, want 636", got)
	}
}
