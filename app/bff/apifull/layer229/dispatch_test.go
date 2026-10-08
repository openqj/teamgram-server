package layer229

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"os"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func TestMain(m *testing.M) {
	dsn := os.Getenv("APIFULL_MYSQL_DSN")
	if dsn == "" {
		// These tests exercise the legacy audit fixture. Production APIFull
		// coverage runs against PostgreSQL probes and does not require MySQL.
		os.Exit(0)
	}
	if err := UseMySQL(dsn); err != nil {
		panic(err)
	}
	os.Exit(m.Run())
}

func TestLayer229ConstructorsRoundTrip(t *testing.T) {
	in := &mtproto.TLAuthInitFirebasePnvLogin{ApiId: 4, ApiHash: "hash"}
	buf := mtproto.NewEncodeBuf(64)
	if err := in.Encode(buf, 229); err != nil {
		t.Fatal(err)
	}
	raw := buf.GetBuf()
	d := mtproto.NewDecodeBuf(raw)
	if got := d.Int(); got != int32(uint32(0x777df37a)) {
		t.Fatalf("constructor %x", uint32(got))
	}
	obj := mtproto.NewTLObjectByClassID(int32(uint32(0x777df37a)))
	if obj == nil {
		t.Fatal("constructor not registered")
	}
	if err := obj.Decode(d); err != nil {
		t.Fatal(err)
	}
	got := obj.(*mtproto.TLAuthInitFirebasePnvLogin)
	if got.ApiId != 4 || got.ApiHash != "hash" {
		t.Fatalf("%+v", got)
	}
}

func TestFirebaseAndWelcome(t *testing.T) {
	md := &metadata.RpcMetadata{PermAuthKeyId: 77, UserId: 77}
	intent, ok, err := Dispatch(nil, md, &mtproto.TLAuthInitFirebasePnvLogin{ApiId: 6, ApiHash: "abc"})
	if err != nil || !ok {
		t.Fatal(err, ok)
	}
	if intent.(*mtproto.TLAuthFirebasePnvIntent).Nonce == "" {
		t.Fatal("empty nonce")
	}
	if _, _, err = Dispatch(context.Background(), md, &mtproto.TLAuthFinishFirebasePnvLogin{GoogleToken: "tok"}); err == nil {
		t.Fatal("plain token was accepted")
	}
	token := signTestToken(t, "kid-1", "+15550001111")
	dir := &memUsers{}
	SetDirectory(dir)
	SetProjectID("demo-project")
	t.Cleanup(func() {
		SetDirectory(nil)
		SetProjectID("")
		SetCertFetcher(nil)
	})
	auth, ok, err := Dispatch(context.Background(), md, &mtproto.TLAuthFinishFirebasePnvLogin{GoogleToken: token})
	if err != nil || !ok || auth == nil {
		t.Fatal(err, ok)
	}
	if len(dir.phones) != 1 || dir.phones[0] != "+15550001111" {
		t.Fatalf("createNewUser phones: %v", dir.phones)
	}
	if got := auth.(*mtproto.Auth_Authorization).GetUser().GetId(); got == 0 || got == md.PermAuthKeyId {
		t.Fatalf("user id %d must come from createNewUser", got)
	}
	signed, ok, err := Dispatch(context.Background(), md, &mtproto.TLAuthFirebasePnvSignUp{FirstName: "Ada", LastName: "Lovelace"})
	if err != nil || !ok {
		t.Fatal(err)
	}
	if signed.(*mtproto.Auth_Authorization).GetUser().GetFirstName().GetValue() != "Ada" {
		t.Fatalf("%+v", signed)
	}
	if len(dir.names) != 1 || dir.names[0] != "Ada" {
		t.Fatalf("update name: %v", dir.names)
	}

	peer := mtproto.MakeTLInputPeerSelf(&mtproto.InputPeer{}).To_InputPeer()
	if _, _, err = Dispatch(nil, md, &mtproto.TLEphemeralEditMessage{Peer: peer, ID: 3, Message: "hi", Welcome: true}); err != nil {
		t.Fatal(err)
	}
	reply, _, err := Dispatch(nil, md, &mtproto.TLEphemeralGetWelcomeMessages{Peer: peer})
	if err != nil {
		t.Fatal(err)
	}
	msgs := reply.(*mtproto.TLEphemeralWelcomeMessages)
	if len(msgs.Messages) != 1 || msgs.Messages[0].Message != "hi" {
		t.Fatalf("%+v", msgs.Messages)
	}
	if _, _, err = Dispatch(nil, md, &mtproto.TLEphemeralDeleteWelcomeMessage{Peer: peer, ID: 3}); err != nil {
		t.Fatal(err)
	}
	reply, _, err = Dispatch(nil, md, &mtproto.TLEphemeralGetWelcomeMessages{Peer: peer})
	if err != nil {
		t.Fatal(err)
	}
	if len(reply.(*mtproto.TLEphemeralWelcomeMessages).Messages) != 0 {
		t.Fatal("expected empty")
	}
	if _, _, err = Dispatch(nil, md, &mtproto.TLEphemeralEditMessage{Peer: peer, ID: 1, Message: "x"}); err != nil {
		t.Fatal(err)
	}
	if _, _, err = Dispatch(nil, md, &mtproto.TLEphemeralDeleteAllWelcomeMessages{Peer: peer}); err != nil {
		t.Fatal(err)
	}
}

type memUsers struct {
	next   int64
	phones []string
	names  []string
}

func (m *memUsers) CreateNewUser(_ context.Context, _ int64, phone, _, first, last string) (*mtproto.User, error) {
	m.next++
	m.phones = append(m.phones, phone)
	return mtproto.MakeTLUser(&mtproto.User{
		Self:       true,
		Id:         1000 + m.next,
		FirstName:  wrapperspb.String(first),
		LastName:   wrapperspb.String(last),
		AccessHash: wrapperspb.Int64(42),
	}).To_User(), nil
}

func (m *memUsers) UpdateName(_ context.Context, _ int64, first, _ string) error {
	m.names = append(m.names, first)
	return nil
}

func signTestToken(t *testing.T, kid, phone string) string {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.CreateCertificate(rand.Reader, &x509.Certificate{
		SerialNumber: big.NewInt(1),
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
	}, &x509.Certificate{
		SerialNumber: big.NewInt(1),
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
	}, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	block, _ := pem.Decode(certPEM)
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	SetCertFetcher(func(context.Context) (map[string]*rsa.PublicKey, error) {
		return map[string]*rsa.PublicKey{kid: cert.PublicKey.(*rsa.PublicKey)}, nil
	})
	header, _ := json.Marshal(map[string]string{"alg": "RS256", "kid": kid})
	payload, _ := json.Marshal(map[string]any{
		"iss":          "https://securetoken.google.com/demo-project",
		"aud":          "demo-project",
		"sub":          "firebase-user",
		"exp":          time.Now().Add(time.Hour).Unix(),
		"phone_number": phone,
	})
	signInput := base64.RawURLEncoding.EncodeToString(header) + "." + base64.RawURLEncoding.EncodeToString(payload)
	sum := sha256.Sum256([]byte(signInput))
	sig, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, sum[:])
	if err != nil {
		t.Fatal(err)
	}
	return signInput + "." + base64.RawURLEncoding.EncodeToString(sig)
}
