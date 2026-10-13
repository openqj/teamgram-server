// Signed in-memory media allocator for isolated group-call acceptance.
//
// This fixture exercises the deployed HTTP provider boundary. It is not a
// production media service and must never be used as a production fallback.
package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
)

const (
	defaultPort     = "23194"
	defaultAPIKey   = "group-call-media-fixture-key"
	defaultSignKey  = "group-call-media-fixture-signing-key-at-least-32-bytes"
	defaultRTMPHost = "127.0.0.1:1935"
	bodyLimit       = 1 << 20
)

type request struct {
	RequestID    string `json:"request_id"`
	Operation    string `json:"operation"`
	UserID       int64  `json:"user_id"`
	CallID       int64  `json:"call_id"`
	ChannelID    int64  `json:"channel_id"`
	Muted        bool   `json:"muted,omitempty"`
	VideoStopped bool   `json:"video_stopped,omitempty"`
	Params       string `json:"params,omitempty"`
	Revoke       bool   `json:"revoke"`
}

type response struct {
	RequestID   string `json:"request_id"`
	Operation   string `json:"operation"`
	Verified    bool   `json:"verified"`
	UserID      int64  `json:"user_id"`
	CallID      int64  `json:"call_id"`
	ChannelID   int64  `json:"channel_id"`
	MediaSource int32  `json:"media_source,omitempty"`
	Channels    []struct {
		Channel       int32 `json:"channel"`
		Scale         int32 `json:"scale"`
		LastTimestamp int64 `json:"last_timestamp_ms"`
	} `json:"channels,omitempty"`
	URL string `json:"url,omitempty"`
	Key string `json:"key,omitempty"`
}

type allocation struct {
	Request request
	Source  int32
}

type server struct {
	apiKey     string
	signingKey []byte
	rtmpHost   string
	mu         sync.Mutex
	next       int32
	joins      map[string]allocation
	byUserCall map[[2]int64]string
}

func newServer(apiKey, signingKey, rtmpHost string) *server {
	return &server{
		apiKey: apiKey, signingKey: []byte(signingKey), rtmpHost: rtmpHost,
		next: 1, joins: make(map[string]allocation), byUserCall: make(map[[2]int64]string),
	}
}

func signature(key []byte, body []byte) string {
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

func validSignature(key []byte, raw string, body []byte) bool {
	provided, err := hex.DecodeString(strings.TrimPrefix(strings.TrimSpace(raw), "sha256="))
	if err != nil || len(provided) != sha256.Size {
		return false
	}
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write(body)
	return hmac.Equal(provided, mac.Sum(nil))
}

func (s *server) writeResponse(w http.ResponseWriter, value response) {
	body, err := json.Marshal(value)
	if err != nil {
		http.Error(w, "encode response", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Teamgram-Group-Call-Signature", signature(s.signingKey, body))
	_, _ = w.Write(body)
}

func (s *server) fail(w http.ResponseWriter, status int, message string) {
	http.Error(w, message, status)
}

func (s *server) handle(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet && r.URL.Path == "/healthz" {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if r.Method != http.MethodPost || r.URL.Path != "/" {
		s.fail(w, http.StatusNotFound, "not found")
		return
	}
	if strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ") != s.apiKey {
		s.fail(w, http.StatusUnauthorized, "invalid provider key")
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, bodyLimit+1))
	if err != nil || len(body) > bodyLimit || !validSignature(s.signingKey, r.Header.Get("X-Teamgram-Group-Call-Request-Signature"), body) {
		s.fail(w, http.StatusUnauthorized, "invalid provider signature")
		return
	}
	var req request
	if json.Unmarshal(body, &req) != nil || req.RequestID == "" || req.UserID <= 0 || req.CallID <= 0 || req.ChannelID < 0 {
		s.fail(w, http.StatusBadRequest, "invalid provider request")
		return
	}
	result := response{RequestID: req.RequestID, Operation: req.Operation, Verified: true, UserID: req.UserID, CallID: req.CallID, ChannelID: req.ChannelID}
	s.mu.Lock()
	switch req.Operation {
	case "join_group_call":
		key := r.Header.Get("Idempotency-Key")
		if key == "" {
			s.mu.Unlock()
			s.fail(w, http.StatusBadRequest, "join requires idempotency key")
			return
		}
		if prior, ok := s.joins[key]; ok {
			if prior.Request.UserID != req.UserID || prior.Request.CallID != req.CallID || prior.Request.ChannelID != req.ChannelID {
				s.mu.Unlock()
				s.fail(w, http.StatusConflict, "idempotency key conflict")
				return
			}
			result.MediaSource = prior.Source
		} else {
			source := s.next
			s.next++
			if s.next <= 0 {
				s.next = 1
			}
			s.joins[key] = allocation{Request: req, Source: source}
			s.byUserCall[[2]int64{req.CallID, req.UserID}] = key
			result.MediaSource = source
		}
	case "leave_group_call":
		delete(s.joins, s.byUserCall[[2]int64{req.CallID, req.UserID}])
		delete(s.byUserCall, [2]int64{req.CallID, req.UserID})
	case "discard_group_call":
		for key, joined := range s.joins {
			if joined.Request.CallID == req.CallID {
				delete(s.joins, key)
				delete(s.byUserCall, [2]int64{req.CallID, joined.Request.UserID})
			}
		}
	case "get_stream_channels":
		result.Channels = []struct {
			Channel       int32 `json:"channel"`
			Scale         int32 `json:"scale"`
			LastTimestamp int64 `json:"last_timestamp_ms"`
		}{{Channel: 0, Scale: 0}}
	case "get_rtmp_url":
		result.URL = fmt.Sprintf("rtmp://%s/teamgram/%d", s.rtmpHost, req.CallID)
		result.Key = "group-call-media-fixture"
	default:
		s.mu.Unlock()
		s.fail(w, http.StatusBadRequest, "unsupported provider operation")
		return
	}
	s.mu.Unlock()
	s.writeResponse(w, result)
}

func (s *server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.handle(w, r)
}

func envOr(name, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
}

func main() {
	healthcheck := flag.Bool("healthcheck", false, "check the local provider")
	flag.Parse()
	port := envOr("GROUP_CALL_MEDIA_PROVIDER_PORT", defaultPort)
	if *healthcheck {
		resp, err := http.Get("http://127.0.0.1:" + port + "/healthz")
		if err != nil || resp.StatusCode != http.StatusNoContent {
			if resp != nil {
				_ = resp.Body.Close()
			}
			os.Exit(1)
		}
		_ = resp.Body.Close()
		return
	}
	apiKey := envOr("GROUP_CALL_MEDIA_PROVIDER_KEY", defaultAPIKey)
	signingKey := envOr("GROUP_CALL_MEDIA_PROVIDER_SIGNING_KEY", defaultSignKey)
	if len(signingKey) < 32 {
		log.Fatal("GROUP_CALL_MEDIA_PROVIDER_SIGNING_KEY must contain at least 32 bytes")
	}
	rtmpHost := envOr("GROUP_CALL_MEDIA_PROVIDER_RTMP_HOST", defaultRTMPHost)
	server := newServer(apiKey, signingKey, rtmpHost)
	log.Printf("group-call media provider listening on :%s", port)
	log.Fatal(http.ListenAndServe(":"+port, server))
}
