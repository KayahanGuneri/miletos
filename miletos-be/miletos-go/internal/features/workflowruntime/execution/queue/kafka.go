// Package queue owns asynchronous execution transport.
package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"
)

const (
	partitionRetryInitialBackoff = 250 * time.Millisecond
	partitionRetryMaximumBackoff = 30 * time.Second
	commitInterval               = time.Second
	shutdownCommitTimeout        = 5 * time.Second
)

type Kafka struct {
	producer        *kgo.Client
	brokers         []string
	clientID        string
	consumerGroup   string
	concurrency     int
	deadLetterTopic string
}

func NewKafka(brokers []string, clientID, consumerGroup string) (*Kafka, error) {
	return NewKafkaWithConcurrency(brokers, clientID, consumerGroup, 2)
}

func NewKafkaWithConcurrency(
	brokers []string,
	clientID string,
	consumerGroup string,
	concurrency int,
	deadLetterTopics ...string,
) (*Kafka, error) {
	if len(brokers) == 0 {
		return nil, fmt.Errorf("Kafka brokers are required")
	}
	if strings.TrimSpace(clientID) == "" {
		return nil, fmt.Errorf("Kafka client ID is required")
	}
	if strings.TrimSpace(consumerGroup) == "" {
		return nil, fmt.Errorf("Kafka consumer group is required")
	}
	if concurrency < 2 || concurrency > 5 {
		return nil, fmt.Errorf("Kafka node concurrency must be between 2 and 5")
	}
	deadLetterTopic := "miletos.workflow.node.dead-letter.v1"
	if len(deadLetterTopics) > 0 && strings.TrimSpace(deadLetterTopics[0]) != "" {
		deadLetterTopic = strings.TrimSpace(deadLetterTopics[0])
	}
	producer, err := kgo.NewClient(
		kgo.SeedBrokers(brokers...),
		kgo.ClientID(clientID),
		kgo.RequiredAcks(kgo.AllISRAcks()),
	)
	if err != nil {
		return nil, fmt.Errorf("create Kafka producer: %w", err)
	}
	return &Kafka{
		producer:        producer,
		brokers:         append([]string(nil), brokers...),
		clientID:        clientID,
		consumerGroup:   consumerGroup,
		concurrency:     concurrency,
		deadLetterTopic: deadLetterTopic,
	}, nil
}

func (kafka *Kafka) Push(ctx context.Context, topic, key string, payload []byte) error {
	if strings.TrimSpace(topic) == "" {
		return fmt.Errorf("Kafka topic is required")
	}
	if len(payload) == 0 {
		return fmt.Errorf("Kafka payload is required")
	}
	result := kafka.producer.ProduceSync(ctx, &kgo.Record{
		Topic: topic,
		Key:   []byte(key),
		Value: payload,
	})
	if err := result.FirstErr(); err != nil {
		return fmt.Errorf("push Kafka message: %w", err)
	}
	return nil
}

type topicPartition struct {
	topic     string
	partition int32
}

type partitionState struct {
	records    []*kgo.Record
	completed  map[int64]bool
	busy       bool
	retrying   bool
	revoked    bool
	assigned   bool
	retryCount int
	retryTimer *time.Timer
}

type kafkaWork struct {
	record  *kgo.Record
	handler func(context.Context, []byte) RecordResult
}

type kafkaWorkResult struct {
	record  *kgo.Record
	result  RecordResult
	durable bool
}

type fetchBatch struct {
	records []*kgo.Record
	errors  []kgo.FetchError
}

type retryEvent struct {
	key    topicPartition
	offset int64
}

type assignmentEvent struct {
	partitions map[string][]int32
}

type revocationRequest struct {
	ctx        context.Context
	partitions map[string][]int32
	done       chan struct{}
}

type coordinator struct {
	kafka         *Kafka
	consumer      *kgo.Client
	commitRecords func(context.Context, ...*kgo.Record) error
	handler       func(context.Context, []byte) RecordResult
	ctx           context.Context
	states        map[topicPartition]*partitionState
	work          chan kafkaWork
	results       chan kafkaWorkResult
	retries       chan retryEvent
	assignments   chan assignmentEvent
	revocations   chan revocationRequest
	pendingRevoke []revocationRequest
}

