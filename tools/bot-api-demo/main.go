// Command bot-api-demo is a small Telegram-style developer bot used for
// end-to-end Bot API acceptance. It deliberately uses only the public HTTP
// contract, so it can also serve as a starter for a real bot project.
package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/teamgram/teamgram-server/app/botapi"
)

type apiError struct {
	Code        int
	Description string
}

func (e *apiError) Error() string {
	return fmt.Sprintf("Bot API %d: %s", e.Code, e.Description)
}

type client struct {
	baseURL string
	token   string
	http    *http.Client
}

func newClient(baseURL, token string) *client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS12}
	return &client{
		baseURL: strings.TrimRight(baseURL, "/"),
		token:   token,
		http:    &http.Client{Timeout: 70 * time.Second, Transport: transport},
	}
}

func (c *client) endpoint(method string) string {
	return c.baseURL + "/bot" + c.token + "/" + method
}

func decodeAPIResponse(data []byte, status int) (json.RawMessage, error) {
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(data, &envelope); err != nil {
		return nil, fmt.Errorf("decode response (HTTP %d): %w", status, err)
	}
	var ok bool
	_ = json.Unmarshal(envelope["ok"], &ok)
	if !ok {
		var code int
		var description string
		_ = json.Unmarshal(envelope["error_code"], &code)
		_ = json.Unmarshal(envelope["description"], &description)
		return nil, &apiError{Code: code, Description: description}
	}
	return envelope["result"], nil
}

func (c *client) call(ctx context.Context, method string, params any) (json.RawMessage, error) {
	body, err := json.Marshal(params)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint(method), bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return nil, err
	}
	return decodeAPIResponse(data, resp.StatusCode)
}

func (c *client) upload(ctx context.Context, method string, fields map[string]string, field, filename string, file io.Reader) (json.RawMessage, error) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for key, value := range fields {
		if err := writer.WriteField(key, value); err != nil {
			return nil, err
		}
	}
	part, err := writer.CreateFormFile(field, filepath.Base(filename))
	if err != nil {
		return nil, err
	}
	if _, err := io.Copy(part, file); err != nil {
		return nil, err
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint(method), &body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return nil, err
	}
	return decodeAPIResponse(data, resp.StatusCode)
}

type report struct {
	passed  int
	failed  int
	skipped int
}

func (r *report) pass(name string) {
	r.passed++
	fmt.Printf("PASS: %s\n", name)
}

func (r *report) skip(name, reason string) {
	r.skipped++
	fmt.Printf("SKIP: %s (%s)\n", name, reason)
}

func (r *report) fail(name string, err error) {
	r.failed++
	fmt.Printf("FAIL: %s (%v)\n", name, err)
}

func (r *report) step(name string, fn func() error) {
	if err := fn(); err != nil {
		var apiErr *apiError
		if errors.As(err, &apiErr) && apiErr.Code == http.StatusNotImplemented {
			r.skip(name, apiErr.Description)
			return
		}
		r.fail(name, err)
		return
	}
	r.pass(name)
}

type developerBot struct {
	client       *client
	report       *report
	miniAppURL   string
	webhookURL   string
	keepWebhook  bool
	webhookKept  bool
	chatID       string
	lastUpdateID int64
}

func (b *developerBot) configure(ctx context.Context) {
	b.report.step("getMe", func() error {
		_, err := b.client.call(ctx, "getMe", map[string]any{})
		return err
	})
	b.report.step("setMyCommands/getMyCommands", func() error {
		commands := []map[string]string{
			{"command": "start", "description": "start the demo business flow"},
			{"command": "help", "description": "show the available demo actions"},
		}
		if _, err := b.client.call(ctx, "setMyCommands", map[string]any{"commands": commands}); err != nil {
			return err
		}
		_, err := b.client.call(ctx, "getMyCommands", map[string]any{})
		return err
	})
	b.report.step("getWebhookInfo", func() error {
		_, err := b.client.call(ctx, "getWebhookInfo", map[string]any{})
		return err
	})
	if b.webhookURL != "" {
		b.report.step("setWebhook/getWebhookInfo", func() error {
			if _, err := b.client.call(ctx, "setWebhook", map[string]any{
				"url":             b.webhookURL,
				"max_connections": 4,
				"allowed_updates": []string{"message", "callback_query", "inline_query", "web_app_query", "shipping_query", "pre_checkout_query"},
				"secret_token":    "bot-api-demo-secret",
			}); err != nil {
				return err
			}
			_, err := b.client.call(ctx, "getWebhookInfo", map[string]any{})
			return err
		})
		if b.keepWebhook {
			b.webhookKept = true
		} else {
			b.report.step("deleteWebhook", func() error {
				_, err := b.client.call(ctx, "deleteWebhook", map[string]any{"drop_pending_updates": false})
				return err
			})
		}
	}
	if b.miniAppURL == "" {
		b.report.skip("setChatMenuButton/getChatMenuButton", "-mini-app-url was not supplied")
		return
	}
	menu := map[string]any{
		"type":    "web_app",
		"text":    "Open demo app",
		"web_app": map[string]string{"url": b.miniAppURL},
	}
	b.report.step("setChatMenuButton/getChatMenuButton", func() error {
		if _, err := b.client.call(ctx, "setChatMenuButton", map[string]any{"menu_button": menu}); err != nil {
			return err
		}
		_, err := b.client.call(ctx, "getChatMenuButton", map[string]any{})
		return err
	})
}

