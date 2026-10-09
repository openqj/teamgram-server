package sess

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"

	"github.com/teamgram/marmota/pkg/queue2"
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/interface/session/internal/dao"
	"github.com/teamgram/teamgram-server/app/service/authsession/authsession"
	statusclient "github.com/teamgram/teamgram-server/app/service/status/client"
	"github.com/teamgram/teamgram-server/app/service/status/status"
	"github.com/zeromicro/go-zero/core/syncx"
)

type deliveryStatusStub struct{ statusclient.StatusClient }

func (deliveryStatusStub) StatusSetSessionOnline(context.Context, *status.TLStatusSetSessionOnline) (*mtproto.Bool, error) {
	return mtproto.BoolTrue, nil
}

func (deliveryStatusStub) StatusSetSessionOffline(context.Context, *status.TLStatusSetSessionOffline) (*mtproto.Bool, error) {
	return mtproto.BoolTrue, nil
}

func deliveryActorFixture(t *testing.T, httpDelivery bool) (*MainAuthWrapper, *session, chan interface{}) {
	t.Helper()
	wrapper := &MainAuthWrapper{
		authKeyId: 11, AuthUserId: 101, state: mtproto.AuthStateNormal,
		client:    authsession.MakeTLClientSession(&authsession.ClientSession{AuthKeyId: 11, Layer: 229}).To_ClientSession(),
		closeChan: make(chan struct{}), sessionDataChan: make(chan interface{}, 16), rpcDataChan: make(chan interface{}, 16),
		rpcQueue: queue2.NewSyncQueue(), running: syncx.NewAtomicBool(), nextNotifyId: math.MaxInt32,
		cb: NewMainAuthWrapperManager(&dao.Dao{StatusClient: deliveryStatusStub{}}),
	}
	wrapper.mainAuth = newSessionList(mtproto.AuthKeyTypePerm, wrapper)
	wrapper.tempAuth = newSessionList(mtproto.AuthKeyTypeTemp, wrapper)
	wrapper.mediaTempAuth = newSessionList(mtproto.AuthKeyTypeMediaTemp, wrapper)
	wrapper.mainAuth.state = mtproto.AuthStateNormal
	wrapper.mainAuth.cacheSalt = mtproto.MakeTLFutureSalt(&mtproto.FutureSalt{Salt: 42})
	session := newSession(22, wrapper.mainAuth)
	session.connState = kStateOnline
	session.canSync = true
	session.isHttp = httpDelivery
	session.setGatewayId("delivery-test")
	wrapper.mainAuth.sessions[22] = session
	wrapper.mainUpdatesSession = session
	response := make(chan interface{}, 1)
	if httpDelivery {
		session.httpQueue.Push(response)
	}
	wrapper.Start()
	t.Cleanup(func() { wrapper.Stop(); wrapper.finish.Wait() })
	return wrapper, session, response
}

func TestSessionActorConfirmsGatewayDeliveryAndKeepsClientAckPending(t *testing.T) {
	for _, kind := range []string{"updates", "session_updates", "rpc_result"} {
		t.Run(kind, func(t *testing.T) {
			wrapper, session, response := deliveryActorFixture(t, true)
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			update := mtproto.MakeTLUpdatesTooLong(nil).To_Updates()
			var err error
			switch kind {
			case "updates":
				err = wrapper.SyncDataArrived(ctx, false, update)
			case "session_updates":
				err = wrapper.SyncSessionDataArrived(ctx, 11, 22, update)
			case "rpc_result":
				buffer := mtproto.NewEncodeBuf(32)
				if err := (&mtproto.TLRpcResult{ReqMsgId: 99, Result: mtproto.BoolTrue}).Encode(buffer, 229); err != nil {
					t.Fatal(err)
				}
				err = wrapper.SyncRpcResultDataArrived(ctx, 11, 22, 99, buffer.GetBuf())
			}
			if err != nil {
				t.Fatal(err)
			}
			select {
			case value := <-response:
				if payload, ok := value.([]byte); !ok || len(payload) < 32 {
					t.Fatalf("invalid gateway payload: %T", value)
				}
			default:
				t.Fatal("actor confirmed before gateway delivery")
			}
			wrapper.Stop()
			wrapper.finish.Wait()
			if session.outQueue.oMsgs.Len() != 1 || session.outQueue.oMsgs.Front().Value.(*outboxMsg).sent == 0 {
				t.Fatal("gateway delivery did not preserve MTProto client acknowledgement state")
			}
		})
	}
}

func TestSessionActorGatewayFailureRemainsUnconfirmed(t *testing.T) {
	wrapper, session, _ := deliveryActorFixture(t, false)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := wrapper.SyncDataArrived(ctx, false, mtproto.MakeTLUpdatesTooLong(nil).To_Updates()); err == nil {
		t.Fatal("missing gateway was confirmed")
	}
	wrapper.Stop()
	wrapper.finish.Wait()
	if session.outQueue.oMsgs.Len() != 1 || session.outQueue.oMsgs.Front().Value.(*outboxMsg).sent != 0 {
		t.Fatal("failed gateway delivery advanced outgoing acknowledgement state")
	}
}

func TestSessionPushWaitsForActorAndPropagatesBackpressure(t *testing.T) {
	wrapper := &MainAuthWrapper{closeChan: make(chan struct{}), sessionDataChan: make(chan interface{}, 1)}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- wrapper.SyncDataArrived(ctx, false, mtproto.MakeTLUpdatesTooLong(nil).To_Updates()) }()
	message := (<-wrapper.sessionDataChan).(*syncDataCtx)
	select {
	case err := <-done:
		t.Fatalf("queue admission confirmed push: %v", err)
	default:
	}
	want := errors.New("gateway failed")
	message.done <- want
	if err := <-done; !errors.Is(err, want) {
		t.Fatalf("actor result=%v want=%v", err, want)
	}
	wrapper.sessionDataChan <- struct{}{}
	if err := wrapper.SyncDataArrived(ctx, false, nil); !errors.Is(err, ErrDataChannelFull) {
		t.Fatalf("full actor queue=%v", err)
	}
	close(wrapper.closeChan)
	if err := wrapper.SyncDataArrived(ctx, false, nil); !errors.Is(err, ErrSessionClosed) {
		t.Fatalf("closed actor=%v", err)
	}
}

func TestSessionPushCanceledBeforeActorDoesNotDeliver(t *testing.T) {
	wrapper := &MainAuthWrapper{closeChan: make(chan struct{}), sessionDataChan: make(chan interface{}, 1)}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- wrapper.SyncDataArrived(ctx, false, mtproto.MakeTLUpdatesTooLong(nil).To_Updates()) }()
	message := (<-wrapper.sessionDataChan).(*syncDataCtx)
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("request cancellation=%v", err)
	}
	runSyncPush(message.done, func() error { return wrapper.onSyncData(message.ctx, &message.syncData) })
	if err := <-message.done; !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled queued message was processed: %v", err)
	}
}

func TestSessionPushEncodingFailureNeverQueuesPartialPayload(t *testing.T) {
	wrapper := &MainAuthWrapper{client: authsession.MakeTLClientSession(&authsession.ClientSession{Layer: 229}).To_ClientSession()}
	session := newSession(22, newSessionList(mtproto.AuthKeyTypePerm, wrapper))
	want := errors.New("encode failed")
	if err := session.sendPushToQueue(context.Background(), "gateway", 1, encodeFailResult{err: want}); !errors.Is(err, want) {
		t.Fatalf("push encoding failure=%v", err)
	}
	if session.outQueue.oMsgs.Len() != 0 {
		t.Fatal("partial update payload was queued")
	}
}
