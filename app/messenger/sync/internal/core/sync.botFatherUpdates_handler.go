package core

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/teamgram/proto/mtproto"
	rpcmetadata "github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/botapi"
	msgpb "github.com/teamgram/teamgram-server/app/messenger/msg/msg/msg"
	"github.com/teamgram/teamgram-server/app/messenger/sync/internal/dao"
	"github.com/teamgram/teamgram-server/app/messenger/sync/sync"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

const (
	BotFatherTopic = "BotFather-T"

	botFatherIdle            = "idle"
	botFatherAwaitName       = "await_name"
	botFatherAwaitUsername   = "await_username"
	botFatherSelectToken     = "select_token"
	botFatherSelectRevoke    = "select_revoke"
	botFatherSelectName      = "select_name"
	botFatherAwaitSetName    = "await_setname"
	botFatherSelectAbout     = "select_about"
	botFatherAwaitAbout      = "await_about"
	botFatherSelectDesc      = "select_description"
	botFatherAwaitDesc       = "await_description"
	botFatherSelectCommands  = "select_commands"
	botFatherAwaitCommands   = "await_commands"
	botFatherSelectPrivacy   = "select_privacy"
	botFatherSelectJoin      = "select_join_groups"
	botFatherSelectInline    = "select_inline"
	botFatherAwaitInline     = "await_inline"
	botFatherSelectInlineGeo = "select_inline_geo"
	botFatherSelectMenu      = "select_menu_button"
	botFatherAwaitMenu       = "await_menu_button"
	botFatherSelectNewApp    = "select_new_app"
	botFatherAwaitAppTitle   = "await_app_title"
	botFatherAwaitAppShort   = "await_app_short_name"
	botFatherAwaitAppDesc    = "await_app_description"
	botFatherAwaitAppURL     = "await_app_url"
	botFatherSelectEditApp   = "select_edit_app"
	botFatherAwaitEditApp    = "await_edit_app"
	botFatherSelectDeleteApp = "select_delete_app"
	botFatherSelectDomain    = "select_app_domain"
	botFatherAwaitDomain     = "await_app_domain"
)

const botFatherHelp = `I can help you create and manage Telegram bots.

/newbot - create a new bot
/mybots - list your bots
/token - get a bot token
/revoke - revoke the current token
/setname - change a bot name
/setdescription - change a bot description
/setabouttext - change a bot about text
/setcommands - replace bot commands
/setprivacy - configure group privacy
/setjoingroups - allow or disallow groups
/setinline - enable inline mode
/setinlinegeo - enable inline location
/setmenubutton - configure the bot's Mini App menu button
/newapp - create a Mini App for a bot
/myapps - list your Mini Apps
/editapp - edit a Mini App
/deleteapp - delete a Mini App
/setdomain - set the Mini App domain
/cancel - cancel the current operation`

// HandleBotFatherUpdates consumes only the dedicated BotFather Kafka topic.
// Ordinary bot updates continue through SyncPushBotUpdates unchanged.
func (c *SyncCore) HandleBotFatherUpdates(in *sync.TLSyncPushBotUpdates) (*mtproto.Void, error) {
	if in == nil || in.GetUserId() != userpb.BotFatherId || in.GetUpdates() == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	var firstErr error
	mtproto.VisitUpdates(in.GetUserId(), in.GetUpdates(), map[string]mtproto.UpdateVisitedFunc{
		mtproto.Predicate_updateNewMessage: func(_ int64, update *mtproto.Update, _ []*mtproto.User, _ []*mtproto.Chat, _ int32) {
			if firstErr != nil || update == nil || update.GetMessage_MESSAGE() == nil {
				return
			}
			if err := c.handleBotFatherMessage(update.GetMessage_MESSAGE()); err != nil {
				firstErr = err
			}
		},
	})
	if firstErr != nil {
		return nil, firstErr
	}
	return mtproto.EmptyVoid, nil
}

func (c *SyncCore) handleBotFatherMessage(message *mtproto.Message) error {
	if c == nil || c.svcCtx == nil || c.svcCtx.Dao == nil || message == nil || message.GetOut() {
		return nil
	}
	sender := message.GetFromId().GetUserId()
	if sender <= 0 || message.GetPeerId().GetUserId() != userpb.BotFatherId || message.GetId() <= 0 {
		return nil
	}
	ctx, err := botFatherRPCContext(c.ctx, sender)
	if err != nil {
		return err
	}
	session, err := c.svcCtx.Dao.LoadBotFatherSession(ctx, sender)
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
		return c.sendBotFatherReply(ctx, sender, message.GetId(), session.LastReply)
	}

	reply, err := c.processBotFatherText(ctx, sender, session, message.GetMessage())
	if err != nil {
		return err
	}
	session.LastMessageID = message.GetId()
	session.LastReply = reply
	if err := c.svcCtx.Dao.SaveBotFatherSession(ctx, session); err != nil {
		return err
	}
	return c.sendBotFatherReply(ctx, sender, message.GetId(), reply)
}

