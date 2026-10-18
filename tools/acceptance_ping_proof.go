// Command acceptance_ping_proof creates one encrypted MTProto ping proof for
// the Identity bridge acceptance probe. It reads the auth key as base64 on
// stdin and writes the raw HTTP payload to stdout; no key material is logged.
package main

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/crypto"
)

func main() {
	authKeyID := flag.Int64("auth-key-id", 0, "authenticated MTProto auth key id")
	pingID := flag.Int64("ping-id", 0, "positive ping id")
	flag.Parse()
	if *authKeyID <= 0 || *pingID <= 0 {
		fatal("auth-key-id and ping-id must be positive")
	}
	encoded, err := io.ReadAll(os.Stdin)
	if err != nil {
		fatal("read auth key: " + err.Error())
	}
	key, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(encoded)))
	if err != nil || len(key) < 256 {
		fatal("auth key must be base64 encoded and at least 256 bytes")
	}

	messageBuf := mtproto.NewEncodeBuf(16)
	ping := &mtproto.TLPing{Constructor: mtproto.TLConstructor_CRC32_ping, PingId: *pingID}
	if err := ping.Encode(messageBuf, 229); err != nil {
		fatal("encode ping: " + err.Error())
	}
	message := messageBuf.GetBuf()
	raw := make([]byte, 0, 32+len(message))
	var randomPart [16]byte
	if _, err := rand.Read(randomPart[:]); err != nil {
		fatal("generate message envelope: " + err.Error())
	}
	var fixed [32]byte
	copy(fixed[:16], randomPart[:])
	binary.LittleEndian.PutUint64(fixed[16:24], uint64(mtproto.GenerateMessageId()))
	binary.LittleEndian.PutUint32(fixed[24:28], 0)
	binary.LittleEndian.PutUint32(fixed[28:32], uint32(len(message)))
	raw = append(raw, fixed[:]...)
	raw = append(raw, message...)

	// Client-side direction is the inverse of the server-side decrypt path:
	// the client encrypts with x=0 and the Teamgram server decrypts with
	// incoming=false.
	msgKey, ciphertext, err := crypto.NewClientAuthKey(*authKeyID, key).AesIgeEncrypt(raw)
	if err != nil {
		fatal("encrypt ping: " + err.Error())
	}
	payload := make([]byte, 8+len(msgKey)+len(ciphertext))
	binary.LittleEndian.PutUint64(payload[:8], uint64(*authKeyID))
	copy(payload[8:24], msgKey)
	copy(payload[24:], ciphertext)
	if _, err := os.Stdout.Write(payload); err != nil {
		fatal("write proof: " + err.Error())
	}
}

func fatal(message string) {
	_, _ = fmt.Fprintln(os.Stderr, message)
	os.Exit(2)
}
