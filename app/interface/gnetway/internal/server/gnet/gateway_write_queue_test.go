package gnet

import (
	"net"
	"testing"
	"time"

	"github.com/panjf2000/gnet/v2"
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/interface/gnetway/internal/server/gnet/codec"
)

type gatewayWriteTestCodec struct{}

func (gatewayWriteTestCodec) Encode(codec.CodecWriter, interface{}) ([]byte, error) {
	return []byte("encoded packet"), nil
}

func (gatewayWriteTestCodec) Decode(codec.CodecReader) (bool, []byte, error) {
	return false, nil, nil
}

func (gatewayWriteTestCodec) EncodeQuickAck(uint32) []byte { return nil }

type closedPeerGatewayConn struct {
	gnet.Conn
	pipe       net.Conn
	ctx        any
	writeCalls int
	writeErr   error
	closed     bool
}

func (c *closedPeerGatewayConn) Context() any { return c.ctx }

func (c *closedPeerGatewayConn) Write(p []byte) (int, error) {
	c.writeCalls++
	n, err := c.pipe.Write(p)
	c.writeErr = err
	return n, err
}

func (c *closedPeerGatewayConn) Close() error {
	c.closed = true
	return c.pipe.Close()
}

func TestGatewayWriteQueueBoundsAndScheduling(t *testing.T) {
	q := new(gatewayWriteQueue)
	item := gatewayWriteItem{msg: &mtproto.MTPRawMessage{}, queuedAt: time.Now()}
	if schedule, accepted := q.enqueue(item); !schedule || !accepted {
		t.Fatalf("first enqueue schedule=%v accepted=%v", schedule, accepted)
	}
	if schedule, accepted := q.enqueue(item); schedule || !accepted {
		t.Fatalf("second enqueue schedule=%v accepted=%v", schedule, accepted)
	}
	if got := len(q.take(1)); got != 1 {
		t.Fatalf("taken=%d want 1", got)
	}
	if !q.reschedule() {
		t.Fatal("queue with pending item did not request reschedule")
	}
	_ = q.take(1)
	if q.reschedule() {
		t.Fatal("empty queue requested reschedule")
	}
}

func TestGatewayWriteQueueRejectsWhenFull(t *testing.T) {
	q := new(gatewayWriteQueue)
	item := gatewayWriteItem{msg: &mtproto.MTPRawMessage{}, queuedAt: time.Now()}
	for i := 0; i < gatewayWriteQueueLimit; i++ {
		if _, accepted := q.enqueue(item); !accepted {
			t.Fatalf("enqueue %d rejected before limit", i)
		}
	}
	if _, accepted := q.enqueue(item); accepted {
		t.Fatal("queue accepted item beyond limit")
	}
}

func TestFlushGatewayWritesClosesConnectionOnSocketWriteFailure(t *testing.T) {
	for _, websocket := range []bool{false, true} {
		name := "tcp"
		if websocket {
			name = "websocket"
		}
		t.Run(name, func(t *testing.T) {
			serverSide, peerSide := net.Pipe()
			if err := peerSide.Close(); err != nil {
				t.Fatal(err)
			}
			conn := &closedPeerGatewayConn{
				pipe: serverSide,
				ctx: &connContext{
					codec:     gatewayWriteTestCodec{},
					websocket: websocket,
				},
			}
			defer conn.Close()

			const connID = int64(1)
			queue := new(gatewayWriteQueue)
			if _, accepted := queue.enqueue(gatewayWriteItem{
				msg:      &mtproto.MTPRawMessage{Payload: []byte("update")},
				queuedAt: time.Now(),
			}); !accepted {
				t.Fatal("failed to seed gateway write queue")
			}
			server := &Server{writeQueues: map[int64]*gatewayWriteQueue{connID: queue}}

			server.flushGatewayWrites(conn, connID)

			if conn.writeCalls == 0 {
				t.Fatal("injected socket failure did not reach the write path")
			}
			if conn.writeErr == nil {
				t.Fatal("closed socket peer did not produce a write error")
			}
			if !conn.closed {
				t.Fatal("connection remained open after socket write failure")
			}
		})
	}
}
