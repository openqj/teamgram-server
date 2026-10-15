package mqconsumer

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/IBM/sarama"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	kafka "github.com/teamgram/marmota/pkg/mq"
	"github.com/teamgram/teamgram-server/pkg/mqproducer"
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

type DeadLetter struct {
	ConsumerGroup string `json:"consumerGroup"`
	Topic         string `json:"topic"`
	Partition     int32  `json:"partition"`
	Offset        int64  `json:"offset"`
	Method        string `json:"method"`
	Key           string `json:"key"`
	Value         []byte `json:"value"`
	ValueSHA256   string `json:"valueSha256"`
	Attempts      int    `json:"attempts"`
	Error         string `json:"error"`
	FailedAt      string `json:"failedAt"`
}

type DeadLetterSink interface {
	Publish(context.Context, DeadLetter) error
}

type kafkaDeadLetterSink struct {
	producer *mqproducer.Producer
}

func (s kafkaDeadLetterSink) Publish(ctx context.Context, value DeadLetter) error {
	payload, err := json.Marshal(value)
	if err != nil {
		return err
	}
	_, _, err = s.producer.SendMessageV2(ctx, "dead_letter", value.Key, payload)
	return err
}

func (s kafkaDeadLetterSink) Close() error {
	return s.producer.Close()
}

var (
	mqconsumerHandlerRetries = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "mqconsumer_handler_retries_total", Help: "Kafka handler retry attempts before acknowledgement or dead-lettering.",
	}, []string{"group", "topic", "partition"})
	mqconsumerDeadLetters = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "mqconsumer_dead_letter_total", Help: "Kafka records successfully published to a dead-letter topic.",
	}, []string{"group", "topic"})
	mqconsumerOldestUncommitted = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "mqconsumer_oldest_uncommitted_seconds", Help: "Age of the record currently blocking a Kafka partition.",
	}, []string{"group", "topic", "partition"})
	mqconsumerRecordAge = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name: "mqconsumer_record_age_seconds", Help: "Age of a Kafka record at the start of handler processing.",
		Buckets: prometheus.ExponentialBuckets(0.001, 2, 16),
	}, []string{"group", "topic", "partition"})
	mqconsumerOffsetCommitDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name: "mqconsumer_offset_commit_seconds", Help: "Synchronous Kafka consumer offset commit duration.",
		Buckets: prometheus.ExponentialBuckets(0.001, 2, 16),
	}, []string{"group", "topic", "partition"})
)

const (
	defaultMaxHandlerAttempts     = 10
	defaultHandlerRetryDelay      = time.Second
	commitBatchWindow             = 250 * time.Millisecond
	commitBatchMaxMessages        = 8
	defaultSafeHandlerConcurrency = 8
	maxHandlerConcurrency         = 1024
	maxHandlerInFlightMultiplier  = 4
)

type Consumer struct {
	group              sarama.ConsumerGroup
	conf               kafka.KafkaConsumerConf
	handler            Handler
	ctx                context.Context
	cancel             context.CancelFunc
	stopOnce           sync.Once
	maxHandlerAttempts int
	handlerRetryDelay  time.Duration
	deadLetterSink     DeadLetterSink
	canBatchCommit     func(topic, method string) bool
	handlerConcurrency int
}

func New(conf kafka.KafkaConsumerConf, handler Handler) (*Consumer, error) {
	return newConsumer(conf, handler, nil, 1)
}

// NewWithSafeCommitBatch batches offsets only for records the handler can
// replay without repeating side effects.
func NewWithSafeCommitBatch(conf kafka.KafkaConsumerConf, handler Handler,
	canBatchCommit func(topic, method string) bool) (*Consumer, error) {
	return NewWithSafeCommitBatchAndConcurrency(conf, handler, canBatchCommit,
		configuredHandlerConcurrency())
}

// NewWithSafeCommitBatchAndConcurrency enables bounded, per-key ordered
// processing for handlers whose side effects are protected by an idempotent
// receipt. Records with the same Kafka key remain strictly ordered; different
// keys may run concurrently.
func NewWithSafeCommitBatchAndConcurrency(conf kafka.KafkaConsumerConf, handler Handler,
	canBatchCommit func(topic, method string) bool, concurrency int) (*Consumer, error) {
	if canBatchCommit == nil {
		return nil, errors.New("Kafka safe commit batching requires a policy")
	}
	return newConsumer(conf, handler, canBatchCommit, concurrency)
}

