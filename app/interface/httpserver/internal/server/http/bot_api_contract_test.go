package http

import (
	"encoding/json"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/teamgram/proto/mtproto"
)

func TestCanonicalBotAPIMethod(t *testing.T) {
	for _, method := range []string{"GETME", "sendmessage", "EditGeneralForumTopic"} {
		if got := canonicalBotAPIMethod(method); got == method {
			t.Fatalf("canonicalBotAPIMethod(%q) did not resolve a known method", method)
		}
	}
	if got := canonicalBotAPIMethod("futureMethod"); got != "futureMethod" {
		t.Fatalf("canonicalBotAPIMethod preserved unknown method as %q", got)
	}
}

func TestBotAPIMethodSurface(t *testing.T) {
	if len(botAPIMethodNames) != 186 {
		t.Fatalf("canonical method table has %d entries, want 183 official methods plus two project extensions and kickChatMember", len(botAPIMethodNames))
	}
	for _, method := range []string{"logOut", "close", "savePreparedInlineMessage", "deleteEphemeralMessage", "sendRichMessage", "sendRichMessageDraft", "kickChatMember"} {
		if got := canonicalBotAPIMethod(strings.ToUpper(method)); got != method {
			t.Fatalf("canonicalBotAPIMethod(%q) = %q, want %q", strings.ToUpper(method), got, method)
		}
	}
}

func TestBusinessAccountProfilePhotoDispatch(t *testing.T) {
	bot := &mtproto.ImmutableUser{User: &mtproto.UserData{Id: 42}}
	request := &botAPIRequest{values: map[string]json.RawMessage{}}
	_, status, code, description := dispatchBotAPIMethod(nil, httptest.NewRequest("POST", "/", nil), bot, "setBusinessAccountProfilePhoto", request)
	if status != 501 || code != 501 || description != "Not Implemented: profile photo service is not configured" {
		t.Fatalf("setBusinessAccountProfilePhoto dispatch = (%d, %d, %q), want profile service error", status, code, description)
	}
}

func TestParseBotAPIAllowedUpdatesPreservesOmittedAndExplicitEmpty(t *testing.T) {
	omitted := &botAPIRequest{values: map[string]json.RawMessage{}}
	got, present, err := parseBotAPIAllowedUpdates(omitted)
	if err != nil || present || got != nil {
		t.Fatalf("omitted allowed_updates = %#v, %t, %v; want nil, false, nil", got, present, err)
	}

	empty := &botAPIRequest{values: map[string]json.RawMessage{
		"allowed_updates": json.RawMessage("[]"),
	}}
	got, present, err = parseBotAPIAllowedUpdates(empty)
	if err != nil || !present || !reflect.DeepEqual(got, []string{}) {
		t.Fatalf("explicit empty allowed_updates = %#v, %t, %v; want empty, true, nil", got, present, err)
	}
}

func TestBotAPILiveLocationMediaValidation(t *testing.T) {
	request := &botAPIRequest{values: map[string]json.RawMessage{
		"longitude": json.RawMessage("116.397"),
		"latitude":  json.RawMessage("39.909"),
	}}
	media, err := botAPILiveLocationMedia(request, false)
	if err != nil || media == nil || media.GetGeoPoint() == nil {
		t.Fatalf("live location media = %#v, %v", media, err)
	}
	if _, err = botAPILiveLocationMedia(&botAPIRequest{values: map[string]json.RawMessage{}}, false); err == nil {
		t.Fatal("live location without coordinates was accepted")
	}
	if _, err = botAPILiveLocationMedia(&botAPIRequest{values: map[string]json.RawMessage{
		"longitude": json.RawMessage("116.397"),
	}}, false); err == nil {
		t.Fatal("live location without latitude was accepted")
	}
}

