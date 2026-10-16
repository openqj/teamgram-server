package http

import (
	"errors"
	stdhtml "html"
	"net/url"
	"strconv"
	"strings"
	"unicode/utf16"

	xhtml "golang.org/x/net/html"
	"golang.org/x/net/html/atom"

	"github.com/teamgram/teamgram-server/app/botapi"
)

var errBotAPIParseMode = errors.New("parse_mode is invalid")

type botAPIEntityRange struct {
	typ      string
	start    int
	end      int
	url      string
	language string
	userID   int64
	customID string
}

// botAPITextEntities parses Telegram's formatting modes and returns the
// rendered text plus Bot API UTF-16 entities. Explicit entity fields always
// win over parse_mode, including an explicitly supplied empty array.
func botAPITextEntities(request *botAPIRequest, textField, entitiesField, parseModeField string) (string, []botapi.MessageEntity, error) {
	if request == nil {
		return "", nil, errBotAPIParseMode
	}
	text := request.text(textField)
	if request.has(entitiesField) && strings.TrimSpace(string(request.raw(entitiesField))) != "null" {
		var entities []botapi.MessageEntity
		if err := request.json(entitiesField, &entities); err != nil {
			return "", nil, errors.New(entitiesField + " is invalid")
		}
		return text, entities, nil
	}
	mode := strings.TrimSpace(request.string(parseModeField))
	if mode == "" {
		return text, nil, nil
	}
	return botAPIParseFormattedText(text, mode)
}

func botAPICaptionEntities(request *botAPIRequest, textField, entitiesField string) (string, []botapi.MessageEntity, error) {
	parseModeField := "parse_mode"
	if request != nil && request.has("caption_parse_mode") {
		parseModeField = "caption_parse_mode"
	}
	return botAPITextEntities(request, textField, entitiesField, parseModeField)
}

func botAPIMediaGroupCaption(item botAPIMediaGroupItem) (string, []botapi.MessageEntity, error) {
	mode := strings.TrimSpace(item.ParseMode)
	if item.CaptionParseMode != "" {
		mode = strings.TrimSpace(item.CaptionParseMode)
	}
	if len(item.CaptionEntities) > 0 || mode == "" {
		return item.Caption, item.CaptionEntities, nil
	}
	return botAPIParseFormattedText(item.Caption, mode)
}

func botAPIParseFormattedText(text, mode string) (string, []botapi.MessageEntity, error) {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "html":
		return botAPIParseHTML(text)
	case "markdown", "markdownv2":
		return botAPIParseMarkdown(text, strings.EqualFold(strings.TrimSpace(mode), "markdownv2"))
	default:
		return "", nil, errBotAPIParseMode
	}
}

func botAPIEntityRangesToMessageEntities(text []rune, ranges []botAPIEntityRange) []botapi.MessageEntity {
	if len(ranges) == 0 {
		return nil
	}
	units := make([]int, len(text)+1)
	for i, r := range text {
		units[i+1] = units[i] + len(utf16.Encode([]rune{r}))
	}
	result := make([]botapi.MessageEntity, 0, len(ranges))
	for _, entity := range ranges {
		if entity.start < 0 || entity.end <= entity.start || entity.end > len(text) {
			continue
		}
		value := botapi.MessageEntity{
			Type:     entity.typ,
			Offset:   units[entity.start],
			Length:   units[entity.end] - units[entity.start],
			URL:      entity.url,
			Language: entity.language,
		}
		if entity.userID > 0 {
			value.User = &botapi.User{ID: entity.userID}
		}
		if entity.customID != "" {
			value.CustomEmojiID = entity.customID
		}
		result = append(result, value)
	}
	return result
}

func botAPIParseHTML(source string) (string, []botapi.MessageEntity, error) {
	root, err := xhtml.ParseFragment(strings.NewReader(source), &xhtml.Node{Type: xhtml.ElementNode, Data: "body", DataAtom: atom.Body})
	if err != nil {
		return "", nil, errBotAPIParseMode
	}
	text := make([]rune, 0, len(source))
	ranges := make([]botAPIEntityRange, 0)
	for _, node := range root {
		if err := botAPIHTMLNode(node, &text, &ranges); err != nil {
			return "", nil, err
		}
	}
	return string(text), botAPIEntityRangesToMessageEntities(text, ranges), nil
}

