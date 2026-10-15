package mqproducer

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/IBM/sarama"
	kafka "github.com/teamgram/marmota/pkg/mq"
	"github.com/zeromicro/go-zero/core/logx"
	"github.com/zeromicro/go-zero/core/proc"
	ztrace "github.com/zeromicro/go-zero/core/trace"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc/metadata"
)

const (
	operationTimeout = 30 * time.Second
	maxOutstanding   = 64
)

var ErrClosed = errors.New("Kafka producer is closing")

type Sender interface {
	SendMessageV2(context.Context, string, string, []byte) (int32, int64, error)
}

type asyncProducer interface {
	Input() chan<- *sarama.ProducerMessage
	Successes() <-chan *sarama.ProducerMessage
	Errors() <-chan *sarama.ProducerError
	AsyncClose()
}

type publicationResult struct {
	partition int32
	offset    int64
	err       error
}

// Producer owns its Kafka client. A caller timeout leaves an accepted record
// in flight; only its final broker result releases the outstanding capacity.
type Producer struct {
	topic     string
	client    asyncProducer
	slots     chan struct{}
	stopping  chan struct{}
	done      chan struct{}
	gate      sync.Mutex
	enqueuing sync.WaitGroup
	closeOnce sync.Once
}

func buildConfig(c *kafka.KafkaProducerConf) (*sarama.Config, error) {
	if c == nil || strings.TrimSpace(c.Topic) == "" || len(c.Brokers) == 0 {
		return nil, errors.New("Kafka producer requires a topic and brokers")
	}
	for _, broker := range c.Brokers {
		if strings.TrimSpace(broker) == "" {
			return nil, errors.New("Kafka producer requires nonempty brokers")
		}
	}
	conf, err := kafka.BuildProducerConfig(*c)
	if err != nil {
		return nil, errors.New("Kafka producer configuration is invalid")
	}
	conf.Producer.RequiredAcks = sarama.WaitForAll
	conf.Producer.Partitioner = sarama.NewHashPartitioner
	conf.Net.MaxOpenRequests = 1
	conf.Net.DialTimeout = 5 * time.Second
	conf.Net.ReadTimeout = 5 * time.Second
	conf.Net.WriteTimeout = 5 * time.Second
	conf.Metadata.Timeout = 5 * time.Second
	conf.Metadata.Retry.Max = 2
	conf.Producer.Timeout = 5 * time.Second
	conf.Producer.Retry.Max = 2
	conf.ChannelBufferSize = maxOutstanding
	return conf, conf.Validate()
}

// New creates an independent client, including when another producer uses the
// same topic. No process-wide topic cache can mix broker or credential sets.
func New(c *kafka.KafkaProducerConf) (*Producer, error) {
	conf, err := buildConfig(c)
	if err != nil {
		return nil, err
	}
	client, err := sarama.NewAsyncProducer(c.Brokers, conf)
	if err != nil {
		return nil, err
	}
	p := newProducer(c.Topic, client)
	proc.AddShutdownListener(func() {
		if err := p.Close(); err != nil {
			logx.Errorf("Kafka producer shutdown failed, topic=%s: %v", p.topic, err)
		}
	})
	return p, nil
}

func MustNew(c *kafka.KafkaProducerConf) *Producer {
	p, err := New(c)
	if err != nil {
		panic(err)
	}
	return p
}

func newProducer(topic string, client asyncProducer) *Producer {
	p := &Producer{
		topic: topic, client: client, slots: make(chan struct{}, maxOutstanding),
		stopping: make(chan struct{}), done: make(chan struct{}),
	}
	go p.drain()
	return p
}

func (p *Producer) Topic() string { return p.topic }