func (kafka *Kafka) Consume(
	ctx context.Context,
	topic string,
	handler func(context.Context, []byte) RecordResult,
) error {
	if strings.TrimSpace(topic) == "" {
		return fmt.Errorf("Kafka topic is required")
	}
	if handler == nil {
		return fmt.Errorf("Kafka message handler is required")
	}

	assignments := make(chan assignmentEvent, 1)
	revocations := make(chan revocationRequest, 1)
	consumer, err := kgo.NewClient(
		kgo.SeedBrokers(kafka.brokers...),
		kgo.ClientID(kafka.clientID+"-consumer"),
		kgo.ConsumerGroup(kafka.consumerGroup),
		kgo.ConsumeTopics(topic),
		kgo.DisableAutoCommit(),
		kgo.FetchMaxBytes(16*1024*1024),
		kgo.FetchMaxPartitionBytes(4*1024*1024),
		kgo.OnPartitionsAssigned(func(
			rebalanceContext context.Context,
			_ *kgo.Client,
			partitions map[string][]int32,
		) {
			select {
			case assignments <- assignmentEvent{partitions: partitions}:
			case <-rebalanceContext.Done():
			}
		}),
		kgo.OnPartitionsRevoked(func(
			rebalanceContext context.Context,
			_ *kgo.Client,
			partitions map[string][]int32,
		) {
			request := revocationRequest{
				ctx:        rebalanceContext,
				partitions: partitions,
				done:       make(chan struct{}),
			}
			select {
			case revocations <- request:
			case <-rebalanceContext.Done():
				return
			}
			select {
			case <-request.done:
			case <-rebalanceContext.Done():
			}
		}),
	)
	if err != nil {
		return fmt.Errorf("create Kafka consumer: %w", err)
	}
	defer consumer.Close()

	coordinatorContext, cancel := context.WithCancel(ctx)
	defer cancel()
	work := make(chan kafkaWork, kafka.concurrency*2)
	results := make(chan kafkaWorkResult, kafka.concurrency*2)
	retries := make(chan retryEvent, kafka.concurrency*8)
	var workers sync.WaitGroup
	for index := 0; index < kafka.concurrency; index++ {
		workers.Add(1)
		go kafka.runWorker(coordinatorContext, work, results, &workers)
	}

	batches := make(chan fetchBatch, 1)
	pollingDone := make(chan struct{})
	go func() {
		defer close(pollingDone)
		for {
			fetches := consumer.PollFetches(coordinatorContext)
			if coordinatorContext.Err() != nil || fetches.IsClientClosed() {
				return
			}
			batch := fetchBatch{errors: fetches.Errors()}
			fetches.EachRecord(func(record *kgo.Record) {
				batch.records = append(batch.records, record)
			})
			select {
			case batches <- batch:
			case <-coordinatorContext.Done():
				return
			}
		}
	}()

	state := &coordinator{
		kafka:    kafka,
		consumer: consumer,
		commitRecords: func(ctx context.Context, records ...*kgo.Record) error {
			return consumer.CommitRecords(ctx, records...)
		},
		handler:     handler,
		ctx:         coordinatorContext,
		states:      make(map[topicPartition]*partitionState),
		work:        work,
		results:     results,
		retries:     retries,
		assignments: assignments,
		revocations: revocations,
	}
	commitTicker := time.NewTicker(commitInterval)
	defer commitTicker.Stop()

	for {
		select {
		case batch := <-batches:
			state.acceptFetch(batch)
		case result := <-results:
			state.finish(result)
		case retry := <-retries:
			state.retry(retry)
		case assigned := <-assignments:
			state.assign(assigned.partitions)
		case revoked := <-revocations:
			state.beginRevocation(revoked)
		case <-commitTicker.C:
			state.commit(nil, coordinatorContext)
			state.finishRevocations()
		case <-ctx.Done():
			cancel()
			<-pollingDone
			state.stopRetries()
			close(work)
			workers.Wait()
			state.drainResults()
			commitContext, cancelCommit := context.WithTimeout(
				context.Background(), shutdownCommitTimeout,
			)
			state.commit(nil, commitContext)
			cancelCommit()
			state.abandonRevocations()
			return nil
		}
		state.dispatchReady()
		state.finishRevocations()
	}
}

func (kafka *Kafka) runWorker(
	ctx context.Context,
	work <-chan kafkaWork,
	results chan<- kafkaWorkResult,
	workers *sync.WaitGroup,
) {
	defer workers.Done()
	for item := range work {
		result := invokeHandler(ctx, item.handler, item.record.Value)
		durable := result.Disposition == RecordHandled
		if result.Disposition == RecordDeadLetter {
			deadLetterErr := kafka.publishDeadLetter(ctx, item.record, result)
			durable = deadLetterErr == nil
			if deadLetterErr != nil {
				result = RecordResult{
					Disposition: RecordRetry,
					Code:        "DLQ_PUBLICATION_FAILED",
					Err:         deadLetterErr,
				}
			}
		}
		results <- kafkaWorkResult{
			record:  item.record,
			result:  result,
			durable: durable,
		}
	}
}

