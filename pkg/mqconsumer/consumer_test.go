package mqconsumer

import (
	"context"
	"errors"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/IBM/sarama"
	kafka "github.com/teamgram/marmota/pkg/mq"
)

type testSession struct {
	sarama.ConsumerGroupSession
	ctx          context.Context
	marked       []*sarama.ConsumerMessage
	commits      int
	commitMarks  []int
	commitSignal chan struct{}
	commitDelay  time.Duration
}

func (s *testSession) Context() context.Context { return s.ctx }
func (s *testSession) MarkMessage(message *sarama.ConsumerMessage, _ string) {
	s.marked = append(s.marked, message)
}
func (s *testSession) Commit() {
	if s.commitDelay > 0 {
		time.Sleep(s.commitDelay)
	}
	s.commits++
	s.commitMarks = append(s.commitMarks, len(s.marked))
	if s.commitSignal != nil {
		select {
		case s.commitSignal <- struct{}{}:
		default:
		}
	}
}

type testClaim struct {
	sarama.ConsumerGroupClaim
	messages chan *sarama.ConsumerMessage
}

type testDeadLetterSink struct {
	values []DeadLetter
	closed bool
}

func (s *testDeadLetterSink) Publish(_ context.Context, value DeadLetter) error {
	s.values = append(s.values, value)
	return nil
}

func (s *testDeadLetterSink) Close() error {
	s.closed = true
	return nil
}

func (c *testClaim) Messages() <-chan *sarama.ConsumerMessage { return c.messages }

func TestMetadataContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if _, ok := MetadataFromContext(ctx); ok {
		t.Fatal("unexpected metadata on parent context")
	}
	want := Metadata{ConsumerGroup: "inbox-test", Topic: "Inbox-T", Partition: 3, Offset: 42}
	delivery := WithMetadata(ctx, want)
	if got, ok := MetadataFromContext(delivery); !ok || got != want {
		t.Fatalf("metadata=%+v present=%v want=%+v", got, ok, want)
	}
	cancel()
	if !errors.Is(delivery.Err(), context.Canceled) {
		t.Fatal("delivery context lost parent cancellation")
	}
}

func TestConsumerDoesNotAcknowledgeHandlerFailure(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	session := &testSession{ctx: ctx}
	claim := &testClaim{messages: make(chan *sarama.ConsumerMessage, 1)}
	claim.messages <- &sarama.ConsumerMessage{Topic: "Sync-T", Partition: 2, Offset: 91, Value: []byte("event")}
	calls := 0
	consumer := &Consumer{conf: kafka.KafkaConsumerConf{Group: "sync-test"}, handler: func(ctx context.Context, method, key string, value []byte) error {
		calls++
		metadata, ok := MetadataFromContext(ctx)
		if !ok || metadata.ConsumerGroup != "sync-test" || metadata.Topic != "Sync-T" || metadata.Partition != 2 || metadata.Offset != 91 {
			t.Fatalf("missing delivery identity: %+v", metadata)
		}
		cancel()
		return errors.New("database write failed")
	}}
	if err := consumer.ConsumeClaim(session, claim); err != nil || calls != 1 || len(session.marked) != 0 || session.commits != 0 {
		t.Fatalf("failed record acknowledged, calls=%d marked=%d commits=%d error=%v", calls, len(session.marked), session.commits, err)
	}
}

