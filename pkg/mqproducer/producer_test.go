package mqproducer

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/IBM/sarama"
	kafka "github.com/teamgram/marmota/pkg/mq"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc/metadata"
)

type fakeAsyncProducer struct {
	input      chan *sarama.ProducerMessage
	accepted   chan *sarama.ProducerMessage
	successes  chan *sarama.ProducerMessage
	failures   chan *sarama.ProducerError
	closing    chan struct{}
	intakeDone chan struct{}
	pending    map[*sarama.ProducerMessage]bool
	mu         sync.Mutex
	closeOnce  sync.Once
	finishOnce sync.Once
	holdClose  bool
}

func newFakeAsyncProducer(paused, holdClose bool) *fakeAsyncProducer {
	f := &fakeAsyncProducer{
		input: make(chan *sarama.ProducerMessage), accepted: make(chan *sarama.ProducerMessage, maxOutstanding),
		successes: make(chan *sarama.ProducerMessage, maxOutstanding), failures: make(chan *sarama.ProducerError, maxOutstanding),
		closing: make(chan struct{}), intakeDone: make(chan struct{}), pending: make(map[*sarama.ProducerMessage]bool), holdClose: holdClose,
	}
	go func() {
		defer close(f.intakeDone)
		defer close(f.accepted)
		if paused {
			<-f.closing
			return
		}
		for message := range f.input {
			f.mu.Lock()
			f.pending[message] = true
			f.mu.Unlock()
			f.accepted <- message
		}
	}()
	return f
}

func (f *fakeAsyncProducer) Input() chan<- *sarama.ProducerMessage     { return f.input }
func (f *fakeAsyncProducer) Successes() <-chan *sarama.ProducerMessage { return f.successes }
func (f *fakeAsyncProducer) Errors() <-chan *sarama.ProducerError      { return f.failures }
func (f *fakeAsyncProducer) AsyncClose() {
	f.closeOnce.Do(func() {
		close(f.closing)
		close(f.input)
		if !f.holdClose {
			go f.finish()
		}
	})
}

func (f *fakeAsyncProducer) ack(message *sarama.ProducerMessage, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.pending[message] {
		return
	}
	delete(f.pending, message)
	if err != nil {
		f.failures <- &sarama.ProducerError{Msg: message, Err: err}
	} else {
		message.Partition, message.Offset = 2, 41
		f.successes <- message
	}
}

func (f *fakeAsyncProducer) finish() {
	f.finishOnce.Do(func() {
		<-f.intakeDone
		f.mu.Lock()
		defer f.mu.Unlock()
		for message := range f.pending {
			f.successes <- message
			delete(f.pending, message)
		}
		close(f.successes)
		close(f.failures)
	})
}

func testProducer(t *testing.T, paused, holdClose bool) (*Producer, *fakeAsyncProducer) {
	t.Helper()
	f := newFakeAsyncProducer(paused, holdClose)
	p := newProducer("producer-test", f)
	t.Cleanup(func() {
		go f.finish()
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if err := p.CloseContext(ctx); err != nil {
			t.Errorf("producer cleanup: %v", err)
		}
	})
	return p, f
}

func sendAsync(p *Producer, ctx context.Context) <-chan error {
	result := make(chan error, 1)
	go func() {
		_, _, err := p.SendMessageV2(ctx, "sync.TL_sync_pushUpdates", "202", []byte("event"))
		result <- err
	}()
	return result
}

func accepted(t *testing.T, f *fakeAsyncProducer) *sarama.ProducerMessage {
	t.Helper()
	select {
	case message := <-f.accepted:
		if message == nil {
			t.Fatal("producer closed before accepting message")
		}
		return message
	case <-time.After(time.Second):
		t.Fatal("publication was not accepted")
		return nil
	}
}

func awaitResult(t *testing.T, result <-chan error, want error) {
	t.Helper()
	select {
	case err := <-result:
		if !errors.Is(err, want) {
			t.Fatalf("publication result = %v, want %v", err, want)
		}
	case <-time.After(time.Second):
		t.Fatal("publication did not finish")
	}
}

