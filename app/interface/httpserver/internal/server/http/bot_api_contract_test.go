package http

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/botapi"
	"github.com/teamgram/teamgram-server/app/botapi/store"
	"github.com/teamgram/teamgram-server/app/interface/httpserver/internal/svc"
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
		t.Fatalf("canonical method table has %d entries, want 185 official methods plus kickChatMember compatibility alias", len(botAPIMethodNames))
	}
	for _, method := range []string{"logOut", "close", "savePreparedInlineMessage", "deleteEphemeralMessage", "sendRichMessage", "sendRichMessageDraft", "kickChatMember"} {
		if got := canonicalBotAPIMethod(strings.ToUpper(method)); got != method {
			t.Fatalf("canonicalBotAPIMethod(%q) = %q, want %q", strings.ToUpper(method), got, method)
		}
	}
}

func TestBotAPIUnimplementedCanonicalMethodsFailClosedWithMethodName(t *testing.T) {
	bot := &mtproto.ImmutableUser{User: &mtproto.UserData{Id: 42}}
	request := &botAPIRequest{values: map[string]json.RawMessage{}}
	_, status, code, description := dispatchBotAPIMethod(nil, httptest.NewRequest("POST", "/", nil), bot, "futureMethod", request)
	if status != 501 || code != 501 || description != "Not Implemented: futureMethod requires a Layer 229/service adapter" {
		t.Fatalf("futureMethod = (%d, %d, %q), want method-specific 501", status, code, description)
	}
}

func TestBotAPILifecycleMethodsFailClosedWithoutInstanceState(t *testing.T) {
	bot := &mtproto.ImmutableUser{User: &mtproto.UserData{Id: 42}}
	request := &botAPIRequest{values: map[string]json.RawMessage{}}
	for _, method := range []string{"logOut", "close"} {
		_, status, code, description := dispatchBotAPIMethod(nil, httptest.NewRequest("POST", "/", nil), bot, method, request)
		if status != 501 || code != 501 || description != "Not Implemented: bot lifecycle store is not configured" {
			t.Fatalf("%s = (%d, %d, %q), want explicit lifecycle 501", method, status, code, description)
		}
	}
}

func TestBotAPISetMyCommandsRequiresCommandsField(t *testing.T) {
	bot := &mtproto.ImmutableUser{User: &mtproto.UserData{Id: 42}}
	request := &botAPIRequest{values: map[string]json.RawMessage{}}
	_, status, code, description := dispatchBotAPIMethod(nil, httptest.NewRequest("POST", "/", nil), bot, "setMyCommands", request)
	if status != 400 || code != 400 || description != "Bad Request: commands must be a JSON array" {
		t.Fatalf("missing commands = (%d, %d, %q), want required array error", status, code, description)
	}
}

