// Signed PostgreSQL-backed CDN provider for isolated Layer 229 probes.
//
// Objects are provisioned in PostgreSQL and are served through the Telegram
// CDN contract: AES-CTR encrypted chunks, SHA-256 file hashes, redirect
// metadata, and a signed reupload token.  The provider never derives trust
// from a client supplied auth-key id.
package main

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	defaultPort = "23191"
	bodyLimit   = 1 << 20
	partSize    = 128 * 1024
)

type requestEnvelope struct {
	RequestID string          `json:"request_id"`
	Operation string          `json:"operation"`
	UserID    int64           `json:"user_id"`
	Payload   json.RawMessage `json:"payload"`
}

type responseEnvelope struct {
	RequestID string          `json:"request_id"`
	Operation string          `json:"operation"`
	Verified  bool            `json:"verified"`
	UserID    int64           `json:"user_id"`
	Error     string          `json:"error,omitempty"`
	Result    json.RawMessage `json:"result,omitempty"`
}

type cdnObject struct {
	Token     []byte
	Source    string
	Data      []byte
	Key       []byte
	IV        []byte
	DCID      int32
	ExpiresAt *time.Time
}

func validatePublicKeys(keys []json.RawMessage) error {
	if len(keys) == 0 || len(keys) > 1000 {
		return errors.New("CDN config must contain between 1 and 1000 public keys")
	}
	for _, raw := range keys {
		var key struct {
			PredicateName string `json:"predicate_name"`
			DCID          int32  `json:"dc_id"`
			PublicKey     string `json:"public_key"`
		}
		if err := json.Unmarshal(raw, &key); err != nil || key.PredicateName != "cdnPublicKey" || key.DCID <= 0 || strings.TrimSpace(key.PublicKey) == "" {
			return errors.New("invalid CDN public key")
		}
	}
	return nil
}

type objectStore interface {
	BySource(context.Context, string) (*cdnObject, error)
	ByToken(context.Context, []byte) (*cdnObject, error)
	Seed(context.Context, *cdnObject) error
	Config(context.Context) ([]byte, error)
}

type postgresStore struct{ db *pgxpool.Pool }

func (s *postgresStore) Ping(ctx context.Context) error {
	if s == nil || s.db == nil {
		return errors.New("CDN provider database is unavailable")
	}
	if err := s.db.Ping(ctx); err != nil {
		return err
	}
	return validateProviderSchema(ctx, s.db)
}

func (s *postgresStore) BySource(ctx context.Context, source string) (*cdnObject, error) {
	if strings.TrimSpace(source) == "" {
		return nil, errors.New("empty source key")
	}
	var object cdnObject
	var expires *time.Time
	err := s.db.QueryRow(ctx, `
		SELECT file_token, source_key, plaintext, encryption_key, encryption_iv, dc_id, expires_at
		FROM teamgram_cdn_object WHERE source_key=$1`, source).
		Scan(&object.Token, &object.Source, &object.Data, &object.Key, &object.IV, &object.DCID, &expires)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, errors.New("CDN object not provisioned")
	}
	if err != nil {
		return nil, err
	}
	object.ExpiresAt = expires
	if expires != nil && !expires.After(time.Now()) {
		return nil, errors.New("CDN object expired")
	}
	return validateObject(&object)
}

func (s *postgresStore) Config(ctx context.Context) ([]byte, error) {
	var value json.RawMessage
	err := s.db.QueryRow(ctx, `SELECT public_keys FROM teamgram_cdn_config WHERE id=1`).Scan(&value)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, errors.New("CDN config is not provisioned")
	}
	if err != nil {
		return nil, err
	}
	var keys []json.RawMessage
	if err = json.Unmarshal(value, &keys); err != nil {
		return nil, errors.New("invalid CDN config")
	}
	if err = validatePublicKeys(keys); err != nil {
		return nil, errors.New("invalid CDN config")
	}
	return json.Marshal(map[string]any{"predicate_name": "cdnConfig", "public_keys": keys})
}

func (s *postgresStore) ByToken(ctx context.Context, token []byte) (*cdnObject, error) {
	var object cdnObject
	var expires *time.Time
	err := s.db.QueryRow(ctx, `
		SELECT file_token, source_key, plaintext, encryption_key, encryption_iv, dc_id, expires_at
		FROM teamgram_cdn_object WHERE file_token=$1`, token).
		Scan(&object.Token, &object.Source, &object.Data, &object.Key, &object.IV, &object.DCID, &expires)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, errors.New("CDN file token is unknown")
	}
	if err != nil {
		return nil, err
	}
	object.ExpiresAt = expires
	if expires != nil && !expires.After(time.Now()) {
		return nil, errors.New("CDN object expired")
	}
	return validateObject(&object)
}