func TestProducerConfigRequiresDurableBoundedKafka(t *testing.T) {
	c := &kafka.KafkaProducerConf{Topic: "Sync-T", Brokers: []string{"127.0.0.1:9092"}, ProducerAck: "no_response"}
	conf, err := buildConfig(c)
	if err != nil {
		t.Fatal(err)
	}
	if conf.Producer.RequiredAcks != sarama.WaitForAll || conf.Net.MaxOpenRequests != 1 || conf.ChannelBufferSize != maxOutstanding || conf.Producer.Retry.Max != 2 || conf.Metadata.Retry.Max != 2 {
		t.Fatal("producer acknowledgements, ordering, buffer, or retries are unbounded")
	}
	for _, timeout := range []time.Duration{conf.Net.DialTimeout, conf.Net.ReadTimeout, conf.Net.WriteTimeout, conf.Metadata.Timeout, conf.Producer.Timeout} {
		if timeout != 5*time.Second {
			t.Fatalf("producer timeout = %v", timeout)
		}
	}
	partitioner := conf.Producer.Partitioner(c.Topic)
	first, _ := partitioner.Partition(&sarama.ProducerMessage{Key: sarama.StringEncoder("202")}, 3)
	second, _ := partitioner.Partition(&sarama.ProducerMessage{Key: sarama.StringEncoder("202")}, 3)
	if first != second || !partitioner.RequiresConsistency() {
		t.Fatal("same recipient must retain its hash partition")
	}
	for _, invalid := range []*kafka.KafkaProducerConf{nil, {}, {Topic: "Sync-T"}, {Topic: "Sync-T", Brokers: []string{" "}}} {
		if _, err := buildConfig(invalid); err == nil {
			t.Fatal("invalid Kafka producer configuration accepted")
		}
	}
}

func TestProducerCancelsBeforeAndDuringEnqueue(t *testing.T) {
	p, f := testProducer(t, true, false)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	awaitResult(t, sendAsync(p, ctx), context.Canceled)
	ctx, cancel = context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	awaitResult(t, sendAsync(p, ctx), context.DeadlineExceeded)
	if len(p.slots) != 0 {
		t.Fatal("cancelled enqueue retained outstanding capacity")
	}
	select {
	case <-f.accepted:
		t.Fatal("cancelled enqueue published a record")
	default:
	}
}

func TestProducerOutstandingLimitIncludesTimedOutCalls(t *testing.T) {
	p, f := testProducer(t, false, false)
	var messages []*sarama.ProducerMessage
	for i := 0; i < maxOutstanding; i++ {
		ctx, cancel := context.WithCancel(context.Background())
		result := sendAsync(p, ctx)
		messages = append(messages, accepted(t, f))
		cancel()
		awaitResult(t, result, context.Canceled)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	awaitResult(t, sendAsync(p, ctx), context.DeadlineExceeded)
	if len(p.slots) != maxOutstanding {
		t.Fatalf("outstanding = %d, want %d", len(p.slots), maxOutstanding)
	}
	f.ack(messages[0], nil)
	result := sendAsync(p, context.Background())
	message := accepted(t, f)
	f.ack(message, nil)
	awaitResult(t, result, nil)
}

func TestProducerLateAcknowledgementAndFailureRemainDrainable(t *testing.T) {
	p, f := testProducer(t, false, false)
	ctx, cancel := context.WithCancel(context.Background())
	result := sendAsync(p, ctx)
	message := accepted(t, f)
	cancel()
	awaitResult(t, result, context.Canceled)
	f.ack(message, nil)
	result = sendAsync(p, context.Background())
	message = accepted(t, f)
	f.ack(message, sarama.ErrMessageSizeTooLarge)
	awaitResult(t, result, sarama.ErrMessageSizeTooLarge)
	result = sendAsync(p, context.Background())
	f.ack(accepted(t, f), nil)
	awaitResult(t, result, nil)
}

func TestProducerOwnsPayloadAfterCallerCancellation(t *testing.T) {
	p, f := testProducer(t, false, false)
	ctx, cancel := context.WithCancel(context.Background())
	value := []byte("original")
	result := make(chan error, 1)
	go func() {
		_, _, err := p.SendMessageV2(ctx, "method", "202", value)
		result <- err
	}()
	message := accepted(t, f)
	cancel()
	awaitResult(t, result, context.Canceled)
	copy(value, "replaced")
	encoded, err := message.Value.Encode()
	if err != nil || string(encoded) != "original" {
		t.Fatalf("in-flight Kafka payload changed after cancellation: %v", err)
	}
	f.ack(message, nil)
}

func TestProducerPreservesMethodBusinessMetadataAndTrace(t *testing.T) {
	previous := otel.GetTextMapPropagator()
	otel.SetTextMapPropagator(propagation.TraceContext{})
	defer otel.SetTextMapPropagator(previous)
	p, f := testProducer(t, false, false)
	parent := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID: trace.TraceID{1, 2, 3}, SpanID: trace.SpanID{4, 5, 6}, TraceFlags: trace.FlagsSampled,
	})
	ctx := trace.ContextWithSpanContext(context.Background(), parent)
	ctx = metadata.NewOutgoingContext(ctx, metadata.Pairs("x-user-id", "9223372036854775807", "method", "must-not-override"))
	result := sendAsync(p, ctx)
	message := accepted(t, f)
	carrier := propagation.MapCarrier{}
	methods := 0
	for _, header := range message.Headers {
		carrier[string(header.Key)] = string(header.Value)
		if string(header.Key) == "method" {
			methods++
		}
	}
	if methods != 1 || carrier["method"] != "sync.TL_sync_pushUpdates" || carrier["x-user-id"] != "9223372036854775807" {
		t.Fatalf("incompatible Kafka headers: %v", carrier)
	}
	received := trace.SpanContextFromContext(propagation.TraceContext{}.Extract(context.Background(), carrier))
	if received.TraceID() != parent.TraceID() || !received.IsSampled() {
		t.Fatal("Kafka tracing context was not propagated")
	}
	key, _ := message.Key.Encode()
	if message.Topic != p.Topic() || string(key) != "202" {
		t.Fatal("Kafka topic or recipient key changed")
	}
	f.ack(message, nil)
	awaitResult(t, result, nil)
}

