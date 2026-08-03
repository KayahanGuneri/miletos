package queue

import (
	"context"
	"errors"
	"testing"

	"github.com/twmb/franz-go/pkg/kgo"
)

func TestCommitCandidatesStopAtFirstUnresolvedOffset(t *testing.T) {
	key := topicPartition{topic: "nodes", partition: 1}
	state := &partitionState{
		records: []*kgo.Record{
			record("nodes", 1, 10),
			record("nodes", 1, 11),
			record("nodes", 1, 12),
		},
		completed: map[int64]bool{10: true, 12: true},
		assigned:  true,
	}
	instance := &coordinator{states: map[topicPartition]*partitionState{key: state}}

	candidates := instance.commitCandidates(nil)
	if len(candidates) != 1 || candidates[0].Offset != 10 {
		t.Fatalf("commitCandidates() = %#v, want only offset 10", candidateOffsets(candidates))
	}
}

func TestCommitFailureRetainsCoordinatorProgress(t *testing.T) {
	key := topicPartition{topic: "nodes", partition: 2}
	state := &partitionState{
		records: []*kgo.Record{
			record("nodes", 2, 20),
			record("nodes", 2, 21),
		},
		completed: map[int64]bool{20: true, 21: true},
		assigned:  true,
	}
	instance := &coordinator{
		states: map[topicPartition]*partitionState{key: state},
		commitRecords: func(context.Context, ...*kgo.Record) error {
			return errors.New("controlled commit failure")
		},
	}

	instance.commit(nil, context.Background())

	if len(state.records) != 2 || !state.completed[20] || !state.completed[21] {
		t.Fatalf("failed commit changed progress: records=%d completed=%v", len(state.records), state.completed)
	}
}

func TestSuccessfulCommitTrimsOnlyContiguousDurableProgress(t *testing.T) {
	key := topicPartition{topic: "nodes", partition: 3}
	state := &partitionState{
		records: []*kgo.Record{
			record("nodes", 3, 30),
			record("nodes", 3, 31),
			record("nodes", 3, 32),
		},
		completed: map[int64]bool{30: true, 31: true},
		assigned:  true,
	}
	var committed []*kgo.Record
	instance := &coordinator{
		states: map[topicPartition]*partitionState{key: state},
		commitRecords: func(_ context.Context, records ...*kgo.Record) error {
			committed = append(committed, records...)
			return nil
		},
	}

	instance.commit(nil, context.Background())

	if len(committed) != 1 || committed[0].Offset != 31 {
		t.Fatalf("committed = %v, want offset 31", candidateOffsets(committed))
	}
	if len(state.records) != 1 || state.records[0].Offset != 32 {
		t.Fatalf("remaining records = %v, want offset 32", candidateOffsets(state.records))
	}
	if state.completed[30] || state.completed[31] {
		t.Fatalf("committed offsets retained: %v", state.completed)
	}
}

func TestRetryAndRevocationKeepUnresolvedPartitionUncommitted(t *testing.T) {
	key := topicPartition{topic: "nodes", partition: 4}
	state := &partitionState{
		records: []*kgo.Record{
			record("nodes", 4, 40),
			record("nodes", 4, 41),
		},
		completed: map[int64]bool{41: true},
		retrying:  true,
		assigned:  true,
	}
	instance := &coordinator{states: map[topicPartition]*partitionState{key: state}}

	if got := instance.commitCandidates(nil); len(got) != 0 {
		t.Fatalf("retrying partition candidates = %v, want none", candidateOffsets(got))
	}
	if unresolved := firstUnresolved(state); unresolved == nil || unresolved.Offset != 40 {
		t.Fatalf("firstUnresolved() = %#v, want offset 40", unresolved)
	}

	state.revoked = true
	state.assigned = false
	if got := instance.commitCandidates(nil); len(got) != 0 {
		t.Fatalf("revoked partition candidates = %v, want none", candidateOffsets(got))
	}
}

func TestValidPartitionProgressIsIndependentOfAnotherPartition(t *testing.T) {
	blockedKey := topicPartition{topic: "nodes", partition: 0}
	validKey := topicPartition{topic: "nodes", partition: 1}
	instance := &coordinator{states: map[topicPartition]*partitionState{
		blockedKey: {
			records:   []*kgo.Record{record("nodes", 0, 1)},
			completed: map[int64]bool{},
			retrying:  true,
			assigned:  true,
		},
		validKey: {
			records:   []*kgo.Record{record("nodes", 1, 7)},
			completed: map[int64]bool{7: true},
			assigned:  true,
		},
	}}

	candidates := instance.commitCandidates(nil)
	if len(candidates) != 1 || candidates[0].Partition != 1 || candidates[0].Offset != 7 {
		t.Fatalf("commitCandidates() = %v, want partition 1 offset 7", candidateOffsets(candidates))
	}
}

func record(topic string, partition int32, offset int64) *kgo.Record {
	return &kgo.Record{Topic: topic, Partition: partition, Offset: offset}
}

func candidateOffsets(records []*kgo.Record) []string {
	result := make([]string, 0, len(records))
	for _, item := range records {
		result = append(result, item.Topic)
	}
	return result
}