func (s *postgresStore) Seed(ctx context.Context, object *cdnObject) error {
	if _, err := validateObject(object); err != nil {
		return err
	}
	_, err := s.db.Exec(ctx, `
		INSERT INTO teamgram_cdn_object (file_token, source_key, plaintext, encryption_key, encryption_iv, dc_id, expires_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7)
		ON CONFLICT (source_key) DO UPDATE SET file_token=EXCLUDED.file_token, plaintext=EXCLUDED.plaintext,
		 encryption_key=EXCLUDED.encryption_key, encryption_iv=EXCLUDED.encryption_iv, dc_id=EXCLUDED.dc_id, expires_at=EXCLUDED.expires_at`,
		object.Token, object.Source, object.Data, object.Key, object.IV, object.DCID, object.ExpiresAt)
	return err
}

type memoryStore struct{ objects map[string]*cdnObject }

func newMemoryStore() *memoryStore { return &memoryStore{objects: map[string]*cdnObject{}} }

func (s *memoryStore) BySource(_ context.Context, source string) (*cdnObject, error) {
	object := s.objects[source]
	if object == nil {
		return nil, errors.New("CDN object not provisioned")
	}
	return validateObject(cloneObject(object))
}

func (s *memoryStore) ByToken(_ context.Context, token []byte) (*cdnObject, error) {
	object := s.objects[hex.EncodeToString(token)]
	if object == nil {
		return nil, errors.New("CDN file token is unknown")
	}
	return validateObject(cloneObject(object))
}

func (s *memoryStore) Seed(_ context.Context, object *cdnObject) error {
	if _, err := validateObject(object); err != nil {
		return err
	}
	copy := cloneObject(object)
	s.objects[copy.Source] = copy
	s.objects[hex.EncodeToString(copy.Token)] = copy
	return nil
}

func (s *memoryStore) Config(_ context.Context) ([]byte, error) {
	object := s.objects["__cdn_config__"]
	if object == nil {
		return nil, errors.New("CDN config is not provisioned")
	}
	var config struct {
		PublicKeys []json.RawMessage `json:"public_keys"`
	}
	if err := json.Unmarshal(object.Data, &config); err != nil || validatePublicKeys(config.PublicKeys) != nil {
		return nil, errors.New("invalid CDN config")
	}
	return append([]byte(nil), object.Data...), nil
}

func cloneObject(object *cdnObject) *cdnObject {
	if object == nil {
		return nil
	}
	copy := *object
	copy.Token = append([]byte(nil), object.Token...)
	copy.Data = append([]byte(nil), object.Data...)
	copy.Key = append([]byte(nil), object.Key...)
	copy.IV = append([]byte(nil), object.IV...)
	return &copy
}

func deriveObject(source string) *cdnObject {
	seed := sha256.Sum256([]byte("teamgram-cdn:" + source))
	keyDigest := sha256.Sum256([]byte("teamgram-cdn-key:" + source))
	ivDigest := sha256.Sum256([]byte("teamgram-cdn-iv:" + source))
	data := []byte("Teamgram CDN fixture object\nsource=" + source + "\n")
	return &cdnObject{Token: append([]byte(nil), seed[:]...), Source: source, Data: data, Key: append([]byte(nil), keyDigest[:]...), IV: append([]byte(nil), ivDigest[:16]...), DCID: 2}
}

func validateObject(object *cdnObject) (*cdnObject, error) {
	if object == nil || len(object.Token) < 16 || len(object.Token) > 128 || strings.TrimSpace(object.Source) == "" || len(object.Data) == 0 || len(object.Key) != 32 || len(object.IV) != 16 || object.DCID <= 0 {
		return nil, errors.New("invalid CDN object")
	}
	return object, nil
}

func encryptedBytes(object *cdnObject) ([]byte, error) {
	block, err := aes.NewCipher(object.Key)
	if err != nil {
		return nil, err
	}
	encrypted := append([]byte(nil), object.Data...)
	cipher.NewCTR(block, object.IV).XORKeyStream(encrypted, encrypted)
	return encrypted, nil
}