func TestBotAPILocationCoordinatesRequireBothFields(t *testing.T) {
	for _, test := range []struct {
		name   string
		values map[string]json.RawMessage
		want   string
	}{
		{name: "missing longitude", values: map[string]json.RawMessage{"latitude": json.RawMessage("0")}, want: "Bad Request: longitude is invalid"},
		{name: "missing latitude", values: map[string]json.RawMessage{"longitude": json.RawMessage("0")}, want: "Bad Request: latitude is invalid"},
		{name: "null longitude", values: map[string]json.RawMessage{"longitude": json.RawMessage("null"), "latitude": json.RawMessage("0")}, want: "Bad Request: longitude is invalid"},
		{name: "null latitude", values: map[string]json.RawMessage{"longitude": json.RawMessage("0"), "latitude": json.RawMessage("null")}, want: "Bad Request: latitude is invalid"},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, _, err := botAPILocationCoordinates(&botAPIRequest{values: test.values})
			if err == nil || err.Error() != test.want {
				t.Fatalf("coordinates error = %v, want %q", err, test.want)
			}
		})
	}
	longitude, latitude, err := botAPILocationCoordinates(&botAPIRequest{values: map[string]json.RawMessage{
		"longitude": json.RawMessage("0"),
		"latitude":  json.RawMessage("0"),
	}})
	if err != nil || longitude != 0 || latitude != 0 {
		t.Fatalf("zero coordinates = (%v, %v, %v), want (0, 0, nil)", longitude, latitude, err)
	}
}

func TestBotAPIPollOpenPeriodRange(t *testing.T) {
	base := func(period string) *botAPIRequest {
		return &botAPIRequest{values: map[string]json.RawMessage{
			"question":    json.RawMessage(`"Question"`),
			"options":     json.RawMessage(`["A","B"]`),
			"open_period": json.RawMessage(period),
		}}
	}
	for _, period := range []string{"1", "4", "601"} {
		_, status, code, description := sendPoll(nil, httptest.NewRequest("POST", "/", nil), &mtproto.ImmutableUser{User: &mtproto.UserData{Id: 42}}, base(period))
		if status != 400 || code != 400 || description != "Bad Request: open_period is invalid" {
			t.Fatalf("open_period=%s result=(%d, %d, %q), want invalid period", period, status, code, description)
		}
	}
	_, status, code, description := sendPoll(nil, httptest.NewRequest("POST", "/", nil), &mtproto.ImmutableUser{User: &mtproto.UserData{Id: 42}}, base("5"))
	if status != 501 || code != 501 || description != "Not Implemented: media service is not configured" {
		t.Fatalf("open_period=5 result=(%d, %d, %q), want validation success then service error", status, code, description)
	}
}

func TestBotAPIInputRichMessage(t *testing.T) {
	html := &botAPIRequest{values: map[string]json.RawMessage{
		"rich_message": json.RawMessage(`{"html":"<b>hello</b>","is_rtl":true}`),
	}}
	got, err := botAPIInputRichMessage(html)
	if err != nil || got == nil || got.GetPredicateName() != "inputRichMessageHTML" || got.GetHtml() != "<b>hello</b>" || !got.GetRtl() {
		t.Fatalf("html rich message = %#v, %v", got, err)
	}

	blocks := &botAPIRequest{values: map[string]json.RawMessage{
		"rich_message": json.RawMessage(`{"blocks":[{"type":"paragraph","text":["hello ",{"type":"bold","text":"world"}]},{"type":"divider"}]}`),
	}}
	got, err = botAPIInputRichMessage(blocks)
	if err != nil || got == nil || got.GetPredicateName() != "inputRichMessage" || len(got.GetBlocks()) != 2 {
		t.Fatalf("block rich message = %#v, %v", got, err)
	}

	invalid := &botAPIRequest{values: map[string]json.RawMessage{
		"rich_message": json.RawMessage(`{"html":"one","markdown":"two"}`),
	}}
	if _, err = botAPIInputRichMessage(invalid); err == nil {
		t.Fatal("rich message accepted html and markdown together")
	}
}