func (b *developerBot) sendBusinessMessages(ctx context.Context) {
	if b.chatID == "" {
		b.report.skip("sendMessage/reply-keyboard", "-chat-id was not supplied")
		b.report.skip("sendMessage/inline-keyboard", "-chat-id was not supplied")
		return
	}
	replyKeyboard := map[string]any{
		"keyboard": [][]map[string]any{
			{
				{"text": "Open Mini App", "web_app": map[string]string{"url": b.miniAppURL}},
				{"text": "Share contact", "request_contact": true},
			},
			{
				{"text": "Share location", "request_location": true},
			},
		},
		"resize_keyboard": true,
	}
	if b.miniAppURL == "" {
		keyboard := replyKeyboard["keyboard"].([][]map[string]any)
		delete(keyboard[0][0], "web_app")
	}
	b.report.step("sendMessage/reply-keyboard", func() error {
		_, err := b.client.call(ctx, "sendMessage", map[string]any{
			"chat_id":      b.chatID,
			"text":         "Bot API demo: Reply Keyboard is ready.",
			"reply_markup": replyKeyboard,
		})
		return err
	})

	rows := [][]map[string]any{
		{
			{"text": "Acknowledge", "callback_data": "demo:ack"},
			{"text": "Search inline", "switch_inline_query_current_chat": "demo"},
		},
	}
	if b.miniAppURL != "" {
		rows = append(rows, []map[string]any{
			{"text": "Open Mini App", "web_app": map[string]string{"url": b.miniAppURL}},
		})
	}
	b.report.step("sendMessage/inline-keyboard", func() error {
		_, err := b.client.call(ctx, "sendMessage", map[string]any{
			"chat_id":      b.chatID,
			"text":         "Inline Bot controls: press a button or use inline mode.",
			"reply_markup": map[string]any{"inline_keyboard": rows},
		})
		return err
	})
}

func (b *developerBot) optionalMedia(ctx context.Context, uploadPath, fileID string) {
	if b.chatID == "" {
		b.report.skip("sendDocument/getFile", "-chat-id was not supplied")
		return
	}
	if uploadPath != "" {
		file, err := os.Open(uploadPath)
		if err != nil {
			b.report.fail("sendDocument multipart", err)
			return
		}
		defer file.Close()
		b.report.step("sendDocument multipart", func() error {
			_, err := b.client.upload(ctx, "sendDocument", map[string]string{
				"chat_id": b.chatID,
				"caption": "Bot API multipart upload",
			}, "document", filepath.Base(uploadPath), file)
			return err
		})
		return
	}
	if fileID == "" {
		b.report.skip("getFile", "-file-id or -upload was not supplied")
		return
	}
	b.report.step("getFile", func() error {
		_, err := b.client.call(ctx, "getFile", map[string]any{"file_id": fileID})
		return err
	})
}

func (b *developerBot) handleUpdate(ctx context.Context, update botapi.Update) error {
	if update.Message != nil && update.Message.Text == "/start" && update.Message.Chat != nil {
		_, err := b.client.call(ctx, "sendMessage", map[string]any{
			"chat_id": update.Message.Chat.ID,
			"text":    "Welcome. This message came from a real polling Bot API flow.",
		})
		return err
	}
	if update.CallbackQuery != nil {
		if _, err := b.client.call(ctx, "answerCallbackQuery", map[string]any{
			"callback_query_id": update.CallbackQuery.ID,
			"text":              "Callback received by the demo bot.",
		}); err != nil {
			return err
		}
		if update.CallbackQuery.Message != nil && update.CallbackQuery.Message.Chat != nil {
			_, err := b.client.call(ctx, "editMessageText", map[string]any{
				"chat_id":    update.CallbackQuery.Message.Chat.ID,
				"message_id": update.CallbackQuery.Message.MessageID,
				"text":       "Callback acknowledged: " + update.CallbackQuery.Data,
			})
			return err
		}
	}
	if update.InlineQuery != nil {
		result := map[string]any{
			"type":  "article",
			"id":    "demo-" + update.InlineQuery.ID,
			"title": "Demo result",
			"input_message_content": map[string]any{
				"message_text": "Inline result for: " + update.InlineQuery.Query,
			},
		}
		_, err := b.client.call(ctx, "answerInlineQuery", map[string]any{
			"inline_query_id": update.InlineQuery.ID,
			"results":         []any{result},
			"cache_time":      0,
		})
		return err
	}
	if update.WebAppQuery != nil {
		_, err := b.client.call(ctx, "answerWebAppQuery", map[string]any{
			"web_app_query_id": update.WebAppQuery.ID,
			"result": map[string]any{
				"type":  "article",
				"id":    "mini-app-" + update.WebAppQuery.ID,
				"title": "Mini App response",
				"input_message_content": map[string]any{
					"message_text": "Mini App payload: " + update.WebAppQuery.Data,
				},
			},
		})
		return err
	}
	if update.ShippingQuery != nil {
		_, err := b.client.call(ctx, "answerShippingQuery", map[string]any{
			"shipping_query_id": update.ShippingQuery.ID,
			"ok":                true,
			"shipping_options": []any{map[string]any{
				"id": "demo-standard", "title": "Demo shipping",
				"prices": []any{map[string]any{"label": "Shipping", "amount": 0}},
			}},
		})
		return err
	}
	if update.PreCheckoutQuery != nil {
		_, err := b.client.call(ctx, "answerPreCheckoutQuery", map[string]any{
			"pre_checkout_query_id": update.PreCheckoutQuery.ID,
			"ok":                    true,
		})
		return err
	}
	return nil
}

