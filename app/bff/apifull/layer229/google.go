package layer229

import (
	"context"
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"math"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Google's Firebase ID token signing certificates.
const googleSecureTokenCerts = "https://www.googleapis.com/robot/v1/metadata/x509/securetoken@system.gserviceaccount.com"

type certFetcher func(context.Context) (map[string]*rsa.PublicKey, error)

var (
	stateMu    sync.Mutex
	projectID  string
	fetchCerts certFetcher = fetchGoogleSecureTokenCerts
)

// SetProjectID is the Firebase project id. Tokens must have this audience.
func SetProjectID(id string) {
	stateMu.Lock()
	projectID = id
	stateMu.Unlock()
}

// SetCertFetcher replaces the Google certificate source. Tests use a local key.
func SetCertFetcher(fn certFetcher) {
	stateMu.Lock()
	if fn == nil {
		fetchCerts = fetchGoogleSecureTokenCerts
	} else {
		fetchCerts = fn
	}
	stateMu.Unlock()
}

// VerifyFirebaseToken checks an RS256 Firebase ID token and returns the subject and phone number.
func VerifyFirebaseToken(ctx context.Context, token string) (sub, phone string, err error) {
	stateMu.Lock()
	project := projectID
	fetch := fetchCerts
	stateMu.Unlock()
	if project == "" {
		return "", "", errors.New("firebase project id is not configured")
	}
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return "", "", errors.New("token is not a jwt")
	}
	headJSON, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return "", "", err
	}
	var head struct {
		Alg string `json:"alg"`
		Kid string `json:"kid"`
	}
	if err = json.Unmarshal(headJSON, &head); err != nil {
		return "", "", err
	}
	if head.Alg != "RS256" || head.Kid == "" {
		return "", "", errors.New("token algorithm is not RS256")
	}
	keys, err := fetch(ctx)
	if err != nil {
		return "", "", err
	}
	pub := keys[head.Kid]
	if pub == nil {
		return "", "", errors.New("token signing key is not a google firebase key")
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return "", "", err
	}
	sum := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if err = rsa.VerifyPKCS1v15(pub, crypto.SHA256, sum[:], sig); err != nil {
		return "", "", err
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return "", "", err
	}
	var claims struct {
		Iss         string  `json:"iss"`
		Aud         string  `json:"aud"`
		Sub         string  `json:"sub"`
		Exp         float64 `json:"exp"`
		PhoneNumber string  `json:"phone_number"`
	}
	if err = json.Unmarshal(payload, &claims); err != nil {
		return "", "", err
	}
	wantIss := "https://securetoken.google.com/" + project
	if claims.Iss != wantIss || claims.Aud != project || claims.Sub == "" {
		return "", "", errors.New("token audience does not match the firebase project")
	}
	if claims.Exp < float64(time.Now().Add(-time.Minute).Unix()) || claims.Exp > math.MaxInt64 {
		return "", "", errors.New("token is expired")
	}
	if claims.PhoneNumber == "" {
		return "", "", errors.New("token has no phone_number")
	}
	return claims.Sub, claims.PhoneNumber, nil
}

func fetchGoogleSecureTokenCerts(ctx context.Context) (map[string]*rsa.PublicKey, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, googleSecureTokenCerts, nil)
	if err != nil {
		return nil, err
	}
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("google certs: %s", resp.Status)
	}
	var pems map[string]string
	if err = json.NewDecoder(resp.Body).Decode(&pems); err != nil {
		return nil, err
	}
	out := make(map[string]*rsa.PublicKey, len(pems))
	for kid, block := range pems {
		der, _ := pem.Decode([]byte(block))
		if der == nil {
			continue
		}
		cert, err := x509.ParseCertificate(der.Bytes)
		if err != nil {
			continue
		}
		pub, ok := cert.PublicKey.(*rsa.PublicKey)
		if ok {
			out[kid] = pub
		}
	}
	if len(out) == 0 {
		return nil, errors.New("google certs contained no rsa keys")
	}
	return out, nil
}