func botAPIHTMLNode(node *xhtml.Node, text *[]rune, ranges *[]botAPIEntityRange) error {
	if node == nil {
		return nil
	}
	switch node.Type {
	case xhtml.TextNode:
		*text = append(*text, []rune(node.Data)...)
		return nil
	case xhtml.CommentNode:
		return nil
	case xhtml.DocumentNode, xhtml.ElementNode:
		// Continue below for element nodes.
	default:
		return errBotAPIParseMode
	}
	if node.Type == xhtml.DocumentNode {
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			if err := botAPIHTMLNode(child, text, ranges); err != nil {
				return err
			}
		}
		return nil
	}
	tag := strings.ToLower(node.Data)
	if tag == "body" || tag == "html" || tag == "head" {
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			if err := botAPIHTMLNode(child, text, ranges); err != nil {
				return err
			}
		}
		return nil
	}
	typ, attrs, recognized := botAPIHTMLEntity(tag, node.Attr)
	if !recognized {
		return errBotAPIParseMode
	}
	start := len(*text)
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if err := botAPIHTMLNode(child, text, ranges); err != nil {
			return err
		}
	}
	if tag == "tg-emoji" && len(*text) == start {
		// Telegram represents an empty tg-emoji tag as one visible custom emoji.
		*text = append(*text, '\uFFFC')
	}
	end := len(*text)
	if end > start && typ != "" {
		entity := botAPIEntityRange{typ: typ, start: start, end: end, url: attrs.url, language: attrs.language, userID: attrs.userID, customID: attrs.customID}
		*ranges = append(*ranges, entity)
	}
	return nil
}

type botAPIHTMLAttrs struct {
	url      string
	language string
	userID   int64
	customID string
}

func botAPIHTMLEntity(tag string, attrs []xhtml.Attribute) (string, botAPIHTMLAttrs, bool) {
	value := botAPIHTMLAttrs{}
	attr := make(map[string]string, len(attrs))
	for _, item := range attrs {
		attr[strings.ToLower(item.Key)] = stdhtml.UnescapeString(item.Val)
	}
	switch tag {
	case "b", "strong":
		return "bold", value, true
	case "i", "em":
		return "italic", value, true
	case "u", "ins":
		return "underline", value, true
	case "s", "strike", "del":
		return "strikethrough", value, true
	case "tg-spoiler":
		return "spoiler", value, true
	case "span":
		if strings.EqualFold(attr["class"], "tg-spoiler") {
			return "spoiler", value, true
		}
		return "", value, false
	case "code":
		return "code", value, true
	case "pre":
		return "pre", value, true
	case "a":
		value.url = attr["href"]
		if value.url == "" {
			return "", value, false
		}
		parsed, err := url.Parse(value.url)
		if err == nil && strings.EqualFold(parsed.Scheme, "tg") && strings.EqualFold(parsed.Host, "user") {
			if id, parseErr := strconv.ParseInt(parsed.Query().Get("id"), 10, 64); parseErr == nil && id > 0 {
				value.userID = id
				return "text_mention", value, true
			}
		}
		return "text_link", value, true
	case "tg-emoji":
		value.customID = attr["emoji-id"]
		if _, err := strconv.ParseInt(value.customID, 10, 64); err != nil || value.customID == "" {
			return "", value, false
		}
		return "custom_emoji", value, true
	case "blockquote":
		if strings.EqualFold(attr["expandable"], "") || strings.EqualFold(attr["expandable"], "false") {
			return "blockquote", value, true
		}
		return "expandable_blockquote", value, true
	default:
		return "", value, false
	}
}

type botAPIMarkdownSegment struct {
	text   []rune
	ranges []botAPIEntityRange
}

func (s *botAPIMarkdownSegment) appendPlain(value []rune) {
	s.text = append(s.text, value...)
}

func (s *botAPIMarkdownSegment) appendSegment(value botAPIMarkdownSegment, start int) {
	s.text = append(s.text, value.text...)
	for _, entity := range value.ranges {
		entity.start += start
		entity.end += start
		s.ranges = append(s.ranges, entity)
	}
}

