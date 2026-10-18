package http

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/teamgram/teamgram-server/app/botapi"
	"github.com/teamgram/teamgram-server/app/botapi/store"
	"github.com/teamgram/teamgram-server/app/interface/httpserver/internal/svc"
)

type botAPICommandScope struct {
	Type   string          `json:"type"`
	ChatID json.RawMessage `json:"chat_id"`
	UserID json.RawMessage `json:"user_id"`
}

// botAPICommandVariant converts the official Bot API scope object into a
// stable key for the HTTP-only command variant store. The default variant
// continues to use user.setBotCommands so MTProto clients keep their source
// of truth.
func botAPICommandVariant(request *botAPIRequest) (string, string, bool, error) {
	scopeKey := "default"
	if request != nil && request.has("scope") {
		raw := strings.TrimSpace(string(request.raw("scope")))
		if raw != "" && raw != "null" {
			var scope botAPICommandScope
			if err := request.json("scope", &scope); err != nil || strings.TrimSpace(scope.Type) == "" {
				return "", "", false, errors.New("scope is invalid")
			}
			typeName := strings.ToLower(strings.TrimSpace(scope.Type))
			switch typeName {
			case "default", "all_private_chats", "all_group_chats", "all_chat_administrators":
				scopeKey = typeName
			case "chat", "chat_administrators":
				chatKey, err := botAPICommandScopeChatKey(scope.ChatID)
				if err != nil {
					return "", "", false, fmt.Errorf("scope.chat_id is invalid")
				}
				scopeKey = typeName + ":" + chatKey
			case "chat_member":
				chatKey, err := botAPICommandScopeChatKey(scope.ChatID)
				if err != nil {
					return "", "", false, fmt.Errorf("scope.chat_id is invalid")
				}
				userID, err := botAPICommandScopeInt64(scope.UserID)
				if err != nil || userID <= 0 {
					return "", "", false, errors.New("scope.user_id is invalid")
				}
				scopeKey = typeName + ":" + chatKey + ":" + strconv.FormatInt(userID, 10)
			default:
				return "", "", false, errors.New("scope type is invalid")
			}
		}
	}
	languageCode := ""
	if request != nil && request.has("language_code") {
		raw := strings.TrimSpace(string(request.raw("language_code")))
		if raw != "" && raw != "null" {
			languageCode = strings.ToLower(request.string("language_code"))
			if len(languageCode) != 2 || !isBotAPILanguageCode(languageCode) {
				return "", "", false, errors.New("language_code is invalid")
			}
		}
	}
	return scopeKey, languageCode, scopeKey == "default" && languageCode == "", nil
}

func botAPICommandScopeChatKey(raw json.RawMessage) (string, error) {
	if len(raw) == 0 || strings.TrimSpace(string(raw)) == "null" {
		return "", errors.New("chat id is missing")
	}
	value := strings.TrimSpace(string(raw))
	if value == "" {
		return "", errors.New("chat id is empty")
	}
	if value[0] == '"' {
		var decoded string
		if err := json.Unmarshal(raw, &decoded); err != nil {
			return "", err
		}
		value = strings.TrimSpace(decoded)
	}
	if value == "" {
		return "", errors.New("chat id is empty")
	}
	if id, err := strconv.ParseInt(value, 10, 64); err == nil {
		if id == 0 {
			return "", errors.New("chat id is zero")
		}
		return strconv.FormatInt(id, 10), nil
	}
	if !strings.HasPrefix(value, "@") || len(value) <= 1 || len(value) > 64 {
		return "", errors.New("chat username is invalid")
	}
	for i := 1; i < len(value); i++ {
		ch := value[i]
		if (ch < 'a' || ch > 'z') && (ch < 'A' || ch > 'Z') && (ch < '0' || ch > '9') && ch != '_' {
			return "", errors.New("chat username is invalid")
		}
	}
	return strings.ToLower(value), nil
}

func botAPICommandScopeInt64(raw json.RawMessage) (int64, error) {
	if len(raw) == 0 || strings.TrimSpace(string(raw)) == "null" {
		return 0, errors.New("id is missing")
	}
	value := strings.TrimSpace(string(raw))
	if value == "" {
		return 0, errors.New("id is empty")
	}
	if value[0] == '"' {
		var decoded string
		if err := json.Unmarshal(raw, &decoded); err != nil {
			return 0, err
		}
		value = strings.TrimSpace(decoded)
	}
	return strconv.ParseInt(value, 10, 64)
}