func (c *SyncCore) processBotFatherText(ctx context.Context, sender int64, session *dao.BotFatherSession, text string) (string, error) {
	command, args := parseBotFatherCommand(text)
	if command == "cancel" {
		clearBotFatherSession(session)
		return "Canceled.", nil
	}

	switch session.State {
	case botFatherAwaitName:
		if strings.HasPrefix(strings.TrimSpace(text), "/") {
			return "Please send the name for your new bot, or /cancel.", nil
		}
		name := strings.TrimSpace(text)
		if !validBotFatherName(name) {
			return "That name is invalid. Please send a name between 1 and 64 characters.", nil
		}
		session.PendingName = name
		session.State = botFatherAwaitUsername
		return "Good. Now choose a username for your bot. It must end in `bot`.", nil
	case botFatherAwaitUsername:
		if strings.HasPrefix(strings.TrimSpace(text), "/") {
			return "Please send the username for your new bot, or /cancel.", nil
		}
		username := normalizeBotUsername(text)
		if !validBotFatherUsername(username) {
			return "That username is invalid. It must use letters, numbers and underscores and end in `bot`.", nil
		}
		return c.createBotFromBotFather(ctx, sender, session, username)
	case botFatherAwaitSetName:
		if !validBotFatherName(strings.TrimSpace(text)) {
			return "That name is invalid. Please send a name between 1 and 64 characters.", nil
		}
		return c.setBotInfoFromBotFather(ctx, sender, session, strings.TrimSpace(text), "name")
	case botFatherAwaitAbout:
		value := strings.TrimSpace(text)
		if strings.EqualFold(value, "/empty") {
			value = ""
		}
		return c.setBotInfoFromBotFather(ctx, sender, session, value, "about")
	case botFatherAwaitDesc:
		value := strings.TrimSpace(text)
		if strings.EqualFold(value, "/empty") {
			value = ""
		}
		return c.setBotInfoFromBotFather(ctx, sender, session, value, "description")
	case botFatherAwaitCommands:
		return c.setBotCommandsFromBotFather(ctx, sender, session, text)
	case botFatherAwaitInline:
		return c.setInlineFromBotFather(ctx, sender, session, text)
	case botFatherAwaitMenu:
		return c.setMenuButtonFromBotFather(ctx, session, text)
	case botFatherAwaitAppTitle:
		value := strings.TrimSpace(text)
		if !validBotFatherAppTitle(value) {
			return "That Mini App title is invalid. Please send 1 to 64 characters.", nil
		}
		session.PendingAppTitle = value
		session.State = botFatherAwaitAppShort
		return "Now send a short name for the Mini App (letters, numbers and underscores).", nil
	case botFatherAwaitAppShort:
		value := strings.TrimSpace(strings.TrimPrefix(text, "@"))
		if !validBotFatherAppShortName(value) {
			return "That short name is invalid. Use 1 to 64 lowercase letters, numbers or underscores.", nil
		}
		session.PendingAppShortName = value
		session.State = botFatherAwaitAppDesc
		return "Send a short description, or /empty.", nil
	case botFatherAwaitAppDesc:
		value := strings.TrimSpace(text)
		if strings.EqualFold(value, "/empty") {
			value = ""
		}
		if !validBotFatherAppDescription(value) {
			return "That description is invalid. Please send at most 512 characters.", nil
		}
		session.PendingAppDescription = value
		session.State = botFatherAwaitAppURL
		return "Finally send the HTTPS URL of the Mini App.", nil
	case botFatherAwaitAppURL:
		value := strings.TrimSpace(text)
		if !validBotFatherAppURL(value) {
			return "That URL is invalid. Mini App URLs must use HTTPS.", nil
		}
		if err := c.svcCtx.Dao.Postgres.UpsertBotMiniApp(ctx, sender, dao.BotMiniApp{
			BotID: session.PendingBotID, ShortName: session.PendingAppShortName, Title: session.PendingAppTitle,
			Description: session.PendingAppDescription, URL: value,
		}); err != nil {
			return "I couldn't save that Mini App right now.", err
		}
		clearBotFatherSession(session)
		return "Done. The Mini App was created and is now the bot's main app.", nil
	case botFatherAwaitEditApp:
		app, ok := parseBotFatherAppEdit(text)
		if !ok {
			return "Send four lines: title, short name, description, and HTTPS URL. Use /empty for the description.", nil
		}
		if err := c.svcCtx.Dao.Postgres.UpsertBotMiniApp(ctx, sender, dao.BotMiniApp{
			BotID: session.PendingBotID, ShortName: app.shortName, Title: app.title,
			Description: app.description, URL: app.url,
		}); err != nil {
			return "I couldn't update that Mini App right now.", err
		}
		clearBotFatherSession(session)
		return "Done. The Mini App was updated.", nil
	case botFatherAwaitDomain:
		domain := strings.TrimSpace(text)
		if !validBotFatherDomain(domain) {
			return "That domain is invalid. Send a hostname such as example.com.", nil
		}
		if err := c.svcCtx.Dao.Postgres.SetBotMiniAppDomain(ctx, sender, session.PendingBotID, domain); err != nil {
			return "I couldn't update the Mini App domain right now.", err
		}
		clearBotFatherSession(session)
		return "Done. The Mini App domain was updated.", nil
	}

	switch session.State {
	case botFatherSelectToken, botFatherSelectRevoke:
		target := firstArg(text)
		if target == "" || strings.HasPrefix(strings.TrimSpace(text), "/") {
			return "Please choose a bot by sending its username.", nil
		}
		bot, err := c.resolveBot(ctx, sender, target)
		if err != nil {
			return "I couldn't find that bot in your bot list.", nil
		}
		reply, err := c.exportBotToken(ctx, bot, session.State == botFatherSelectRevoke)
		if err == nil {
			clearBotFatherSession(session)
		}
		return reply, err
	case botFatherSelectName, botFatherSelectAbout, botFatherSelectDesc, botFatherSelectCommands:
		target := firstArg(text)
		if target == "" || strings.HasPrefix(strings.TrimSpace(text), "/") {
			return "Please choose a bot by sending its username.", nil
		}
		bot, err := c.resolveBot(ctx, sender, target)
		if err != nil {
			return "I couldn't find that bot in your bot list.", nil
		}
		session.PendingBotID = bot.GetUser().GetId()
		switch session.State {
		case botFatherSelectName:
			session.State = botFatherAwaitSetName
			return "Please send the new name for your bot.", nil
		case botFatherSelectAbout:
			session.State = botFatherAwaitAbout
			return "Please send the new about text.", nil
		case botFatherSelectDesc:
			session.State = botFatherAwaitDesc
			return "Please send the new description.", nil
		default:
			session.State = botFatherAwaitCommands
			return "Send one command per line in the form `command - description`.", nil
		}
	case botFatherSelectMenu:
		target := firstArg(text)
		if target == "" || strings.HasPrefix(strings.TrimSpace(text), "/") {
			return "Please choose a bot by sending its username.", nil
		}
		bot, err := c.resolveBot(ctx, sender, target)
		if err != nil {
			return "I couldn't find that bot in your bot list.", nil
		}
		session.PendingBotID = bot.GetUser().GetId()
		session.State = botFatherAwaitMenu
		return "Send the menu button text and its HTTPS Mini App URL separated by a newline. Send /default to restore the default button.", nil
	case botFatherSelectNewApp, botFatherSelectEditApp, botFatherSelectDeleteApp, botFatherSelectDomain:
		target := firstArg(text)
		if target == "" || strings.HasPrefix(strings.TrimSpace(text), "/") {
			return "Please choose a bot by sending its username.", nil
		}
		bot, err := c.resolveBot(ctx, sender, target)
		if err != nil {
			return "I couldn't find that bot in your bot list.", nil
		}
		session.PendingBotID = bot.GetUser().GetId()
		switch session.State {
		case botFatherSelectNewApp:
			session.State = botFatherAwaitAppTitle
			return "How should we call this Mini App? Send its title.", nil
		case botFatherSelectEditApp:
			session.State = botFatherAwaitEditApp
			return "Send four lines: title, short name, description, and HTTPS URL.", nil
		case botFatherSelectDeleteApp:
			if err := c.svcCtx.Dao.Postgres.DeleteBotMiniApp(ctx, sender, session.PendingBotID, ""); err != nil {
				return "I couldn't delete that Mini App right now.", err
			}
			clearBotFatherSession(session)
			return "Done. The Mini App was deleted.", nil
		default:
			session.State = botFatherAwaitDomain
			return "Send the Mini App domain, such as example.com.", nil
		}
	case botFatherSelectPrivacy, botFatherSelectJoin, botFatherSelectInlineGeo:
		parts := strings.Fields(text)
		if len(parts) == 0 || strings.HasPrefix(strings.TrimSpace(text), "/") {
			return "Please choose a bot by sending its username, followed by `enable` or `disable`.", nil
		}
		kind := "privacy"
		prompt := "Please reply with `enable` or `disable`."
		switch session.State {
		case botFatherSelectJoin:
			kind = "groups"
		case botFatherSelectInlineGeo:
			kind = "inline location"
		}
		if session.PendingBotID > 0 && len(parts) == 1 && (strings.EqualFold(parts[0], "enable") || strings.EqualFold(parts[0], "disable")) {
			return c.applyToggle(ctx, session, session.PendingBotID, kind, parts[0])
		}
		bot, err := c.resolveBot(ctx, sender, parts[0])
		if err != nil {
			return "I couldn't find that bot in your bot list.", nil
		}
		session.PendingBotID = bot.GetUser().GetId()
		if len(parts) > 1 {
			return c.applyToggle(ctx, session, session.PendingBotID, kind, parts[1])
		}
		return prompt, nil
	case botFatherSelectInline:
		parts := strings.Fields(text)
		if len(parts) == 0 || strings.HasPrefix(strings.TrimSpace(text), "/") {
			return "Please choose a bot by sending its username, followed by an inline placeholder.", nil
		}
		bot, err := c.resolveBot(ctx, sender, parts[0])
		if err != nil {
			return "I couldn't find that bot in your bot list.", nil
		}
		session.PendingBotID = bot.GetUser().GetId()
		placeholder := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(text), parts[0]))
		if placeholder == "" {
			session.State = botFatherAwaitInline
			return "Please send the inline placeholder text.", nil
		}
		session.State = botFatherAwaitInline
		return c.setInlineFromBotFather(ctx, sender, session, placeholder)
	}

	if command == "" {
		return "I don't understand that command. Send /help to see what I can do.", nil
	}
	switch command {
	case "start", "help":
		return botFatherHelp, nil
	case "newbot":
		if session.State != botFatherIdle {
			return "Please finish the current operation or send /cancel first.", nil
		}
		clearBotFatherSession(session)
		session.State = botFatherAwaitName
		return "Alright, a new bot. How are we going to call it? Please choose a name for your bot.", nil
	case "mybots":
		return c.listBotFatherBots(ctx), nil
	case "token", "revoke":
		return c.handleTokenCommand(ctx, sender, session, command, args)
	case "setname", "setabouttext", "setdescription", "setcommands":
		return c.handleProfileCommand(ctx, sender, session, command, args)
	case "setmenubutton":
		return c.handleMenuButtonCommand(ctx, sender, session, args)
	case "newapp":
		return c.handleNewAppCommand(ctx, sender, session, args)
	case "myapps":
		return c.listBotFatherMiniApps(ctx, sender)
	case "editapp":
		return c.handleEditAppCommand(ctx, sender, session, args)
	case "deleteapp":
		return c.handleDeleteAppCommand(ctx, sender, session, args)
	case "setdomain":
		return c.handleDomainCommand(ctx, sender, session, args)
	case "setprivacy":
		return c.handleToggleCommand(ctx, sender, session, botFatherSelectPrivacy, "privacy", args)
	case "setjoingroups":
		return c.handleToggleCommand(ctx, sender, session, botFatherSelectJoin, "groups", args)
	case "setinline":
		return c.handleInlineCommand(ctx, sender, session, args)
	case "setinlinegeo":
		return c.handleToggleCommand(ctx, sender, session, botFatherSelectInlineGeo, "inline location", args)
	default:
		return "I don't understand that command. Send /help to see what I can do.", nil
	}
}

