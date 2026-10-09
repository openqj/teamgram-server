package mqconsumer

import (
	"context"
	"errors"
	"testing"

	"github.com/IBM/sarama"
	kafka "github.com/teamgram/marmota/pkg/mq"
)

type testSession struct {
	sarama.ConsumerGroupSession
	ctx     context.Context
	marked  []*sarama.ConsumerMessage
	commits int
}

func (s *testSession) Context() context.Context { return s.ctx }
func (s *testSession) MarkMessage(message *sarama.ConsumerMessage, _ string) {
	s.marked = append(s.marked, message)
}
func (s *testSession) Commit() { s.commits++ }

type testClaim struct {
	sarama.ConsumerGroupClaim
	messages chan *sarama.ConsumerMessage
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