func newConsumer(conf kafka.KafkaConsumerConf, handler Handler,
	canBatchCommit func(topic, method string) bool, handlerConcurrency int) (*Consumer, error) {
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
	maxAttempts := envInt("TEAMGRAM_KAFKA_MAX_HANDLER_ATTEMPTS", defaultMaxHandlerAttempts)
	if maxAttempts < 1 || maxAttempts > 1000 {
		maxAttempts = defaultMaxHandlerAttempts
	}
	retryDelayMillis := envInt("TEAMGRAM_KAFKA_RETRY_DELAY_MS", 1000)
	if retryDelayMillis < 1 || retryDelayMillis > 60000 {
		retryDelayMillis = int(defaultHandlerRetryDelay / time.Millisecond)
	}
	consumer := &Consumer{group: group, conf: conf, handler: handler, ctx: ctx, cancel: cancel,
		maxHandlerAttempts: maxAttempts, handlerRetryDelay: time.Duration(retryDelayMillis) * time.Millisecond,
		canBatchCommit: canBatchCommit, handlerConcurrency: normalizeHandlerConcurrency(handlerConcurrency)}
	if topic := os.Getenv("TEAMGRAM_KAFKA_DLQ_TOPIC"); topic != "" {
		producer, err := mqproducer.New(&kafka.KafkaProducerConf{
			Username: conf.Username, Password: conf.Password, TLS: conf.TLS, Brokers: conf.Brokers, Topic: topic,
		})
		if err != nil {
			group.Close()
			cancel()
			return nil, fmt.Errorf("Kafka dead-letter producer: %w", err)
		}
		consumer.deadLetterSink = kafkaDeadLetterSink{producer: producer}
	}
	return consumer, nil
}

func envInt(name string, fallback int) int {
	value, err := strconv.Atoi(os.Getenv(name))
	if err != nil {
		return fallback
	}
	return value
}

func normalizeHandlerConcurrency(value int) int {
	if value < 1 {
		return 1
	}
	if value > maxHandlerConcurrency {
		return maxHandlerConcurrency
	}
	return value
}

func configuredHandlerConcurrency() int {
	value := envInt("TEAMGRAM_KAFKA_HANDLER_CONCURRENCY", defaultSafeHandlerConcurrency)
	if value < 1 || value > maxHandlerConcurrency {
		return defaultSafeHandlerConcurrency
	}
	return value
}

func (*Consumer) Setup(sarama.ConsumerGroupSession) error   { return nil }
func (*Consumer) Cleanup(sarama.ConsumerGroupSession) error { return nil }

func commitMarkedOffset(session sarama.ConsumerGroupSession, topic string, partition int32, group string) time.Duration {
	started := time.Now()
	session.Commit()
	duration := time.Since(started)
	partitionLabel := strconv.FormatInt(int64(partition), 10)
	mqconsumerOffsetCommitDuration.WithLabelValues(group, topic, partitionLabel).Observe(duration.Seconds())
	return duration
}

func commitMarkedMessage(session sarama.ConsumerGroupSession, message *sarama.ConsumerMessage, group, metadata string) time.Duration {
	session.MarkMessage(message, metadata)
	return commitMarkedOffset(session, message.Topic, message.Partition, group)
}

type deliveryJob struct {
	seq     uint64
	key     string
	ctx     context.Context
	method  string
	message *sarama.ConsumerMessage
}

type deliveryResult struct {
	job deliveryJob
	ack bool
	err error
}

type deliveryKeyQueue struct {
	jobs    []deliveryJob
	running bool
}

// deliveryExecutor keeps one active handler per Kafka key and bounds the
// number of different keys being processed at once. This preserves update
// order for one user while allowing independent users to make progress.
type deliveryExecutor struct {
	consumer *Consumer
	ctx      context.Context
	cancel   context.CancelFunc
	stop     chan struct{}
	stopOnce sync.Once
	ready    chan deliveryJob
	results  chan deliveryResult
	slots    chan struct{}
	mu       sync.Mutex
	queues   map[string]*deliveryKeyQueue
	workers  sync.WaitGroup
}

func newDeliveryExecutor(consumer *Consumer, concurrency int, parent context.Context) *deliveryExecutor {
	concurrency = normalizeHandlerConcurrency(concurrency)
	inFlight := concurrency * maxHandlerInFlightMultiplier
	if inFlight < concurrency {
		inFlight = concurrency
	}
	ctx, cancel := context.WithCancel(parent)
	executor := &deliveryExecutor{
		consumer: consumer,
		ctx:      ctx,
		cancel:   cancel,
		stop:     make(chan struct{}),
		ready:    make(chan deliveryJob, inFlight),
		results:  make(chan deliveryResult, inFlight),
		slots:    make(chan struct{}, inFlight),
		queues:   make(map[string]*deliveryKeyQueue),
	}
	for i := 0; i < inFlight; i++ {
		executor.slots <- struct{}{}
	}
	for i := 0; i < concurrency; i++ {
		executor.workers.Add(1)
		go executor.worker()
	}
	return executor
}