func fileHashes(encrypted []byte, offset int64) []map[string]any {
	if offset < 0 || offset >= int64(len(encrypted)) {
		return []map[string]any{}
	}
	hashes := make([]map[string]any, 0, 64)
	for offset < int64(len(encrypted)) && len(hashes) < 64 {
		end := offset + partSize
		if end > int64(len(encrypted)) {
			end = int64(len(encrypted))
		}
		part := encrypted[offset:end]
		sum := sha256.Sum256(part)
		hashes = append(hashes, map[string]any{"predicate_name": "fileHash", "offset_INT64": offset, "limit": len(part), "hash": sum[:]})
		offset = end
	}
	return hashes
}

type server struct {
	key     []byte
	authKey string
	store   objectStore
	timeout time.Duration
}

func (s *server) sign(body []byte) string {
	mac := hmac.New(sha256.New, s.key)
	_, _ = mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

func verifySignature(key []byte, signature string, body []byte) bool {
	signature = strings.TrimPrefix(strings.TrimSpace(signature), "sha256=")
	provided, err := hex.DecodeString(signature)
	if err != nil || len(provided) != sha256.Size || len(key) == 0 {
		return false
	}
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write(body)
	return hmac.Equal(provided, mac.Sum(nil))
}

func validBearerAuthorization(value, expected string) bool {
	if strings.TrimSpace(expected) == "" || !strings.HasPrefix(value, "Bearer ") {
		return false
	}
	provided := strings.TrimPrefix(value, "Bearer ")
	if provided == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(provided), []byte(expected)) == 1
}

func (s *server) handle(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPut && r.URL.Path == "/seed" {
		s.handleSeed(w, r)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.authKey != "" && !validBearerAuthorization(r.Header.Get("Authorization"), s.authKey) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, bodyLimit+1))
	if err != nil || len(body) == 0 || len(body) > bodyLimit || !verifySignature(s.key, r.Header.Get("X-Teamgram-CDN-Provider-Request-Signature"), body) {
		http.Error(w, "invalid signed request", http.StatusUnauthorized)
		return
	}
	var request requestEnvelope
	if err = json.Unmarshal(body, &request); err != nil || request.RequestID == "" || request.UserID < 0 {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), s.timeout)
	defer cancel()
	result, providerErr := s.dispatch(ctx, request)
	response := responseEnvelope{RequestID: request.RequestID, Operation: request.Operation, Verified: providerErr == nil, UserID: request.UserID}
	if providerErr != nil {
		response.Error = providerErr.Error()
	} else {
		response.Result = result
	}
	encoded, err := json.Marshal(response)
	if err != nil {
		http.Error(w, "encode error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Teamgram-CDN-Provider-Signature", s.sign(encoded))
	_, _ = w.Write(encoded)
}

func (s *server) healthz(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if s == nil || s.store == nil {
		http.Error(w, "provider database unavailable", http.StatusServiceUnavailable)
		return
	}
	if checker, ok := s.store.(interface{ Ping(context.Context) error }); ok {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if err := checker.Ping(ctx); err != nil {
			http.Error(w, "provider database unavailable", http.StatusServiceUnavailable)
			return
		}
	}
	w.WriteHeader(http.StatusOK)
}