func isBotAPILanguageCode(value string) bool {
	for i := 0; i < len(value); i++ {
		if value[i] < 'a' || value[i] > 'z' {
			return false
		}
	}
	return true
}

func validateBotAPICommandVariant(commands []botapi.BotCommand) error {
	if len(commands) > 100 {
		return errors.New("commands must contain at most 100 items")
	}
	seen := make(map[string]struct{}, len(commands))
	for _, command := range commands {
		if command.IsEphemeral || command.Command == "" || len(command.Command) > 32 {
			return errors.New("commands contains an invalid command")
		}
		for i := 0; i < len(command.Command); i++ {
			ch := command.Command[i]
			if (ch < 'a' || ch > 'z') && (ch < '0' || ch > '9') && ch != '_' {
				return errors.New("commands contains an invalid command")
			}
		}
		if _, ok := seen[command.Command]; ok {
			return errors.New("commands contains a duplicate command")
		}
		seen[command.Command] = struct{}{}
		if strings.TrimSpace(command.Description) == "" || !utf8.ValidString(command.Description) || utf8.RuneCountInString(command.Description) > 256 {
			return errors.New("commands contains an invalid description")
		}
	}
	return nil
}

func botAPICommandStoreRequired(ctx *svc.ServiceContext) (int, int, string, bool) {
	if ctx == nil || ctx.BotAPIStore == nil || ctx.BotAPIStore.Pool == nil {
		return 501, 501, "Not Implemented: scoped bot command store is not configured", true
	}
	return 0, 0, "", false
}

// botAPIGetCommandVariant follows Telegram's fallback order for a scoped or
// localized command lookup. The HTTP-only table stores exact variants; a
// missing exact row falls back to the same scope's default language, then the
// default scope for the requested language, and finally the default variant.
func botAPIGetCommandVariant(ctx *svc.ServiceContext, requestCtx context.Context, botID int64, scopeKey, languageCode string) ([]botapi.BotCommand, bool, error) {
	candidates := make([][2]string, 0, 4)
	add := func(scope, language string) {
		for _, candidate := range candidates {
			if candidate[0] == scope && candidate[1] == language {
				return
			}
		}
		candidates = append(candidates, [2]string{scope, language})
	}
	add(scopeKey, languageCode)
	if languageCode != "" {
		add(scopeKey, "")
	}
	if scopeKey != "default" {
		add("default", languageCode)
	}
	if scopeKey != "default" || languageCode != "" {
		add("default", "")
	}
	for _, candidate := range candidates {
		commands, found, err := ctx.BotAPIStore.GetBotCommands(requestCtx, botID, candidate[0], candidate[1])
		if err != nil || found {
			return commands, found, err
		}
	}
	return []botapi.BotCommand{}, false, nil
}

func botAPISetLifecycle(ctx *svc.ServiceContext, requestCtx context.Context, botID int64, method string) (any, int, int, string) {
	if ctx == nil || ctx.BotAPIStore == nil || ctx.BotAPIStore.Pool == nil {
		return nil, 501, 501, "Not Implemented: bot lifecycle store is not configured"
	}
	next := store.BotLifecycleLoggedOut
	if method == "close" {
		next = store.BotLifecycleClosed
	}
	allowed, err := ctx.BotAPIStore.TransitionBotLifecycle(requestCtx, botID, next)
	if err != nil {
		return nil, 500, 500, "Internal Server Error: bot lifecycle state could not be saved"
	}
	if !allowed {
		return nil, 401, 401, "Unauthorized"
	}
	return true, 200, 0, ""
}

func botAPIRejectInactiveLifecycle(ctx *svc.ServiceContext, requestCtx context.Context, botID int64, method string) (int, int, string, bool) {
	if method == "logOut" || method == "close" {
		return 0, 0, "", false
	}
	if ctx == nil || ctx.BotAPIStore == nil || ctx.BotAPIStore.Pool == nil {
		// The lifecycle side channel is optional for unrelated Bot API calls;
		// lifecycle methods themselves fail closed in botAPISetLifecycle.
		return 0, 0, "", false
	}
	state, err := ctx.BotAPIStore.GetBotLifecycle(requestCtx, botID)
	if err != nil {
		return 500, 500, "Internal Server Error: bot lifecycle state could not be read", true
	}
	if state != store.BotLifecycleActive {
		return 401, 401, "Unauthorized", true
	}
	return 0, 0, "", false
}
