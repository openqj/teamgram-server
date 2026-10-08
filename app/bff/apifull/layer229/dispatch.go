package layer229

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strconv"
	"sync"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/core"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/domain"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/persist"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

var (
	openOnce      sync.Once
	openErr       error
	configuredDSN string
)

// UseMySQL is retained for isolated legacy tests only. Production callers must
// use UsePostgres so the session process shares the PostgreSQL 18 domain store.
func UseMySQL(dsn string) error {
	if dsn == "" || domain.Ready() {
		return nil
	}
	configuredDSN = dsn
	return domain.Open(dsn)
}

// UsePostgres opens the Layer 229 domain store in the session process. The
// session process does not share the BFF DAO, so it owns its own PG handle.
func UsePostgres(dsn string) error {
	if dsn == "" {
		return errors.New("layer229: PostgresDSN is required")
	}
	if domain.Ready() {
		return nil
	}
	configuredDSN = dsn
	return domain.OpenPostgres(dsn)
}

func ensure() error {
	openOnce.Do(func() {
		if domain.Ready() {
			return
		}
		if configuredDSN == "" {
			openErr = errors.New("layer229: PostgresDSN is required")
			return
		}
		openErr = domain.OpenPostgres(configuredDSN)
	})
	return openErr
}

// Dispatch handles the Layer 229 methods that v0.228.0 did not generate.
// ok is false when object is not one of those methods.
func Dispatch(ctx context.Context, md *metadata.RpcMetadata, object mtproto.TLObject) (mtproto.TLObject, bool, error) {
	switch in := object.(type) {
	case *mtproto.TLAuthInitFirebasePnvLogin:
		reply, err := initFirebase(md, in)
		return reply, true, err
	case *mtproto.TLAuthFinishFirebasePnvLogin:
		reply, err := finishFirebase(ctx, md, in)
		return reply, true, err
	case *mtproto.TLAuthFirebasePnvSignUp:
		reply, err := signUpFirebase(ctx, md, in)
		return reply, true, err
	case *mtproto.TLEphemeralGetWelcomeMessages:
		reply, err := getWelcome(md, in)
		return reply, true, err
	case *mtproto.TLEphemeralEditMessage:
		reply, err := editWelcome(md, in)
		return reply, true, err
	case *mtproto.TLEphemeralDeleteWelcomeMessage:
		reply, err := deleteWelcome(md, in)
		return reply, true, err
	case *mtproto.TLEphemeralDeleteAllWelcomeMessages:
		reply, err := deleteAllWelcome(md, in)
		return reply, true, err
	case *mtproto.TLEphemeralSendMessageBA8D5F35:
		reply, err := sendEphemeral229(md, in)
		return reply, true, err
	case *mtproto.TLEphemeralDeleteMessage92F6E797:
		reply, err := deleteEphemeral229(md, in)
		return reply, true, err
	default:
		return nil, false, nil
	}
}

func authKey(md *metadata.RpcMetadata) int64 {
	if md == nil {
		return 0
	}
	if md.PermAuthKeyId != 0 {
		return md.PermAuthKeyId
	}
	return md.UserId
}

func initFirebase(md *metadata.RpcMetadata, in *mtproto.TLAuthInitFirebasePnvLogin) (mtproto.TLObject, error) {
	if err := ensure(); err != nil {
		return nil, err
	}
	if in == nil || in.ApiId == 0 || in.ApiHash == "" {
		return nil, mtproto.ErrApiIdInvalid
	}
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return nil, err
	}
	nonce := hex.EncodeToString(buf[:])
	key := pnvKey(authKey(md))
	raw, _ := json.Marshal(pnvState{Nonce: nonce, APIID: in.ApiId, APIHash: in.ApiHash})
	if err := persist.Default.Set(key, string(raw)); err != nil {
		return nil, err
	}
	return &mtproto.TLAuthFirebasePnvIntent{Nonce: nonce, DigitalCredentialPayload: nonce}, nil
}