func TestConsumerAcknowledgesOnlyAfterSuccessfulRetry(t *testing.T) {
	session := &testSession{ctx: context.Background()}
	claim := &testClaim{messages: make(chan *sarama.ConsumerMessage, 2)}
	claim.messages <- &sarama.ConsumerMessage{Topic: "Sync-T", Offset: 91, Value: []byte("first"), Headers: []*sarama.RecordHeader{{Key: []byte("method"), Value: []byte("updates")}}}
	claim.messages <- &sarama.ConsumerMessage{Topic: "Sync-T", Offset: 92, Value: []byte("second")}
	close(claim.messages)
	var offsets []int64
	consumer := &Consumer{conf: kafka.KafkaConsumerConf{Group: "sync-test"}, handler: func(ctx context.Context, method, key string, value []byte) error {
		metadata, _ := MetadataFromContext(ctx)
		offsets = append(offsets, metadata.Offset)
		if len(offsets) == 1 {
			if len(session.marked) != 0 {
				t.Fatal("message marked before successful processing")
			}
			return errors.New("transient database failure")
		}
		return nil
	}}
	if err := consumer.ConsumeClaim(session, claim); err != nil {
		t.Fatal(err)
	}
	if len(offsets) != 3 || offsets[0] != 91 || offsets[1] != 91 || offsets[2] != 92 || len(session.marked) != 2 || session.commits != 2 {
		t.Fatalf("retry ordering=%v marked=%d commits=%d", offsets, len(session.marked), session.commits)
	}
}

func TestConsumerRunsDifferentKeysConcurrentlyAndPreservesKeyOrder(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	session := &testSession{ctx: ctx}
	claim := &testClaim{messages: make(chan *sarama.ConsumerMessage, 5)}
	for _, item := range []struct {
		key    string
		offset int64
	}{
		{key: "a", offset: 1}, {key: "a", offset: 2}, {key: "b", offset: 3},
		{key: "b", offset: 4}, {key: "a", offset: 5},
	} {
		claim.messages <- &sarama.ConsumerMessage{Topic: "Sync-T", Key: []byte(item.key), Offset: item.offset,
			Value: []byte(strconv.FormatInt(item.offset, 10))}
	}
	close(claim.messages)

	firstA := make(chan struct{})
	allowA := make(chan struct{})
	secondKey := make(chan struct{})
	allowB := make(chan struct{})
	var onceA, onceB sync.Once
	var mu sync.Mutex
	var order = map[string][]int64{}
	active := 0
	maxActive := 0
	consumer := &Consumer{conf: kafka.KafkaConsumerConf{Group: "sync-test"}, handlerConcurrency: 2,
		handler: func(_ context.Context, _ string, key string, value []byte) error {
			metadataKey := key
			if metadataKey == "" {
				return errors.New("missing key")
			}
			offset, err := strconv.ParseInt(string(value), 10, 64)
			if err != nil {
				return err
			}
			mu.Lock()
			active++
			if active > maxActive {
				maxActive = active
			}
			mu.Unlock()
			defer func() {
				mu.Lock()
				active--
				mu.Unlock()
			}()
			mu.Lock()
			order[metadataKey] = append(order[metadataKey], offset)
			firstForKey := len(order[metadataKey]) == 1
			mu.Unlock()
			if metadataKey == "a" && firstForKey {
				onceA.Do(func() { close(firstA) })
				<-allowA
			}
			if metadataKey == "b" {
				onceB.Do(func() { close(secondKey) })
				<-allowB
			}
			return nil
		}}

	done := make(chan error, 1)
	go func() { done <- consumer.ConsumeClaim(session, claim) }()
	select {
	case <-firstA:
	case <-time.After(time.Second):
		t.Fatal("first key did not start")
	}
	select {
	case <-secondKey:
	case <-time.After(time.Second):
		t.Fatal("a different key did not run concurrently")
	}
	close(allowA)
	close(allowB)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	gotMaxActive := maxActive
	gotOrder := map[string][]int64{"a": append([]int64(nil), order["a"]...), "b": append([]int64(nil), order["b"]...)}
	mu.Unlock()
	if gotMaxActive < 2 {
		t.Fatalf("different keys were not concurrent, max active=%d", gotMaxActive)
	}
	if len(gotOrder["a"]) != 3 || len(gotOrder["b"]) != 2 ||
		!equalInt64s(gotOrder["a"], []int64{1, 2, 5}) || !equalInt64s(gotOrder["b"], []int64{3, 4}) {
		t.Fatalf("unexpected per-key delivery counts: %+v", gotOrder)
	}
}

func equalInt64s(got, want []int64) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

