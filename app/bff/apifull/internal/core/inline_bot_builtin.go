package core

import (
	"encoding/json"
	"net/url"
	"strings"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/persist"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// These bots intentionally return provider search links. The deployment does
// not carry provider API credentials, so returning a real search hand-off is
// more accurate than presenting static or fabricated media as search results.
type builtinInlineBotSpec struct {
	id          int64
	name        string
	label       string
	searchBase  string
	description string
}

func builtinInlineBotSpecForID(id int64) (builtinInlineBotSpec, bool) {
	switch id {
	case userpb.BotGifId:
		return builtinInlineBotSpec{id: id, name: userpb.BotGifName, label: "GIF", searchBase: "https://giphy.com/search/", description: "Open GIPHY GIF search"}, true
	case userpb.BotVidId:
		return builtinInlineBotSpec{id: id, name: userpb.BotVidName, label: "Video", searchBase: "https://www.youtube.com/results?search_query=", description: "Open YouTube video search"}, true
	case userpb.BotPicId:
		return builtinInlineBotSpec{id: id, name: userpb.BotPicName, label: "Yandex image", searchBase: "https://yandex.com/images/search?text=", description: "Open Yandex image search"}, true
	case userpb.BotBingId:
		return builtinInlineBotSpec{id: id, name: userpb.BotBingName, label: "Bing image", searchBase: "https://www.bing.com/images/search?q=", description: "Open Bing image search"}, true
	case userpb.BotWikiId:
		return builtinInlineBotSpec{id: id, name: userpb.BotWikiName, label: "Wikipedia", searchBase: "https://en.wikipedia.org/w/index.php?search=", description: "Open Wikipedia search"}, true
	default:
		return builtinInlineBotSpec{}, false
	}
}

// publishBuiltinInlineBotResults satisfies the normal durable inline-query
// contract, so selecting the result still goes through sendInlineBotResult.
func publishBuiltinInlineBotResults(query persist.InlineBotQuery) (bool, error) {
	spec, ok := builtinInlineBotSpecForID(query.BotID)
	if !ok {
		return false, nil
	}
	searchURL := builtinInlineSearchURL(spec, query.Query)
	term := strings.TrimSpace(query.Query)
	title := spec.label + " search"
	description := spec.description
	if term != "" {
		title += ": " + term
		description += " for \"" + term + "\""
	} else {
		description += " for a keyword"
	}
	sendMessage := mtproto.MakeTLInputBotInlineMessageMediaWebPage(&mtproto.InputBotInlineMessage{
		Message: title,
		Url:     searchURL,
	}).To_InputBotInlineMessage()
	result := mtproto.MakeTLInputBotInlineResult(&mtproto.InputBotInlineResult{
		Id:          "search-" + spec.name,
		Type:        "article",
		Title:       wrapperspb.String(title),
		Description: wrapperspb.String(description),
		Url:         wrapperspb.String(searchURL),
		SendMessage: sendMessage,
	}).To_InputBotInlineResult()
	raw, err := json.Marshal(result)
	if err != nil {
		return true, err
	}
	return true, persist.SetInlineBotResultsWithSwitches(
		query.BotID,
		query.QueryID,
		false,
		false,
		30,
		"",
		nil,
		nil,
		[]json.RawMessage{raw},
	)
}

func builtinInlineSearchURL(spec builtinInlineBotSpec, rawQuery string) string {
	term := strings.TrimSpace(rawQuery)
	if term == "" {
		if spec.name == userpb.BotGifName {
			return "https://giphy.com/"
		}
		return strings.TrimSuffix(spec.searchBase, "?search_query=")
	}
	if spec.name == userpb.BotGifName {
		return spec.searchBase + url.PathEscape(term)
	}
	return spec.searchBase + url.QueryEscape(term)
}