func finishFirebase(ctx context.Context, md *metadata.RpcMetadata, in *mtproto.TLAuthFinishFirebasePnvLogin) (mtproto.TLObject, error) {
	if err := ensure(); err != nil {
		return nil, err
	}
	if in == nil || in.GoogleToken == "" {
		return nil, mtproto.ErrTokenInvalid
	}
	sub, phone, err := VerifyFirebaseToken(ctx, in.GoogleToken)
	if err != nil {
		return nil, mtproto.ErrTokenInvalid
	}
	st, err := loadPNV(authKey(md))
	if err != nil {
		return nil, err
	}
	if st.Nonce == "" {
		return nil, mtproto.ErrTokenInvalid
	}
	dir := currentUsers()
	if dir == nil {
		return nil, mtproto.ErrInternalServerError
	}
	if st.UserID == 0 {
		user, err := dir.CreateNewUser(ctx, authKey(md), phone, countryCode(phone), st.FirstName, st.LastName)
		if err != nil {
			return nil, err
		}
		st.UserID = user.GetId()
		if user.GetAccessHash() != nil {
			st.AccessHash = user.GetAccessHash().GetValue()
		}
	}
	st.Token = in.GoogleToken
	st.Subject = sub
	st.Phone = phone
	if err = savePNV(authKey(md), st); err != nil {
		return nil, err
	}
	return authorization(st), nil
}

func signUpFirebase(ctx context.Context, md *metadata.RpcMetadata, in *mtproto.TLAuthFirebasePnvSignUp) (mtproto.TLObject, error) {
	if err := ensure(); err != nil {
		return nil, err
	}
	if in == nil || in.FirstName == "" {
		return nil, mtproto.ErrFirstnameInvalid
	}
	st, err := loadPNV(authKey(md))
	if err != nil {
		return nil, err
	}
	if st.Token == "" || st.UserID == 0 {
		return nil, mtproto.ErrTokenInvalid
	}
	dir := currentUsers()
	if dir == nil {
		return nil, mtproto.ErrInternalServerError
	}
	if err = dir.UpdateName(ctx, st.UserID, in.FirstName, in.LastName); err != nil {
		return nil, err
	}
	st.FirstName = in.FirstName
	st.LastName = in.LastName
	st.NoJoinedNotifications = in.NoJoinedNotifications
	if err = savePNV(authKey(md), st); err != nil {
		return nil, err
	}
	return authorization(st), nil
}

type pnvState struct {
	Nonce                 string `json:"nonce"`
	APIID                 int32  `json:"api_id"`
	APIHash               string `json:"api_hash"`
	Token                 string `json:"token"`
	Subject               string `json:"subject"`
	Phone                 string `json:"phone"`
	UserID                int64  `json:"user_id"`
	AccessHash            int64  `json:"access_hash"`
	FirstName             string `json:"first_name"`
	LastName              string `json:"last_name"`
	NoJoinedNotifications bool   `json:"no_joined_notifications"`
}

func pnvKey(id int64) string { return "pnv:" + strconv.FormatInt(id, 10) }

func countryCode(phone string) string {
	digits := phone
	if len(digits) > 0 && digits[0] == '+' {
		digits = digits[1:]
	}
	n := 0
	for n < len(digits) && digits[n] >= '0' && digits[n] <= '9' && n < 3 {
		n++
	}
	if n == 0 {
		return ""
	}
	return digits[:n]
}

func loadPNV(id int64) (pnvState, error) {
	var st pnvState
	raw, err := persist.Default.Get(pnvKey(id))
	if err != nil || raw == "" {
		return st, err
	}
	err = json.Unmarshal([]byte(raw), &st)
	return st, err
}

func savePNV(id int64, st pnvState) error {
	raw, err := json.Marshal(st)
	if err != nil {
		return err
	}
	return persist.Default.Set(pnvKey(id), string(raw))
}

func authorization(st pnvState) mtproto.TLObject {
	return mtproto.MakeTLAuthAuthorization(&mtproto.Auth_Authorization{
		User: mtproto.MakeTLUser(&mtproto.User{
			Self:       true,
			Id:         st.UserID,
			FirstName:  wrapperspb.String(st.FirstName),
			LastName:   wrapperspb.String(st.LastName),
			AccessHash: wrapperspb.Int64(st.AccessHash),
		}).To_User(),
	}).To_Auth_Authorization()
}

type welcomeItem struct {
	ID   int32  `json:"id"`
	Text string `json:"text"`
	Date int32  `json:"date"`
}

func welcomeKey(md *metadata.RpcMetadata, peer mtproto.TLObject) string {
	uid := int64(0)
	if md != nil {
		uid = md.UserId
	}
	raw, _ := json.Marshal(peer)
	return "welcome:" + strconv.FormatInt(uid, 10) + ":" + string(raw)
}

func loadWelcome(key string) ([]welcomeItem, error) {
	raw, err := persist.Default.Get(key)
	if err != nil || raw == "" {
		return nil, err
	}
	var items []welcomeItem
	err = json.Unmarshal([]byte(raw), &items)
	return items, err
}