func (s *server) handleSeed(w http.ResponseWriter, r *http.Request) {
	if s.authKey == "" || !validBearerAuthorization(r.Header.Get("Authorization"), s.authKey) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, bodyLimit+1))
	if err != nil || len(body) > bodyLimit {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}
	var input struct {
		Source    string `json:"source_key"`
		Token     string `json:"file_token"`
		Plaintext string `json:"plaintext"`
		Key       string `json:"encryption_key"`
		IV        string `json:"encryption_iv"`
		DCID      int32  `json:"dc_id"`
	}
	if err = json.Unmarshal(body, &input); err != nil || strings.TrimSpace(input.Source) == "" {
		http.Error(w, "invalid seed", http.StatusBadRequest)
		return
	}
	object := deriveObject(input.Source)
	decode := func(value string) ([]byte, error) {
		if value == "" {
			return nil, nil
		}
		return base64.RawStdEncoding.DecodeString(value)
	}
	if input.Token != "" {
		if object.Token, err = decode(input.Token); err != nil {
			http.Error(w, "invalid token", http.StatusBadRequest)
			return
		}
	}
	if input.Plaintext != "" {
		if object.Data, err = decode(input.Plaintext); err != nil {
			http.Error(w, "invalid plaintext", http.StatusBadRequest)
			return
		}
	}
	if input.Key != "" {
		if object.Key, err = decode(input.Key); err != nil {
			http.Error(w, "invalid key", http.StatusBadRequest)
			return
		}
	}
	if input.IV != "" {
		if object.IV, err = decode(input.IV); err != nil {
			http.Error(w, "invalid iv", http.StatusBadRequest)
			return
		}
	}
	if input.DCID > 0 {
		object.DCID = input.DCID
	}
	if err = s.store.Seed(r.Context(), object); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) dispatch(ctx context.Context, request requestEnvelope) (json.RawMessage, error) {
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(request.Payload, &payload); err != nil {
		return nil, errors.New("payload must be an object")
	}
	getObject := func() (*cdnObject, error) {
		if request.Operation == "upload.getFile" {
			return s.store.BySource(ctx, sourceKey(payload["location"]))
		}
		var token []byte
		if err := json.Unmarshal(payload["file_token"], &token); err != nil || len(token) == 0 {
			return nil, errors.New("invalid file token")
		}
		return s.store.ByToken(ctx, token)
	}
	switch request.Operation {
	case "help.getCdnConfig":
		return s.store.Config(ctx)
	case "upload.getFile":
		object, err := getObject()
		if err != nil {
			return nil, err
		}
		encrypted, err := encryptedBytes(object)
		if err != nil {
			return nil, err
		}
		return json.Marshal(map[string]any{
			"predicate_name": "upload_fileCdnRedirect",
			"dc_id":          object.DCID,
			"file_token":     object.Token,
			"encryption_key": object.Key,
			"encryption_iv":  object.IV,
			"file_hashes":    fileHashes(encrypted, 0),
		})
	case "upload.getCdnFile":
		object, err := getObject()
		if err != nil {
			return nil, err
		}
		encrypted, err := encryptedBytes(object)
		if err != nil {
			return nil, err
		}
		offset, limit := rawInt64(payload, "offset"), rawInt64(payload, "limit")
		if offset < 0 || limit <= 0 || limit > 1<<20 || offset >= int64(len(encrypted)) {
			return nil, errors.New("invalid CDN range")
		}
		end := offset + limit
		if end > int64(len(encrypted)) {
			end = int64(len(encrypted))
		}
		return json.Marshal(map[string]any{"predicate_name": "upload_cdnFile", "bytes": encrypted[offset:end]})
	case "upload.getCdnFileHashes":
		object, err := getObject()
		if err != nil {
			return nil, err
		}
		encrypted, err := encryptedBytes(object)
		if err != nil {
			return nil, err
		}
		return json.Marshal(map[string]any{"datas": fileHashes(encrypted, rawInt64(payload, "offset"))})
	case "upload.reuploadCdnFile":
		object, err := getObject()
		if err != nil {
			return nil, err
		}
		var requestToken []byte
		if err = json.Unmarshal(payload["request_token"], &requestToken); err != nil || !validRequestToken(s.key, object.Token, requestToken) {
			return nil, errors.New("invalid CDN request token")
		}
		encrypted, err := encryptedBytes(object)
		if err != nil {
			return nil, err
		}
		return json.Marshal(map[string]any{"datas": fileHashes(encrypted, 0)})
	default:
		return nil, fmt.Errorf("operation %q is not provisioned", request.Operation)
	}
}

func sourceKey(raw json.RawMessage) string {
	digest := sha256.Sum256(bytes.TrimSpace(raw))
	return hex.EncodeToString(digest[:])
}

func rawInt64(fields map[string]json.RawMessage, name string) int64 {
	var value int64
	_ = json.Unmarshal(fields[name], &value)
	return value
}

func validRequestToken(key, token, requestToken []byte) bool {
	if len(requestToken) == 0 || len(key) == 0 {
		return false
	}
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write(token)
	return hmac.Equal(mac.Sum(nil), requestToken)
}

func requestToken(key, token []byte) []byte {
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write(token)
	return mac.Sum(nil)
}

func openPostgres(ctx context.Context, dsn string) (*pgxpool.Pool, error) {
	if strings.TrimSpace(dsn) == "" {
		return nil, errors.New("TEAMGRAM_POSTGRES_DSN is required")
	}
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, err
	}
	if err = pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	var version int
	if err = pool.QueryRow(ctx, `SELECT current_setting('server_version_num')::integer`).Scan(&version); err != nil || version/10000 != 18 {
		pool.Close()
		if err == nil {
			err = fmt.Errorf("PostgreSQL 18 is required (server_version_num=%d)", version)
		}
		return nil, err
	}
	if err = validateProviderSchema(ctx, pool); err != nil {
		pool.Close()
		return nil, err
	}
	return pool, nil
}