func (c *SyncCore) handleMenuButtonCommand(ctx context.Context, sender int64, session *dao.BotFatherSession, args string) (string, error) {
	parts := strings.Fields(args)
	if len(parts) == 0 {
		session.State = botFatherSelectMenu
		return "Please choose a bot by sending its username.", nil
	}
	bot, err := c.resolveBot(ctx, sender, parts[0])
	if err != nil {
		return "I couldn't find that bot in your bot list.", nil
	}
	session.PendingBotID = bot.GetUser().GetId()
	session.State = botFatherAwaitMenu
	rest := strings.TrimSpace(strings.TrimPrefix(args, parts[0]))
	if rest == "" {
		return "Send the menu button text and its HTTPS Mini App URL separated by a newline. Send /default to restore the default button.", nil
	}
	return c.setMenuButtonFromBotFather(ctx, session, rest)
}

func (c *SyncCore) setMenuButtonFromBotFather(ctx context.Context, session *dao.BotFatherSession, text string) (string, error) {
	button, ok := parseBotFatherMenuButton(text)
	if !ok {
		return "Send the menu button text and an HTTPS Mini App URL separated by a newline, or send /default or /commands.", nil
	}
	if c == nil || c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.Postgres == nil || c.svcCtx.Dao.Postgres.BotAPI == nil {
		return "I couldn't update that bot's menu button right now.", mtproto.ErrMethodNotImpl
	}
	if err := c.svcCtx.Dao.Postgres.BotAPI.SetBotMenuButton(ctx, session.PendingBotID, 0, button); err != nil {
		return "I couldn't update that bot's menu button right now.", err
	}
	clearBotFatherSession(session)
	return "Done. The bot's menu button has been updated.", nil
}

