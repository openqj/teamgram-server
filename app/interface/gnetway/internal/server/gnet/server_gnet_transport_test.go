package gnet

import "testing"

func TestClassifyMultiplexTransport(t *testing.T) {
	tests := []struct {
		name string
		data []byte
		want multiplexTransport
	}{
		{name: "partial websocket method", data: []byte("GE"), want: multiplexTransportPending},
		{name: "websocket upgrade", data: []byte("GET /apiws HTTP/1.1"), want: multiplexTransportWebsocket},
		{name: "partial post", data: []byte("POS"), want: multiplexTransportPending},
		{name: "http post", data: []byte("POST /apiw1 HTTP/1.1"), want: multiplexTransportHTTP},
		{name: "cors preflight", data: []byte("OPTIONS /apiw1 HTTP/1.1"), want: multiplexTransportHTTP},
		{name: "obfuscated payload", data: []byte{0xef, 1, 2, 3}, want: multiplexTransportWebsocket},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := classifyMultiplexTransport(tt.data); got != tt.want {
				t.Fatalf("classifyMultiplexTransport(%q) = %d, want %d", tt.data, got, tt.want)
			}
		})
	}
}