func (b *developerBot) poll(ctx context.Context, timeout int, once bool) error {
	if timeout < 0 || timeout > 50 {
		return fmt.Errorf("poll-timeout must be between 0 and 50")
	}
	for {
		result, err := b.client.call(ctx, "getUpdates", map[string]any{
			"offset":          b.lastUpdateID,
			"limit":           100,
			"timeout":         timeout,
			"allowed_updates": []string{"message", "callback_query", "inline_query", "web_app_query", "shipping_query", "pre_checkout_query"},
		})
		if err != nil {
			return err
		}
		var updates []botapi.Update
		if err := json.Unmarshal(result, &updates); err != nil {
			return fmt.Errorf("decode getUpdates: %w", err)
		}
		for _, update := range updates {
			if update.UpdateID >= b.lastUpdateID {
				b.lastUpdateID = update.UpdateID + 1
			}
			if err := b.handleUpdate(ctx, update); err != nil {
				return err
			}
			b.report.pass("process update " + strconv.FormatInt(update.UpdateID, 10))
		}
		if once {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
	}
}

func main() {
	var (
		baseURL     = flag.String("base-url", getenv("BOT_API_BASE_URL", "http://127.0.0.1:8801"), "Bot API HTTP base URL")
		token       = flag.String("token", os.Getenv("BOT_TOKEN"), "BotFather token (or BOT_TOKEN)")
		chatID      = flag.String("chat-id", os.Getenv("BOT_CHAT_ID"), "chat that receives business-flow messages")
		miniAppURL  = flag.String("mini-app-url", os.Getenv("BOT_MINI_APP_URL"), "HTTPS Mini App URL")
		webhookURL  = flag.String("webhook-url", os.Getenv("BOT_WEBHOOK_URL"), "HTTPS webhook URL to configure and verify")
		keepWebhook = flag.Bool("keep-webhook", false, "keep -webhook-url configured instead of deleting it")
		initData    = flag.String("init-data", os.Getenv("BOT_WEBAPP_INIT_DATA"), "optional Web App initData to verify locally")
		uploadPath  = flag.String("upload", "", "optional document path for multipart sendDocument")
		fileID      = flag.String("file-id", "", "optional existing Bot API file_id for getFile")
		pollMode    = flag.Bool("poll", false, "keep polling and handle updates")
		once        = flag.Bool("once", false, "poll once and exit")
		pollTimeout = flag.Int("poll-timeout", 2, "getUpdates long-poll timeout in seconds")
	)
	flag.Parse()
	if strings.TrimSpace(*token) == "" {
		fmt.Fprintln(os.Stderr, "BOT_TOKEN or -token is required")
		os.Exit(2)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	bot := &developerBot{
		client:      newClient(*baseURL, *token),
		report:      &report{},
		miniAppURL:  strings.TrimSpace(*miniAppURL),
		webhookURL:  strings.TrimSpace(*webhookURL),
		keepWebhook: *keepWebhook,
		chatID:      strings.TrimSpace(*chatID),
	}
	bot.configure(ctx)
	bot.sendBusinessMessages(ctx)
	bot.optionalMedia(ctx, *uploadPath, *fileID)
	if *initData != "" {
		bot.report.step("Mini App initData HMAC", func() error {
			_, err := botapi.ValidateWebAppInitData(*initData, *token, time.Now(), 24*time.Hour)
			return err
		})
	} else {
		bot.report.skip("Mini App initData HMAC", "BOT_WEBAPP_INIT_DATA was not supplied")
	}
	if (*pollMode || *once) && bot.webhookKept {
		bot.report.skip("getUpdates", "webhook was kept; delete it before polling")
	} else if *pollMode || *once {
		bot.report.step("getUpdates", func() error { return bot.poll(ctx, *pollTimeout, *once) })
	}
	fmt.Printf("SUMMARY: PASS=%d FAIL=%d SKIP=%d\n", bot.report.passed, bot.report.failed, bot.report.skipped)
	if bot.report.failed > 0 {
		os.Exit(1)
	}
}

func getenv(name, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
}