func (coordinator *coordinator) acceptFetch(batch fetchBatch) {
	for _, fetchError := range batch.errors {
		slog.Warn(
			"recoverable Kafka fetch error",
			"topic", fetchError.Topic,
			"partition", fetchError.Partition,
			"error", fetchError.Err,
		)
	}
	paused := make(map[string][]int32)
	for _, record := range batch.records {
		key := topicPartition{topic: record.Topic, partition: record.Partition}
		state := coordinator.states[key]
		if state == nil {
			state = &partitionState{
				completed: make(map[int64]bool),
				assigned:  true,
			}
			coordinator.states[key] = state
		}
		if state.revoked || containsOffset(state.records, record.Offset) {
			continue
		}
		state.records = append(state.records, record)
		paused[key.topic] = append(paused[key.topic], key.partition)
	}
	if len(paused) > 0 {
		coordinator.consumer.PauseFetchPartitions(paused)
	}
}

func (coordinator *coordinator) dispatchReady() {
	for key, state := range coordinator.states {
		if state.busy || state.retrying || state.revoked || !state.assigned {
			continue
		}
		record := firstUnresolved(state)
		if record == nil {
			coordinator.consumer.ResumeFetchPartitions(
				map[string][]int32{key.topic: {key.partition}},
			)
			continue
		}
		select {
		case coordinator.work <- kafkaWork{
			record:  record,
			handler: coordinator.handler,
		}:
			state.busy = true
		default:
			return
		}
	}
}

func (coordinator *coordinator) finish(result kafkaWorkResult) {
	key := topicPartition{
		topic:     result.record.Topic,
		partition: result.record.Partition,
	}
	state := coordinator.states[key]
	if state == nil {
		return
	}
	state.busy = false
	if result.durable {
		state.completed[result.record.Offset] = true
		state.retryCount = 0
		slog.Debug(
			"Kafka record durably handled",
			"code", result.result.Code,
			"partition", result.record.Partition,
			"offset", result.record.Offset,
		)
		return
	}
	if state.revoked {
		return
	}
	state.retryCount++
	state.retrying = true
	backoff := partitionRetryBackoff(state.retryCount)
	slog.Warn(
		"Kafka record remains uncommitted and will be retried",
		"code", result.result.Code,
		"partition", result.record.Partition,
		"offset", result.record.Offset,
		"backoff", backoff,
	)
	state.retryTimer = time.AfterFunc(backoff, func() {
		select {
		case coordinator.retries <- retryEvent{key: key, offset: result.record.Offset}:
		case <-coordinator.ctx.Done():
		}
	})
}

func (coordinator *coordinator) retry(event retryEvent) {
	state := coordinator.states[event.key]
	if state == nil || state.revoked {
		return
	}
	record := firstUnresolved(state)
	if record == nil || record.Offset != event.offset {
		return
	}
	state.retrying = false
	state.retryTimer = nil
}

func (coordinator *coordinator) assign(partitions map[string][]int32) {
	for topic, values := range partitions {
		for _, partition := range values {
			key := topicPartition{topic: topic, partition: partition}
			state := coordinator.states[key]
			if state == nil {
				state = &partitionState{completed: make(map[int64]bool)}
				coordinator.states[key] = state
			}
			state.assigned = true
			state.revoked = false
		}
	}
}

func (coordinator *coordinator) beginRevocation(request revocationRequest) {
	for topic, values := range request.partitions {
		for _, partition := range values {
			key := topicPartition{topic: topic, partition: partition}
			if state := coordinator.states[key]; state != nil {
				state.revoked = true
				state.assigned = false
				if state.retryTimer != nil {
					state.retryTimer.Stop()
					state.retryTimer = nil
				}
				state.retrying = false
			}
		}
	}
	coordinator.pendingRevoke = append(coordinator.pendingRevoke, request)
}

func (coordinator *coordinator) finishRevocations() {
	pending := coordinator.pendingRevoke[:0]
	for _, request := range coordinator.pendingRevoke {
		if request.ctx.Err() != nil || coordinator.revocationBusy(request.partitions) {
			if request.ctx.Err() == nil {
				pending = append(pending, request)
				continue
			}
		}
		only := partitionSet(request.partitions)
		coordinator.commit(only, request.ctx)
		for key := range only {
			delete(coordinator.states, key)
		}
		close(request.done)
	}
	coordinator.pendingRevoke = pending
}

