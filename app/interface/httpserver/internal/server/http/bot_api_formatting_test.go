package http

import (
	"encoding/json"
	"testing"
)

func TestBotAPIParseHTMLUsesUTF16Offsets(t *testing.T) {
	text, entities, err := botAPIParseFormattedText("<b>😀</b>", "HTML")
	if err != nil {
		t.Fatalf("parse HTML: %v", err)
	}
	if text != "😀" {
		t.Fatalf("text = %q, want emoji", text)
	}
	if len(entities) != 1 || entities[0].Type != "bold" || entities[0].Offset != 0 || entities[0].Length != 2 {
		t.Fatalf("entities = %+v, want bold UTF-16 length 2", entities)
	}
}

func TestBotAPIParseMarkdownV2NestedEntities(t *testing.T) {
	text, entities, err := botAPIParseFormattedText("*bold _😀_*", "MarkdownV2")
	if err != nil {
		t.Fatalf("parse MarkdownV2: %v", err)
	}
	if text != "bold 😀" {
		t.Fatalf("text = %q, want rendered text", text)
	}
	if len(entities) != 2 {
		t.Fatalf("entities = %+v, want nested bold and italic", entities)
	}
	if entities[0].Type != "italic" || entities[0].Offset != 5 || entities[0].Length != 2 {
		t.Fatalf("inner entity = %+v, want italic at UTF-16 offset 5", entities[0])
	}
	if entities[1].Type != "bold" || entities[1].Offset != 0 || entities[1].Length != 7 {
		t.Fatalf("outer entity = %+v, want bold length 7", entities[1])
	}
}

func TestBotAPIExplicitEntitiesOverrideParseMode(t *testing.T) {
	request := &botAPIRequest{values: map[string]json.RawMessage{
		"text":       json.RawMessage(`"*literal*"`),
		"entities":   json.RawMessage(`[]`),
		"parse_mode": json.RawMessage(`"unknown"`),
	}}
	text, entities, err := botAPITextEntities(request, "text", "entities", "parse_mode")
	if err != nil {
		t.Fatalf("explicit entities should bypass parse mode: %v", err)
	}
	if text != "*literal*" || len(entities) != 0 {
		t.Fatalf("got text=%q entities=%+v, want literal text and empty entities", text, entities)
	}
}

func TestBotAPICaptionParseMode(t *testing.T) {
	request := &botAPIRequest{values: map[string]json.RawMessage{
		"caption":            json.RawMessage(`"<i>caption</i>"`),
		"caption_parse_mode": json.RawMessage(`"HTML"`),
	}}
	text, entities, err := botAPICaptionEntities(request, "caption", "caption_entities")
	if err != nil {
		t.Fatalf("parse caption: %v", err)
	}
	if text != "caption" || len(entities) != 1 || entities[0].Type != "italic" {
		t.Fatalf("got text=%q entities=%+v, want italic caption", text, entities)
	}
}

func TestBotAPIParseModeRejectsUnknownMode(t *testing.T) {
	if _, _, err := botAPIParseFormattedText("text", "RichText"); err == nil {
		t.Fatal("unknown parse mode should fail")
	}
}

func TestBotAPIMediaGroupCaptionParsesMode(t *testing.T) {
	text, entities, err := botAPIMediaGroupCaption(botAPIMediaGroupItem{
		Caption:   "[link](https://example.com)",
		ParseMode: "Markdown",
	})
	if err != nil {
		t.Fatalf("parse media group caption: %v", err)
	}
	if text != "link" || len(entities) != 1 || entities[0].Type != "text_link" || entities[0].URL != "https://example.com" {
		t.Fatalf("got text=%q entities=%+v, want text link", text, entities)
	}
}