func TestProducerCloseTimeoutStopsAdmissionAndContinuesDraining(t *testing.T) {
	p, f := testProducer(t, false, true)
	result := sendAsync(p, context.Background())
	message := accepted(t, f)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := p.CloseContext(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("blocked shutdown = %v", err)
	}
	awaitResult(t, sendAsync(p, context.Background()), ErrClosed)
	f.ack(message, nil)
	awaitResult(t, result, nil)
	f.finish()
	if err := p.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestProducerConcurrentSendAndClose(t *testing.T) {
	p, f := testProducer(t, false, false)
	acked := make(chan struct{})
	go func() {
		defer close(acked)
		for message := range f.accepted {
			f.ack(message, nil)
		}
	}()
	var calls sync.WaitGroup
	for i := 0; i < 128; i++ {
		calls.Add(1)
		go func() {
			defer calls.Done()
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			_, _, err := p.SendMessageV2(ctx, "method", "202", []byte("event"))
			if err != nil && !errors.Is(err, ErrClosed) {
				t.Errorf("concurrent send/close: %v", err)
			}
		}()
	}
	for i := 0; i < 8; i++ {
		calls.Add(1)
		go func() {
			defer calls.Done()
			if err := p.Close(); err != nil {
				t.Errorf("concurrent close: %v", err)
			}
		}()
	}
	calls.Wait()
	<-acked
	if len(p.slots) != 0 {
		t.Fatal("closed producer retained outstanding capacity")
	}
	awaitResult(t, sendAsync(p, context.Background()), ErrClosed)
}

func TestProducerCloseAllSharesOneBudget(t *testing.T) {
	var producers []*Producer
	var clients []*fakeAsyncProducer
	for range 3 {
		p, f := testProducer(t, false, true)
		producers = append(producers, p)
		clients = append(clients, f)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	started := time.Now()
	result := make(chan error, 1)
	go func() { result <- closeAll(ctx, producers...) }()
	for _, client := range clients {
		select {
		case <-client.closing:
		case <-time.After(time.Second):
			t.Fatal("CloseAll did not begin every producer shutdown")
		}
	}
	awaitResult(t, result, context.DeadlineExceeded)
	if time.Since(started) > 250*time.Millisecond {
		t.Fatal("CloseAll multiplied the shutdown budget")
	}
	for _, producer := range producers {
		awaitResult(t, sendAsync(producer, context.Background()), ErrClosed)
	}
}

func TestProducerMockBrokerFailure(t *testing.T) {
	broker := sarama.NewMockBroker(t, 1)
	defer broker.Close()
	const topic = "producer-mock-test"
	broker.SetHandlerByMap(map[string]sarama.MockResponse{
		"MetadataRequest": sarama.NewMockMetadataResponse(t).SetBroker(broker.Addr(), 1).SetLeader(topic, 0, 1),
		"ProduceRequest":  sarama.NewMockProduceResponse(t).SetError(topic, 0, sarama.ErrMessageSizeTooLarge),
	})
	p, err := New(&kafka.KafkaProducerConf{Topic: topic, Brokers: []string{broker.Addr()}})
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_, _, err = p.SendMessageV2(ctx, "method", "202", []byte("event"))
	if !errors.Is(err, sarama.ErrMessageSizeTooLarge) {
		t.Fatalf("broker failure = %v", err)
	}
}