func parseBotFatherMenuButton(text string) (botapi.MenuButton, bool) {
	text = strings.TrimSpace(text)
	switch strings.ToLower(text) {
	case "/default", "default":
		return botapi.MenuButton{Type: "default"}, true
	case "/commands", "commands":
		return botapi.MenuButton{Type: "commands"}, true
	}
	parts := strings.SplitN(text, "\n", 2)
	if len(parts) != 2 {
		return botapi.MenuButton{}, false
	}
	buttonText, rawURL := strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
	if buttonText == "" || rawURL == "" {
		return botapi.MenuButton{}, false
	}
	return botapi.MenuButton{Type: "web_app", Text: buttonText, WebApp: &botapi.WebAppInfo{URL: rawURL}}, true
}

func (c *SyncCore) handleNewAppCommand(ctx context.Context, sender int64, session *dao.BotFatherSession, args string) (string, error) {
	target := firstArg(args)
	if target == "" {
		session.State = botFatherSelectNewApp
		return "Please choose the bot that will own this Mini App.", nil
	}
	bot, err := c.resolveBot(ctx, sender, target)
	if err != nil {
		return "I couldn't find that bot in your bot list.", nil
	}
	session.PendingBotID = bot.GetUser().GetId()
	session.State = botFatherAwaitAppTitle
	return "How should we call this Mini App? Send its title.", nil
}