func validateProviderSchema(ctx context.Context, pool *pgxpool.Pool) error {
	if err := requireMigrationCatalog(ctx, pool, "078_apifull_payment_provider_control_plane", "079_provider_control_plane"); err != nil {
		return fmt.Errorf("provider migration catalog is unavailable: %w", err)
	}
	for _, query := range []string{
		`SELECT file_token, source_key, plaintext, encryption_key, encryption_iv, dc_id, expires_at FROM teamgram_cdn_object LIMIT 0`,
		`SELECT id, public_keys FROM teamgram_cdn_config LIMIT 0`,
	} {
		if _, err := pool.Exec(ctx, query); err != nil {
			return fmt.Errorf("CDN provider schema is unavailable: %w", err)
		}
	}
	return nil
}

func requireMigrationCatalog(ctx context.Context, pool *pgxpool.Pool, versions ...string) error {
	if pool == nil || len(versions) == 0 {
		return errors.New("provider migration catalog is unavailable")
	}
	for _, version := range versions {
		var checksum string
		var appliedAt time.Time
		err := pool.QueryRow(ctx, `
			SELECT checksum, applied_at
			FROM public.schema_migrations
			WHERE version = $1`, version).Scan(&checksum, &appliedAt)
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("migration %q is not applied", version)
		}
		if err != nil {
			return err
		}
		if !validMigrationChecksum(checksum) || appliedAt.IsZero() {
			return fmt.Errorf("migration %q has an invalid catalog record", version)
		}
	}
	return nil
}

func validMigrationChecksum(value string) bool {
	if len(value) != 64 {
		return false
	}
	for _, ch := range value {
		if (ch < '0' || ch > '9') && (ch < 'a' || ch > 'f') {
			return false
		}
	}
	return true
}

func main() {
	key := strings.TrimSpace(os.Getenv("TEAMGRAM_CDN_PROVIDER_SIGNING_KEY"))
	if len(key) < 32 {
		log.Fatal("TEAMGRAM_CDN_PROVIDER_SIGNING_KEY must contain at least 32 bytes")
	}
	authKey := strings.TrimSpace(os.Getenv("TEAMGRAM_CDN_PROVIDER_KEY"))
	if authKey == "" {
		log.Fatal("TEAMGRAM_CDN_PROVIDER_KEY must be set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pool, err := openPostgres(ctx, os.Getenv("TEAMGRAM_POSTGRES_DSN"))
	if err != nil {
		log.Fatal(err)
	}
	defer pool.Close()
	timeout := 5 * time.Second
	if value := strings.TrimSpace(os.Getenv("TEAMGRAM_CDN_PROVIDER_TIMEOUT_SECONDS")); value != "" {
		seconds, parseErr := strconv.Atoi(value)
		if parseErr != nil || seconds <= 0 || seconds > 300 {
			log.Fatal("TEAMGRAM_CDN_PROVIDER_TIMEOUT_SECONDS must be from 1 to 300")
		}
		timeout = time.Duration(seconds) * time.Second
	}
	s := &server{key: []byte(key), authKey: authKey, store: &postgresStore{db: pool}, timeout: timeout}
	port := strings.TrimSpace(os.Getenv("PORT"))
	if port == "" {
		port = defaultPort
	}
	portNumber, parseErr := strconv.Atoi(port)
	if parseErr != nil || portNumber < 1 || portNumber > 65535 {
		log.Fatal("PORT must be an integer from 1 to 65535")
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", s.healthz)
	mux.HandleFunc("/", s.handle)
	server := &http.Server{Addr: ":" + port, Handler: mux}
	if len(os.Args) == 2 && os.Args[1] == "--healthcheck" {
		healthCtx, healthCancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer healthCancel()
		if err := (&postgresStore{db: pool}).Ping(healthCtx); err != nil {
			log.Fatalf("provider readiness check failed: %v", err)
		}
		return
	}
	if len(os.Args) > 1 {
		log.Fatal("unsupported argument")
	}
	log.Printf("CDN provider fixture listening on :%s", port)
	log.Fatal(server.ListenAndServe())
}
