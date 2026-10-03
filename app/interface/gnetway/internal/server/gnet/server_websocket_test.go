package gnet

import (
	"bytes"
	"encoding/binary"
	"errors"
	"testing"

	"github.com/gobwas/ws/wsutil"
	"github.com/teamgram/proto/mtproto/crypto"
	"github.com/teamgram/teamgram-server/app/interface/gnetway/internal/server/gnet/codec"
	"github.com/teamgram/teamgram-server/app/interface/gnetway/internal/server/gnet/ws"
)

func TestBufferWebsocketMessages_HeaderAndFirstFrameSplit(t *testing.T) {
	header, packet, payload := obfuscatedAbridgedWire(t)
	conn := new(ws.WsConn)
	t.Cleanup(conn.Release)

	bufferWebsocketMessages(conn, []wsutil.Message{
		{Payload: header},
		{Payload: packet},
	})
	transport, err := codec.CreateCodec(conn)
	if err != nil {
		t.Fatalf("CreateCodec() error = %v", err)
	}
	_, frame, err := transport.Decode(conn)
	if err != nil {
		t.Fatalf("Decode() error = %v", err)
	}
	if !bytes.Equal(frame, payload) {
		t.Fatalf("Decode() frame = %x, want %x", frame, payload)
	}
}

func TestBufferWebsocketMessages_FirstFrameSplit(t *testing.T) {
	header, packet, payload := obfuscatedAbridgedWire(t)
	conn := new(ws.WsConn)
	t.Cleanup(conn.Release)

	bufferWebsocketMessages(conn, []wsutil.Message{
		{Payload: header},
		{Payload: packet[:2]},
	})
	transport, err := codec.CreateCodec(conn)
	if err != nil {
		t.Fatalf("CreateCodec() error = %v", err)
	}

	if _, _, err := transport.Decode(conn); !errors.Is(err, codec.ErrUnexpectedEOF) {
		t.Fatalf("Decode() error = %v, want ErrUnexpectedEOF", err)
	}
	if got, want := conn.InboundBuffered(), 1; got != want {
		t.Fatalf("InboundBuffered() = %d, want %d", got, want)
	}

	bufferWebsocketMessages(conn, []wsutil.Message{{Payload: packet[2:]}})
	_, frame, err := transport.Decode(conn)
	if err != nil {
		t.Fatalf("Decode() error = %v", err)
	}
	if !bytes.Equal(frame, payload) {
		t.Fatalf("Decode() frame = %x, want %x", frame, payload)
	}
}

func obfuscatedAbridgedWire(t *testing.T) (header, packet, payload []byte) {
	t.Helper()

	random := make([]byte, 64)
	for i := range random {
		random[i] = byte(i + 1)
	}
	binary.BigEndian.PutUint32(random[56:60], codec.ABRIDGED_INT32_FLAG)
	binary.BigEndian.PutUint16(random[60:62], 1)

	encryptor, err := crypto.NewAesCTR128Encrypt(random[8:40], random[40:56])
	if err != nil {
		t.Fatalf("NewAesCTR128Encrypt() error = %v", err)
	}
	ciphertextHeader := append([]byte(nil), random...)
	encryptor.Encrypt(ciphertextHeader)
	header = append([]byte(nil), random[:56]...)
	header = append(header, ciphertextHeader[56:64]...)

	payload = []byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11}
	packet = append([]byte{byte(len(payload) / 4)}, payload...)
	encryptor.Encrypt(packet)
	return header, packet, payload
}
