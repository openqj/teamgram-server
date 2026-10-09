package mqconsumer

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/IBM/sarama"
	kafka "github.com/teamgram/marmota/pkg/mq"
	"github.com/zeromicro/go-zero/core/logx"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
)

type Metadata struct {
	ConsumerGroup string
	Topic         string
	Partition     int32
	Offset        int64
}

type metadataKey struct{}

func WithMetadata(ctx context.Context, value Metadata) context.Context {
	return context.WithValue(ctx, metadataKey{}, value)
}

func MetadataFromContext(ctx context.Context) (Metadata, bool) {
	value, ok := ctx.Value(metadataKey{}).(Metadata)
	return value, ok
}

type Handler func(context.Context, string, string, []byte) error

type Consumer struct {
	group    sarama.ConsumerGroup
	conf     kafka.KafkaConsumerConf
	handler  Handler
	ctx      context.Context
	cancel   context.CancelFunc
	stopOnce sync.Once
}

func New(conf kafka.KafkaConsumerConf, handler Handler) (*Consumer, error) {
	if conf.Group == "" || len(conf.Topics) == 0 || len(conf.Brokers) == 0 || handler == nil {
		return nil, errors.New("Kafka consumer requires a group, topics, brokers, and handler")
	}
	config, err := kafka.BuildConsumerGroupConfig(&conf, sarama.OffsetOldest, false)
	if err != nil {
		return nil, err
	}
	group, err := sarama.NewConsumerGroup(conf.Brokers, conf.Group, config)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &Consumer{group: group, conf: conf, handler: handler, ctx: ctx, cancel: cancel}, nil
}

func (*Consumer) Setup(sarama.ConsumerGroupSession) error   { return nil }
func (*Consumer) Cleanup(sarama.ConsumerGroupSession) error { return nil }

// A failed handler keeps its offset unacknowledged and blocks later messages
// in this partition. Rebalance cancellation leaves the same record retryable.
func (c *Consumer) ConsumeClaim(session sarama.ConsumerGroupSession, claim sarama.ConsumerGroupClaim) error {
	for {
		select {
		case <-session.Context().Done():
			return nil
		case message, ok := <-claim.Messages():
			if !ok {
				return nil
			}
			carrier := propagation.MapCarrier{}
			method := ""
			for _, header := range message.Headers {
				if header == nil {
					continue
				}
				carrier[string(header.Key)] = string(header.Value)
				if string(header.Key) == "method" {
					method = string(header.Value)
				}
			}
			ctx := otel.GetTextMapPropagator().Extract(session.Context(), carrier)
			ctx = WithMetadata(ctx, Metadata{
				ConsumerGroup: c.conf.Group, Topic: message.Topic, Partition: message.Partition, Offset: message.Offset,
			})
			for {
				if err := ctx.Err(); err != nil {
					return nil
				}
				err := c.handler(ctx, method, string(message.Key), message.Value)
				if err == nil {
					if ctx.Err() != nil {
						return nil
					}
					session.MarkMessage(message, "")
					session.Commit()
					break
				}
				logx.WithContext(ctx).Errorf("Kafka handler failed, group=%s topic=%s partition=%d offset=%d: %v",
					c.conf.Group, message.Topic, message.Partition, message.Offset, err)
				timer := time.NewTimer(time.Second)
				select {
				case <-ctx.Done():
					timer.Stop()
					return nil
				case <-timer.C:
				}
			}
		}
	}
}

func (c *Consumer) Start() {
	for c.ctx.Err() == nil {
		err := c.group.Consume(c.ctx, c.conf.Topics, c)
		if errors.Is(err, sarama.ErrClosedConsumerGroup) || c.ctx.Err() != nil {
			return
		}
		if err != nil {
			logx.Errorf("Kafka consume failed, group=%s: %v", c.conf.Group, err)
			timer := time.NewTimer(time.Second)
			select {
			case <-c.ctx.Done():
				timer.Stop()
				return
			case <-timer.C:
			}
		}
	}
}

func (c *Consumer) Stop() {
	c.stopOnce.Do(func() {
		c.cancel()
		if err := c.group.Close(); err != nil && !errors.Is(err, sarama.ErrClosedConsumerGroup) {
			logx.Errorf("Kafka consumer close failed, group=%s: %v", c.conf.Group, err)
		}
	})
}
