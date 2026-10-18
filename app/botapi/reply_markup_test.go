package botapi

import (
	"testing"
)

func TestReplyKeyboardWebAppConversion(t *testing.T) {
	markup := ReplyMarkupToMTProto(ReplyMarkupInput{
		Keyboard: [][]map[string]any{{
			{"text": "Open app", "web_app": map[string]any{"url": "https://example.test/app"}},
		}},
	})
	if markup == nil || markup.GetPredicateName() != "replyKeyboardMarkup" {
		t.Fatalf("reply markup = %#v, want replyKeyboardMarkup", markup)
	}
	rows := markup.GetRows()
	if len(rows) != 1 || len(rows[0].GetButtons()) != 1 {
		t.Fatalf("rows = %#v, want one button", rows)
	}
	button := rows[0].GetButtons()[0]
	if button.GetPredicateName() != "keyboardButtonSimpleWebView" || button.GetUrl() != "https://example.test/app" {
		t.Fatalf("button = %#v, want keyboardButtonSimpleWebView with Mini App URL", button)
	}
}

func TestInlineKeyboardURLConversion(t *testing.T) {
	markup := ReplyMarkupToMTProto(ReplyMarkupInput{
		InlineKeyboard: [][]InlineKeyboardButton{{{
			Text: "Open Mini App",
			URL:  "https://t.me/web_e2e_bot?startapp=direct-e2e",
		}}},
	})
	if markup == nil || markup.GetPredicateName() != "replyInlineMarkup" {
		t.Fatalf("markup = %#v, want replyInlineMarkup", markup)
	}
	rows := markup.GetRows()
	if len(rows) != 1 || len(rows[0].GetButtons()) != 1 {
		t.Fatalf("rows = %#v, want one button", rows)
	}
	button := rows[0].GetButtons()[0]
	if button.GetPredicateName() != "keyboardButtonUrl" || button.GetUrl() != "https://t.me/web_e2e_bot?startapp=direct-e2e" {
		t.Fatalf("button = %#v, want keyboardButtonUrl with direct Mini App link", button)
	}
}

func TestReplyKeyboardConversionPreservesPlaceholderAndFlags(t *testing.T) {
	markup := ReplyMarkupToMTProto(ReplyMarkupInput{
		Keyboard:              [][]map[string]any{{{"text": "Keyboard ping"}}},
		ResizeKeyboard:        true,
		OneTimeKeyboard:       true,
		Selective:             true,
		IsPersistent:          true,
		InputFieldPlaceholder: "Choose an action",
	})
	if markup == nil || markup.GetPredicateName() != "replyKeyboardMarkup" {
		t.Fatalf("markup = %#v, want replyKeyboardMarkup", markup)
	}
	if got := markup.GetPlaceholder(); got == nil || got.GetValue() != "Choose an action" {
		t.Fatalf("placeholder = %#v, want Choose an action", got)
	}
	if !markup.GetResize() || !markup.GetSingleUse() || !markup.GetSelective() || !markup.GetPersistent() {
		t.Fatalf("keyboard flags = resize:%v single_use:%v selective:%v persistent:%v, want all true", markup.GetResize(), markup.GetSingleUse(), markup.GetSelective(), markup.GetPersistent())
	}

	withoutPlaceholder := ReplyMarkupToMTProto(ReplyMarkupInput{
		Keyboard: [][]map[string]any{{{"text": "Keyboard ping"}}},
	})
	if withoutPlaceholder == nil || withoutPlaceholder.GetPlaceholder() != nil {
		t.Fatalf("empty placeholder = %#v, want omitted", withoutPlaceholder.GetPlaceholder())
	}
}