func TestBotAPIEphemeralSendRejectsDroppedMediaFlags(t *testing.T) {
	bot := &mtproto.ImmutableUser{User: &mtproto.UserData{Id: 42}}
	base := map[string]json.RawMessage{
		"chat_id":                      json.RawMessage("-10042"),
		"text":                         json.RawMessage(`"hello"`),
		"ephemeral_message_parameters": json.RawMessage(`{"receiver_user_id":7}`),
	}
	for name, value := range map[string]json.RawMessage{
		"show caption": json.RawMessage("true"),
		"link preview": json.RawMessage(`{"show_above_text":true}`),
	} {
		t.Run(name, func(t *testing.T) {
			values := make(map[string]json.RawMessage, len(base)+1)
			for key, item := range base {
				values[key] = item
			}
			if name == "show caption" {
				values["show_caption_above_media"] = value
			} else {
				values["link_preview_options"] = value
			}
			_, status, code, description := sendMessage(nil, httptest.NewRequest("POST", "/", nil), bot, &botAPIRequest{values: values})
			if status != 501 || code != 501 || !strings.Contains(description, "not represented") {
				t.Fatalf("ephemeral dropped flag = (%d, %d, %q), want explicit 501", status, code, description)
			}
		})
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

func TestBotAPIProfileOptionsFailClosedForScopedAndLocalizedVariants(t *testing.T) {
	bot := &mtproto.ImmutableUser{User: &mtproto.UserData{Id: 42, FirstName: "Bot"}}
	request := func(raw string) *botAPIRequest {
		return &botAPIRequest{values: map[string]json.RawMessage{
			"scope": json.RawMessage(raw),
		}}
	}
	for _, method := range []string{
		"setMyCommands", "deleteMyCommands", "getMyCommands", "setMyName",
		"getMyName", "setMyDescription", "getMyDescription",
		"setMyShortDescription", "getMyShortDescription",
	} {
		scopedRequest := request(`{"type":"chat","chat_id":42}`)
		if method == "setMyCommands" {
			scopedRequest.values["commands"] = json.RawMessage(`[]`)
		}
		_, status, code, description := dispatchBotAPIMethod(nil, httptest.NewRequest("POST", "/", nil), bot, method, scopedRequest)
		if status != 501 || code != 501 {
			t.Fatalf("%s scoped request = (%d, %d, %q), want scoped 501", method, status, code, description)
		}
		if method == "setMyCommands" || method == "deleteMyCommands" || method == "getMyCommands" {
			if !strings.Contains(description, "scoped bot command store") {
				t.Fatalf("%s scoped request = %q, want command store error", method, description)
			}
		} else if !strings.Contains(description, "scope variants") {
			t.Fatalf("%s scoped request = %q, want profile scope error", method, description)
		}
	}
	_, status, code, description := dispatchBotAPIMethod(nil, httptest.NewRequest("POST", "/", nil), bot, "getMyName", &botAPIRequest{values: map[string]json.RawMessage{
		"language_code": json.RawMessage(`"zh"`),
	}})
	if status != 501 || code != 501 || !strings.Contains(description, "language_code variants") {
		t.Fatalf("localized profile request = (%d, %d, %q), want language_code 501", status, code, description)
	}
	_, status, code, description = dispatchBotAPIMethod(nil, httptest.NewRequest("POST", "/", nil), bot, "getMyName", &botAPIRequest{values: map[string]json.RawMessage{
		"language_code": json.RawMessage(`"zh-hans"`),
	}})
	if status != 400 || code != 400 || description != "Bad Request: language_code is invalid" {
		t.Fatalf("malformed localized profile request = (%d, %d, %q), want language_code 400", status, code, description)
	}
	_, status, code, description = dispatchBotAPIMethod(nil, httptest.NewRequest("POST", "/", nil), bot, "getMyName", &botAPIRequest{values: map[string]json.RawMessage{
		"scope": json.RawMessage(`{"type":"bad"}`),
	}})
	if status != 501 || code != 501 || !strings.Contains(description, "scope variants") {
		t.Fatalf("unknown profile scope = (%d, %d, %q), want scope 501", status, code, description)
	}
	_, status, code, description = dispatchBotAPIMethod(nil, httptest.NewRequest("POST", "/", nil), bot, "getMyName", &botAPIRequest{values: map[string]json.RawMessage{
		"scope": json.RawMessage(`{"type":`),
	}})
	if status != 400 || code != 400 || description != "Bad Request: scope is invalid" {
		t.Fatalf("malformed profile scope = (%d, %d, %q), want scope 400", status, code, description)
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

func TestSetWebhookFailClosedForUnsupportedFieldsAndAllowsRemoval(t *testing.T) {
	for _, field := range []string{"certificate", "ip_address"} {
		_, status, code, description := setWebhook(nil, context.Background(), 42, &botAPIRequest{values: map[string]json.RawMessage{
			"url": json.RawMessage(`"https://example.test/hook"`),
			field: json.RawMessage(`"unsupported"`),
		}})
		if status != 501 || code != 501 || !strings.Contains(description, field+" is not supported") {
			t.Fatalf("setWebhook %s = (%d, %d, %q), want explicit unsupported-field 501", field, status, code, description)
		}
	}

	storeWithoutPool := store.NewWithPool(nil)
	_, status, code, description := setWebhook(&svc.ServiceContext{BotAPIStore: storeWithoutPool}, context.Background(), 42, &botAPIRequest{values: map[string]json.RawMessage{
		"drop_pending_updates": json.RawMessage("true"),
	}})
	if status != 500 || code != 500 || description == "Bad Request: url is required" {
		t.Fatalf("empty webhook URL removal = (%d, %d, %q), want removal path", status, code, description)
	}
}

func TestWebhookMethodsFailClosedWithoutStore(t *testing.T) {
	r := httptest.NewRequest("POST", "/", nil)
	request := &botAPIRequest{values: map[string]json.RawMessage{"url": json.RawMessage(`"https://example.test/hook"`)}}
	if _, status, code, _ := getUpdates(nil, r, 42, request); status != 501 || code != 501 {
		t.Fatalf("getUpdates without store = (%d, %d), want 501", status, code)
	}
	if _, status, code, _ := setWebhook(nil, context.Background(), 42, request); status != 501 || code != 501 {
		t.Fatalf("setWebhook without store = (%d, %d), want 501", status, code)
	}
	if _, status, code, _ := deleteWebhook(nil, context.Background(), 42, &botAPIRequest{values: map[string]json.RawMessage{}}); status != 501 || code != 501 {
		t.Fatalf("deleteWebhook without store = (%d, %d), want 501", status, code)
	}
	bot := &mtproto.ImmutableUser{User: &mtproto.UserData{Id: 42}}
	if _, status, code, _ := dispatchBotAPIMethod(nil, r, bot, "getWebhookInfo", &botAPIRequest{values: map[string]json.RawMessage{}}); status != 501 || code != 501 {
		t.Fatalf("getWebhookInfo without store = (%d, %d), want 501", status, code)
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

func TestSendLocationRequiresCoordinates(t *testing.T) {
	bot := &mtproto.ImmutableUser{User: &mtproto.UserData{Id: 42}}
	request := &botAPIRequest{values: map[string]json.RawMessage{}}
	_, status, code, description := sendLocation(nil, httptest.NewRequest("POST", "/", nil), bot, request)
	if status != 400 || code != 400 || description != "Bad Request: longitude is invalid" {
		t.Fatalf("missing location coordinates = (%d, %d, %q), want required longitude error", status, code, description)
	}
	request.values["longitude"] = json.RawMessage("0")
	_, status, code, description = sendLocation(nil, httptest.NewRequest("POST", "/", nil), bot, request)
	if status != 400 || code != 400 || description != "Bad Request: latitude is invalid" {
		t.Fatalf("missing latitude = (%d, %d, %q), want required latitude error", status, code, description)
	}
}

func TestBotAPITextAndCaptionLengthLimits(t *testing.T) {
	if err := validateBotAPITextLength(strings.Repeat("界", 4096), 4096, "text"); err != nil {
		t.Fatalf("text at limit rejected: %v", err)
	}
	if err := validateBotAPITextLength(strings.Repeat("界", 4097), 4096, "text"); err == nil || err.Error() != "text is too long" {
		t.Fatalf("text over limit error = %v, want text is too long", err)
	}
	if err := validateBotAPITextLength(strings.Repeat("界", 1024), 1024, "caption"); err != nil {
		t.Fatalf("caption at limit rejected: %v", err)
	}
	if err := validateBotAPITextLength(strings.Repeat("界", 1025), 1024, "caption"); err == nil || err.Error() != "caption is too long" {
		t.Fatalf("caption over limit error = %v, want caption is too long", err)
	}
}

func TestBotAPIMediaGroupInputTypes(t *testing.T) {
	if _, err := botAPIMediaGroupInput(nil, nil, 42, botAPIMediaGroupItem{Type: "animation", Media: "document_1_2"}); err == nil {
		t.Fatal("sendMediaGroup accepted unsupported animation type")
	}
	livePhoto, err := botAPIMediaGroupInput(nil, nil, 42, botAPIMediaGroupItem{
		Type:  "live_photo",
		Media: "document_3_4",
		Photo: "photo_1_2",
	})
	if err != nil || livePhoto == nil || !livePhoto.GetLivePhoto() || livePhoto.GetVideo() == nil {
		t.Fatalf("live photo media = %#v, %v; want live photo with video document", livePhoto, err)
	}
}

func TestBotAPIMediaUnsupportedFieldsFailClosed(t *testing.T) {
	bot := &mtproto.ImmutableUser{User: &mtproto.UserData{Id: 42}}
	for _, test := range []struct {
		kind  string
		field string
	}{
		{kind: "document", field: "thumbnail"},
		{kind: "document", field: "disable_content_type_detection"},
		{kind: "video", field: "thumbnail"},
		{kind: "video", field: "cover"},
		{kind: "video", field: "start_timestamp"},
		{kind: "animation", field: "thumbnail"},
		{kind: "video_note", field: "thumbnail"},
	} {
		t.Run(test.kind+"/"+test.field, func(t *testing.T) {
			request := &botAPIRequest{values: map[string]json.RawMessage{
				test.field: json.RawMessage(`"unsupported"`),
			}}
			_, status, code, description := sendBotAPIMedia(nil, httptest.NewRequest("POST", "/", nil), bot, request, test.kind, test.kind)
			if status != 501 || code != 501 || !strings.Contains(description, test.field+" is not supported") {
				t.Fatalf("%s unsupported %s = (%d, %d, %q), want explicit 501", test.kind, test.field, status, code, description)
			}
		})
	}

	_, err := botAPIMediaGroupInput(nil, nil, bot.GetUser().GetId(), botAPIMediaGroupItem{
		Type:  "video",
		Media: "document_1_2",
		Cover: json.RawMessage(`"document_3_4"`),
	})
	if !errors.Is(err, errBotAPIMediaFieldUnsupported) {
		t.Fatalf("media-group unsupported cover error = %v, want media field sentinel", err)
	}
}

func TestBotAPIEditMessageTextRichMessageUsesLayer229RichField(t *testing.T) {
	bot := &mtproto.ImmutableUser{User: &mtproto.UserData{Id: 42}}
	request := &botAPIRequest{values: map[string]json.RawMessage{
		"inline_message_id": json.RawMessage(`"inline-1"`),
		"rich_message":      json.RawMessage(`{"html":"<b>hello</b>"}`),
	}}
	_, status, code, description := editMessageText(nil, httptest.NewRequest("POST", "/", nil), bot, request)
	if status != 501 || code != 501 || description != "Not Implemented: inline message service is not configured" {
		t.Fatalf("rich edit = (%d, %d, %q), want inline service error after rich parsing", status, code, description)
	}

	request.values["text"] = json.RawMessage(`"plain"`)
	_, status, code, description = editMessageText(nil, httptest.NewRequest("POST", "/", nil), bot, request)
	if status != 400 || code != 400 || description != "Bad Request: text and rich_message are mutually exclusive" {
		t.Fatalf("text plus rich edit = (%d, %d, %q), want mutual exclusion", status, code, description)
	}
}

func TestBotAPICopyMessageUnsupportedCaptionFailsClosed(t *testing.T) {
	bot := &mtproto.ImmutableUser{User: &mtproto.UserData{Id: 42}}
	request := &botAPIRequest{values: map[string]json.RawMessage{
		"caption": json.RawMessage(`"replacement"`),
	}}
	_, status, code, description := forwardMessage(nil, httptest.NewRequest("POST", "/", nil), bot, request, true)
	if status != 501 || code != 501 || !strings.Contains(description, "caption is not supported") {
		t.Fatalf("copy caption = (%d, %d, %q), want explicit 501", status, code, description)
	}
}

func TestBotAPIProfilePhotoPublicOptionFailsClosed(t *testing.T) {
	if _, err := botAPIProfilePhotoPublicOption(&botAPIRequest{values: map[string]json.RawMessage{
		"is_public": json.RawMessage("true"),
	}}); err == nil || err.Error() != "is_public is not supported by the Layer 229 profile photo adapter" {
		t.Fatalf("public profile photo option error = %v, want explicit unsupported error", err)
	}
	if value, err := botAPIProfilePhotoPublicOption(&botAPIRequest{values: map[string]json.RawMessage{
		"is_public": json.RawMessage("false"),
	}}); err != nil || value {
		t.Fatalf("private profile photo option = (%t, %v), want (false, nil)", value, err)
	}
}

func TestAnswerShippingQueryRequiresOptionsWhenAccepted(t *testing.T) {
	base := func(values map[string]json.RawMessage) *botAPIRequest {
		return &botAPIRequest{values: values}
	}
	for _, raw := range []json.RawMessage{nil, json.RawMessage("null"), json.RawMessage("[]")} {
		values := map[string]json.RawMessage{
			"shipping_query_id": json.RawMessage("7"),
			"ok":                json.RawMessage("true"),
		}
		if raw != nil {
			values["shipping_options"] = raw
		}
		_, status, code, description := answerShippingQuery(nil, nil, 42, base(values))
		want := "Bad Request: shipping_options is invalid"
		if raw == nil || string(raw) == "null" {
			want = "Bad Request: shipping_options is required when ok is true"
		}
		if status != 400 || code != 400 || description != want {
			t.Fatalf("shipping_options=%s result=(%d, %d, %q)", raw, status, code, description)
		}
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
	for _, period := range []string{"1", "4", "2628001"} {
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

func TestBotAPIPollCurrentFieldsAndUnsupportedMedia(t *testing.T) {
	options := json.RawMessage(`[{"text":"A","text_entities":[]}]`)
	request := &botAPIRequest{values: map[string]json.RawMessage{
		"question":                  json.RawMessage(`"Question"`),
		"options":                   options,
		"is_anonymous":              json.RawMessage("false"),
		"allows_revoting":           json.RawMessage("true"),
		"shuffle_options":           json.RawMessage("true"),
		"allow_adding_options":      json.RawMessage("true"),
		"hide_results_until_closes": json.RawMessage("true"),
		"members_only":              json.RawMessage("true"),
		"country_codes":             json.RawMessage(`["cn","FT"]`),
		"explanation":               json.RawMessage(`"why"`),
		"is_closed":                 json.RawMessage("true"),
	}}
	answers, err := botAPIPollOptions(request)
	if err != nil || len(answers) != 1 || answers[0].GetText_TEXTWITHENTITIES() == nil {
		t.Fatalf("poll option object = %#v, %v; want one parsed option", answers, err)
	}
	codes, err := botAPIPollCountryCodes(request)
	if err != nil || !reflect.DeepEqual(codes, []string{"CN", "FT"}) {
		t.Fatalf("country_codes = %#v, %v; want normalized codes", codes, err)
	}
	_, status, code, description := sendPoll(nil, httptest.NewRequest("POST", "/", nil), &mtproto.ImmutableUser{User: &mtproto.UserData{Id: 42}}, request)
	if status != 501 || code != 501 || description != "Not Implemented: media service is not configured" {
		t.Fatalf("current poll fields = (%d, %d, %q), want validation success then service error", status, code, description)
	}

	request.values["description"] = json.RawMessage(`"unsupported"`)
	_, status, code, description = sendPoll(nil, httptest.NewRequest("POST", "/", nil), &mtproto.ImmutableUser{User: &mtproto.UserData{Id: 42}}, request)
	if status != 501 || code != 501 || !strings.Contains(description, "description is not supported") {
		t.Fatalf("poll description = (%d, %d, %q), want explicit unsupported 501", status, code, description)
	}

	delete(request.values, "description")
	request.values["options"] = json.RawMessage(`[{"text":"A","media":{"type":"photo","media":"photo_1_2"}}]`)
	_, status, code, description = sendPoll(nil, httptest.NewRequest("POST", "/", nil), &mtproto.ImmutableUser{User: &mtproto.UserData{Id: 42}}, request)
	if status != 501 || code != 501 || !strings.Contains(description, "poll option media is not supported") {
		t.Fatalf("poll option media = (%d, %d, %q), want explicit unsupported 501", status, code, description)
	}
}

func TestBotAPIAdministratorBotFilter(t *testing.T) {
	currentBot := &botapi.ChatMember{User: &botapi.User{ID: 42, IsBot: true}}
	otherBot := &botapi.ChatMember{User: &botapi.User{ID: 43, IsBot: true}}
	human := &botapi.ChatMember{User: &botapi.User{ID: 44}}
	if !botAPIShouldReturnAdministrator(currentBot, 42, false) {
		t.Fatal("current bot administrator was filtered")
	}
	if botAPIShouldReturnAdministrator(otherBot, 42, false) {
		t.Fatal("other bot administrator was returned by default")
	}
	if !botAPIShouldReturnAdministrator(otherBot, 42, true) || !botAPIShouldReturnAdministrator(human, 42, false) {
		t.Fatal("return_bots did not preserve expected administrators")
	}
}

func TestBotAPIUnsupportedAdministratorRightsFailClosed(t *testing.T) {
	for _, field := range []string{"can_manage_tags", "can_send_welcome_messages"} {
		parameter, err := botAPIUnsupportedAdminRight(&botAPIRequest{values: map[string]json.RawMessage{field: json.RawMessage("true")}})
		if err != nil || parameter != field {
			t.Fatalf("%s = (%q, %v), want unsupported right", field, parameter, err)
		}
	}
	if _, err := botAPIUnsupportedAdminRight(&botAPIRequest{values: map[string]json.RawMessage{
		"can_manage_tags": json.RawMessage(`"invalid"`),
	}}); err == nil {
		t.Fatal("invalid unsupported administrator right was accepted")
	}
}

func TestBotAPIInvoiceCurrentBounds(t *testing.T) {
	base := func() *botAPIRequest {
		return &botAPIRequest{values: map[string]json.RawMessage{
			"title":       json.RawMessage(`"Product"`),
			"description": json.RawMessage(`"Description"`),
			"payload":     json.RawMessage(`"payload"`),
			"currency":    json.RawMessage(`"XTR"`),
			"prices":      json.RawMessage(`[{"label":"Total","amount":100}]`),
		}}
	}
	if media, err := botAPIInvoiceMedia(base()); err != nil || media == nil {
		t.Fatalf("valid Stars invoice = %#v, %v", media, err)
	}
	tooLong := base()
	tooLong.values["title"] = json.RawMessage(`"` + strings.Repeat("x", 33) + `"`)
	if _, err := botAPIInvoiceMedia(tooLong); err == nil {
		t.Fatal("invoice accepted title longer than 32 characters")
	}
	tooLong = base()
	tooLong.values["payload"] = json.RawMessage(`"` + strings.Repeat("x", 129) + `"`)
	if _, err := botAPIInvoiceMedia(tooLong); err == nil {
		t.Fatal("invoice accepted payload longer than 128 bytes")
	}
	multiplePrices := base()
	multiplePrices.values["prices"] = json.RawMessage(`[{"label":"A","amount":1},{"label":"B","amount":2}]`)
	if _, err := botAPIInvoiceMedia(multiplePrices); err == nil {
		t.Fatal("Stars invoice accepted multiple price components")
	}
	tips := base()
	tips.values["max_tip_amount"] = json.RawMessage("10")
	if _, err := botAPIInvoiceMedia(tips); err == nil || !strings.Contains(err.Error(), "tips") {
		t.Fatalf("Stars invoice tips error = %v, want explicit unsupported tips", err)
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

func TestBotAPIRichMediaBlocks(t *testing.T) {
	request := &botAPIRequest{values: map[string]json.RawMessage{
		"rich_message": json.RawMessage(`{"blocks":[
			{"type":"photo","photo":"photo_11_12","caption":{"text":"caption","credit":"credit"},"is_spoiler":true},
			{"type":"video","video":"document_21_22","caption":"video","is_autoplay":true,"is_loop":true},
			{"type":"audio","audio":"document_31_32"}
		]}`),
	}}
	got, err := botAPIInputRichMessage(request)
	if err != nil || got == nil {
		t.Fatalf("rich media message = %#v, %v", got, err)
	}
	if len(got.GetPhotos()) != 1 || got.GetPhotos()[0].GetId() != 11 || got.GetPhotos()[0].GetAccessHash() != 12 {
		t.Fatalf("rich photo refs = %#v, want photo_11_12", got.GetPhotos())
	}
	if len(got.GetDocuments()) != 2 || got.GetDocuments()[0].GetId() != 21 || got.GetDocuments()[1].GetId() != 31 {
		t.Fatalf("rich document refs = %#v, want video/audio documents", got.GetDocuments())
	}
	if len(got.GetBlocks()) != 3 {
		t.Fatalf("rich media blocks = %d, want 3", len(got.GetBlocks()))
	}
	photo := got.GetBlocks()[0]
	if photo.GetPredicateName() != "pageBlockPhoto" || photo.GetPhotoId() != 11 || !photo.GetSpoiler() || photo.GetCaption_PAGECAPTION().GetText().GetText_STRING() != "caption" || photo.GetCaption_PAGECAPTION().GetCredit().GetText_STRING() != "credit" {
		t.Fatalf("photo block = %#v, want photo id/caption/spoiler", photo)
	}
	video := got.GetBlocks()[1]
	if video.GetPredicateName() != "pageBlockVideo" || video.GetVideoId() != 21 || !video.GetAutoplay() || !video.GetLoop() || video.GetCaption_PAGECAPTION().GetText().GetText_STRING() != "video" {
		t.Fatalf("video block = %#v, want video id/flags/caption", video)
	}
	audio := got.GetBlocks()[2]
	if audio.GetPredicateName() != "pageBlockAudio" || audio.GetAudioId() != 31 || audio.GetCaption_PAGECAPTION() == nil {
		t.Fatalf("audio block = %#v, want audio id/caption", audio)
	}
}

func TestBotAPIRichMediaBlockRejectsWrongFileKind(t *testing.T) {
	for _, block := range []string{
		`{"type":"photo","photo":"document_11_12"}`,
		`{"type":"video","video":"photo_21_22"}`,
		`{"type":"audio","audio":"photo_31_32"}`,
	} {
		request := &botAPIRequest{values: map[string]json.RawMessage{
			"rich_message": json.RawMessage(`{"blocks":[` + block + `]}`),
		}}
		if _, err := botAPIInputRichMessage(request); err == nil {
			t.Fatalf("rich media block accepted wrong file kind: %s", block)
		}
	}
}

func TestBotAPIMessageOptions(t *testing.T) {
	request := &botAPIRequest{values: map[string]json.RawMessage{
		"message_effect_id":        json.RawMessage(`"123"`),
		"show_caption_above_media": json.RawMessage(`true`),
		"link_preview_options":     json.RawMessage(`{"show_above_text":true}`),
	}}
	effect, err := botAPIMessageEffect(request)
	if err != nil || effect == nil || effect.GetValue() != 123 {
		t.Fatalf("message effect = %#v, %v; want 123", effect, err)
	}
	invert, err := botAPIMessageInvertMedia(request)
	if err != nil || !invert {
		t.Fatalf("invert media = (%v, %v), want true", invert, err)
	}
	for _, raw := range []json.RawMessage{json.RawMessage(`"bad"`), json.RawMessage(`0`)} {
		request.values["message_effect_id"] = raw
		if _, err := botAPIMessageEffect(request); err == nil {
			t.Fatalf("message_effect_id=%s was accepted", raw)
		}
	}
	request.values["show_caption_above_media"] = json.RawMessage(`"bad"`)
	if _, err := botAPIMessageInvertMedia(request); err != errBotAPIShowCaptionAboveMediaInvalid {
		t.Fatalf("show_caption_above_media error = %v, want %v", err, errBotAPIShowCaptionAboveMediaInvalid)
	}
}

func TestBotAPIUnsupportedMessageParameters(t *testing.T) {
	for _, name := range []string{"direct_messages_topic_id", "ephemeral_message_parameters"} {
		request := &botAPIRequest{values: map[string]json.RawMessage{name: json.RawMessage(`{"value":1}`)}}
		if got := botAPIUnsupportedMessageParameter(request); got != name {
			t.Fatalf("unsupported parameter = %q, want %q", got, name)
		}
	}
	for _, raw := range []json.RawMessage{nil, json.RawMessage(`null`), json.RawMessage(`""`)} {
		request := &botAPIRequest{values: map[string]json.RawMessage{"ephemeral_message_parameters": raw}}
		if got := botAPIUnsupportedMessageParameter(request); got != "" {
			t.Fatalf("empty unsupported parameter %s returned %q", raw, got)
		}
	}
}

func TestBotAPISuggestedPost(t *testing.T) {
	now := time.Now().Unix()
	if post, err := botAPISuggestedPost(&botAPIRequest{values: map[string]json.RawMessage{
		"suggested_post_parameters": json.RawMessage(`{}`),
	}}); err != nil || post == nil || post.GetPrice() != nil || post.GetScheduleDate() != nil {
		t.Fatalf("empty suggested post = %#v, %v", post, err)
	}
	request := &botAPIRequest{values: map[string]json.RawMessage{
		"suggested_post_parameters": json.RawMessage(fmt.Sprintf(`{"price":{"currency":"TON","amount":10000000},"send_date":%d}`, now+600)),
	}}
	post, err := botAPISuggestedPost(request)
	if err != nil || post == nil || post.GetPrice().GetAmount() != 10000000 || post.GetPrice().GetPredicateName() != mtproto.Predicate_starsTonAmount || post.GetScheduleDate().GetValue() != int32(now+600) {
		t.Fatalf("suggested post = %#v, %v", post, err)
	}

	for _, test := range []struct {
		name string
		raw  string
	}{
		{name: "invalid currency", raw: fmt.Sprintf(`{"price":{"currency":"USD","amount":5},"send_date":%d}`, now+600)},
		{name: "stars below minimum", raw: fmt.Sprintf(`{"price":{"currency":"XTR","amount":4},"send_date":%d}`, now+600)},
		{name: "ton above maximum", raw: fmt.Sprintf(`{"price":{"currency":"TON","amount":10000000000001},"send_date":%d}`, now+600)},
		{name: "send date too soon", raw: fmt.Sprintf(`{"send_date":%d}`, now+299)},
		{name: "send date too late", raw: fmt.Sprintf(`{"send_date":%d}`, now+30*24*60*60+1)},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := botAPISuggestedPost(&botAPIRequest{values: map[string]json.RawMessage{"suggested_post_parameters": json.RawMessage(test.raw)}}); err == nil {
				t.Fatal("invalid suggested post was accepted")
			}
		})
	}
}

func TestBotAPIDraftStopControlsValidateBeforeServiceLookup(t *testing.T) {
	bot := &mtproto.ImmutableUser{User: &mtproto.UserData{Id: 42}}
	for _, test := range []struct {
		name  string
		key   string
		value string
	}{
		{name: "text can_stop", key: "can_stop", value: "true"},
		{name: "text keep_on_stop", key: "keep_on_stop", value: "true"},
		{name: "rich can_stop", key: "can_stop", value: "true"},
		{name: "rich keep_on_stop", key: "keep_on_stop", value: "true"},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := &botAPIRequest{values: map[string]json.RawMessage{
				test.key: json.RawMessage(test.value),
			}}
			if strings.HasPrefix(test.name, "rich") {
				_, status, code, description := sendRichMessageDraft(nil, nil, bot, request)
				if status != 501 || code != 501 || description != "Not Implemented: rich draft stop controls are not supported" {
					t.Fatalf("rich draft stop control = (%d, %d, %q), want method-specific 501", status, code, description)
				}
				return
			}
			_, status, code, description := sendMessageDraft(nil, nil, bot, request)
			if status != 501 || code != 501 || description != "Not Implemented: rich draft stop controls are not supported" {
				t.Fatalf("draft stop control = (%d, %d, %q), want method-specific 501", status, code, description)
			}
		})
	}
	for _, test := range []struct {
		name string
		call func(*botAPIRequest) (any, int, int, string)
	}{
		{name: "text malformed", call: func(request *botAPIRequest) (any, int, int, string) {
			return sendMessageDraft(nil, nil, bot, request)
		}},
		{name: "rich malformed", call: func(request *botAPIRequest) (any, int, int, string) {
			return sendRichMessageDraft(nil, nil, bot, request)
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := &botAPIRequest{values: map[string]json.RawMessage{
				"can_stop": json.RawMessage(`"invalid"`),
			}}
			_, status, code, description := test.call(request)
			if status != 400 || code != 400 || description != "Bad Request: can_stop is invalid" {
				t.Fatalf("malformed draft stop control = (%d, %d, %q), want 400", status, code, description)
			}
		})
	}
}
