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
	if button.GetPredicateName() != "keyboardButtonWebView" || button.GetUrl() != "https://example.test/app" {
		t.Fatalf("button = %#v, want keyboardButtonWebView with Mini App URL", button)
	}
}