func saveWelcome(key string, items []welcomeItem) error {
	raw, err := json.Marshal(items)
	if err != nil {
		return err
	}
	return persist.Default.Set(key, string(raw))
}

func requireUser(md *metadata.RpcMetadata) error {
	if md == nil || md.UserId == 0 {
		return mtproto.ErrAuthKeyUnregistered
	}
	return nil
}

func sendEphemeral229(md *metadata.RpcMetadata, in *mtproto.TLEphemeralSendMessageBA8D5F35) (mtproto.TLObject, error) {
	c := &core.ApiFullCore{MD: md}
	return c.EphemeralSendMessage(in.Legacy())
}

func deleteEphemeral229(md *metadata.RpcMetadata, in *mtproto.TLEphemeralDeleteMessage92F6E797) (mtproto.TLObject, error) {
	c := &core.ApiFullCore{MD: md}
	return c.EphemeralDeleteMessage(in.Legacy())
}

func getWelcome(md *metadata.RpcMetadata, in *mtproto.TLEphemeralGetWelcomeMessages) (mtproto.TLObject, error) {
	if err := requireUser(md); err != nil {
		return nil, err
	}
	if err := ensure(); err != nil {
		return nil, err
	}
	items, err := loadWelcome(welcomeKey(md, in.Peer))
	if err != nil {
		return nil, err
	}
	sum := welcomeHash(items)
	if in.Hash != 0 && in.Hash == sum {
		return &mtproto.TLEphemeralWelcomeMessagesNotModified{}, nil
	}
	return &mtproto.TLEphemeralWelcomeMessages{Hash: sum, Messages: toEphemeral(md.UserId, items)}, nil
}

func editWelcome(md *metadata.RpcMetadata, in *mtproto.TLEphemeralEditMessage) (mtproto.TLObject, error) {
	if err := requireUser(md); err != nil {
		return nil, err
	}
	if err := ensure(); err != nil {
		return nil, err
	}
	if in == nil {
		return nil, mtproto.ErrMessageIdInvalid
	}
	key := welcomeKey(md, in.Peer)
	items, err := loadWelcome(key)
	if err != nil {
		return nil, err
	}
	found := false
	for i := range items {
		if items[i].ID == in.ID {
			items[i].Text = in.Message
			found = true
			break
		}
	}
	if !found {
		items = append(items, welcomeItem{ID: in.ID, Text: in.Message})
	}
	if err = saveWelcome(key, items); err != nil {
		return nil, err
	}
	return mtproto.MakeEmptyUpdates(), nil
}

func deleteWelcome(md *metadata.RpcMetadata, in *mtproto.TLEphemeralDeleteWelcomeMessage) (mtproto.TLObject, error) {
	if err := requireUser(md); err != nil {
		return nil, err
	}
	if err := ensure(); err != nil {
		return nil, err
	}
	key := welcomeKey(md, in.Peer)
	items, err := loadWelcome(key)
	if err != nil {
		return nil, err
	}
	kept := items[:0]
	for _, item := range items {
		if item.ID != in.ID {
			kept = append(kept, item)
		}
	}
	if err = saveWelcome(key, kept); err != nil {
		return nil, err
	}
	return mtproto.BoolTrue, nil
}

func deleteAllWelcome(md *metadata.RpcMetadata, in *mtproto.TLEphemeralDeleteAllWelcomeMessages) (mtproto.TLObject, error) {
	if err := requireUser(md); err != nil {
		return nil, err
	}
	if err := ensure(); err != nil {
		return nil, err
	}
	if err := saveWelcome(welcomeKey(md, in.Peer), nil); err != nil {
		return nil, err
	}
	return mtproto.BoolTrue, nil
}

func welcomeHash(items []welcomeItem) int64 {
	var h int64
	for _, item := range items {
		h = h*131 + int64(item.ID)
		for _, c := range item.Text {
			h = h*131 + int64(c)
		}
	}
	return h
}

func toEphemeral(uid int64, items []welcomeItem) []*mtproto.EphemeralMessage {
	peer := mtproto.MakeTLPeerUser(&mtproto.Peer{UserId: uid}).To_Peer()
	out := make([]*mtproto.EphemeralMessage, 0, len(items))
	for _, item := range items {
		out = append(out, &mtproto.EphemeralMessage{
			PredicateName: mtproto.Predicate_ephemeralMessage,
			Id:            item.ID,
			FromId:        peer,
			PeerId:        peer,
			ReceiverId:    uid,
			Date:          item.Date,
			Message:       item.Text,
		})
	}
	return out
}