func (c *SyncCore) listBotFatherMiniApps(ctx context.Context, sender int64) (string, error) {
	if c == nil || c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.Postgres == nil {
		return "Mini Apps are not available right now.", mtproto.ErrMethodNotImpl
	}
	apps, err := c.svcCtx.Dao.Postgres.ListBotMiniApps(ctx, sender)
	if err != nil {
		return "I couldn't load your Mini Apps right now.", err
	}
	if len(apps) == 0 {
		return "You don't have any Mini Apps yet. Use /newapp to create one.", nil
	}
	bots, err := c.svcCtx.Dao.UserGetCreatedBots(ctx)
	if err != nil {
		return "I couldn't load your bot list right now.", err
	}
	names := make(map[int64]string)
	if bots != nil {
		for _, bot := range bots.GetDatas() {
			if bot != nil && bot.GetUser() != nil {
				names[bot.GetUser().GetId()] = bot.GetUser().GetUsername()
			}
		}
	}
	lines := []string{"Your Mini Apps:"}
	for _, app := range apps {
		botName := names[app.BotID]
		if botName == "" {
			botName = strconv.FormatInt(app.BotID, 10)
		}
		line := fmt.Sprintf("@%s / %s - %s (%s)", botName, app.ShortName, app.Title, app.URL)
		if app.Domain != "" {
			line += " [" + app.Domain + "]"
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n"), nil
}

func (c *SyncCore) handleEditAppCommand(ctx context.Context, sender int64, session *dao.BotFatherSession, args string) (string, error) {
	target := firstArg(args)
	if target == "" {
		session.State = botFatherSelectEditApp
		return "Please choose the bot whose Mini App you want to edit.", nil
	}
	bot, err := c.resolveBot(ctx, sender, target)
	if err != nil {
		return "I couldn't find that bot in your bot list.", nil
	}
	session.PendingBotID = bot.GetUser().GetId()
	session.State = botFatherAwaitEditApp
	return "Send four lines: title, short name, description, and HTTPS URL.", nil
}

func (c *SyncCore) handleDeleteAppCommand(ctx context.Context, sender int64, session *dao.BotFatherSession, args string) (string, error) {
	target := firstArg(args)
	if target == "" {
		session.State = botFatherSelectDeleteApp
		return "Please choose the bot whose main Mini App you want to delete.", nil
	}
	bot, err := c.resolveBot(ctx, sender, target)
	if err != nil {
		return "I couldn't find that bot in your bot list.", nil
	}
	if err := c.svcCtx.Dao.Postgres.DeleteBotMiniApp(ctx, sender, bot.GetUser().GetId(), ""); err != nil {
		return "I couldn't delete that Mini App right now.", err
	}
	clearBotFatherSession(session)
	return "Done. The Mini App was deleted.", nil
}

func (c *SyncCore) handleDomainCommand(ctx context.Context, sender int64, session *dao.BotFatherSession, args string) (string, error) {
	parts := strings.Fields(args)
	if len(parts) == 0 {
		session.State = botFatherSelectDomain
		return "Please choose the bot whose Mini App domain you want to set.", nil
	}
	bot, err := c.resolveBot(ctx, sender, parts[0])
	if err != nil {
		return "I couldn't find that bot in your bot list.", nil
	}
	session.PendingBotID = bot.GetUser().GetId()
	session.State = botFatherAwaitDomain
	if len(parts) == 1 {
		return "Send the Mini App domain, such as example.com.", nil
	}
	domain := strings.TrimSpace(parts[1])
	if !validBotFatherDomain(domain) {
		return "That domain is invalid. Send a hostname such as example.com.", nil
	}
	if err := c.svcCtx.Dao.Postgres.SetBotMiniAppDomain(ctx, sender, session.PendingBotID, domain); err != nil {
		return "I couldn't update the Mini App domain right now.", err
	}
	clearBotFatherSession(session)
	return "Done. The Mini App domain was updated.", nil
}

type botFatherAppEdit struct {
	title, shortName, description, url string
}

func parseBotFatherAppEdit(text string) (botFatherAppEdit, bool) {
	lines := strings.Split(text, "\n")
	if len(lines) != 4 {
		return botFatherAppEdit{}, false
	}
	app := botFatherAppEdit{title: strings.TrimSpace(lines[0]), shortName: strings.TrimPrefix(strings.TrimSpace(lines[1]), "@"), description: strings.TrimSpace(lines[2]), url: strings.TrimSpace(lines[3])}
	if strings.EqualFold(app.description, "/empty") {
		app.description = ""
	}
	return app, validBotFatherAppTitle(app.title) && validBotFatherAppShortName(app.shortName) && validBotFatherAppDescription(app.description) && validBotFatherAppURL(app.url)
}

func validBotFatherAppTitle(value string) bool {
	return utf8.ValidString(value) && utf8.RuneCountInString(value) >= 1 && utf8.RuneCountInString(value) <= 64
}

func validBotFatherAppShortName(value string) bool {
	if value == "" || len(value) > 64 {
		return false
	}
	for _, r := range value {
		if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '_' {
			return false
		}
	}
	return true
}

func validBotFatherAppDescription(value string) bool {
	return utf8.ValidString(value) && utf8.RuneCountInString(value) <= 512
}

func validBotFatherAppURL(value string) bool {
	parsed, err := url.ParseRequestURI(strings.TrimSpace(value))
	return err == nil && parsed.Scheme == "https" && parsed.Host != ""
}

func validBotFatherDomain(value string) bool {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" || strings.ContainsAny(value, "/?#:@") {
		return false
	}
	parsed, err := url.Parse("https://" + value)
	return err == nil && parsed.Host == value && parsed.Path == "" && strings.Contains(value, ".")
}

func (c *SyncCore) createBotFromBotFather(ctx context.Context, sender int64, session *dao.BotFatherSession, username string) (string, error) {
	// Looking up the username first makes a retry after a committed create safe.
	if existing, err := c.resolveBot(ctx, sender, username); err != nil && err != mtproto.ErrBotInvalid {
		return "I couldn't check that username right now. Please try again.", err
	} else if existing != nil {
		return c.finishCreatedBot(ctx, sender, session, existing)
	}
	bot, err := c.svcCtx.Dao.UserCreateBot(ctx, &userpb.TLUserCreateBot{Name: session.PendingName, Username: username})
	if err != nil {
		if existing, lookupErr := c.resolveBot(ctx, sender, username); lookupErr == nil && existing != nil {
			return c.finishCreatedBot(ctx, sender, session, existing)
		}
		return "Sorry, that username is not available. Please choose another one.", err
	}
	return c.finishCreatedBot(ctx, sender, session, bot)
}

func (c *SyncCore) finishCreatedBot(ctx context.Context, sender int64, session *dao.BotFatherSession, bot *mtproto.ImmutableUser) (string, error) {
	if bot == nil || bot.GetUser() == nil || bot.GetUser().GetId() <= 0 {
		return "The bot was created, but its profile could not be loaded. Please use /mybots.", nil
	}
	token, err := c.svcCtx.Dao.UserExportBotToken(ctx, &userpb.TLUserExportBotToken{BotId: bot.GetUser().GetId(), AccessHash: bot.GetUser().GetAccessHash()})
	if err != nil {
		return "The bot was created, but I couldn't issue its token. Please use /token.", err
	}
	clearBotFatherSession(session)
	return fmt.Sprintf("Done! Your bot is available at https://t.me/%s.\n\nUse this token to access the HTTP API:\n%s", bot.GetUser().GetUsername(), token.GetV()), nil
}

func (c *SyncCore) listBotFatherBots(ctx context.Context) string {
	bots, err := c.svcCtx.Dao.UserGetCreatedBots(ctx)
	if err != nil || bots == nil || len(bots.GetDatas()) == 0 {
		return "You don't have any bots yet. Use /newbot to create one."
	}
	lines := make([]string, 0, len(bots.GetDatas())+1)
	lines = append(lines, "Your bots:")
	for _, bot := range bots.GetDatas() {
		if bot == nil || bot.GetUser() == nil {
			continue
		}
		lines = append(lines, fmt.Sprintf("@%s (%d)", bot.GetUser().GetUsername(), bot.GetUser().GetId()))
	}
	if len(lines) == 1 {
		return "You don't have any bots yet. Use /newbot to create one."
	}
	return strings.Join(lines, "\n")
}

func (c *SyncCore) handleTokenCommand(ctx context.Context, sender int64, session *dao.BotFatherSession, command, args string) (string, error) {
	target := firstArg(args)
	if target == "" {
		if command == "token" {
			session.State = botFatherSelectToken
		} else {
			session.State = botFatherSelectRevoke
		}
		return "Please choose a bot by sending its username.", nil
	}
	bot, err := c.resolveBot(ctx, sender, target)
	if err != nil {
		return "I couldn't find that bot in your bot list.", nil
	}
	session.PendingBotID = bot.GetUser().GetId()
	if command == "token" {
		session.State = botFatherSelectToken
	} else {
		session.State = botFatherSelectRevoke
	}
	reply, err := c.exportBotToken(ctx, bot, command == "revoke")
	if err == nil {
		clearBotFatherSession(session)
	}
	return reply, err
}

func (c *SyncCore) exportBotToken(ctx context.Context, bot *mtproto.ImmutableUser, revoke bool) (string, error) {
	if bot == nil || bot.GetUser() == nil {
		return "I couldn't find that bot in your bot list.", nil
	}
	token, err := c.svcCtx.Dao.UserExportBotToken(ctx, &userpb.TLUserExportBotToken{BotId: bot.GetUser().GetId(), AccessHash: bot.GetUser().GetAccessHash(), Revoke: revoke})
	if err != nil {
		return "I couldn't issue a token for that bot.", err
	}
	if revoke {
		return "The old token has been revoked. Your new token is:\n" + token.GetV(), nil
	}
	return "Your bot token is:\n" + token.GetV(), nil
}

func (c *SyncCore) handleProfileCommand(ctx context.Context, sender int64, session *dao.BotFatherSession, command, args string) (string, error) {
	state, prompt := botFatherSelectName, "Please choose a bot by sending its username."
	switch command {
	case "setabouttext":
		state, prompt = botFatherSelectAbout, "Please choose a bot by sending its username."
	case "setdescription":
		state, prompt = botFatherSelectDesc, "Please choose a bot by sending its username."
	case "setcommands":
		state, prompt = botFatherSelectCommands, "Please choose a bot by sending its username."
	}
	parts := strings.Fields(args)
	if len(parts) == 0 {
		session.State = state
		return prompt, nil
	}
	bot, err := c.resolveBot(ctx, sender, parts[0])
	if err != nil {
		return "I couldn't find that bot in your bot list.", nil
	}
	rest := strings.TrimSpace(strings.TrimPrefix(args, parts[0]))
	switch state {
	case botFatherSelectName:
		session.PendingBotID, session.State = bot.GetUser().GetId(), botFatherAwaitSetName
		if rest == "" {
			return "Please send the new name for your bot.", nil
		}
		return c.setBotInfoFromBotFather(ctx, sender, session, rest, "name")
	case botFatherSelectAbout:
		session.PendingBotID, session.State = bot.GetUser().GetId(), botFatherAwaitAbout
		if rest == "" {
			return "Please send the new about text.", nil
		}
		return c.setBotInfoFromBotFather(ctx, sender, session, rest, "about")
	case botFatherSelectDesc:
		session.PendingBotID, session.State = bot.GetUser().GetId(), botFatherAwaitDesc
		if rest == "" {
			return "Please send the new description.", nil
		}
		return c.setBotInfoFromBotFather(ctx, sender, session, rest, "description")
	case botFatherSelectCommands:
		session.PendingBotID, session.State = bot.GetUser().GetId(), botFatherAwaitCommands
		if rest == "" {
			return "Send one command per line in the form `command - description`.", nil
		}
		return c.setBotCommandsFromBotFather(ctx, sender, session, rest)
	}
	return "I couldn't process that request.", nil
}

func (c *SyncCore) setBotInfoFromBotFather(ctx context.Context, sender int64, session *dao.BotFatherSession, value, kind string) (string, error) {
	value = strings.TrimSpace(value)
	if (kind == "about" || kind == "description") && strings.EqualFold(value, "/empty") {
		value = ""
	}
	if (kind == "name" && value == "") || !utf8.ValidString(value) || utf8.RuneCountInString(value) > map[string]int{"name": 64, "about": 128, "description": 10240}[kind] {
		return "That text is too long or invalid. Please try again.", nil
	}
	request := &userpb.BotRegistrySetBotInfoRequest{BotId: session.PendingBotID}
	switch kind {
	case "name":
		request.Name = wrapperspb.String(value)
	case "about":
		request.About = wrapperspb.String(value)
	case "description":
		request.Description = wrapperspb.String(value)
	}
	if _, err := c.svcCtx.Dao.SetBotInfo(ctx, request); err != nil {
		return "I couldn't update that bot right now.", err
	}
	clearBotFatherSession(session)
	return "Done.", nil
}

func (c *SyncCore) setBotCommandsFromBotFather(ctx context.Context, sender int64, session *dao.BotFatherSession, text string) (string, error) {
	var (
		commands []*mtproto.BotCommand
		ok       bool
	)
	if strings.EqualFold(strings.TrimSpace(text), "/empty") {
		commands, ok = []*mtproto.BotCommand{}, true
	} else {
		commands, ok = parseBotFatherCommands(text)
	}
	if !ok {
		return "Each line must contain a command and a description, for example `start - Start the bot`.", nil
	}
	if _, err := c.svcCtx.Dao.UserSetBotCommands(ctx, &userpb.TLUserSetBotCommands{UserId: sender, BotId: session.PendingBotID, Commands: commands}); err != nil {
		return "I couldn't update that bot's commands right now.", err
	}
	clearBotFatherSession(session)
	return "Done. The command list has been updated.", nil
}

func (c *SyncCore) handleToggleCommand(ctx context.Context, sender int64, session *dao.BotFatherSession, selectState, kind, args string) (string, error) {
	parts := strings.Fields(args)
	if len(parts) == 0 {
		session.State = selectState
		return "Please choose a bot by sending its username, followed by `enable` or `disable`.", nil
	}
	bot, err := c.resolveBot(ctx, sender, parts[0])
	if err != nil {
		return "I couldn't find that bot in your bot list.", nil
	}
	if len(parts) < 2 {
		session.PendingBotID = bot.GetUser().GetId()
		session.State = selectState
		return "Please reply with `enable` or `disable`.", nil
	}
	session.PendingBotID = bot.GetUser().GetId()
	session.State = selectState
	return c.applyToggle(ctx, session, bot.GetUser().GetId(), kind, parts[1])
}

func (c *SyncCore) handleInlineCommand(ctx context.Context, sender int64, session *dao.BotFatherSession, args string) (string, error) {
	parts := strings.Fields(args)
	if len(parts) == 0 {
		session.State = botFatherSelectInline
		return "Please choose a bot by sending its username, followed by an inline placeholder.", nil
	}
	bot, err := c.resolveBot(ctx, sender, parts[0])
	if err != nil {
		return "I couldn't find that bot in your bot list.", nil
	}
	placeholder := strings.TrimSpace(strings.TrimPrefix(args, parts[0]))
	if placeholder == "" {
		session.PendingBotID, session.State = bot.GetUser().GetId(), botFatherAwaitInline
		return "Please send the inline placeholder text.", nil
	}
	session.PendingBotID = bot.GetUser().GetId()
	session.State = botFatherAwaitInline
	return c.setInlineFromBotFather(ctx, sender, session, placeholder)
}

func (c *SyncCore) setInlineFromBotFather(ctx context.Context, _ int64, session *dao.BotFatherSession, text string) (string, error) {
	text = strings.TrimSpace(text)
	if strings.EqualFold(text, "/empty") {
		if _, err := c.svcCtx.Dao.UserUpdateBotData(ctx, &userpb.TLUserUpdateBotData{BotId: session.PendingBotID, BotInlinePlaceholder: wrapperspb.String("")}); err != nil {
			return "I couldn't disable inline mode for that bot.", err
		}
		clearBotFatherSession(session)
		return "Done. Inline mode is disabled.", nil
	}
	if text == "" || !utf8.ValidString(text) || utf8.RuneCountInString(text) > 128 {
		return "The inline placeholder must be between 1 and 128 characters.", nil
	}
	if _, err := c.svcCtx.Dao.UserUpdateBotData(ctx, &userpb.TLUserUpdateBotData{BotId: session.PendingBotID, BotInlinePlaceholder: wrapperspb.String(text)}); err != nil {
		return "I couldn't enable inline mode for that bot.", err
	}
	clearBotFatherSession(session)
	return "Done. Inline mode is enabled.", nil
}

func (c *SyncCore) applyToggle(ctx context.Context, session *dao.BotFatherSession, botID int64, kind, value string) (string, error) {
	value = strings.ToLower(value)
	if value != "enable" && value != "disable" {
		return "Please reply with `enable` or `disable`.", nil
	}
	enabled := value == "enable"
	request := &userpb.TLUserUpdateBotData{BotId: botID}
	switch kind {
	case "groups":
		request.BotNochats = mtproto.ToBool(!enabled)
	case "inline location":
		request.BotInlineGeo = mtproto.ToBool(enabled)
	case "privacy":
		// bot_chat_history is the server's persisted equivalent of receiving
		// all group messages; it is the inverse of privacy mode.
		request.BotChatHistory = mtproto.ToBool(!enabled)
	}
	if _, err := c.svcCtx.Dao.UserUpdateBotData(ctx, request); err != nil {
		return "I couldn't update that bot right now.", err
	}
	clearBotFatherSession(session)
	return "Done.", nil
}

func (c *SyncCore) resolveBot(ctx context.Context, _ int64, target string) (*mtproto.ImmutableUser, error) {
	target = strings.TrimPrefix(strings.TrimSpace(target), "@")
	if target == "" {
		return nil, errors.New("empty bot target")
	}
	bots, err := c.svcCtx.Dao.UserGetCreatedBots(ctx)
	if err != nil {
		return nil, err
	}
	if bots == nil {
		return nil, mtproto.ErrBotInvalid
	}
	for _, bot := range bots.GetDatas() {
		if bot == nil || bot.GetUser() == nil {
			continue
		}
		if strings.EqualFold(bot.GetUser().GetUsername(), target) || strconv.FormatInt(bot.GetUser().GetId(), 10) == target {
			return bot, nil
		}
	}
	return nil, mtproto.ErrBotInvalid
}

func (c *SyncCore) sendBotFatherReply(ctx context.Context, sender int64, messageID int32, text string) error {
	if c.svcCtx.Dao.MsgClient == nil {
		return mtproto.ErrMethodNotImpl
	}
	raw := sha256.Sum256([]byte(fmt.Sprintf("botfather:%d:%d", sender, messageID)))
	randomID := int64(binary.LittleEndian.Uint64(raw[:]) & 0x7fffffffffffffff)
	if randomID == 0 {
		randomID = 1
	}
	_, err := c.svcCtx.Dao.MsgSendMessageV2(ctx, &msgpb.TLMsgSendMessageV2{
		UserId: userpb.BotFatherId, PeerType: mtproto.PEER_USER, PeerId: sender,
		Message: []*msgpb.OutboxMessage{msgpb.MakeTLOutboxMessage(&msgpb.OutboxMessage{
			RandomId: randomID,
			Message:  mtproto.MakeTLMessage(&mtproto.Message{Out: true, FromId: mtproto.MakePeerUser(userpb.BotFatherId), PeerId: mtproto.MakePeerUser(sender), Date: int32(time.Now().Unix()), Message: text}).To_Message(),
		}).To_OutboxMessage()},
	})
	return err
}

func botFatherRPCContext(ctx context.Context, userID int64) (context.Context, error) {
	withMetadata, err := rpcmetadata.RpcMetadataToOutgoing(ctx, &rpcmetadata.RpcMetadata{UserId: userID, PermAuthKeyId: 0})
	if err != nil {
		return nil, err
	}
	outgoing, ok := metadata.FromOutgoingContext(withMetadata)
	if !ok {
		return nil, errors.New("sync: failed to construct BotFather metadata")
	}
	return metadata.NewIncomingContext(withMetadata, outgoing), nil
}

func clearBotFatherSession(session *dao.BotFatherSession) {
	session.State = botFatherIdle
	session.PendingName = ""
	session.PendingBotID = 0
	session.PendingMenuText = ""
	session.PendingMenuURL = ""
	session.PendingAppTitle = ""
	session.PendingAppShortName = ""
	session.PendingAppDescription = ""
	session.PendingAppURL = ""
}

func parseBotFatherCommand(text string) (string, string) {
	text = strings.TrimSpace(text)
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
	args := strings.TrimSpace(strings.TrimPrefix(text, parts[0]))
	return strings.ToLower(command), args
}

func firstArg(args string) string {
	parts := strings.Fields(args)
	if len(parts) == 0 {
		return ""
	}
	return parts[0]
}

func normalizeBotUsername(value string) string {
	value = strings.TrimSpace(value)
	value = strings.TrimPrefix(value, "@")
	return value
}

func validBotFatherName(value string) bool {
	if !utf8.ValidString(value) || utf8.RuneCountInString(value) == 0 || utf8.RuneCountInString(value) > 64 {
		return false
	}
	for _, r := range value {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}

func validBotFatherUsername(value string) bool {
	if len(value) < 5 || len(value) > 32 || !strings.HasSuffix(strings.ToLower(value), "bot") {
		return false
	}
	for i, r := range value {
		if (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && (r < '0' || r > '9') && r != '_' {
			return false
		}
		if i == 0 && (r < 'A' || r > 'Z') && (r < 'a' || r > 'z') {
			return false
		}
	}
	return true
}

func parseBotFatherCommands(text string) ([]*mtproto.BotCommand, bool) {
	lines := strings.Split(strings.TrimSpace(text), "\n")
	commands := make([]*mtproto.BotCommand, 0, len(lines))
	seen := make(map[string]struct{}, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "-", 2)
		if len(parts) != 2 {
			return nil, false
		}
		command := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(parts[0]), "/"))
		description := strings.TrimSpace(parts[1])
		if command == "" || description == "" || len(command) > 32 || len(commands) >= 100 {
			return nil, false
		}
		for _, r := range command {
			if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '_' {
				return nil, false
			}
		}
		if _, exists := seen[command]; exists {
			return nil, false
		}
		seen[command] = struct{}{}
		commands = append(commands, &mtproto.BotCommand{Command: command, Description: description})
	}
	return commands, len(commands) > 0
}
