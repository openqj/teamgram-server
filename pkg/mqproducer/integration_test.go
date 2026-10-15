package mqproducer

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/IBM/sarama"
	kafka "github.com/teamgram/marmota/pkg/mq"
	"google.golang.org/grpc/metadata"
)

func TestProducerKafkaIntegration(t *testing.T) {
	broker := os.Getenv("RELIABILITY_KAFKA_BROKER")
	if broker == "" {
		t.Skip("RELIABILITY_KAFKA_BROKER must point to an isolated Kafka broker")
	}
	const topic = "reliability-producer-acceptance"
	c := &kafka.KafkaProducerConf{Topic: topic, Brokers: strings.Split(broker, ",")}
	p, err := New(c)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := p.Close(); err != nil {
			t.Errorf("Kafka close: %v", err)
		}
	})
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	marker := fmt.Sprintf("probe-%d", time.Now().UnixNano())
	ctx = metadata.NewOutgoingContext(ctx, metadata.Pairs("x-reliability-probe", marker))
	partition, first, err := p.SendMessageV2(ctx, "reliability.acceptance", marker, []byte(marker+"-1"))
	if err != nil {
		t.Fatal(err)
	}
	secondPartition, second, err := p.SendMessageV2(ctx, "reliability.acceptance", marker, []byte(marker+"-2"))
	if err != nil || partition < 0 || partition != secondPartition || second <= first {
		t.Fatalf("Kafka ordering: partition=%d/%d offset=%d/%d err=%v", partition, secondPartition, first, second, err)
	}
	conf, err := buildConfig(c)
	if err != nil {
		t.Fatal(err)
	}
	consumer, err := sarama.NewConsumer(c.Brokers, conf)
	if err != nil {
		t.Fatal(err)
	}
	defer consumer.Close()
	stream, err := consumer.ConsumePartition(topic, partition, first)
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
	for _, offset := range []int64{first, second} {
		select {
		case message := <-stream.Messages():
			if message.Offset != offset || string(message.Key) != marker || !strings.HasPrefix(string(message.Value), marker) {
				t.Fatalf("incompatible Kafka record at offset %d", message.Offset)
			}
			headers := make(map[string]string)
			for _, header := range message.Headers {
				headers[string(header.Key)] = string(header.Value)
			}
			if headers["method"] != "reliability.acceptance" || headers["x-reliability-probe"] != marker {
				t.Fatalf("Kafka record lost method or business metadata")
			}
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
	}
	t.Logf("Kafka acknowledged and consumed two ordered records, partition=%d offsets=%d,%d", partition, first, second)
}