func (p *Producer) SendMessageV2(ctx context.Context, method, key string, value []byte) (partition int32, offset int64, err error) {
	partition, offset = -1, -1
	started := time.Now()
	ctx, cancel := context.WithTimeout(ctx, operationTimeout)
	defer cancel()
	ctx, span := otel.GetTracerProvider().Tracer(ztrace.TraceName).Start(ctx, "kafka.producer", trace.WithSpanKind(trace.SpanKindClient))
	span.SetAttributes(attribute.String("kafka.method", method))
	defer func() {
		result := callResult(err)
		producerCalls.WithLabelValues(p.topic, result).Inc()
		producerCallDuration.WithLabelValues(p.topic, result).Observe(time.Since(started).Seconds())
		if err != nil {
			span.SetStatus(codes.Error, err.Error())
			span.RecordError(err)
		} else {
			span.SetStatus(codes.Ok, "")
		}
		span.End()
	}()
	if err = ctx.Err(); err != nil {
		return
	}
	if method == "" || key == "" || len(value) == 0 {
		err = errors.New("Kafka publication requires a method, key, and value")
		return
	}
	select {
	case <-p.stopping:
		return -1, -1, ErrClosed
	case <-ctx.Done():
		return -1, -1, ctx.Err()
	case p.slots <- struct{}{}:
	}
	producerOutstanding.WithLabelValues(p.topic).Inc()
	result := make(chan publicationResult, 1)
	message := &sarama.ProducerMessage{
		Topic: p.topic, Key: sarama.StringEncoder(key), Value: sarama.ByteEncoder(append([]byte(nil), value...)),
		Headers: messageHeaders(ctx, method), Metadata: result,
	}
	if err = p.enqueue(ctx, message); err != nil {
		p.release()
		return
	}
	select {
	case receipt := <-result:
		return receipt.partition, receipt.offset, receipt.err
	case <-ctx.Done():
		return -1, -1, ctx.Err()
	}
}

func messageHeaders(ctx context.Context, method string) []sarama.RecordHeader {
	carrier := propagation.MapCarrier{}
	if md, ok := metadata.FromOutgoingContext(ctx); ok {
		for key, values := range md {
			if len(values) != 0 && key != "method" {
				carrier[key] = values[0]
			}
		}
	}
	otel.GetTextMapPropagator().Inject(ctx, carrier)
	headers := []sarama.RecordHeader{{Key: []byte("method"), Value: []byte(method)}}
	for key, value := range carrier {
		if key != "method" {
			headers = append(headers, sarama.RecordHeader{Key: []byte(key), Value: []byte(value)})
		}
	}
	return headers
}

func (p *Producer) enqueue(ctx context.Context, message *sarama.ProducerMessage) error {
	p.gate.Lock()
	select {
	case <-p.stopping:
		p.gate.Unlock()
		return ErrClosed
	default:
		p.enqueuing.Add(1)
	}
	p.gate.Unlock()
	defer p.enqueuing.Done()
	select {
	case <-p.stopping:
		return ErrClosed
	case <-ctx.Done():
		return ctx.Err()
	case p.client.Input() <- message:
		return nil
	}
}

func (p *Producer) release() {
	<-p.slots
	producerOutstanding.WithLabelValues(p.topic).Dec()
}

func (p *Producer) drain() {
	defer close(p.done)
	successes, failures := p.client.Successes(), p.client.Errors()
	for successes != nil || failures != nil {
		select {
		case message, ok := <-successes:
			if !ok {
				successes = nil
				continue
			}
			message.Metadata.(chan publicationResult) <- publicationResult{partition: message.Partition, offset: message.Offset}
			producerDeliveryResults.WithLabelValues(p.topic, "success").Inc()
			p.release()
		case failure, ok := <-failures:
			if !ok {
				failures = nil
				continue
			}
			failure.Msg.Metadata.(chan publicationResult) <- publicationResult{partition: -1, offset: -1, err: failure.Err}
			producerDeliveryResults.WithLabelValues(p.topic, "error").Inc()
			p.release()
		}
	}
}

func (p *Producer) Close() error {
	ctx, cancel := context.WithTimeout(context.Background(), operationTimeout)
	defer cancel()
	return p.CloseContext(ctx)
}

// CloseAll gives a service's independent producers one shared shutdown budget.
func CloseAll(producers ...*Producer) error {
	ctx, cancel := context.WithTimeout(context.Background(), operationTimeout)
	defer cancel()
	return closeAll(ctx, producers...)
}

func closeAll(ctx context.Context, producers ...*Producer) error {
	results := make(chan error, len(producers))
	for _, producer := range producers {
		go func(p *Producer) { results <- p.CloseContext(ctx) }(producer)
	}
	var err error
	for range producers {
		err = errors.Join(err, <-results)
	}
	return err
}

// CloseContext stops new sends before draining accepted records. A timeout
// bounds the caller's wait; the result drainer keeps running until Kafka closes.
func (p *Producer) CloseContext(ctx context.Context) error {
	p.closeOnce.Do(func() {
		p.gate.Lock()
		close(p.stopping)
		p.gate.Unlock()
		go func() {
			p.enqueuing.Wait()
			p.client.AsyncClose()
		}()
	})
	select {
	case <-p.done:
		return nil
	default:
	}
	select {
	case <-p.done:
		return nil
	case <-ctx.Done():
		producerShutdownTimeouts.WithLabelValues(p.topic).Inc()
		return ctx.Err()
	}
}