func (e *deliveryExecutor) worker() {
	defer e.workers.Done()
	for {
		select {
		case <-e.stop:
			return
		case job := <-e.ready:
			select {
			case <-e.stop:
				return
			default:
			}
			result := e.consumer.processDelivery(job, e.ctx)
			select {
			case e.results <- result:
			case <-e.stop:
				e.release()
				return
			}
			e.scheduleNext(job.key)
		}
	}
}

func (e *deliveryExecutor) release() {
	e.slots <- struct{}{}
}

func (e *deliveryExecutor) submit(job deliveryJob) bool {
	select {
	case <-e.stop:
		return false
	case <-e.slots:
	}

	e.mu.Lock()
	select {
	case <-e.stop:
		e.mu.Unlock()
		e.release()
		return false
	default:
	}
	queue := e.queues[job.key]
	if queue == nil {
		queue = &deliveryKeyQueue{}
		e.queues[job.key] = queue
	}
	queue.jobs = append(queue.jobs, job)
	var first deliveryJob
	ready := false
	if !queue.running {
		queue.running = true
		first = queue.jobs[0]
		queue.jobs = queue.jobs[1:]
		ready = true
	}
	e.mu.Unlock()
	if ready {
		select {
		case e.ready <- first:
		case <-e.stop:
			return false
		}
	}
	return true
}

func (e *deliveryExecutor) scheduleNext(key string) {
	e.mu.Lock()
	queue := e.queues[key]
	if queue == nil || len(queue.jobs) == 0 {
		if queue != nil {
			queue.running = false
			delete(e.queues, key)
		}
		e.mu.Unlock()
		return
	}
	next := queue.jobs[0]
	queue.jobs = queue.jobs[1:]
	e.mu.Unlock()
	select {
	case e.ready <- next:
	case <-e.stop:
	}
}

func (e *deliveryExecutor) stopAndWait() {
	e.stopOnce.Do(func() {
		e.cancel()
		close(e.stop)
	})
	e.workers.Wait()
}

// processDelivery retries a single record without acknowledging it until the
// handler succeeds or a configured dead-letter sink durably accepts it.
func (c *Consumer) processDelivery(job deliveryJob, executorCtx context.Context) deliveryResult {
	handlerCtx, cancel := context.WithCancel(job.ctx)
	stop := context.AfterFunc(executorCtx, cancel)
	defer stop()
	defer cancel()
	partitionLabel := strconv.FormatInt(int64(job.message.Partition), 10)
	maxAttempts := c.maxHandlerAttempts
	if maxAttempts < 1 || maxAttempts > 1000 {
		maxAttempts = defaultMaxHandlerAttempts
	}
	retryDelay := c.handlerRetryDelay
	if retryDelay <= 0 || retryDelay > time.Minute {
		retryDelay = defaultHandlerRetryDelay
	}
	started := time.Now()
	mqconsumerOldestUncommitted.WithLabelValues(c.conf.Group, job.message.Topic, partitionLabel).Set(0)
	for attempt := 1; ; attempt++ {
		if err := handlerCtx.Err(); err != nil {
			return deliveryResult{job: job}
		}
		mqconsumerOldestUncommitted.WithLabelValues(c.conf.Group, job.message.Topic, partitionLabel).Set(time.Since(started).Seconds())
		err := c.handler(handlerCtx, job.method, string(job.message.Key), job.message.Value)
		if err == nil {
			if handlerCtx.Err() != nil {
				return deliveryResult{job: job}
			}
			mqconsumerOldestUncommitted.WithLabelValues(c.conf.Group, job.message.Topic, partitionLabel).Set(0)
			return deliveryResult{job: job, ack: true}
		}
		if attempt >= maxAttempts {
			if c.deadLetterSink == nil {
				logx.WithContext(handlerCtx).Errorf("Kafka handler reached max attempts but no dead-letter sink is configured, group=%s topic=%s partition=%d offset=%d", c.conf.Group, job.message.Topic, job.message.Partition, job.message.Offset)
				return deliveryResult{job: job, err: fmt.Errorf("kafka handler max attempts reached without dead-letter sink")}
			}
			sum := sha256.Sum256(job.message.Value)
			deadLetter := DeadLetter{ConsumerGroup: c.conf.Group, Topic: job.message.Topic, Partition: job.message.Partition, Offset: job.message.Offset,
				Method: job.method, Key: string(job.message.Key), Value: append([]byte(nil), job.message.Value...), ValueSHA256: hex.EncodeToString(sum[:]),
				Attempts: attempt, Error: err.Error(), FailedAt: time.Now().UTC().Format(time.RFC3339Nano)}
			if publishErr := c.deadLetterSink.Publish(handlerCtx, deadLetter); publishErr != nil {
				logx.WithContext(handlerCtx).Errorf("Kafka dead-letter publish failed, group=%s topic=%s partition=%d offset=%d: %v", c.conf.Group, job.message.Topic, job.message.Partition, job.message.Offset, publishErr)
				return deliveryResult{job: job, err: publishErr}
			}
			mqconsumerDeadLetters.WithLabelValues(c.conf.Group, job.message.Topic).Inc()
			mqconsumerOldestUncommitted.WithLabelValues(c.conf.Group, job.message.Topic, partitionLabel).Set(0)
			return deliveryResult{job: job, ack: true}
		}
		if handlerCtx.Err() != nil {
			return deliveryResult{job: job}
		}
		mqconsumerHandlerRetries.WithLabelValues(c.conf.Group, job.message.Topic, partitionLabel).Inc()
		logx.WithContext(handlerCtx).Errorf("Kafka handler failed, group=%s topic=%s partition=%d offset=%d: %v",
			c.conf.Group, job.message.Topic, job.message.Partition, job.message.Offset, err)
		timer := time.NewTimer(retryDelay)
		select {
		case <-handlerCtx.Done():
			timer.Stop()
			return deliveryResult{job: job}
		case <-timer.C:
		}
	}
}