func botAPIParseMarkdown(source string, markdownV2 bool) (string, []botapi.MessageEntity, error) {
	segment, _, ok := botAPIMarkdownSegmentParse([]rune(source), markdownV2, "")
	if !ok {
		return "", nil, errBotAPIParseMode
	}
	return string(segment.text), botAPIEntityRangesToMessageEntities(segment.text, segment.ranges), nil
}

func botAPIMarkdownSegmentParse(source []rune, markdownV2 bool, stop string) (botAPIMarkdownSegment, int, bool) {
	result := botAPIMarkdownSegment{}
	for i := 0; i < len(source); {
		if stop != "" && strings.HasPrefix(string(source[i:]), stop) {
			return result, i + len([]rune(stop)), true
		}
		if source[i] == '\\' {
			if i+1 < len(source) && (!markdownV2 || strings.ContainsRune("_*[]()~`>#+-=|{}.!\\", source[i+1])) {
				result.appendPlain([]rune{source[i+1]})
				i += 2
				continue
			}
			result.appendPlain([]rune{'\\'})
			i++
			continue
		}
		if source[i] == '`' {
			delim := "`"
			entityType := "code"
			if markdownV2 && i+2 < len(source) && source[i+1] == '`' && source[i+2] == '`' {
				delim = "```"
				entityType = "pre"
			}
			inner, consumed, found := botAPIMarkdownSegmentParse(source[i+len([]rune(delim)):], markdownV2, delim)
			if found {
				start := len(result.text)
				result.appendSegment(inner, start)
				end := len(result.text)
				result.ranges = append(result.ranges, botAPIEntityRange{typ: entityType, start: start, end: end})
				i += len([]rune(delim)) + consumed
				continue
			}
		}
		if source[i] == '[' {
			close := botAPIFindUnescaped(source[i+1:], ']')
			if close >= 0 {
				urlStart := i + 1 + close + 1
				if urlStart < len(source) && source[urlStart] == '(' {
					urlEnd := botAPIFindUnescaped(source[urlStart+1:], ')')
					if urlEnd >= 0 {
						label, _, labelOK := botAPIMarkdownSegmentParse(source[i+1:i+1+close], markdownV2, "")
						if labelOK || len(label.text) > 0 {
							start := len(result.text)
							result.appendSegment(label, start)
							end := len(result.text)
							target := string(source[urlStart+1 : urlStart+1+urlEnd])
							result.ranges = append(result.ranges, botAPIEntityRange{typ: "text_link", start: start, end: end, url: target})
							i = urlStart + 1 + urlEnd + 1
							continue
						}
					}
				}
			}
		}
		if delim, typ, allowed := botAPIMarkdownDelimiter(source, i, markdownV2); allowed {
			inner, consumed, found := botAPIMarkdownSegmentParse(source[i+len(delim):], markdownV2, delim)
			if found && len(inner.text) > 0 {
				start := len(result.text)
				result.appendSegment(inner, start)
				end := len(result.text)
				result.ranges = append(result.ranges, botAPIEntityRange{typ: typ, start: start, end: end})
				i += len(delim) + consumed
				continue
			}
		}
		result.appendPlain([]rune{source[i]})
		i++
	}
	return result, len(source), stop == ""
}

func botAPIMarkdownDelimiter(source []rune, i int, markdownV2 bool) (string, string, bool) {
	if markdownV2 && i+1 < len(source) {
		switch string(source[i : i+2]) {
		case "__":
			return "__", "underline", true
		case "||":
			return "||", "spoiler", true
		}
	}
	switch source[i] {
	case '*':
		return "*", "bold", true
	case '_':
		return "_", "italic", true
	case '~':
		if markdownV2 {
			return "~", "strikethrough", true
		}
	}
	return "", "", false
}

func botAPIFindUnescaped(source []rune, want rune) int {
	escaped := false
	for i, r := range source {
		if escaped {
			escaped = false
			continue
		}
		if r == '\\' {
			escaped = true
			continue
		}
		if r == want {
			return i
		}
	}
	return -1
}