func (coordinator *coordinator) revocationBusy(
	partitions map[string][]int32,
) bool {
	for key := range partitionSet(partitions) {
		if state := coordinator.states[key]; state != nil && state.busy {
			return true
		}
	}
	return false
}

func (coordinator *coordinator) commit(
	only map[topicPartition]bool,
	ctx context.Context,
) {
	candidates := coordinator.commitCandidates(only)
	if len(candidates) == 0 || ctx.Err() != nil {
		return
	}
	if err := coordinator.commitRecords(ctx, candidates...); err != nil {
		slog.Warn("Kafka contiguous offset commit failed", "error", err)
		return
	}
	for _, committed := range candidates {
		key := topicPartition{
			topic:     committed.Topic,
			partition: committed.Partition,
		}
		state := coordinator.states[key]
		if state == nil {
			continue
		}
		index := 0
		for index < len(state.records) &&
			state.records[index].Offset <= committed.Offset {
			delete(state.completed, state.records[index].Offset)
			index++
		}
		state.records = append([]*kgo.Record(nil), state.records[index:]...)
	}
}

func (coordinator *coordinator) commitCandidates(
	only map[topicPartition]bool,
) []*kgo.Record {
	result := make([]*kgo.Record, 0)
	for key, state := range coordinator.states {
		if only != nil && !only[key] {
			continue
		}
		if only == nil && (!state.assigned || state.revoked) {
			continue
		}
		var candidate *kgo.Record
		for _, record := range state.records {
			if !state.completed[record.Offset] {
				break
			}
			candidate = record
		}
		if candidate != nil {
			result = append(result, candidate)
		}
	}
	return result
}

func (coordinator *coordinator) stopRetries() {
	for _, state := range coordinator.states {
		if state.retryTimer != nil {
			state.retryTimer.Stop()
			state.retryTimer = nil
		}
		state.retrying = false
	}
}

func (coordinator *coordinator) drainResults() {
	for {
		select {
		case result := <-coordinator.results:
			coordinator.finish(result)
		default:
			return
		}
	}
}

func (coordinator *coordinator) abandonRevocations() {
	for _, request := range coordinator.pendingRevoke {
		close(request.done)
	}
	coordinator.pendingRevoke = nil
}

func firstUnresolved(state *partitionState) *kgo.Record {
	for _, record := range state.records {
		if !state.completed[record.Offset] {
			return record
		}
	}
	return nil
}

func containsOffset(records []*kgo.Record, offset int64) bool {
	for _, record := range records {
		if record.Offset == offset {
			return true
		}
	}
	return false
}

func partitionSet(partitions map[string][]int32) map[topicPartition]bool {
	result := make(map[topicPartition]bool)
	for topic, values := range partitions {
		for _, partition := range values {
			result[topicPartition{topic: topic, partition: partition}] = true
		}
	}
	return result
}

func partitionRetryBackoff(attempt int) time.Duration {
	backoff := partitionRetryInitialBackoff
	for index := 1; index < attempt && backoff < partitionRetryMaximumBackoff; index++ {
		backoff *= 2
		if backoff > partitionRetryMaximumBackoff {
			return partitionRetryMaximumBackoff
		}
	}
	return backoff
}

func (kafka *Kafka) publishDeadLetter(
	ctx context.Context,
	source *kgo.Record,
	result RecordResult,
) error {
	envelope, err := json.Marshal(map[string]any{
		"sourceTopic":     source.Topic,
		"sourcePartition": source.Partition,
		"sourceOffset":    source.Offset,
		"code":            result.Code,
	})
	if err != nil {
		return err
	}
	return kafka.Push(ctx, kafka.deadLetterTopic, string(source.Key), envelope)
}

func invokeHandler(
	ctx context.Context,
	handler func(context.Context, []byte) RecordResult,
	payload []byte,
) (result RecordResult) {
	defer func() {
		if recover() != nil {
			result = RecordResult{
				Disposition: RecordDeadLetter,
				Code:        "HANDLER_PANIC",
				Err:         fmt.Errorf("Kafka message handler panicked"),
			}
		}
	}()
	result = handler(ctx, payload)
	if result.Disposition == "" {
		result.Disposition = RecordRetry
	}
	return result
}

func (kafka *Kafka) Close() {
	if kafka != nil && kafka.producer != nil {
		kafka.producer.Close()
	}
}