func TestConsumerCommitsOnlyContiguousCompletions(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	session := &testSession{ctx: ctx, commitSignal: make(chan struct{}, 4)}
	claim := &testClaim{messages: make(chan *sarama.ConsumerMessage, 3)}
	for _, item := range []struct {
		key    string
		offset int64
	}{{"a", 1}, {"b", 2}, {"c", 3}} {
		claim.messages <- &sarama.ConsumerMessage{Topic: "Sync-T", Key: []byte(item.key), Offset: item.offset,
			Value: []byte(strconv.FormatInt(item.offset, 10))}
	}
	close(claim.messages)
	startedFirst := make(chan struct{})
	allowFirst := make(chan struct{})
	var once sync.Once
	consumer := &Consumer{conf: kafka.KafkaConsumerConf{Group: "sync-test"}, handlerConcurrency: 3,
		handler: func(_ context.Context, _ string, key string, _ []byte) error {
			if key == "a" {
				once.Do(func() { close(startedFirst) })
				<-allowFirst
			}
			return nil
		}}
	done := make(chan error, 1)
	go func() { done <- consumer.ConsumeClaim(session, claim) }()
	select {
	case <-startedFirst:
	case <-time.After(time.Second):
		t.Fatal("first key did not start")
	}
	select {
	case <-session.commitSignal:
		t.Fatal("a later completion was committed before the first offset")
	default:
	}
	close(allowFirst)
	for i := 0; i < 3; i++ {
		select {
		case <-session.commitSignal:
		case <-time.After(time.Second):
			t.Fatal("contiguous completion was not committed")
		}
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if len(session.commitMarks) != 3 || session.commitMarks[0] != 1 || session.commitMarks[1] != 2 || session.commitMarks[2] != 3 {
		t.Fatalf("commit marks=%v", session.commitMarks)
	}
}

func TestConsumerBatchesOnlyPolicyApprovedOffsets(t *testing.T) {
	session := &testSession{ctx: context.Background()}
	claim := &testClaim{messages: make(chan *sarama.ConsumerMessage, 4)}
	for offset, method := range []string{"safe", "safe", "unsafe", "safe"} {
		claim.messages <- &sarama.ConsumerMessage{Topic: "Sync-T", Partition: 3, Offset: int64(offset + 1),
			Headers: []*sarama.RecordHeader{{Key: []byte("method"), Value: []byte(method)}}}
	}
	close(claim.messages)
	consumer := &Consumer{conf: kafka.KafkaConsumerConf{Group: "sync-test"}, handler: func(context.Context, string, string, []byte) error {
		return nil
	}, canBatchCommit: func(topic, method string) bool { return topic == "Sync-T" && method == "safe" }}
	if err := consumer.ConsumeClaim(session, claim); err != nil {
		t.Fatal(err)
	}
	if session.commits != 2 || len(session.marked) != 4 || len(session.commitMarks) != 2 ||
		session.commitMarks[0] != 3 || session.commitMarks[1] != 4 {
		t.Fatalf("commit marks=%v commits=%d marked=%d", session.commitMarks, session.commits, len(session.marked))
	}
}

func TestConsumerFlushesSafeOffsetBatchWithinWindow(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	session := &testSession{ctx: ctx, commitSignal: make(chan struct{}, 1)}
	claim := &testClaim{messages: make(chan *sarama.ConsumerMessage, 1)}
	claim.messages <- &sarama.ConsumerMessage{Topic: "Sync-T", Partition: 2, Offset: 9,
		Headers: []*sarama.RecordHeader{{Key: []byte("method"), Value: []byte("safe")}}}
	consumer := &Consumer{conf: kafka.KafkaConsumerConf{Group: "sync-test"}, handler: func(context.Context, string, string, []byte) error {
		return nil
	}, canBatchCommit: func(topic, method string) bool { return topic == "Sync-T" && method == "safe" }}
	done := make(chan error, 1)
	go func() { done <- consumer.ConsumeClaim(session, claim) }()
	select {
	case <-session.commitSignal:
	case <-time.After(time.Second):
		t.Fatal("safe offset batch was not committed during the bounded window")
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("consumer did not stop after rebalance cancellation")
	}
	if session.commits != 1 || len(session.marked) != 1 {
		t.Fatalf("commits=%d marked=%d", session.commits, len(session.marked))
	}
}

func TestCommitMeasurementIncludesSynchronousSessionCommit(t *testing.T) {
	const delay = 15 * time.Millisecond
	session := &testSession{ctx: context.Background(), commitDelay: delay}
	message := &sarama.ConsumerMessage{Topic: "Sync-T", Offset: 1}
	started := time.Now()
	measured := commitMarkedMessage(session, message, "sync-test", "")
	if measured < delay || time.Since(started) < delay {
		t.Fatalf("measured commit duration=%v wall duration=%v, want at least %v", measured, time.Since(started), delay)
	}
	if len(session.marked) != 1 || session.commits != 1 || session.marked[0] != message {
		t.Fatalf("commit did not mark and synchronously commit the record: marked=%d commits=%d", len(session.marked), session.commits)
	}
}

func TestConsumerCancellationAfterHandlerSuccessDoesNotAck(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	session := &testSession{ctx: ctx}
	claim := &testClaim{messages: make(chan *sarama.ConsumerMessage, 1)}
	claim.messages <- &sarama.ConsumerMessage{Topic: "Sync-T", Offset: 1}
	consumer := &Consumer{handler: func(context.Context, string, string, []byte) error { cancel(); return nil }}
	if err := consumer.ConsumeClaim(session, claim); err != nil || len(session.marked) != 0 || session.commits != 0 {
		t.Fatalf("cancelled claim acknowledged, marked=%d commits=%d error=%v", len(session.marked), session.commits, err)
	}
}

func TestConsumerDeadLettersAfterBoundedFailures(t *testing.T) {
	session := &testSession{ctx: context.Background()}
	claim := &testClaim{messages: make(chan *sarama.ConsumerMessage, 1)}
	claim.messages <- &sarama.ConsumerMessage{Topic: "Sync-T", Partition: 2, Offset: 91, Key: []byte("user"), Value: []byte("event")}
	close(claim.messages)
	sink := new(testDeadLetterSink)
	calls := 0
	consumer := &Consumer{conf: kafka.KafkaConsumerConf{Group: "sync-test"}, maxHandlerAttempts: 3, handlerRetryDelay: time.Millisecond, deadLetterSink: sink,
		handler: func(context.Context, string, string, []byte) error {
			calls++
			return errors.New("permanent database failure")
		}}
	if err := consumer.ConsumeClaim(session, claim); err != nil {
		t.Fatal(err)
	}
	if calls != 3 || len(sink.values) != 1 || len(session.marked) != 1 || session.commits != 1 {
		t.Fatalf("dead-letter handling calls=%d letters=%d marked=%d commits=%d", calls, len(sink.values), len(session.marked), session.commits)
	}
	letter := sink.values[0]
	if letter.Topic != "Sync-T" || letter.Partition != 2 || letter.Offset != 91 || letter.Attempts != 3 || letter.ValueSHA256 == "" || letter.Error == "" {
		t.Fatalf("dead-letter metadata=%+v", letter)
	}
}

func TestConsumerDefaultRetryDelayIsBounded(t *testing.T) {
	consumer := &Consumer{}
	if consumer.handlerRetryDelay != 0 {
		t.Fatal("zero-value test consumer should be configured by ConsumeClaim")
	}
	if defaultHandlerRetryDelay != time.Second {
		t.Fatalf("default retry delay=%v", defaultHandlerRetryDelay)
	}
}

func TestConsumerStopClosesDeadLetterSink(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	sink := new(testDeadLetterSink)
	consumer := &Consumer{ctx: ctx, cancel: cancel, deadLetterSink: sink}
	consumer.Stop()
	if !sink.closed || !errors.Is(ctx.Err(), context.Canceled) {
		t.Fatalf("consumer stop closed sink=%v contextErr=%v", sink.closed, ctx.Err())
	}
}