// A failed handler keeps its offset unacknowledged. Completions are committed
// only in input order, while independent keys may be handled concurrently.
func (c *Consumer) ConsumeClaim(session sarama.ConsumerGroupSession, claim sarama.ConsumerGroupClaim) error {
	concurrency := c.handlerConcurrency
	if concurrency < 1 {
		concurrency = 1
	}
	executor := newDeliveryExecutor(c, concurrency, session.Context())
	defer executor.stopAndWait()

	inputDone := make(chan uint64, 1)
	go func() {
		var seq uint64
		defer func() { inputDone <- seq }()
		for {
			select {
			case <-session.Context().Done():
				return
			case message, ok := <-claim.Messages():
				if !ok {
					return
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
				partitionLabel := strconv.FormatInt(int64(message.Partition), 10)
				if !message.Timestamp.IsZero() {
					if age := time.Since(message.Timestamp); age >= 0 {
						mqconsumerRecordAge.WithLabelValues(c.conf.Group, message.Topic, partitionLabel).Observe(age.Seconds())
					}
				}
				job := deliveryJob{seq: seq, key: string(message.Key), ctx: ctx, method: method, message: message}
				if !executor.submit(job) {
					return
				}
				seq++
			}
		}
	}()

	var (
		batchTick    <-chan time.Time
		ticker       *time.Ticker
		pendingBatch *sarama.ConsumerMessage
		pendingCount int
		inputClosed  bool
		submittedN   uint64
		completedN   uint64
		nextSeq      uint64
		completed    = make(map[uint64]deliveryResult)
	)
	if c.canBatchCommit != nil {
		ticker = time.NewTicker(commitBatchWindow)
		batchTick = ticker.C
		defer ticker.Stop()
	}
	flushBatch := func() {
		if pendingBatch == nil || session.Context().Err() != nil {
			return
		}
		commitMarkedOffset(session, pendingBatch.Topic, pendingBatch.Partition, c.conf.Group)
		pendingBatch = nil
		pendingCount = 0
	}
	commitResult := func(result deliveryResult) error {
		if session.Context().Err() != nil {
			return nil
		}
		if !result.ack {
			return result.err
		}
		safe := c.canBatchCommit != nil && c.canBatchCommit(result.job.message.Topic, result.job.method)
		session.MarkMessage(result.job.message, "")
		if safe {
			pendingBatch = result.job.message
			pendingCount++
			if pendingCount >= commitBatchMaxMessages {
				flushBatch()
			}
		} else {
			commitMarkedOffset(session, result.job.message.Topic, result.job.message.Partition, c.conf.Group)
			pendingBatch = nil
			pendingCount = 0
		}
		return nil
	}

	for {
		if inputClosed && completedN == submittedN {
			flushBatch()
			return nil
		}
		select {
		case <-session.Context().Done():
			return nil
		case total := <-inputDone:
			inputClosed = true
			submittedN = total
			inputDone = nil
		case <-batchTick:
			flushBatch()
		case result := <-executor.results:
			completedN++
			executor.release()
			completed[result.job.seq] = result
			for {
				item, ok := completed[nextSeq]
				if !ok {
					break
				}
				delete(completed, nextSeq)
				nextSeq++
				if err := commitResult(item); err != nil {
					return err
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
		if c.group != nil {
			if err := c.group.Close(); err != nil && !errors.Is(err, sarama.ErrClosedConsumerGroup) {
				logx.Errorf("Kafka consumer close failed, group=%s: %v", c.conf.Group, err)
			}
		}
		if sink, ok := c.deadLetterSink.(interface{ Close() error }); ok {
			if err := sink.Close(); err != nil {
				logx.Errorf("Kafka dead-letter producer close failed, group=%s: %v", c.conf.Group, err)
			}
		}
	})
}
