package runtime

import (
	context "context"
	errors "errors"
	fmt "fmt"
	plugin "miletos-go/internal/engine/plugin"
	execution "miletos-go/internal/features/execution"
	workflow "miletos-go/internal/features/workflow"
	reflect "reflect"
	goruntime "runtime"
	strings "strings"
	sync "sync"
	testing "testing"
	time "time"
)

func TestNewEdgeCacheRejectsNonPositiveCapacity(
	t *testing.T,
) {
	tests := map[string]int{
		"zero":     0,
		"negative": -1,
	}

	for name, capacity := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := NewEdgeCache(capacity)

			requireRuntimeValidationField(
				t,
				err,
				"cache.capacity",
			)
		})
	}
}

func TestEdgeCacheReportsCapacityAndInitialState(
	t *testing.T,
) {
	cache := newEdgeCacheForTest(t, 3)

	if actual := cache.Capacity(); actual != 3 {
		t.Fatalf(
			"Capacity() = %d, want %d",
			actual,
			3,
		)
	}

	if actual := cache.Len(); actual != 0 {
		t.Fatalf(
			"Len() = %d, want 0",
			actual,
		)
	}

	if keys := cache.Keys(); keys != nil {
		t.Fatalf(
			"Keys() = %#v, want nil",
			keys,
		)
	}

	if snapshot := cache.Snapshot(); snapshot != nil {
		t.Fatalf(
			"Snapshot() = %#v, want nil",
			snapshot,
		)
	}
}

func TestEdgeCacheSetGetAndDelete(
	t *testing.T,
) {
	cache := newEdgeCacheForTest(t, 2)
	value := newStringRuntimeValueForCacheTest(
		t,
		"TRY",
	)

	evicted, didEvict, err := cache.Set(
		" currency ",
		value,
	)
	if err != nil {
		t.Fatalf(
			"Set() returned an unexpected error: %v",
			err,
		)
	}

	if didEvict {
		t.Fatalf(
			"Set() unexpectedly evicted key %q",
			evicted.Key(),
		)
	}

	if actual := cache.Len(); actual != 1 {
		t.Fatalf(
			"Len() = %d, want 1",
			actual,
		)
	}

	actual, exists, err := cache.Get("currency")
	if err != nil {
		t.Fatalf(
			"Get() returned an unexpected error: %v",
			err,
		)
	}

	if !exists {
		t.Fatal(
			"Get() exists = false",
		)
	}

	if decoded := runtimeValueStringForCacheTest(
		t,
		actual,
	); decoded != "TRY" {
		t.Fatalf(
			"Get() = %q, want %q",
			decoded,
			"TRY",
		)
	}

	deleted, exists, err := cache.Delete(
		" currency ",
	)
	if err != nil {
		t.Fatalf(
			"Delete() returned an unexpected error: %v",
			err,
		)
	}

	if !exists {
		t.Fatal(
			"Delete() exists = false",
		)
	}

	if decoded := runtimeValueStringForCacheTest(
		t,
		deleted,
	); decoded != "TRY" {
		t.Fatalf(
			"deleted value = %q, want %q",
			decoded,
			"TRY",
		)
	}

	if actual := cache.Len(); actual != 0 {
		t.Fatalf(
			"Len() after Delete() = %d, want 0",
			actual,
		)
	}
}

func TestEdgeCacheGetAndDeleteHandleMissingKey(
	t *testing.T,
) {
	cache := newEdgeCacheForTest(t, 1)

	if _, exists, err := cache.Get("missing"); err != nil {
		t.Fatalf(
			"Get() returned an unexpected error: %v",
			err,
		)
	} else if exists {
		t.Fatal(
			"Get() exists = true for missing key",
		)
	}

	if _, exists, err := cache.Delete("missing"); err != nil {
		t.Fatalf(
			"Delete() returned an unexpected error: %v",
			err,
		)
	} else if exists {
		t.Fatal(
			"Delete() exists = true for missing key",
		)
	}
}

func TestEdgeCacheDropOldestUsesInsertionOrder(
	t *testing.T,
) {
	cache := newEdgeCacheForTest(t, 2)

	setCacheStringForTest(t, cache, "a", "A")
	setCacheStringForTest(t, cache, "b", "B")

	evicted, didEvict, err := cache.Set(
		"c",
		newStringRuntimeValueForCacheTest(
			t,
			"C",
		),
	)
	if err != nil {
		t.Fatalf(
			"Set(c) returned an unexpected error: %v",
			err,
		)
	}

	if !didEvict {
		t.Fatal(
			"Set(c) didEvict = false for a full cache",
		)
	}

	if actual := evicted.Key(); actual != "a" {
		t.Fatalf(
			"evicted key = %q, want %q",
			actual,
			"a",
		)
	}

	if actual := runtimeValueStringForCacheTest(
		t,
		evicted.Value(),
	); actual != "A" {
		t.Fatalf(
			"evicted value = %q, want %q",
			actual,
			"A",
		)
	}

	requireCacheKeys(
		t,
		cache,
		[]string{
			"b",
			"c",
		},
	)
}

func TestEdgeCacheUpdatingExistingKeyPreservesInsertionOrder(
	t *testing.T,
) {
	cache := newEdgeCacheForTest(t, 2)

	setCacheStringForTest(t, cache, "a", "A1")
	setCacheStringForTest(t, cache, "b", "B")

	evicted, didEvict, err := cache.Set(
		"a",
		newStringRuntimeValueForCacheTest(
			t,
			"A2",
		),
	)
	if err != nil {
		t.Fatalf(
			"updating a returned an unexpected error: %v",
			err,
		)
	}

	if didEvict {
		t.Fatalf(
			"updating a unexpectedly evicted key %q",
			evicted.Key(),
		)
	}

	requireCacheKeys(
		t,
		cache,
		[]string{
			"a",
			"b",
		},
	)

	evicted, didEvict, err = cache.Set(
		"c",
		newStringRuntimeValueForCacheTest(
			t,
			"C",
		),
	)
	if err != nil {
		t.Fatalf(
			"Set(c) returned an unexpected error: %v",
			err,
		)
	}

	if !didEvict {
		t.Fatal(
			"Set(c) didEvict = false",
		)
	}

	if actual := evicted.Key(); actual != "a" {
		t.Fatalf(
			"evicted key = %q, want %q",
			actual,
			"a",
		)
	}

	if actual := runtimeValueStringForCacheTest(
		t,
		evicted.Value(),
	); actual != "A2" {
		t.Fatalf(
			"evicted updated value = %q, want %q",
			actual,
			"A2",
		)
	}

	requireCacheKeys(
		t,
		cache,
		[]string{
			"b",
			"c",
		},
	)
}

func TestEdgeCacheGetDoesNotChangeInsertionOrder(
	t *testing.T,
) {
	cache := newEdgeCacheForTest(t, 2)

	setCacheStringForTest(t, cache, "a", "A")
	setCacheStringForTest(t, cache, "b", "B")

	if _, exists, err := cache.Get("a"); err != nil {
		t.Fatalf(
			"Get(a) returned an unexpected error: %v",
			err,
		)
	} else if !exists {
		t.Fatal(
			"Get(a) exists = false",
		)
	}

	evicted, didEvict, err := cache.Set(
		"c",
		newStringRuntimeValueForCacheTest(
			t,
			"C",
		),
	)
	if err != nil {
		t.Fatalf(
			"Set(c) returned an unexpected error: %v",
			err,
		)
	}

	if !didEvict {
		t.Fatal(
			"Set(c) didEvict = false",
		)
	}

	if actual := evicted.Key(); actual != "a" {
		t.Fatalf(
			"evicted key = %q, want %q",
			actual,
			"a",
		)
	}
}

func TestEdgeCacheDeleteRemovesKeyFromOrder(
	t *testing.T,
) {
	cache := newEdgeCacheForTest(t, 3)

	setCacheStringForTest(t, cache, "a", "A")
	setCacheStringForTest(t, cache, "b", "B")
	setCacheStringForTest(t, cache, "c", "C")

	if _, exists, err := cache.Delete("b"); err != nil {
		t.Fatalf(
			"Delete(b) returned an unexpected error: %v",
			err,
		)
	} else if !exists {
		t.Fatal(
			"Delete(b) exists = false",
		)
	}

	evicted, didEvict, err := cache.Set(
		"d",
		newStringRuntimeValueForCacheTest(
			t,
			"D",
		),
	)
	if err != nil {
		t.Fatalf(
			"Set(d) returned an unexpected error: %v",
			err,
		)
	}

	if didEvict {
		t.Fatalf(
			"Set(d) unexpectedly evicted key %q",
			evicted.Key(),
		)
	}

	requireCacheKeys(
		t,
		cache,
		[]string{
			"a",
			"c",
			"d",
		},
	)
}

func TestEdgeCacheCapacityOneDropsPreviousEntry(
	t *testing.T,
) {
	cache := newEdgeCacheForTest(t, 1)

	setCacheStringForTest(t, cache, "a", "A")

	evicted, didEvict, err := cache.Set(
		"b",
		newStringRuntimeValueForCacheTest(
			t,
			"B",
		),
	)
	if err != nil {
		t.Fatalf(
			"Set(b) returned an unexpected error: %v",
			err,
		)
	}

	if !didEvict {
		t.Fatal(
			"Set(b) didEvict = false",
		)
	}

	if actual := evicted.Key(); actual != "a" {
		t.Fatalf(
			"evicted key = %q, want %q",
			actual,
			"a",
		)
	}

	requireCacheKeys(
		t,
		cache,
		[]string{"b"},
	)
}

func TestEdgeCacheRejectsInvalidKeyAndValue(
	t *testing.T,
) {
	cache := newEdgeCacheForTest(t, 1)
	validValue := newStringRuntimeValueForCacheTest(
		t,
		"value",
	)

	t.Run("blank set key", func(t *testing.T) {
		_, _, err := cache.Set(
			" ",
			validValue,
		)

		requireRuntimeValidationField(
			t,
			err,
			"cache.key",
		)
	})

	t.Run("blank get key", func(t *testing.T) {
		_, _, err := cache.Get(" ")

		requireRuntimeValidationField(
			t,
			err,
			"cache.key",
		)
	})

	t.Run("blank delete key", func(t *testing.T) {
		_, _, err := cache.Delete(" ")

		requireRuntimeValidationField(
			t,
			err,
			"cache.key",
		)
	})

	t.Run("invalid value", func(t *testing.T) {
		_, _, err := cache.Set(
			"key",
			RuntimeValue{},
		)

		requireRuntimeValidationField(
			t,
			err,
			"cache.value",
		)
	})
}

func TestEdgeCacheKeysAndSnapshotAreIndependent(
	t *testing.T,
) {
	cache := newEdgeCacheForTest(t, 2)

	setCacheStringForTest(t, cache, "a", "A")
	setCacheStringForTest(t, cache, "b", "B")

	keys := cache.Keys()
	keys[0] = "changed"

	requireCacheKeys(
		t,
		cache,
		[]string{
			"a",
			"b",
		},
	)

	snapshot := cache.Snapshot()
	if len(snapshot) != 2 {
		t.Fatalf(
			"len(Snapshot()) = %d, want 2",
			len(snapshot),
		)
	}

	snapshot[0].key = "changed"
	snapshot[0].value.raw[0] = '['

	actual, exists, err := cache.Get("a")
	if err != nil {
		t.Fatalf(
			"Get(a) returned an unexpected error: %v",
			err,
		)
	}

	if !exists {
		t.Fatal(
			"Get(a) exists = false",
		)
	}

	if decoded := runtimeValueStringForCacheTest(
		t,
		actual,
	); decoded != "A" {
		t.Fatalf(
			"cache value changed through snapshot: got %q",
			decoded,
		)
	}
}

func TestCacheEntryValueReturnsIndependentCopy(
	t *testing.T,
) {
	cache := newEdgeCacheForTest(t, 1)

	setCacheStringForTest(t, cache, "a", "A")

	entry := cache.Snapshot()[0]
	exposed := entry.Value()
	exposed.raw[0] = '['

	second := entry.Value()

	if decoded := runtimeValueStringForCacheTest(
		t,
		second,
	); decoded != "A" {
		t.Fatalf(
			"CacheEntry changed through Value(): got %q",
			decoded,
		)
	}
}

func TestNilEdgeCacheMethodsAreSafe(
	t *testing.T,
) {
	var cache *EdgeCache
	value := newStringRuntimeValueForCacheTest(
		t,
		"value",
	)

	_, _, err := cache.Set(
		"key",
		value,
	)
	requireRuntimeValidationField(
		t,
		err,
		"cache",
	)

	_, _, err = cache.Get("key")
	requireRuntimeValidationField(
		t,
		err,
		"cache",
	)

	_, _, err = cache.Delete("key")
	requireRuntimeValidationField(
		t,
		err,
		"cache",
	)

	if actual := cache.Len(); actual != 0 {
		t.Fatalf(
			"nil cache Len() = %d, want 0",
			actual,
		)
	}

	if actual := cache.Capacity(); actual != 0 {
		t.Fatalf(
			"nil cache Capacity() = %d, want 0",
			actual,
		)
	}

	if keys := cache.Keys(); keys != nil {
		t.Fatalf(
			"nil cache Keys() = %#v, want nil",
			keys,
		)
	}

	if snapshot := cache.Snapshot(); snapshot != nil {
		t.Fatalf(
			"nil cache Snapshot() = %#v, want nil",
			snapshot,
		)
	}
}

func TestEdgeCacheConcurrentAccess(
	t *testing.T,
) {
	const (
		capacity    = 64
		writerCount = 8
		readerCount = 4
		deleteCount = 2
		operations  = 200
	)

	cache := newEdgeCacheForTest(
		t,
		capacity,
	)

	errorChannel := make(
		chan error,
		(writerCount+readerCount+deleteCount)*operations,
	)

	var waitGroup sync.WaitGroup

	for writer := 0; writer < writerCount; writer++ {
		writerID := writer

		waitGroup.Add(1)

		go func() {
			defer waitGroup.Done()

			for operation := 0; operation < operations; operation++ {
				key := fmt.Sprintf(
					"writer-%d-key-%d",
					writerID,
					operation,
				)

				value, err := NewRuntimeValue(
					[]byte(
						fmt.Sprintf(
							`{"writer":%d,"operation":%d}`,
							writerID,
							operation,
						),
					),
				)
				if err != nil {
					errorChannel <- err
					continue
				}

				if _, _, err := cache.Set(
					key,
					value,
				); err != nil {
					errorChannel <- err
				}
			}
		}()
	}

	for reader := 0; reader < readerCount; reader++ {
		readerID := reader

		waitGroup.Add(1)

		go func() {
			defer waitGroup.Done()

			for operation := 0; operation < operations; operation++ {
				key := fmt.Sprintf(
					"writer-%d-key-%d",
					readerID%writerCount,
					operation,
				)

				if _, _, err := cache.Get(key); err != nil {
					errorChannel <- err
				}

				cache.Keys()
				cache.Snapshot()
				cache.Len()
				cache.Capacity()
			}
		}()
	}

	for deleter := 0; deleter < deleteCount; deleter++ {
		deleterID := deleter

		waitGroup.Add(1)

		go func() {
			defer waitGroup.Done()

			for operation := 0; operation < operations; operation++ {
				key := fmt.Sprintf(
					"writer-%d-key-%d",
					deleterID,
					operation,
				)

				if _, _, err := cache.Delete(key); err != nil {
					errorChannel <- err
				}
			}
		}()
	}

	waitGroup.Wait()
	close(errorChannel)

	for err := range errorChannel {
		t.Errorf(
			"concurrent cache operation returned an error: %v",
			err,
		)
	}

	if actual := cache.Len(); actual > capacity {
		t.Fatalf(
			"Len() = %d, must not exceed capacity %d",
			actual,
			capacity,
		)
	}

	keys := cache.Keys()
	snapshot := cache.Snapshot()

	if len(keys) != cache.Len() {
		t.Fatalf(
			"len(Keys()) = %d, want Len() %d",
			len(keys),
			cache.Len(),
		)
	}

	if len(snapshot) != cache.Len() {
		t.Fatalf(
			"len(Snapshot()) = %d, want Len() %d",
			len(snapshot),
			cache.Len(),
		)
	}

	for index, entry := range snapshot {
		if entry.Key() != keys[index] {
			t.Fatalf(
				"Snapshot()[%d].Key() = %q, want Keys()[%d] = %q",
				index,
				entry.Key(),
				index,
				keys[index],
			)
		}
	}
}

func newEdgeCacheForTest(
	t *testing.T,
	capacity int,
) *EdgeCache {
	t.Helper()

	cache, err := NewEdgeCache(capacity)
	if err != nil {
		t.Fatalf(
			"NewEdgeCache() returned an unexpected error: %v",
			err,
		)
	}

	return cache
}

func newStringRuntimeValueForCacheTest(
	t *testing.T,
	value string,
) RuntimeValue {
	t.Helper()

	runtimeValue, err := NewRuntimeValue(
		[]byte(
			fmt.Sprintf(
				"%q",
				value,
			),
		),
	)
	if err != nil {
		t.Fatalf(
			"NewRuntimeValue(%q) returned an unexpected error: %v",
			value,
			err,
		)
	}

	return runtimeValue
}

func runtimeValueStringForCacheTest(
	t *testing.T,
	value RuntimeValue,
) string {
	t.Helper()

	decoded, err := DecodeRuntimeValue[string](
		value,
	)
	if err != nil {
		t.Fatalf(
			"DecodeRuntimeValue() returned an unexpected error: %v",
			err,
		)
	}

	return decoded
}

func setCacheStringForTest(
	t *testing.T,
	cache *EdgeCache,
	key string,
	value string,
) {
	t.Helper()

	evicted, didEvict, err := cache.Set(
		key,
		newStringRuntimeValueForCacheTest(
			t,
			value,
		),
	)
	if err != nil {
		t.Fatalf(
			"Set(%q) returned an unexpected error: %v",
			key,
			err,
		)
	}

	if didEvict {
		t.Fatalf(
			"Set(%q) unexpectedly evicted key %q",
			key,
			evicted.Key(),
		)
	}
}

func requireCacheKeys(
	t *testing.T,
	cache *EdgeCache,
	expected []string,
) {
	t.Helper()

	actual := cache.Keys()

	if !reflect.DeepEqual(actual, expected) {
		t.Fatalf(
			"Keys() = %#v, want %#v",
			actual,
			expected,
		)
	}
}

func TestExecutionContextApplyContextChangesSetsDeletesAndCopiesValues(
	t *testing.T,
) {
	initialVariables := map[string]RuntimeValue{
		"replace": newContextChangeStringValueForTest(
			t,
			"before",
		),
		"remove": newContextChangeStringValueForTest(
			t,
			"remove-me",
		),
		"untouched": newContextChangeStringValueForTest(
			t,
			"stable",
		),
	}

	executionContext := newExecutionContextForContextTest(
		t,
		context.Background(),
		nil,
		initialVariables,
	)

	replaceValue := newContextChangeStringValueForTest(
		t,
		"after",
	)

	addedValue := newContextChangeStringValueForTest(
		t,
		"added-value",
	)

	changes, err := NewContextChanges(
		map[string]RuntimeValue{
			" replace ": replaceValue,
			" added ":   addedValue,
		},
		[]string{
			" remove ",
		},
	)
	if err != nil {
		t.Fatalf(
			"NewContextChanges() returned an unexpected error: %v",
			err,
		)
	}

	/*
		The values passed to NewContextChanges must be copied.

		Mutating the original RuntimeValue variables after construction
		must not change the stored ContextChanges.
	*/
	replaceValue.raw[0] = '['
	addedValue.raw[0] = '['

	/*
		SetValues must also return an independent snapshot.

		Mutating the value returned by this accessor must not change
		the ContextChanges object.
	*/
	exposedValues := changes.SetValues()
	exposedReplaceValue := exposedValues["replace"]
	exposedReplaceValue.raw[0] = '['
	exposedValues["replace"] = exposedReplaceValue

	err = executionContext.ApplyContextChanges(changes)
	if err != nil {
		t.Fatalf(
			"ApplyContextChanges() returned an unexpected error: %v",
			err,
		)
	}

	requireAppliedContextString(
		t,
		executionContext,
		"replace",
		"after",
	)

	requireAppliedContextString(
		t,
		executionContext,
		"added",
		"added-value",
	)

	requireAppliedContextString(
		t,
		executionContext,
		"untouched",
		"stable",
	)

	_, exists, err := executionContext.Variable("remove")
	if err != nil {
		t.Fatalf(
			"Variable(remove) returned an unexpected error: %v",
			err,
		)
	}

	if exists {
		t.Fatal(
			"deleted variable remove still exists",
		)
	}

	snapshot := executionContext.VariablesSnapshot()

	if actual := len(snapshot); actual != 3 {
		t.Fatalf(
			"VariablesSnapshot() length = %d, want 3",
			actual,
		)
	}

	/*
		VariablesSnapshot must return independent RuntimeValue copies.
	*/
	snapshotValue := snapshot["replace"]
	snapshotValue.raw[0] = '['
	snapshot["replace"] = snapshotValue

	requireAppliedContextString(
		t,
		executionContext,
		"replace",
		"after",
	)
}

func TestExecutionContextApplyContextChangesAcceptsEmptyChanges(
	t *testing.T,
) {
	executionContext := newExecutionContextForContextTest(
		t,
		context.Background(),
		nil,
		map[string]RuntimeValue{
			"stable": newContextChangeStringValueForTest(
				t,
				"original",
			),
		},
	)

	var changes ContextChanges

	err := executionContext.ApplyContextChanges(changes)
	if err != nil {
		t.Fatalf(
			"ApplyContextChanges(empty) returned an unexpected error: %v",
			err,
		)
	}

	requireAppliedContextString(
		t,
		executionContext,
		"stable",
		"original",
	)

	snapshot := executionContext.VariablesSnapshot()

	if actual := len(snapshot); actual != 1 {
		t.Fatalf(
			"variable count after empty changes = %d, want 1",
			actual,
		)
	}
}

func TestExecutionContextApplyContextChangesValidatesBeforeMutation(
	t *testing.T,
) {
	executionContext := newExecutionContextForContextTest(
		t,
		context.Background(),
		nil,
		map[string]RuntimeValue{
			"stable": newContextChangeStringValueForTest(
				t,
				"original",
			),
		},
	)

	/*
		This ContextChanges object is deliberately created without the
		public constructor so that an invalid internal state can be tested.

		The invalid RuntimeValue must cause complete rejection before
		any delete or set operation is applied.
	*/
	invalidChanges := ContextChanges{
		setValues: map[string]RuntimeValue{
			"broken": {},
		},
		setOrder: []string{
			"broken",
		},
		deleteKeys: map[string]struct{}{
			"stable": {},
		},
		deleteOrder: []string{
			"stable",
		},
	}

	err := executionContext.ApplyContextChanges(
		invalidChanges,
	)

	requireRuntimeValidationField(
		t,
		err,
		"contextChanges",
	)

	requireAppliedContextString(
		t,
		executionContext,
		"stable",
		"original",
	)

	_, exists, lookupErr := executionContext.Variable("broken")
	if lookupErr != nil {
		t.Fatalf(
			"Variable(broken) returned an unexpected error: %v",
			lookupErr,
		)
	}

	if exists {
		t.Fatal(
			"invalid set operation mutated the execution context",
		)
	}

	snapshot := executionContext.VariablesSnapshot()

	if actual := len(snapshot); actual != 1 {
		t.Fatalf(
			"variable count after rejected changes = %d, want 1",
			actual,
		)
	}
}

func TestNilExecutionContextRejectsApplyContextChanges(
	t *testing.T,
) {
	var executionContext *ExecutionContext

	err := executionContext.ApplyContextChanges(
		ContextChanges{},
	)

	requireRuntimeValidationField(
		t,
		err,
		"executionContext",
	)
}

func TestExecutionContextApplyContextChangesIsAtomicForSnapshots(
	t *testing.T,
) {
	executionContext := newExecutionContextForContextTest(
		t,
		context.Background(),
		nil,
		map[string]RuntimeValue{
			"left": newContextChangeStringValueForTest(
				t,
				"A",
			),
			"right": newContextChangeStringValueForTest(
				t,
				"A",
			),
		},
	)

	changesA, err := NewContextChanges(
		map[string]RuntimeValue{
			"left": newContextChangeStringValueForTest(
				t,
				"A",
			),
			"right": newContextChangeStringValueForTest(
				t,
				"A",
			),
		},
		nil,
	)
	if err != nil {
		t.Fatalf(
			"NewContextChanges(A) returned an unexpected error: %v",
			err,
		)
	}

	changesB, err := NewContextChanges(
		map[string]RuntimeValue{
			"left": newContextChangeStringValueForTest(
				t,
				"B",
			),
			"right": newContextChangeStringValueForTest(
				t,
				"B",
			),
		},
		nil,
	)
	if err != nil {
		t.Fatalf(
			"NewContextChanges(B) returned an unexpected error: %v",
			err,
		)
	}

	started := make(chan struct{})
	done := make(chan struct{})
	writerErrors := make(chan error, 1)

	go func() {
		defer close(done)
		close(started)

		for index := 0; index < 5000; index++ {
			err := executionContext.ApplyContextChanges(
				changesB,
			)
			if err != nil {
				writerErrors <- err
				return
			}

			goruntime.Gosched()

			err = executionContext.ApplyContextChanges(
				changesA,
			)
			if err != nil {
				writerErrors <- err
				return
			}

			goruntime.Gosched()
		}
	}()

	<-started

	snapshotCount := 0

	for {
		snapshot := executionContext.VariablesSnapshot()

		leftValue, leftExists := snapshot["left"]
		rightValue, rightExists := snapshot["right"]

		if !leftExists || !rightExists {
			t.Fatalf(
				"snapshot is missing required values: %#v",
				snapshot,
			)
		}

		left := contextChangeStringValueForTest(
			t,
			leftValue,
		)

		right := contextChangeStringValueForTest(
			t,
			rightValue,
		)

		/*
			A snapshot may observe state A or state B.

			It must never observe a partially applied state such as:

			left=A, right=B
			left=B, right=A
		*/
		if left != right {
			t.Fatalf(
				"partial change set observed: left=%q right=%q",
				left,
				right,
			)
		}

		if left != "A" && left != "B" {
			t.Fatalf(
				"unexpected snapshot state: left=%q right=%q",
				left,
				right,
			)
		}

		snapshotCount++

		select {
		case writerErr := <-writerErrors:
			t.Fatalf(
				"concurrent ApplyContextChanges() returned an error: %v",
				writerErr,
			)

		case <-done:
			select {
			case writerErr := <-writerErrors:
				t.Fatalf(
					"concurrent ApplyContextChanges() returned an error: %v",
					writerErr,
				)
			default:
			}

			if snapshotCount == 0 {
				t.Fatal(
					"no concurrent variable snapshot was observed",
				)
			}

			requireAppliedContextString(
				t,
				executionContext,
				"left",
				"A",
			)

			requireAppliedContextString(
				t,
				executionContext,
				"right",
				"A",
			)

			return

		default:
		}
	}
}

func requireAppliedContextString(
	t *testing.T,
	executionContext *ExecutionContext,
	key string,
	expected string,
) {
	t.Helper()

	value, exists, err := executionContext.Variable(key)
	if err != nil {
		t.Fatalf(
			"Variable(%q) returned an unexpected error: %v",
			key,
			err,
		)
	}

	if !exists {
		t.Fatalf(
			"Variable(%q) exists = false",
			key,
		)
	}

	actual := contextChangeStringValueForTest(
		t,
		value,
	)

	if actual != expected {
		t.Fatalf(
			"Variable(%q) = %q, want %q",
			key,
			actual,
			expected,
		)
	}
}

func TestNewContextChangesAllowsEmptyChanges(
	t *testing.T,
) {
	changes, err := NewContextChanges(
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf(
			"NewContextChanges() returned an unexpected error: %v",
			err,
		)
	}

	if !changes.IsEmpty() {
		t.Fatal(
			"IsEmpty() = false for empty context changes",
		)
	}

	if !changes.IsValid() {
		t.Fatal(
			"IsValid() = false for empty context changes",
		)
	}

	if keys := changes.SetKeys(); keys != nil {
		t.Fatalf(
			"SetKeys() = %#v, want nil",
			keys,
		)
	}

	if values := changes.SetValues(); values != nil {
		t.Fatalf(
			"SetValues() = %#v, want nil",
			values,
		)
	}

	if keys := changes.DeleteKeys(); keys != nil {
		t.Fatalf(
			"DeleteKeys() = %#v, want nil",
			keys,
		)
	}
}

func TestContextChangesNormalizesAndSortsOperations(
	t *testing.T,
) {
	changes, err := NewContextChanges(
		map[string]RuntimeValue{
			" z-value ": newContextChangeStringValueForTest(
				t,
				"Z",
			),
			"a-value": newContextChangeStringValueForTest(
				t,
				"A",
			),
		},
		[]string{
			" temp-z ",
			"temp-a",
		},
	)
	if err != nil {
		t.Fatalf(
			"NewContextChanges() returned an unexpected error: %v",
			err,
		)
	}

	if changes.IsEmpty() {
		t.Fatal(
			"IsEmpty() = true for non-empty changes",
		)
	}

	expectedSetKeys := []string{
		"a-value",
		"z-value",
	}

	if actual := changes.SetKeys(); !reflect.DeepEqual(
		actual,
		expectedSetKeys,
	) {
		t.Fatalf(
			"SetKeys() = %#v, want %#v",
			actual,
			expectedSetKeys,
		)
	}

	expectedDeleteKeys := []string{
		"temp-a",
		"temp-z",
	}

	if actual := changes.DeleteKeys(); !reflect.DeepEqual(
		actual,
		expectedDeleteKeys,
	) {
		t.Fatalf(
			"DeleteKeys() = %#v, want %#v",
			actual,
			expectedDeleteKeys,
		)
	}

	value, exists, err := changes.SetValue(
		" a-value ",
	)
	if err != nil {
		t.Fatalf(
			"SetValue(a-value) returned an unexpected error: %v",
			err,
		)
	}

	if !exists {
		t.Fatal(
			"SetValue(a-value) exists = false",
		)
	}

	if actual := contextChangeStringValueForTest(
		t,
		value,
	); actual != "A" {
		t.Fatalf(
			"SetValue(a-value) = %q, want %q",
			actual,
			"A",
		)
	}

	deletes, err := changes.Deletes(
		" temp-a ",
	)
	if err != nil {
		t.Fatalf(
			"Deletes(temp-a) returned an unexpected error: %v",
			err,
		)
	}

	if !deletes {
		t.Fatal(
			"Deletes(temp-a) = false",
		)
	}
}

func TestContextChangesHandlesMissingOperations(
	t *testing.T,
) {
	changes, err := NewContextChanges(
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf(
			"NewContextChanges() returned an unexpected error: %v",
			err,
		)
	}

	value, exists, err := changes.SetValue(
		"missing",
	)
	if err != nil {
		t.Fatalf(
			"SetValue(missing) returned an unexpected error: %v",
			err,
		)
	}

	if exists {
		t.Fatal(
			"SetValue(missing) exists = true",
		)
	}

	if value.IsValid() {
		t.Fatal(
			"missing SetValue result must be zero value",
		)
	}

	deletes, err := changes.Deletes(
		"missing",
	)
	if err != nil {
		t.Fatalf(
			"Deletes(missing) returned an unexpected error: %v",
			err,
		)
	}

	if deletes {
		t.Fatal(
			"Deletes(missing) = true",
		)
	}
}

func TestContextChangesRejectsInvalidSetOperations(
	t *testing.T,
) {
	validValue := newContextChangeStringValueForTest(
		t,
		"value",
	)

	t.Run("blank set key", func(t *testing.T) {
		_, err := NewContextChanges(
			map[string]RuntimeValue{
				" ": validValue,
			},
			nil,
		)

		requireRuntimeValidationField(
			t,
			err,
			"contextChanges.set.key",
		)
	})

	t.Run("protected set key", func(t *testing.T) {
		_, err := NewContextChanges(
			map[string]RuntimeValue{
				" CompanyID ": validValue,
			},
			nil,
		)

		requireRuntimeValidationField(
			t,
			err,
			"contextChanges.set.key",
		)
	})

	t.Run("duplicate normalized set key", func(t *testing.T) {
		_, err := NewContextChanges(
			map[string]RuntimeValue{
				"key":   validValue,
				" key ": validValue,
			},
			nil,
		)

		requireRuntimeValidationField(
			t,
			err,
			"contextChanges.set",
		)
	})

	t.Run("invalid set value", func(t *testing.T) {
		_, err := NewContextChanges(
			map[string]RuntimeValue{
				"key": {},
			},
			nil,
		)

		requireRuntimeValidationField(
			t,
			err,
			"contextChanges.set.value",
		)
	})
}

func TestContextChangesRejectsInvalidDeleteOperations(
	t *testing.T,
) {
	t.Run("blank delete key", func(t *testing.T) {
		_, err := NewContextChanges(
			nil,
			[]string{
				" ",
			},
		)

		requireRuntimeValidationField(
			t,
			err,
			"contextChanges.delete.key",
		)
	})

	t.Run("protected delete key", func(t *testing.T) {
		_, err := NewContextChanges(
			nil,
			[]string{
				"CORRELATIONid",
			},
		)

		requireRuntimeValidationField(
			t,
			err,
			"contextChanges.delete.key",
		)
	})

	t.Run("duplicate normalized delete key", func(t *testing.T) {
		_, err := NewContextChanges(
			nil,
			[]string{
				"temporary",
				" temporary ",
			},
		)

		requireRuntimeValidationField(
			t,
			err,
			"contextChanges.delete",
		)
	})
}

func TestContextChangesRejectsSetDeleteConflict(
	t *testing.T,
) {
	_, err := NewContextChanges(
		map[string]RuntimeValue{
			" currency ": newContextChangeStringValueForTest(
				t,
				"TRY",
			),
		},
		[]string{
			"currency",
		},
	)

	requireRuntimeValidationField(
		t,
		err,
		"contextChanges",
	)
}

func TestContextChangesLookupRejectsInvalidKeys(
	t *testing.T,
) {
	changes, err := NewContextChanges(
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf(
			"NewContextChanges() returned an unexpected error: %v",
			err,
		)
	}

	t.Run("blank set lookup", func(t *testing.T) {
		_, _, err := changes.SetValue(" ")

		requireRuntimeValidationField(
			t,
			err,
			"contextChanges.key",
		)
	})

	t.Run("protected set lookup", func(t *testing.T) {
		_, _, err := changes.SetValue(
			"workflowExecutionID",
		)

		requireRuntimeValidationField(
			t,
			err,
			"contextChanges.key",
		)
	})

	t.Run("blank delete lookup", func(t *testing.T) {
		_, err := changes.Deletes(" ")

		requireRuntimeValidationField(
			t,
			err,
			"contextChanges.key",
		)
	})

	t.Run("protected delete lookup", func(t *testing.T) {
		_, err := changes.Deletes(
			"executionMode",
		)

		requireRuntimeValidationField(
			t,
			err,
			"contextChanges.key",
		)
	})
}

func TestContextChangesCopiesConstructorData(
	t *testing.T,
) {
	originalValue := newContextChangeStringValueForTest(
		t,
		"original",
	)

	setValues := map[string]RuntimeValue{
		" key ": originalValue,
	}

	deleteKeys := []string{
		" remove ",
	}

	changes, err := NewContextChanges(
		setValues,
		deleteKeys,
	)
	if err != nil {
		t.Fatalf(
			"NewContextChanges() returned an unexpected error: %v",
			err,
		)
	}

	originalValue.raw[0] = '['

	changedMapValue := setValues[" key "]
	changedMapValue.raw[0] = '['
	setValues[" key "] = changedMapValue

	delete(
		setValues,
		" key ",
	)

	deleteKeys[0] = "changed"

	actualValue, exists, err := changes.SetValue(
		"key",
	)
	if err != nil {
		t.Fatalf(
			"SetValue(key) returned an unexpected error: %v",
			err,
		)
	}

	if !exists {
		t.Fatal(
			"SetValue(key) exists = false",
		)
	}

	if actual := contextChangeStringValueForTest(
		t,
		actualValue,
	); actual != "original" {
		t.Fatalf(
			"stored set value = %q, want %q",
			actual,
			"original",
		)
	}

	deletes, err := changes.Deletes(
		"remove",
	)
	if err != nil {
		t.Fatalf(
			"Deletes(remove) returned an unexpected error: %v",
			err,
		)
	}

	if !deletes {
		t.Fatal(
			"Deletes(remove) = false",
		)
	}
}

func TestContextChangesReturnedValuesAreIndependent(
	t *testing.T,
) {
	changes, err := NewContextChanges(
		map[string]RuntimeValue{
			"key": newContextChangeStringValueForTest(
				t,
				"original",
			),
		},
		[]string{
			"remove",
		},
	)
	if err != nil {
		t.Fatalf(
			"NewContextChanges() returned an unexpected error: %v",
			err,
		)
	}

	setKeys := changes.SetKeys()
	setKeys[0] = "changed"

	deleteKeys := changes.DeleteKeys()
	deleteKeys[0] = "changed"

	setValues := changes.SetValues()
	exposedValue := setValues["key"]
	exposedValue.raw[0] = '['
	setValues["key"] = exposedValue
	delete(
		setValues,
		"key",
	)

	secondSetKeys := changes.SetKeys()
	if actual := secondSetKeys[0]; actual != "key" {
		t.Fatalf(
			"set key changed through returned slice: got %q",
			actual,
		)
	}

	secondDeleteKeys := changes.DeleteKeys()
	if actual := secondDeleteKeys[0]; actual != "remove" {
		t.Fatalf(
			"delete key changed through returned slice: got %q",
			actual,
		)
	}

	actualValue, exists, err := changes.SetValue(
		"key",
	)
	if err != nil {
		t.Fatalf(
			"SetValue(key) returned an unexpected error: %v",
			err,
		)
	}

	if !exists {
		t.Fatal(
			"SetValue(key) exists = false",
		)
	}

	if actual := contextChangeStringValueForTest(
		t,
		actualValue,
	); actual != "original" {
		t.Fatalf(
			"set value changed through returned map: got %q",
			actual,
		)
	}
}

func TestContextChangesCloneCreatesIndependentCopy(
	t *testing.T,
) {
	changes, err := NewContextChanges(
		map[string]RuntimeValue{
			"key": newContextChangeStringValueForTest(
				t,
				"original",
			),
		},
		[]string{
			"remove",
		},
	)
	if err != nil {
		t.Fatalf(
			"NewContextChanges() returned an unexpected error: %v",
			err,
		)
	}

	cloned := cloneContextChanges(changes)

	clonedSetValue := cloned.setValues["key"]
	clonedSetValue.raw[0] = '['
	cloned.setValues["key"] = clonedSetValue

	cloned.setOrder[0] = "changed"

	delete(
		cloned.deleteKeys,
		"remove",
	)

	cloned.deleteOrder[0] = "changed"

	actualValue, exists, err := changes.SetValue(
		"key",
	)
	if err != nil {
		t.Fatalf(
			"SetValue(key) returned an unexpected error: %v",
			err,
		)
	}

	if !exists {
		t.Fatal(
			"SetValue(key) exists = false",
		)
	}

	if actual := contextChangeStringValueForTest(
		t,
		actualValue,
	); actual != "original" {
		t.Fatalf(
			"original set value changed through clone: got %q",
			actual,
		)
	}

	if actual := changes.SetKeys()[0]; actual != "key" {
		t.Fatalf(
			"original set order changed through clone: got %q",
			actual,
		)
	}

	deletes, err := changes.Deletes(
		"remove",
	)
	if err != nil {
		t.Fatalf(
			"Deletes(remove) returned an unexpected error: %v",
			err,
		)
	}

	if !deletes {
		t.Fatal(
			"original delete membership changed through clone",
		)
	}

	if actual := changes.DeleteKeys()[0]; actual != "remove" {
		t.Fatalf(
			"original delete order changed through clone: got %q",
			actual,
		)
	}
}

func TestZeroContextChangesAreValidAndEmpty(
	t *testing.T,
) {
	var changes ContextChanges

	if !changes.IsValid() {
		t.Fatal(
			"zero ContextChanges IsValid() = false",
		)
	}

	if !changes.IsEmpty() {
		t.Fatal(
			"zero ContextChanges IsEmpty() = false",
		)
	}

	if changes.SetKeys() != nil {
		t.Fatal(
			"zero ContextChanges SetKeys() must return nil",
		)
	}

	if changes.SetValues() != nil {
		t.Fatal(
			"zero ContextChanges SetValues() must return nil",
		)
	}

	if changes.DeleteKeys() != nil {
		t.Fatal(
			"zero ContextChanges DeleteKeys() must return nil",
		)
	}
}

func TestContextChangesDetectsMalformedInternalState(
	t *testing.T,
) {
	validValue := newContextChangeStringValueForTest(
		t,
		"value",
	)

	tests := map[string]ContextChanges{
		"protected set key": {
			setValues: map[string]RuntimeValue{
				ProtectedKeyCompanyID: validValue,
			},
			setOrder: []string{
				ProtectedKeyCompanyID,
			},
			deleteKeys:  map[string]struct{}{},
			deleteOrder: nil,
		},
		"invalid set value": {
			setValues: map[string]RuntimeValue{
				"key": {},
			},
			setOrder: []string{
				"key",
			},
			deleteKeys:  map[string]struct{}{},
			deleteOrder: nil,
		},
		"set delete conflict": {
			setValues: map[string]RuntimeValue{
				"key": validValue,
			},
			setOrder: []string{
				"key",
			},
			deleteKeys: map[string]struct{}{
				"key": {},
			},
			deleteOrder: []string{
				"key",
			},
		},
	}

	for name, changes := range tests {
		t.Run(name, func(t *testing.T) {
			if changes.IsValid() {
				t.Fatal(
					"IsValid() = true for malformed ContextChanges",
				)
			}
		})
	}
}

func newContextChangeStringValueForTest(
	t *testing.T,
	value string,
) RuntimeValue {
	t.Helper()

	runtimeValue, err := NewRuntimeValue(
		[]byte(
			`"` + value + `"`,
		),
	)
	if err != nil {
		t.Fatalf(
			"NewRuntimeValue(%q) returned an unexpected error: %v",
			value,
			err,
		)
	}

	return runtimeValue
}

func contextChangeStringValueForTest(
	t *testing.T,
	value RuntimeValue,
) string {
	t.Helper()

	decoded, err := DecodeRuntimeValue[string](
		value,
	)
	if err != nil {
		t.Fatalf(
			"DecodeRuntimeValue() returned an unexpected error: %v",
			err,
		)
	}

	return decoded
}

func TestNewEdgeRuntimeUsesTargetPluginPolicies(
	t *testing.T,
) {
	edge := newEdgeDefinitionForRuntimeTest(
		t,
		"edge-1",
		"source-node",
		"output",
		"target-node",
		"input",
	)

	targetNode := newNodeDefinitionForRuntimeTest(
		t,
		"target-node",
		"test.target",
		"v1",
	)

	targetDescriptor := newDescriptorForRuntimeTest(
		t,
		"test.target",
		"v1",
		"input",
		2,
		3,
	)

	limits := newRuntimeLimitsForTest(
		t,
		2,
		3,
		1024,
	)

	edgeRuntime, err := NewEdgeRuntime(
		edge,
		targetNode,
		targetDescriptor,
		limits,
	)
	if err != nil {
		t.Fatalf(
			"NewEdgeRuntime() returned an unexpected error: %v",
			err,
		)
	}

	if actual := edgeRuntime.ID().String(); actual != "edge-1" {
		t.Fatalf(
			"ID() = %q, want %q",
			actual,
			"edge-1",
		)
	}

	if actual := edgeRuntime.SourceNodeID().String(); actual != "source-node" {
		t.Fatalf(
			"SourceNodeID() = %q, want %q",
			actual,
			"source-node",
		)
	}

	if actual := edgeRuntime.SourceOutputPort(); actual != "output" {
		t.Fatalf(
			"SourceOutputPort() = %q, want %q",
			actual,
			"output",
		)
	}

	if actual := edgeRuntime.TargetNodeID().String(); actual != "target-node" {
		t.Fatalf(
			"TargetNodeID() = %q, want %q",
			actual,
			"target-node",
		)
	}

	if actual := edgeRuntime.TargetInputPort(); actual != "input" {
		t.Fatalf(
			"TargetInputPort() = %q, want %q",
			actual,
			"input",
		)
	}

	if edgeRuntime.Queue() == nil {
		t.Fatal(
			"Queue() = nil",
		)
	}

	if actual := edgeRuntime.Queue().Capacity(); actual != 2 {
		t.Fatalf(
			"Queue().Capacity() = %d, want target policy capacity %d",
			actual,
			2,
		)
	}

	if edgeRuntime.Cache() == nil {
		t.Fatal(
			"Cache() = nil",
		)
	}

	if actual := edgeRuntime.Cache().Capacity(); actual != 3 {
		t.Fatalf(
			"Cache().Capacity() = %d, want target policy capacity %d",
			actual,
			3,
		)
	}
}

func TestNewEdgeRuntimeAcceptsPolicyCapacityEqualToLimit(
	t *testing.T,
) {
	_, err := NewEdgeRuntime(
		newEdgeDefinitionForRuntimeTest(
			t,
			"edge-1",
			"source-node",
			"output",
			"target-node",
			"input",
		),
		newNodeDefinitionForRuntimeTest(
			t,
			"target-node",
			"test.target",
			"v1",
		),
		newDescriptorForRuntimeTest(
			t,
			"test.target",
			"v1",
			"input",
			64,
			8,
		),
		newRuntimeLimitsForTest(
			t,
			64,
			8,
			1024,
		),
	)
	if err != nil {
		t.Fatalf(
			"NewEdgeRuntime() rejected capacities equal to limits: %v",
			err,
		)
	}
}

func TestNewEdgeRuntimeRejectsQueuePolicyAboveLimit(
	t *testing.T,
) {
	_, err := NewEdgeRuntime(
		newEdgeDefinitionForRuntimeTest(
			t,
			"edge-1",
			"source-node",
			"output",
			"target-node",
			"input",
		),
		newNodeDefinitionForRuntimeTest(
			t,
			"target-node",
			"test.target",
			"v1",
		),
		newDescriptorForRuntimeTest(
			t,
			"test.target",
			"v1",
			"input",
			65,
			1,
		),
		newRuntimeLimitsForTest(
			t,
			64,
			1,
			1024,
		),
	)

	requireRuntimeValidationField(
		t,
		err,
		"queuePolicy.defaultCapacity",
	)
}

func TestNewEdgeRuntimeRejectsCachePolicyAboveLimit(
	t *testing.T,
) {
	_, err := NewEdgeRuntime(
		newEdgeDefinitionForRuntimeTest(
			t,
			"edge-1",
			"source-node",
			"output",
			"target-node",
			"input",
		),
		newNodeDefinitionForRuntimeTest(
			t,
			"target-node",
			"test.target",
			"v1",
		),
		newDescriptorForRuntimeTest(
			t,
			"test.target",
			"v1",
			"input",
			1,
			9,
		),
		newRuntimeLimitsForTest(
			t,
			1,
			8,
			1024,
		),
	)

	requireRuntimeValidationField(
		t,
		err,
		"cachePolicy.defaultCapacity",
	)
}

func TestNewEdgeRuntimeRejectsTargetNodeMismatch(
	t *testing.T,
) {
	_, err := NewEdgeRuntime(
		newEdgeDefinitionForRuntimeTest(
			t,
			"edge-1",
			"source-node",
			"output",
			"expected-target",
			"input",
		),
		newNodeDefinitionForRuntimeTest(
			t,
			"different-target",
			"test.target",
			"v1",
		),
		newDescriptorForRuntimeTest(
			t,
			"test.target",
			"v1",
			"input",
			1,
			1,
		),
		newRuntimeLimitsForTest(
			t,
			1,
			1,
			1024,
		),
	)

	requireRuntimeValidationField(
		t,
		err,
		"edge.targetNodeID",
	)
}

func TestNewEdgeRuntimeRejectsDescriptorIdentityMismatch(
	t *testing.T,
) {
	_, err := NewEdgeRuntime(
		newEdgeDefinitionForRuntimeTest(
			t,
			"edge-1",
			"source-node",
			"output",
			"target-node",
			"input",
		),
		newNodeDefinitionForRuntimeTest(
			t,
			"target-node",
			"test.target",
			"v1",
		),
		newDescriptorForRuntimeTest(
			t,
			"test.other-target",
			"v1",
			"input",
			1,
			1,
		),
		newRuntimeLimitsForTest(
			t,
			1,
			1,
			1024,
		),
	)

	requireRuntimeValidationField(
		t,
		err,
		"targetDescriptor.identity",
	)
}

func TestNewEdgeRuntimeRejectsUnknownTargetInputPort(
	t *testing.T,
) {
	_, err := NewEdgeRuntime(
		newEdgeDefinitionForRuntimeTest(
			t,
			"edge-1",
			"source-node",
			"output",
			"target-node",
			"missing-input",
		),
		newNodeDefinitionForRuntimeTest(
			t,
			"target-node",
			"test.target",
			"v1",
		),
		newDescriptorForRuntimeTest(
			t,
			"test.target",
			"v1",
			"input",
			1,
			1,
		),
		newRuntimeLimitsForTest(
			t,
			1,
			1,
			1024,
		),
	)

	requireRuntimeValidationField(
		t,
		err,
		"edge.targetInputPort",
	)
}

func TestNewEdgeRuntimeRejectsInvalidDescriptor(
	t *testing.T,
) {
	_, err := NewEdgeRuntime(
		newEdgeDefinitionForRuntimeTest(
			t,
			"edge-1",
			"source-node",
			"output",
			"target-node",
			"input",
		),
		newNodeDefinitionForRuntimeTest(
			t,
			"target-node",
			"test.target",
			"v1",
		),
		plugin.Descriptor{},
		newRuntimeLimitsForTest(
			t,
			1,
			1,
			1024,
		),
	)

	requireRuntimeValidationField(
		t,
		err,
		"targetDescriptor",
	)
}

func TestNewEdgeRuntimeRejectsInvalidLimits(
	t *testing.T,
) {
	_, err := NewEdgeRuntime(
		newEdgeDefinitionForRuntimeTest(
			t,
			"edge-1",
			"source-node",
			"output",
			"target-node",
			"input",
		),
		newNodeDefinitionForRuntimeTest(
			t,
			"target-node",
			"test.target",
			"v1",
		),
		newDescriptorForRuntimeTest(
			t,
			"test.target",
			"v1",
			"input",
			1,
			1,
		),
		RuntimeLimits{},
	)

	requireRuntimeValidationField(
		t,
		err,
		"maximumQueueCapacity",
	)
}

func TestNewEdgeRuntimeRejectsInvalidDefinition(
	t *testing.T,
) {
	_, err := NewEdgeRuntime(
		workflow.EdgeDefinition{},
		newNodeDefinitionForRuntimeTest(
			t,
			"target-node",
			"test.target",
			"v1",
		),
		newDescriptorForRuntimeTest(
			t,
			"test.target",
			"v1",
			"input",
			1,
			1,
		),
		newRuntimeLimitsForTest(
			t,
			1,
			1,
			1024,
		),
	)

	requireRuntimeValidationField(
		t,
		err,
		"edge",
	)
}

func TestEdgeRuntimeDoesNotMutateDefinition(
	t *testing.T,
) {
	edge := newEdgeDefinitionForRuntimeTest(
		t,
		"edge-1",
		"source-node",
		"output",
		"target-node",
		"input",
	)

	edgeRuntime := newEdgeRuntimeForTest(
		t,
		edge,
	)

	pushQueuePayloadForTest(
		t,
		edgeRuntime.Queue(),
		"A",
	)

	setCacheStringForTest(
		t,
		edgeRuntime.Cache(),
		"key",
		"value",
	)

	if actual := edge.ID().String(); actual != "edge-1" {
		t.Fatalf(
			"definition ID changed: got %q",
			actual,
		)
	}

	if actual := edge.SourceNodeID().String(); actual != "source-node" {
		t.Fatalf(
			"definition source node changed: got %q",
			actual,
		)
	}

	if actual := edge.SourceOutputPort(); actual != "output" {
		t.Fatalf(
			"definition source port changed: got %q",
			actual,
		)
	}

	if actual := edge.TargetNodeID().String(); actual != "target-node" {
		t.Fatalf(
			"definition target node changed: got %q",
			actual,
		)
	}

	if actual := edge.TargetInputPort(); actual != "input" {
		t.Fatalf(
			"definition target port changed: got %q",
			actual,
		)
	}
}

func TestEdgeRuntimeInstancesHaveIsolatedState(
	t *testing.T,
) {
	edge := newEdgeDefinitionForRuntimeTest(
		t,
		"edge-1",
		"source-node",
		"output",
		"target-node",
		"input",
	)

	first := newEdgeRuntimeForTest(
		t,
		edge,
	)
	second := newEdgeRuntimeForTest(
		t,
		edge,
	)

	pushQueuePayloadForTest(
		t,
		first.Queue(),
		"A",
	)

	setCacheStringForTest(
		t,
		first.Cache(),
		"key",
		"value",
	)

	if actual := first.Queue().Len(); actual != 1 {
		t.Fatalf(
			"first Queue().Len() = %d, want 1",
			actual,
		)
	}

	if actual := second.Queue().Len(); actual != 0 {
		t.Fatalf(
			"second Queue().Len() = %d, want isolated state 0",
			actual,
		)
	}

	if actual := first.Cache().Len(); actual != 1 {
		t.Fatalf(
			"first Cache().Len() = %d, want 1",
			actual,
		)
	}

	if actual := second.Cache().Len(); actual != 0 {
		t.Fatalf(
			"second Cache().Len() = %d, want isolated state 0",
			actual,
		)
	}
}

func TestNilEdgeRuntimeAccessorsAreSafe(
	t *testing.T,
) {
	var edgeRuntime *EdgeRuntime

	if actual := edgeRuntime.ID().String(); actual != "" {
		t.Fatalf(
			"nil ID() = %q, want empty",
			actual,
		)
	}

	if actual := edgeRuntime.SourceNodeID().String(); actual != "" {
		t.Fatalf(
			"nil SourceNodeID() = %q, want empty",
			actual,
		)
	}

	if actual := edgeRuntime.SourceOutputPort(); actual != "" {
		t.Fatalf(
			"nil SourceOutputPort() = %q, want empty",
			actual,
		)
	}

	if actual := edgeRuntime.TargetNodeID().String(); actual != "" {
		t.Fatalf(
			"nil TargetNodeID() = %q, want empty",
			actual,
		)
	}

	if actual := edgeRuntime.TargetInputPort(); actual != "" {
		t.Fatalf(
			"nil TargetInputPort() = %q, want empty",
			actual,
		)
	}

	if edgeRuntime.Queue() != nil {
		t.Fatal(
			"nil Queue() must return nil",
		)
	}

	if edgeRuntime.Cache() != nil {
		t.Fatal(
			"nil Cache() must return nil",
		)
	}
}

func newEdgeRuntimeForTest(
	t *testing.T,
	edge workflow.EdgeDefinition,
) *EdgeRuntime {
	t.Helper()

	edgeRuntime, err := NewEdgeRuntime(
		edge,
		newNodeDefinitionForRuntimeTest(
			t,
			edge.TargetNodeID().String(),
			"test.target",
			"v1",
		),
		newDescriptorForRuntimeTest(
			t,
			"test.target",
			"v1",
			edge.TargetInputPort(),
			2,
			2,
		),
		newRuntimeLimitsForTest(
			t,
			2,
			2,
			1024,
		),
	)
	if err != nil {
		t.Fatalf(
			"NewEdgeRuntime() returned an unexpected error: %v",
			err,
		)
	}

	return edgeRuntime
}

func newEdgeDefinitionForRuntimeTest(
	t *testing.T,
	id string,
	sourceNodeID string,
	sourceOutputPort string,
	targetNodeID string,
	targetInputPort string,
) workflow.EdgeDefinition {
	t.Helper()

	edge, err := workflow.NewEdgeDefinition(
		workflow.EdgeID(id),
		workflow.NodeID(sourceNodeID),
		sourceOutputPort,
		workflow.NodeID(targetNodeID),
		targetInputPort,
	)
	if err != nil {
		t.Fatalf(
			"workflow.NewEdgeDefinition() returned an unexpected error: %v",
			err,
		)
	}

	return edge
}

func newNodeDefinitionForRuntimeTest(
	t *testing.T,
	id string,
	pluginType string,
	pluginVersion string,
) workflow.NodeDefinition {
	t.Helper()

	node, err := workflow.NewNodeDefinition(
		workflow.NodeID(id),
		workflow.PluginType(pluginType),
		workflow.PluginVersion(pluginVersion),
		[]byte(`{}`),
		nil,
	)
	if err != nil {
		t.Fatalf(
			"workflow.NewNodeDefinition() returned an unexpected error: %v",
			err,
		)
	}

	return node
}

func newDescriptorForRuntimeTest(
	t *testing.T,
	pluginType string,
	pluginVersion string,
	inputPortName string,
	queueCapacity uint,
	cacheCapacity uint,
) plugin.Descriptor {
	t.Helper()

	identity, err := plugin.NewPluginIdentity(
		workflow.PluginType(pluginType),
		workflow.PluginVersion(pluginVersion),
	)
	if err != nil {
		t.Fatalf(
			"plugin.NewPluginIdentity() returned an unexpected error: %v",
			err,
		)
	}

	metadata, err := plugin.NewPluginMetadata(
		"Runtime Test Plugin",
		"Used to verify EdgeRuntime policy ownership",
	)
	if err != nil {
		t.Fatalf(
			"plugin.NewPluginMetadata() returned an unexpected error: %v",
			err,
		)
	}

	inputPort, err := plugin.NewPort(
		inputPortName,
		"Input",
		"Receives runtime test payloads",
	)
	if err != nil {
		t.Fatalf(
			"plugin.NewPort() returned an unexpected error: %v",
			err,
		)
	}

	queuePolicy, err := plugin.NewQueuePolicy(
		queueCapacity,
		plugin.OverflowStrategyDropOldest,
	)
	if err != nil {
		t.Fatalf(
			"plugin.NewQueuePolicy() returned an unexpected error: %v",
			err,
		)
	}

	cachePolicy, err := plugin.NewCachePolicy(
		cacheCapacity,
		plugin.OverflowStrategyDropOldest,
	)
	if err != nil {
		t.Fatalf(
			"plugin.NewCachePolicy() returned an unexpected error: %v",
			err,
		)
	}

	descriptor, err := plugin.NewDescriptor(
		plugin.DescriptorConfig{
			Identity: identity,
			Metadata: metadata,

			InputPorts: []plugin.Port{
				inputPort,
			},
			OutputPorts: nil,

			InputEdgeConstraint: plugin.NewUnlimitedEdgeConstraint(
				0,
			),
			OutputEdgeConstraint: plugin.NewExactEdgeConstraint(
				0,
			),

			QueuePolicy: queuePolicy,
			CachePolicy: cachePolicy,

			Distribution: plugin.DistributionDistributable,

			ConfigurationValidator: func(
				workflow.JSONObject,
			) []plugin.ConfigurationIssue {
				return nil
			},
		},
	)
	if err != nil {
		t.Fatalf(
			"plugin.NewDescriptor() returned an unexpected error: %v",
			err,
		)
	}

	return descriptor
}

func TestExecutionContextStoresExecutionIdentity(
	t *testing.T,
) {
	parent := context.WithValue(
		context.Background(),
		executionContextTestKey("request"),
		"request-value",
	)

	workflowExecution, startedAt :=
		newRunningWorkflowExecutionForContextTest(
			t,
			execution.ExecutionModeSync,
		)

	executionContext, err := NewExecutionContext(
		parent,
		workflowExecution,
		" correlation-1 ",
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf(
			"NewExecutionContext() returned an unexpected error: %v",
			err,
		)
	}

	if actual := executionContext.WorkflowExecutionID().String(); actual != "workflow-execution-1" {
		t.Fatalf(
			"WorkflowExecutionID() = %q, want %q",
			actual,
			"workflow-execution-1",
		)
	}

	if actual := executionContext.CompanyID().String(); actual != "company-1" {
		t.Fatalf(
			"CompanyID() = %q, want %q",
			actual,
			"company-1",
		)
	}

	if actual := executionContext.WorkflowID().String(); actual != "workflow-1" {
		t.Fatalf(
			"WorkflowID() = %q, want %q",
			actual,
			"workflow-1",
		)
	}

	if actual := executionContext.Mode(); actual != execution.ExecutionModeSync {
		t.Fatalf(
			"Mode() = %q, want %q",
			actual,
			execution.ExecutionModeSync,
		)
	}

	if actual := executionContext.CorrelationID(); actual != "correlation-1" {
		t.Fatalf(
			"CorrelationID() = %q, want %q",
			actual,
			"correlation-1",
		)
	}

	if actual := executionContext.StartedAt(); !actual.Equal(startedAt) {
		t.Fatalf(
			"StartedAt() = %v, want %v",
			actual,
			startedAt,
		)
	}

	if actual := executionContext.Context().Value(
		executionContextTestKey("request"),
	); actual != "request-value" {
		t.Fatalf(
			"Context().Value() = %#v, want %q",
			actual,
			"request-value",
		)
	}
}

func TestExecutionContextRejectsNilParentContext(
	t *testing.T,
) {
	workflowExecution, _ :=
		newRunningWorkflowExecutionForContextTest(
			t,
			execution.ExecutionModeSync,
		)

	_, err := NewExecutionContext(
		nil,
		workflowExecution,
		"correlation-1",
		nil,
		nil,
	)

	requireRuntimeValidationField(
		t,
		err,
		"context",
	)
}

func TestExecutionContextRequiresRunningWorkflowExecution(
	t *testing.T,
) {
	workflowExecution :=
		newCreatedWorkflowExecutionForContextTest(
			t,
			execution.ExecutionModeSync,
		)

	_, err := NewExecutionContext(
		context.Background(),
		workflowExecution,
		"correlation-1",
		nil,
		nil,
	)

	requireRuntimeValidationField(
		t,
		err,
		"workflowExecution.status",
	)
}

func TestExecutionContextRejectsInvalidWorkflowExecution(
	t *testing.T,
) {
	_, err := NewExecutionContext(
		context.Background(),
		execution.WorkflowExecution{},
		"correlation-1",
		nil,
		nil,
	)

	requireRuntimeValidationField(
		t,
		err,
		"workflowExecution.id",
	)
}

func TestExecutionContextRequiresCorrelationID(
	t *testing.T,
) {
	workflowExecution, _ :=
		newRunningWorkflowExecutionForContextTest(
			t,
			execution.ExecutionModeSync,
		)

	_, err := NewExecutionContext(
		context.Background(),
		workflowExecution,
		"   ",
		nil,
		nil,
	)

	requireRuntimeValidationField(
		t,
		err,
		"correlationID",
	)
}

func TestExecutionContextProtectedMetadataIsIndependent(
	t *testing.T,
) {
	executionContext :=
		newExecutionContextForContextTest(
			t,
			context.Background(),
			nil,
			nil,
		)

	metadata := executionContext.ProtectedMetadata()

	expected := map[string]string{
		ProtectedKeyCompanyID:           "company-1",
		ProtectedKeyWorkflowID:          "workflow-1",
		ProtectedKeyWorkflowExecutionID: "workflow-execution-1",
		ProtectedKeyExecutionMode:       "SYNC",
		ProtectedKeyCorrelationID:       "correlation-1",
	}

	if len(metadata) != len(expected) {
		t.Fatalf(
			"len(ProtectedMetadata()) = %d, want %d",
			len(metadata),
			len(expected),
		)
	}

	for key, expectedValue := range expected {
		if actual := metadata[key]; actual != expectedValue {
			t.Fatalf(
				"ProtectedMetadata()[%q] = %q, want %q",
				key,
				actual,
				expectedValue,
			)
		}
	}

	metadata[ProtectedKeyCompanyID] =
		"changed-company"

	secondMetadata :=
		executionContext.ProtectedMetadata()

	if actual := secondMetadata[ProtectedKeyCompanyID]; actual != "company-1" {
		t.Fatalf(
			"protected metadata changed through returned map: got %q",
			actual,
		)
	}
}

func TestExecutionContextRegistersEdgesInDeterministicOrder(
	t *testing.T,
) {
	edgeB := newEdgeRuntimeForTest(
		t,
		newEdgeDefinitionForRuntimeTest(
			t,
			"edge-b",
			"source-b",
			"output",
			"target-node",
			"input",
		),
	)

	edgeA := newEdgeRuntimeForTest(
		t,
		newEdgeDefinitionForRuntimeTest(
			t,
			"edge-a",
			"source-a",
			"output",
			"target-node",
			"input",
		),
	)

	executionContext :=
		newExecutionContextForContextTest(
			t,
			context.Background(),
			[]*EdgeRuntime{
				edgeB,
				edgeA,
			},
			nil,
		)

	edges := executionContext.Edges()

	if len(edges) != 2 {
		t.Fatalf(
			"len(Edges()) = %d, want 2",
			len(edges),
		)
	}

	if actual := edges[0].ID().String(); actual != "edge-a" {
		t.Fatalf(
			"Edges()[0].ID() = %q, want %q",
			actual,
			"edge-a",
		)
	}

	if actual := edges[1].ID().String(); actual != "edge-b" {
		t.Fatalf(
			"Edges()[1].ID() = %q, want %q",
			actual,
			"edge-b",
		)
	}

	actualEdge, exists, err :=
		executionContext.Edge(
			workflow.EdgeID(" edge-a "),
		)
	if err != nil {
		t.Fatalf(
			"Edge() returned an unexpected error: %v",
			err,
		)
	}

	if !exists {
		t.Fatal(
			"Edge(edge-a) exists = false",
		)
	}

	if actualEdge != edgeA {
		t.Fatal(
			"Edge(edge-a) did not return the registered runtime edge",
		)
	}

	edges[0] = nil

	secondEdges := executionContext.Edges()
	if secondEdges[0] == nil {
		t.Fatal(
			"edge registry changed through Edges() slice",
		)
	}
}

func TestExecutionContextEdgeHandlesMissingAndInvalidIDs(
	t *testing.T,
) {
	executionContext :=
		newExecutionContextForContextTest(
			t,
			context.Background(),
			nil,
			nil,
		)

	_, exists, err := executionContext.Edge(
		workflow.EdgeID("missing-edge"),
	)
	if err != nil {
		t.Fatalf(
			"Edge(missing-edge) returned an unexpected error: %v",
			err,
		)
	}

	if exists {
		t.Fatal(
			"Edge(missing-edge) exists = true",
		)
	}

	_, _, err = executionContext.Edge(
		workflow.EdgeID(" "),
	)

	requireRuntimeValidationField(
		t,
		err,
		"edgeID",
	)
}

func TestExecutionContextRejectsDuplicateEdges(
	t *testing.T,
) {
	edgeRuntime := newEdgeRuntimeForTest(
		t,
		newEdgeDefinitionForRuntimeTest(
			t,
			"edge-1",
			"source-node",
			"output",
			"target-node",
			"input",
		),
	)

	workflowExecution, _ :=
		newRunningWorkflowExecutionForContextTest(
			t,
			execution.ExecutionModeSync,
		)

	_, err := NewExecutionContext(
		context.Background(),
		workflowExecution,
		"correlation-1",
		[]*EdgeRuntime{
			edgeRuntime,
			edgeRuntime,
		},
		nil,
	)

	requireRuntimeValidationField(
		t,
		err,
		"edges",
	)
}

func TestExecutionContextRejectsNilEdge(
	t *testing.T,
) {
	workflowExecution, _ :=
		newRunningWorkflowExecutionForContextTest(
			t,
			execution.ExecutionModeSync,
		)

	_, err := NewExecutionContext(
		context.Background(),
		workflowExecution,
		"correlation-1",
		[]*EdgeRuntime{
			nil,
		},
		nil,
	)

	requireRuntimeValidationField(
		t,
		err,
		"edges[0]",
	)
}

func TestExecutionContextCopiesInitialVariables(
	t *testing.T,
) {
	initialValue :=
		newRuntimeValueForExecutionContextTest(
			t,
			`{"count":1}`,
		)

	initialVariables := map[string]RuntimeValue{
		" counter ": initialValue,
	}

	executionContext :=
		newExecutionContextForContextTest(
			t,
			context.Background(),
			nil,
			initialVariables,
		)

	initialValue.raw[0] = '['
	delete(
		initialVariables,
		" counter ",
	)

	actual, exists, err :=
		executionContext.Variable("counter")
	if err != nil {
		t.Fatalf(
			"Variable(counter) returned an unexpected error: %v",
			err,
		)
	}

	if !exists {
		t.Fatal(
			"Variable(counter) exists = false",
		)
	}

	if actual.String() != `{"count":1}` {
		t.Fatalf(
			"Variable(counter) = %q, want %q",
			actual.String(),
			`{"count":1}`,
		)
	}

	actual.raw[0] = '['

	second, exists, err :=
		executionContext.Variable("counter")
	if err != nil {
		t.Fatalf(
			"second Variable(counter) returned an unexpected error: %v",
			err,
		)
	}

	if !exists {
		t.Fatal(
			"second Variable(counter) exists = false",
		)
	}

	if second.String() != `{"count":1}` {
		t.Fatalf(
			"variable changed through returned value: got %q",
			second.String(),
		)
	}
}

func TestExecutionContextRejectsInvalidInitialVariables(
	t *testing.T,
) {
	validValue :=
		newRuntimeValueForExecutionContextTest(
			t,
			`"value"`,
		)

	tests := []struct {
		name      string
		variables map[string]RuntimeValue
		field     string
	}{
		{
			name: "blank key",
			variables: map[string]RuntimeValue{
				" ": validValue,
			},
			field: "initialVariables.key",
		},
		{
			name: "protected key",
			variables: map[string]RuntimeValue{
				" CompanyID ": validValue,
			},
			field: "initialVariables.key",
		},
		{
			name: "duplicate normalized key",
			variables: map[string]RuntimeValue{
				"key":   validValue,
				" key ": validValue,
			},
			field: "initialVariables",
		},
		{
			name: "invalid value",
			variables: map[string]RuntimeValue{
				"key": {},
			},
			field: "initialVariables.value",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			workflowExecution, _ :=
				newRunningWorkflowExecutionForContextTest(
					t,
					execution.ExecutionModeSync,
				)

			_, err := NewExecutionContext(
				context.Background(),
				workflowExecution,
				"correlation-1",
				nil,
				test.variables,
			)

			requireRuntimeValidationField(
				t,
				err,
				test.field,
			)
		})
	}
}

func TestExecutionContextSetGetAndDeleteVariable(
	t *testing.T,
) {
	executionContext :=
		newExecutionContextForContextTest(
			t,
			context.Background(),
			nil,
			nil,
		)

	value :=
		newRuntimeValueForExecutionContextTest(
			t,
			`{"currency":"TRY"}`,
		)

	if err := executionContext.SetVariable(
		" currency ",
		value,
	); err != nil {
		t.Fatalf(
			"SetVariable() returned an unexpected error: %v",
			err,
		)
	}

	value.raw[0] = '['

	actual, exists, err :=
		executionContext.Variable("currency")
	if err != nil {
		t.Fatalf(
			"Variable() returned an unexpected error: %v",
			err,
		)
	}

	if !exists {
		t.Fatal(
			"Variable() exists = false",
		)
	}

	if actual.String() != `{"currency":"TRY"}` {
		t.Fatalf(
			"Variable() = %q, want %q",
			actual.String(),
			`{"currency":"TRY"}`,
		)
	}

	deleted, exists, err :=
		executionContext.DeleteVariable(
			" currency ",
		)
	if err != nil {
		t.Fatalf(
			"DeleteVariable() returned an unexpected error: %v",
			err,
		)
	}

	if !exists {
		t.Fatal(
			"DeleteVariable() exists = false",
		)
	}

	if deleted.String() != `{"currency":"TRY"}` {
		t.Fatalf(
			"deleted variable = %q, want %q",
			deleted.String(),
			`{"currency":"TRY"}`,
		)
	}

	_, exists, err =
		executionContext.Variable("currency")
	if err != nil {
		t.Fatalf(
			"Variable() after delete returned an unexpected error: %v",
			err,
		)
	}

	if exists {
		t.Fatal(
			"Variable() exists = true after delete",
		)
	}

	_, exists, err =
		executionContext.DeleteVariable("currency")
	if err != nil {
		t.Fatalf(
			"second DeleteVariable() returned an unexpected error: %v",
			err,
		)
	}

	if exists {
		t.Fatal(
			"second DeleteVariable() exists = true",
		)
	}
}

func TestExecutionContextRejectsInvalidVariableOperations(
	t *testing.T,
) {
	executionContext :=
		newExecutionContextForContextTest(
			t,
			context.Background(),
			nil,
			nil,
		)

	validValue :=
		newRuntimeValueForExecutionContextTest(
			t,
			`"value"`,
		)

	t.Run("blank set key", func(t *testing.T) {
		err := executionContext.SetVariable(
			" ",
			validValue,
		)

		requireRuntimeValidationField(
			t,
			err,
			"variable.key",
		)
	})

	t.Run("protected set key", func(t *testing.T) {
		err := executionContext.SetVariable(
			"WORKFLOWid",
			validValue,
		)

		requireRuntimeValidationField(
			t,
			err,
			"variable.key",
		)
	})

	t.Run("invalid set value", func(t *testing.T) {
		err := executionContext.SetVariable(
			"key",
			RuntimeValue{},
		)

		requireRuntimeValidationField(
			t,
			err,
			"variable.value",
		)
	})

	t.Run("protected get key", func(t *testing.T) {
		_, _, err := executionContext.Variable(
			"correlationID",
		)

		requireRuntimeValidationField(
			t,
			err,
			"variable.key",
		)
	})

	t.Run("protected delete key", func(t *testing.T) {
		_, _, err :=
			executionContext.DeleteVariable(
				"executionMode",
			)

		requireRuntimeValidationField(
			t,
			err,
			"variable.key",
		)
	})
}

func TestExecutionContextVariableSnapshotIsIndependent(
	t *testing.T,
) {
	executionContext :=
		newExecutionContextForContextTest(
			t,
			context.Background(),
			nil,
			map[string]RuntimeValue{
				"first": newRuntimeValueForExecutionContextTest(
					t,
					`{"value":1}`,
				),
				"second": newRuntimeValueForExecutionContextTest(
					t,
					`{"value":2}`,
				),
			},
		)

	snapshot :=
		executionContext.VariablesSnapshot()

	if len(snapshot) != 2 {
		t.Fatalf(
			"len(VariablesSnapshot()) = %d, want 2",
			len(snapshot),
		)
	}

	first := snapshot["first"]
	first.raw[0] = '['
	snapshot["first"] = first

	delete(
		snapshot,
		"second",
	)

	snapshot["third"] =
		newRuntimeValueForExecutionContextTest(
			t,
			`{"value":3}`,
		)

	actualFirst, exists, err :=
		executionContext.Variable("first")
	if err != nil {
		t.Fatalf(
			"Variable(first) returned an unexpected error: %v",
			err,
		)
	}

	if !exists {
		t.Fatal(
			"Variable(first) exists = false",
		)
	}

	if actualFirst.String() != `{"value":1}` {
		t.Fatalf(
			"first variable changed through snapshot: got %q",
			actualFirst.String(),
		)
	}

	if _, exists, err :=
		executionContext.Variable("second"); err != nil {
		t.Fatalf(
			"Variable(second) returned an unexpected error: %v",
			err,
		)
	} else if !exists {
		t.Fatal(
			"second variable was deleted through snapshot",
		)
	}

	if _, exists, err :=
		executionContext.Variable("third"); err != nil {
		t.Fatalf(
			"Variable(third) returned an unexpected error: %v",
			err,
		)
	} else if exists {
		t.Fatal(
			"third variable was added through snapshot",
		)
	}
}

func TestExecutionContextPropagatesCancellation(
	t *testing.T,
) {
	parent, cancel :=
		context.WithCancel(
			context.Background(),
		)

	executionContext :=
		newExecutionContextForContextTest(
			t,
			parent,
			nil,
			nil,
		)

	cancel()

	select {
	case <-executionContext.Done():
	default:
		t.Fatal(
			"Done() was not closed after parent cancellation",
		)
	}

	if !errors.Is(
		executionContext.Err(),
		context.Canceled,
	) {
		t.Fatalf(
			"Err() = %v, want context.Canceled",
			executionContext.Err(),
		)
	}
}

func TestExecutionContextPropagatesDeadline(
	t *testing.T,
) {
	expectedDeadline := time.Now().
		Add(time.Hour).
		Round(0)

	parent, cancel := context.WithDeadline(
		context.Background(),
		expectedDeadline,
	)
	defer cancel()

	executionContext :=
		newExecutionContextForContextTest(
			t,
			parent,
			nil,
			nil,
		)

	actualDeadline, exists :=
		executionContext.Deadline()

	if !exists {
		t.Fatal(
			"Deadline() exists = false",
		)
	}

	if !actualDeadline.Equal(expectedDeadline) {
		t.Fatalf(
			"Deadline() = %v, want %v",
			actualDeadline,
			expectedDeadline,
		)
	}
}

func TestExecutionContextConcurrentVariableAccess(
	t *testing.T,
) {
	const (
		writerCount  = 8
		readerCount  = 4
		deleterCount = 2
		operations   = 200
	)

	executionContext :=
		newExecutionContextForContextTest(
			t,
			context.Background(),
			nil,
			nil,
		)

	errorChannel := make(
		chan error,
		(writerCount+readerCount+deleterCount)*operations,
	)

	var waitGroup sync.WaitGroup

	for writer := 0; writer < writerCount; writer++ {
		writerID := writer

		waitGroup.Add(1)

		go func() {
			defer waitGroup.Done()

			for operation := 0; operation < operations; operation++ {
				key := fmt.Sprintf(
					"writer-%d-key-%d",
					writerID,
					operation,
				)

				value, err := NewRuntimeValue(
					[]byte(
						fmt.Sprintf(
							`{"writer":%d,"operation":%d}`,
							writerID,
							operation,
						),
					),
				)
				if err != nil {
					errorChannel <- err
					continue
				}

				if err := executionContext.SetVariable(
					key,
					value,
				); err != nil {
					errorChannel <- err
				}
			}
		}()
	}

	for reader := 0; reader < readerCount; reader++ {
		readerID := reader

		waitGroup.Add(1)

		go func() {
			defer waitGroup.Done()

			for operation := 0; operation < operations; operation++ {
				key := fmt.Sprintf(
					"writer-%d-key-%d",
					readerID%writerCount,
					operation,
				)

				if _, _, err :=
					executionContext.Variable(key); err != nil {
					errorChannel <- err
				}

				executionContext.VariablesSnapshot()
			}
		}()
	}

	for deleter := 0; deleter < deleterCount; deleter++ {
		deleterID := deleter

		waitGroup.Add(1)

		go func() {
			defer waitGroup.Done()

			for operation := 0; operation < operations; operation++ {
				key := fmt.Sprintf(
					"writer-%d-key-%d",
					deleterID,
					operation,
				)

				if _, _, err :=
					executionContext.DeleteVariable(key); err != nil {
					errorChannel <- err
				}
			}
		}()
	}

	waitGroup.Wait()
	close(errorChannel)

	for err := range errorChannel {
		t.Errorf(
			"concurrent variable operation returned an error: %v",
			err,
		)
	}

	for key, value := range executionContext.VariablesSnapshot() {
		if key == "" {
			t.Fatal(
				"VariablesSnapshot() contains an empty key",
			)
		}

		if !value.IsValid() {
			t.Fatalf(
				"VariablesSnapshot()[%q] is invalid",
				key,
			)
		}
	}
}

func TestNilExecutionContextMethodsAreSafe(
	t *testing.T,
) {
	var executionContext *ExecutionContext

	if executionContext.Context() != nil {
		t.Fatal(
			"nil Context() must return nil",
		)
	}

	if executionContext.Done() != nil {
		t.Fatal(
			"nil Done() must return nil",
		)
	}

	if executionContext.Err() != nil {
		t.Fatal(
			"nil Err() must return nil",
		)
	}

	if _, exists := executionContext.Deadline(); exists {
		t.Fatal(
			"nil Deadline() exists = true",
		)
	}

	if executionContext.WorkflowExecutionID().String() != "" {
		t.Fatal(
			"nil WorkflowExecutionID() must be empty",
		)
	}

	if executionContext.CompanyID().String() != "" {
		t.Fatal(
			"nil CompanyID() must be empty",
		)
	}

	if executionContext.WorkflowID().String() != "" {
		t.Fatal(
			"nil WorkflowID() must be empty",
		)
	}

	if executionContext.Mode().String() != "" {
		t.Fatal(
			"nil Mode() must be empty",
		)
	}

	if executionContext.CorrelationID() != "" {
		t.Fatal(
			"nil CorrelationID() must be empty",
		)
	}

	if !executionContext.StartedAt().IsZero() {
		t.Fatal(
			"nil StartedAt() must be zero",
		)
	}

	if executionContext.ProtectedMetadata() != nil {
		t.Fatal(
			"nil ProtectedMetadata() must return nil",
		)
	}

	if executionContext.Edges() != nil {
		t.Fatal(
			"nil Edges() must return nil",
		)
	}

	if executionContext.VariablesSnapshot() != nil {
		t.Fatal(
			"nil VariablesSnapshot() must return nil",
		)
	}

	err := executionContext.SetVariable(
		"key",
		newRuntimeValueForExecutionContextTest(
			t,
			`"value"`,
		),
	)

	requireRuntimeValidationField(
		t,
		err,
		"executionContext",
	)

	_, _, err = executionContext.Variable("key")

	requireRuntimeValidationField(
		t,
		err,
		"executionContext",
	)

	_, _, err =
		executionContext.DeleteVariable("key")

	requireRuntimeValidationField(
		t,
		err,
		"executionContext",
	)

	_, _, err =
		executionContext.Edge(
			workflow.EdgeID("edge-1"),
		)

	requireRuntimeValidationField(
		t,
		err,
		"executionContext",
	)
}

type executionContextTestKey string

func newExecutionContextForContextTest(
	t *testing.T,
	parent context.Context,
	edges []*EdgeRuntime,
	initialVariables map[string]RuntimeValue,
) *ExecutionContext {
	t.Helper()

	workflowExecution, _ :=
		newRunningWorkflowExecutionForContextTest(
			t,
			execution.ExecutionModeSync,
		)

	executionContext, err := NewExecutionContext(
		parent,
		workflowExecution,
		"correlation-1",
		edges,
		initialVariables,
	)
	if err != nil {
		t.Fatalf(
			"NewExecutionContext() returned an unexpected error: %v",
			err,
		)
	}

	return executionContext
}

func newCreatedWorkflowExecutionForContextTest(
	t *testing.T,
	mode execution.ExecutionMode,
) execution.WorkflowExecution {
	t.Helper()

	workflowExecution, err :=
		execution.NewWorkflowExecution(
			execution.WorkflowExecutionID(
				" workflow-execution-1 ",
			),
			workflow.CompanyID(
				" company-1 ",
			),
			workflow.WorkflowID(
				" workflow-1 ",
			),
			1,
			mode,
			executionContextTestTime(
				10,
				0,
			),
		)
	if err != nil {
		t.Fatalf(
			"execution.NewWorkflowExecution() returned an unexpected error: %v",
			err,
		)
	}

	return workflowExecution
}

func newRunningWorkflowExecutionForContextTest(
	t *testing.T,
	mode execution.ExecutionMode,
) (
	execution.WorkflowExecution,
	time.Time,
) {
	t.Helper()

	workflowExecution :=
		newCreatedWorkflowExecutionForContextTest(
			t,
			mode,
		)

	if err := workflowExecution.StartValidation(
		executionContextTestTime(
			10,
			1,
		),
	); err != nil {
		t.Fatalf(
			"StartValidation() returned an unexpected error: %v",
			err,
		)
	}

	startedAt :=
		executionContextTestTime(
			10,
			2,
		)

	if err := workflowExecution.Start(
		startedAt,
	); err != nil {
		t.Fatalf(
			"Start() returned an unexpected error: %v",
			err,
		)
	}

	return workflowExecution, startedAt
}

func newRuntimeValueForExecutionContextTest(
	t *testing.T,
	value string,
) RuntimeValue {
	t.Helper()

	runtimeValue, err := NewRuntimeValue(
		[]byte(value),
	)
	if err != nil {
		t.Fatalf(
			"NewRuntimeValue(%q) returned an unexpected error: %v",
			value,
			err,
		)
	}

	return runtimeValue
}

func executionContextTestTime(
	hour int,
	minute int,
) time.Time {
	return time.Date(
		2026,
		time.July,
		16,
		hour,
		minute,
		0,
		0,
		time.UTC,
	)
}

func TestNewExecutorRegistrationStoresNormalizedIdentityAndExecutor(
	t *testing.T,
) {
	identity := mustExecutorRegistryIdentity(
		t,
		" core.pass-through ",
		" v1 ",
	)

	executor := &executorRegistryTestExecutor{
		name: "pass-through",
	}

	registration, err := NewExecutorRegistration(
		identity,
		executor,
	)
	if err != nil {
		t.Fatalf(
			"NewExecutorRegistration() returned an unexpected error: %v",
			err,
		)
	}

	if actual := registration.Identity().String(); actual != "core.pass-through@v1" {
		t.Fatalf(
			"Identity() = %q, want %q",
			actual,
			"core.pass-through@v1",
		)
	}

	actualExecutor, ok :=
		registration.Executor().(*executorRegistryTestExecutor)

	if !ok {
		t.Fatalf(
			"Executor() type = %T, want *executorRegistryTestExecutor",
			registration.Executor(),
		)
	}

	if actualExecutor != executor {
		t.Fatal(
			"Executor() returned a different executor instance",
		)
	}
}

func TestNewExecutorRegistrationRejectsInvalidIdentity(
	t *testing.T,
) {
	_, err := NewExecutorRegistration(
		plugin.PluginIdentity{},
		&executorRegistryTestExecutor{
			name: "executor",
		},
	)

	requireRuntimeValidationField(
		t,
		err,
		"executor.identity",
	)
}

func TestNewExecutorRegistrationRejectsNilExecutors(
	t *testing.T,
) {
	identity := mustExecutorRegistryIdentity(
		t,
		"core.pass-through",
		"v1",
	)

	t.Run("nil interface", func(t *testing.T) {
		var executor NodeExecutor

		_, err := NewExecutorRegistration(
			identity,
			executor,
		)

		requireRuntimeValidationField(
			t,
			err,
			"executor",
		)
	})

}

func TestNewExecutorRegistryAllowsEmptyRegistry(
	t *testing.T,
) {
	registry, err := NewExecutorRegistry(nil)
	if err != nil {
		t.Fatalf(
			"NewExecutorRegistry() returned an unexpected error: %v",
			err,
		)
	}

	if !registry.IsEmpty() {
		t.Fatal(
			"IsEmpty() = false for an empty registry",
		)
	}

	if actual := registry.Len(); actual != 0 {
		t.Fatalf(
			"Len() = %d, want 0",
			actual,
		)
	}

	if identities := registry.Identities(); identities != nil {
		t.Fatalf(
			"Identities() = %#v, want nil",
			identities,
		)
	}

	_, exists := registry.Lookup(
		mustExecutorRegistryIdentity(
			t,
			"core.pass-through",
			"v1",
		),
	)

	if exists {
		t.Fatal(
			"Lookup() exists = true in an empty registry",
		)
	}
}

func TestExecutorRegistrySortsIdentitiesDeterministically(
	t *testing.T,
) {
	registry := mustExecutorRegistry(
		t,
		executorRegistryTestRegistration(
			t,
			"core.z-node",
			"v1",
			"z-v1",
		),
		executorRegistryTestRegistration(
			t,
			"core.a-node",
			"v2",
			"a-v2",
		),
		executorRegistryTestRegistration(
			t,
			"core.a-node",
			"v1",
			"a-v1",
		),
	)

	actualIdentities := registry.Identities()

	actual := make(
		[]string,
		len(actualIdentities),
	)

	for index, identity := range actualIdentities {
		actual[index] = identity.String()
	}

	expected := []string{
		"core.a-node@v1",
		"core.a-node@v2",
		"core.z-node@v1",
	}

	if !reflect.DeepEqual(actual, expected) {
		t.Fatalf(
			"Identities() = %#v, want %#v",
			actual,
			expected,
		)
	}
}

func TestExecutorRegistryLooksUpByIdentity(
	t *testing.T,
) {
	expectedExecutor := &executorRegistryTestExecutor{
		name: "pass-through",
	}

	registration, err := NewExecutorRegistration(
		mustExecutorRegistryIdentity(
			t,
			"core.pass-through",
			"v1",
		),
		expectedExecutor,
	)
	if err != nil {
		t.Fatalf(
			"NewExecutorRegistration() returned an unexpected error: %v",
			err,
		)
	}

	registry := mustExecutorRegistry(
		t,
		registration,
	)

	actualExecutor, exists := registry.Lookup(
		mustExecutorRegistryIdentity(
			t,
			" core.pass-through ",
			" v1 ",
		),
	)

	if !exists {
		t.Fatal(
			"Lookup() exists = false",
		)
	}

	actual, ok :=
		actualExecutor.(*executorRegistryTestExecutor)

	if !ok {
		t.Fatalf(
			"Lookup() executor type = %T, want *executorRegistryTestExecutor",
			actualExecutor,
		)
	}

	if actual != expectedExecutor {
		t.Fatal(
			"Lookup() returned a different executor instance",
		)
	}
}

func TestExecutorRegistryLooksUpByTypeAndVersion(
	t *testing.T,
) {
	expectedExecutor := &executorRegistryTestExecutor{
		name: "delay",
	}

	registry := mustExecutorRegistry(
		t,
		mustExecutorRegistration(
			t,
			mustExecutorRegistryIdentity(
				t,
				"core.delay",
				"v1",
			),
			expectedExecutor,
		),
	)

	actualExecutor, exists :=
		registry.LookupByTypeAndVersion(
			workflow.PluginType(" core.delay "),
			workflow.PluginVersion(" v1 "),
		)

	if !exists {
		t.Fatal(
			"LookupByTypeAndVersion() exists = false",
		)
	}

	actual, ok :=
		actualExecutor.(*executorRegistryTestExecutor)

	if !ok {
		t.Fatalf(
			"LookupByTypeAndVersion() executor type = %T, want *executorRegistryTestExecutor",
			actualExecutor,
		)
	}

	if actual != expectedExecutor {
		t.Fatal(
			"LookupByTypeAndVersion() returned a different executor instance",
		)
	}
}

func TestExecutorRegistryHandlesMissingAndInvalidLookups(
	t *testing.T,
) {
	registry := mustExecutorRegistry(
		t,
		executorRegistryTestRegistration(
			t,
			"core.pass-through",
			"v1",
			"pass-through",
		),
	)

	t.Run("missing identity", func(t *testing.T) {
		_, exists := registry.Lookup(
			mustExecutorRegistryIdentity(
				t,
				"core.pass-through",
				"v2",
			),
		)

		if exists {
			t.Fatal(
				"Lookup() exists = true for a missing identity",
			)
		}
	})

	t.Run("invalid identity", func(t *testing.T) {
		_, exists := registry.Lookup(
			plugin.PluginIdentity{},
		)

		if exists {
			t.Fatal(
				"Lookup() exists = true for an invalid identity",
			)
		}
	})

	t.Run("invalid type and version", func(t *testing.T) {
		_, exists :=
			registry.LookupByTypeAndVersion(
				workflow.PluginType(" "),
				workflow.PluginVersion(" "),
			)

		if exists {
			t.Fatal(
				"LookupByTypeAndVersion() exists = true for invalid values",
			)
		}
	})
}

func TestNewExecutorRegistryRejectsDuplicateIdentity(
	t *testing.T,
) {
	identity := mustExecutorRegistryIdentity(
		t,
		"core.pass-through",
		"v1",
	)

	_, err := NewExecutorRegistry(
		[]ExecutorRegistration{
			mustExecutorRegistration(
				t,
				identity,
				&executorRegistryTestExecutor{
					name: "first",
				},
			),
			mustExecutorRegistration(
				t,
				identity,
				&executorRegistryTestExecutor{
					name: "second",
				},
			),
		},
	)

	if err == nil {
		t.Fatal(
			"NewExecutorRegistry() returned nil error for duplicate identity",
		)
	}

	var duplicateError *DuplicateExecutorError

	if !errors.As(err, &duplicateError) {
		t.Fatalf(
			"error type = %T, want *DuplicateExecutorError",
			err,
		)
	}

	if actual := duplicateError.Identity.String(); actual != "core.pass-through@v1" {
		t.Fatalf(
			"duplicate identity = %q, want %q",
			actual,
			"core.pass-through@v1",
		)
	}

	expected :=
		"executor registry contains duplicate executor core.pass-through@v1"

	if actual := err.Error(); actual != expected {
		t.Fatalf(
			"Error() = %q, want %q",
			actual,
			expected,
		)
	}
}

func TestNewExecutorRegistryRejectsMalformedRegistration(
	t *testing.T,
) {
	_, err := NewExecutorRegistry(
		[]ExecutorRegistration{
			{},
		},
	)

	requireRuntimeValidationField(
		t,
		err,
		"registrations[0]",
	)
}

func TestExecutorRegistryIdentitiesAreIndependent(
	t *testing.T,
) {
	registry := mustExecutorRegistry(
		t,
		executorRegistryTestRegistration(
			t,
			"core.delay",
			"v1",
			"delay",
		),
		executorRegistryTestRegistration(
			t,
			"core.pass-through",
			"v1",
			"pass-through",
		),
	)

	first := registry.Identities()

	first[0] = plugin.PluginIdentity{}

	second := registry.Identities()

	expected := []string{
		"core.delay@v1",
		"core.pass-through@v1",
	}

	actual := make(
		[]string,
		len(second),
	)

	for index, identity := range second {
		actual[index] = identity.String()
	}

	if !reflect.DeepEqual(actual, expected) {
		t.Fatalf(
			"registry identities changed through returned slice: got %#v, want %#v",
			actual,
			expected,
		)
	}
}

func TestExecutorRegistryCopiesRegistrationSliceMembership(
	t *testing.T,
) {
	firstRegistration :=
		executorRegistryTestRegistration(
			t,
			"core.delay",
			"v1",
			"delay",
		)

	registrations := []ExecutorRegistration{
		firstRegistration,
	}

	registry := mustExecutorRegistry(
		t,
		registrations...,
	)

	registrations[0] =
		executorRegistryTestRegistration(
			t,
			"core.terminal",
			"v1",
			"terminal",
		)

	if _, exists := registry.Lookup(
		firstRegistration.Identity(),
	); !exists {
		t.Fatal(
			"registry membership changed through constructor slice mutation",
		)
	}

	if _, exists :=
		registry.LookupByTypeAndVersion(
			workflow.PluginType("core.terminal"),
			workflow.PluginVersion("v1"),
		); exists {
		t.Fatal(
			"registry unexpectedly contains replacement registration",
		)
	}
}

func TestExecutorRegistrySupportsConcurrentReads(
	t *testing.T,
) {
	registry := mustExecutorRegistry(
		t,
		executorRegistryTestRegistration(
			t,
			"core.delay",
			"v1",
			"delay",
		),
		executorRegistryTestRegistration(
			t,
			"core.pass-through",
			"v1",
			"pass-through",
		),
		executorRegistryTestRegistration(
			t,
			"core.static-input",
			"v1",
			"static-input",
		),
		executorRegistryTestRegistration(
			t,
			"core.terminal",
			"v1",
			"terminal",
		),
	)

	const (
		readerCount = 32
		iterations  = 100
	)

	var waitGroup sync.WaitGroup

	errorsChannel := make(
		chan string,
		readerCount,
	)

	for reader := 0; reader < readerCount; reader++ {
		waitGroup.Add(1)

		go func() {
			defer waitGroup.Done()

			for iteration := 0; iteration < iterations; iteration++ {
				if registry.Len() != 4 {
					errorsChannel <- "unexpected registry length"
					return
				}

				if registry.IsEmpty() {
					errorsChannel <- "registry unexpectedly empty"
					return
				}

				identities := registry.Identities()
				if len(identities) != 4 {
					errorsChannel <- "unexpected identity count"
					return
				}

				for _, identity := range identities {
					executor, exists :=
						registry.Lookup(identity)

					if !exists {
						errorsChannel <- "registered identity was not found"
						return
					}

					if executor == nil {
						errorsChannel <- "lookup returned nil executor"
						return
					}
				}
			}
		}()
	}

	waitGroup.Wait()
	close(errorsChannel)

	for message := range errorsChannel {
		t.Fatal(message)
	}
}

func TestZeroExecutorRegistryMethodsAreSafe(
	t *testing.T,
) {
	var registry ExecutorRegistry

	if actual := registry.Len(); actual != 0 {
		t.Fatalf(
			"zero registry Len() = %d, want 0",
			actual,
		)
	}

	if !registry.IsEmpty() {
		t.Fatal(
			"zero registry IsEmpty() = false",
		)
	}

	if registry.Identities() != nil {
		t.Fatal(
			"zero registry Identities() must return nil",
		)
	}

	_, exists := registry.Lookup(
		mustExecutorRegistryIdentity(
			t,
			"core.pass-through",
			"v1",
		),
	)

	if exists {
		t.Fatal(
			"zero registry Lookup() exists = true",
		)
	}

	_, exists =
		registry.LookupByTypeAndVersion(
			workflow.PluginType("core.pass-through"),
			workflow.PluginVersion("v1"),
		)

	if exists {
		t.Fatal(
			"zero registry LookupByTypeAndVersion() exists = true",
		)
	}
}

func TestDuplicateExecutorErrorFallbackFormatting(
	t *testing.T,
) {
	var nilError *DuplicateExecutorError

	if actual := nilError.Error(); actual !=
		"executor registry contains a duplicate executor" {
		t.Fatalf(
			"nil Error() = %q",
			actual,
		)
	}

	zeroError := &DuplicateExecutorError{}

	if actual := zeroError.Error(); actual !=
		"executor registry contains a duplicate executor" {
		t.Fatalf(
			"zero Error() = %q",
			actual,
		)
	}
}

type executorRegistryTestExecutor struct {
	name string
}

func (executor *executorRegistryTestExecutor) IsValid() bool {
	return executor != nil
}

func (executor *executorRegistryTestExecutor) Execute(
	*NodeExecutionContext,
	NodeInput,
	workflow.JSONObject,
) (NodeResult, error) {
	return NewNodeSuccessResult(
		nil,
		ContextChanges{},
	)
}

func mustExecutorRegistry(
	t *testing.T,
	registrations ...ExecutorRegistration,
) ExecutorRegistry {
	t.Helper()

	registry, err := NewExecutorRegistry(
		registrations,
	)
	if err != nil {
		t.Fatalf(
			"NewExecutorRegistry() returned an unexpected error: %v",
			err,
		)
	}

	return registry
}

func executorRegistryTestRegistration(
	t *testing.T,
	pluginType string,
	version string,
	executorName string,
) ExecutorRegistration {
	t.Helper()

	return mustExecutorRegistration(
		t,
		mustExecutorRegistryIdentity(
			t,
			pluginType,
			version,
		),
		&executorRegistryTestExecutor{
			name: executorName,
		},
	)
}

func mustExecutorRegistration(
	t *testing.T,
	identity plugin.PluginIdentity,
	executor NodeExecutor,
) ExecutorRegistration {
	t.Helper()

	registration, err := NewExecutorRegistration(
		identity,
		executor,
	)
	if err != nil {
		t.Fatalf(
			"NewExecutorRegistration() returned an unexpected error: %v",
			err,
		)
	}

	return registration
}

func mustExecutorRegistryIdentity(
	t *testing.T,
	pluginType string,
	version string,
) plugin.PluginIdentity {
	t.Helper()

	identity, err := plugin.NewPluginIdentity(
		workflow.PluginType(pluginType),
		workflow.PluginVersion(version),
	)
	if err != nil {
		t.Fatalf(
			"plugin.NewPluginIdentity() returned an unexpected error: %v",
			err,
		)
	}

	return identity
}

func TestRuntimeLimitsStoresPositiveValues(
	t *testing.T,
) {
	limits, err := NewRuntimeLimits(
		128,
		16,
		1024,
	)
	if err != nil {
		t.Fatalf(
			"NewRuntimeLimits() returned an unexpected error: %v",
			err,
		)
	}

	if !limits.IsValid() {
		t.Fatal(
			"IsValid() = false for valid runtime limits",
		)
	}

	if actual := limits.MaximumQueueCapacity(); actual != 128 {
		t.Fatalf(
			"MaximumQueueCapacity() = %d, want %d",
			actual,
			128,
		)
	}

	if actual := limits.MaximumCacheCapacity(); actual != 16 {
		t.Fatalf(
			"MaximumCacheCapacity() = %d, want %d",
			actual,
			16,
		)
	}

	if actual := limits.MaximumInlinePayloadBytes(); actual != 1024 {
		t.Fatalf(
			"MaximumInlinePayloadBytes() = %d, want %d",
			actual,
			1024,
		)
	}
}

func TestRuntimeLimitsRejectsInvalidConstructorValues(
	t *testing.T,
) {
	t.Run("zero queue capacity", func(t *testing.T) {
		_, err := NewRuntimeLimits(
			0,
			1,
			1,
		)

		requireRuntimeValidationField(
			t,
			err,
			"maximumQueueCapacity",
		)
	})

	t.Run("queue capacity exceeds platform integer", func(t *testing.T) {
		_, err := NewRuntimeLimits(
			maximumSupportedCapacity()+1,
			1,
			1,
		)

		requireRuntimeValidationField(
			t,
			err,
			"maximumQueueCapacity",
		)
	})

	t.Run("zero cache capacity", func(t *testing.T) {
		_, err := NewRuntimeLimits(
			1,
			0,
			1,
		)

		requireRuntimeValidationField(
			t,
			err,
			"maximumCacheCapacity",
		)
	})

	t.Run("cache capacity exceeds platform integer", func(t *testing.T) {
		_, err := NewRuntimeLimits(
			1,
			maximumSupportedCapacity()+1,
			1,
		)

		requireRuntimeValidationField(
			t,
			err,
			"maximumCacheCapacity",
		)
	})

	t.Run("zero inline payload limit", func(t *testing.T) {
		_, err := NewRuntimeLimits(
			1,
			1,
			0,
		)

		requireRuntimeValidationField(
			t,
			err,
			"maximumInlinePayloadBytes",
		)
	})

	t.Run("negative inline payload limit", func(t *testing.T) {
		_, err := NewRuntimeLimits(
			1,
			1,
			-1,
		)

		requireRuntimeValidationField(
			t,
			err,
			"maximumInlinePayloadBytes",
		)
	})
}

func TestRuntimeLimitsAcceptsCapacitiesEqualToLimits(
	t *testing.T,
) {
	limits := newRuntimeLimitsForTest(
		t,
		64,
		8,
		1024,
	)

	if err := limits.ValidateQueueCapacity(64); err != nil {
		t.Fatalf(
			"ValidateQueueCapacity(equal limit) returned an unexpected error: %v",
			err,
		)
	}

	if err := limits.ValidateCacheCapacity(8); err != nil {
		t.Fatalf(
			"ValidateCacheCapacity(equal limit) returned an unexpected error: %v",
			err,
		)
	}

	if err := limits.ValidateInlinePayloadSize(1024); err != nil {
		t.Fatalf(
			"ValidateInlinePayloadSize(equal limit) returned an unexpected error: %v",
			err,
		)
	}
}

func TestRuntimeLimitsRejectsCapacitiesAboveLimits(
	t *testing.T,
) {
	limits := newRuntimeLimitsForTest(
		t,
		64,
		8,
		1024,
	)

	err := limits.ValidateQueueCapacity(65)
	requireRuntimeValidationField(
		t,
		err,
		"queue.capacity",
	)

	err = limits.ValidateCacheCapacity(9)
	requireRuntimeValidationField(
		t,
		err,
		"cache.capacity",
	)

	err = limits.ValidateInlinePayloadSize(1025)
	requireRuntimeValidationField(
		t,
		err,
		"payload.inlineData",
	)
}

func TestRuntimeLimitsRejectsZeroRequestedCapacities(
	t *testing.T,
) {
	limits := newRuntimeLimitsForTest(
		t,
		64,
		8,
		1024,
	)

	err := limits.ValidateQueueCapacity(0)
	requireRuntimeValidationField(
		t,
		err,
		"queue.capacity",
	)

	err = limits.ValidateCacheCapacity(0)
	requireRuntimeValidationField(
		t,
		err,
		"cache.capacity",
	)
}

func TestRuntimeLimitsValidatesInlinePayloadSize(
	t *testing.T,
) {
	limits := newRuntimeLimitsForTest(
		t,
		64,
		8,
		10,
	)

	if err := limits.ValidateInlinePayloadSize(0); err != nil {
		t.Fatalf(
			"ValidateInlinePayloadSize(0) returned an unexpected error: %v",
			err,
		)
	}

	if err := limits.ValidateInlinePayloadSize(5); err != nil {
		t.Fatalf(
			"ValidateInlinePayloadSize(5) returned an unexpected error: %v",
			err,
		)
	}

	err := limits.ValidateInlinePayloadSize(-1)

	requireRuntimeValidationField(
		t,
		err,
		"payload.inlineData",
	)
}

func TestZeroRuntimeLimitsAreInvalid(
	t *testing.T,
) {
	var limits RuntimeLimits

	if limits.IsValid() {
		t.Fatal(
			"IsValid() = true for zero RuntimeLimits",
		)
	}

	err := limits.ValidateQueueCapacity(1)

	requireRuntimeValidationField(
		t,
		err,
		"maximumQueueCapacity",
	)
}

func TestRuntimeCapacityToIntValidatesCapacity(
	t *testing.T,
) {
	actual, err := runtimeCapacityToInt(
		"capacity",
		64,
	)
	if err != nil {
		t.Fatalf(
			"runtimeCapacityToInt() returned an unexpected error: %v",
			err,
		)
	}

	if actual != 64 {
		t.Fatalf(
			"runtimeCapacityToInt() = %d, want %d",
			actual,
			64,
		)
	}

	_, err = runtimeCapacityToInt(
		"capacity",
		0,
	)

	requireRuntimeValidationField(
		t,
		err,
		"capacity",
	)

	_, err = runtimeCapacityToInt(
		"capacity",
		maximumSupportedCapacity()+1,
	)

	requireRuntimeValidationField(
		t,
		err,
		"capacity",
	)
}

func newRuntimeLimitsForTest(
	t *testing.T,
	maximumQueueCapacity uint,
	maximumCacheCapacity uint,
	maximumInlinePayloadBytes int,
) RuntimeLimits {
	t.Helper()

	limits, err := NewRuntimeLimits(
		maximumQueueCapacity,
		maximumCacheCapacity,
		maximumInlinePayloadBytes,
	)
	if err != nil {
		t.Fatalf(
			"NewRuntimeLimits() returned an unexpected error: %v",
			err,
		)
	}

	return limits
}

func TestNodeExecutionContextStoresExecutionIdentity(
	t *testing.T,
) {
	parent := context.WithValue(
		context.Background(),
		nodeContextTestKey("request"),
		"request-value",
	)

	executionContext :=
		newExecutionContextForContextTest(
			t,
			parent,
			nil,
			nil,
		)

	nodeExecution, startedAt :=
		newRunningNodeExecutionForContextTest(
			t,
			"workflow-execution-1",
			"target-node",
		)

	nodeContext, err := NewNodeExecutionContext(
		executionContext,
		nodeExecution,
		nil,
	)
	if err != nil {
		t.Fatalf(
			"NewNodeExecutionContext() returned an unexpected error: %v",
			err,
		)
	}

	if actual := nodeContext.NodeExecutionID().String(); actual != "node-execution-1" {
		t.Fatalf(
			"NodeExecutionID() = %q, want %q",
			actual,
			"node-execution-1",
		)
	}

	if actual := nodeContext.NodeID().String(); actual != "target-node" {
		t.Fatalf(
			"NodeID() = %q, want %q",
			actual,
			"target-node",
		)
	}

	if actual := nodeContext.StartedAt(); !actual.Equal(startedAt) {
		t.Fatalf(
			"StartedAt() = %v, want %v",
			actual,
			startedAt,
		)
	}

	if actual := nodeContext.WorkflowExecutionID().String(); actual != "workflow-execution-1" {
		t.Fatalf(
			"WorkflowExecutionID() = %q, want %q",
			actual,
			"workflow-execution-1",
		)
	}

	if actual := nodeContext.CompanyID().String(); actual != "company-1" {
		t.Fatalf(
			"CompanyID() = %q, want %q",
			actual,
			"company-1",
		)
	}

	if actual := nodeContext.WorkflowID().String(); actual != "workflow-1" {
		t.Fatalf(
			"WorkflowID() = %q, want %q",
			actual,
			"workflow-1",
		)
	}

	if actual := nodeContext.Mode(); actual != execution.ExecutionModeSync {
		t.Fatalf(
			"Mode() = %q, want %q",
			actual,
			execution.ExecutionModeSync,
		)
	}

	if actual := nodeContext.CorrelationID(); actual != "correlation-1" {
		t.Fatalf(
			"CorrelationID() = %q, want %q",
			actual,
			"correlation-1",
		)
	}

	if actual := nodeContext.Context().Value(
		nodeContextTestKey("request"),
	); actual != "request-value" {
		t.Fatalf(
			"Context().Value() = %#v, want %q",
			actual,
			"request-value",
		)
	}
}

func TestNodeExecutionContextRequiresExecutionContext(
	t *testing.T,
) {
	nodeExecution, _ :=
		newRunningNodeExecutionForContextTest(
			t,
			"workflow-execution-1",
			"target-node",
		)

	_, err := NewNodeExecutionContext(
		nil,
		nodeExecution,
		nil,
	)

	requireRuntimeValidationField(
		t,
		err,
		"executionContext",
	)
}

func TestNodeExecutionContextRejectsInvalidNodeExecution(
	t *testing.T,
) {
	executionContext :=
		newExecutionContextForContextTest(
			t,
			context.Background(),
			nil,
			nil,
		)

	_, err := NewNodeExecutionContext(
		executionContext,
		execution.NodeExecution{},
		nil,
	)

	requireRuntimeValidationField(
		t,
		err,
		"nodeExecution.id",
	)
}

func TestNodeExecutionContextRequiresRunningNodeExecution(
	t *testing.T,
) {
	executionContext :=
		newExecutionContextForContextTest(
			t,
			context.Background(),
			nil,
			nil,
		)

	nodeExecution :=
		newPendingNodeExecutionForContextTest(
			t,
			"workflow-execution-1",
			"target-node",
		)

	_, err := NewNodeExecutionContext(
		executionContext,
		nodeExecution,
		nil,
	)

	requireRuntimeValidationField(
		t,
		err,
		"nodeExecution.status",
	)
}

func TestNodeExecutionContextRequiresMatchingWorkflowExecution(
	t *testing.T,
) {
	executionContext :=
		newExecutionContextForContextTest(
			t,
			context.Background(),
			nil,
			nil,
		)

	nodeExecution, _ :=
		newRunningNodeExecutionForContextTest(
			t,
			"different-workflow-execution",
			"target-node",
		)

	_, err := NewNodeExecutionContext(
		executionContext,
		nodeExecution,
		nil,
	)

	requireRuntimeValidationField(
		t,
		err,
		"nodeExecution.workflowExecutionID",
	)
}

func TestNodeExecutionContextAllowsEmptyIncomingEdges(
	t *testing.T,
) {
	executionContext :=
		newExecutionContextForContextTest(
			t,
			context.Background(),
			nil,
			nil,
		)

	nodeExecution, _ :=
		newRunningNodeExecutionForContextTest(
			t,
			"workflow-execution-1",
			"static-input-node",
		)

	nodeContext, err := NewNodeExecutionContext(
		executionContext,
		nodeExecution,
		nil,
	)
	if err != nil {
		t.Fatalf(
			"NewNodeExecutionContext() returned an unexpected error: %v",
			err,
		)
	}

	if incomingEdges := nodeContext.IncomingEdges(); incomingEdges != nil {
		t.Fatalf(
			"IncomingEdges() = %#v, want nil",
			incomingEdges,
		)
	}
}

func TestNodeExecutionContextRegistersIncomingEdgesInDeterministicOrder(
	t *testing.T,
) {
	edgeB := newEdgeRuntimeForTest(
		t,
		newEdgeDefinitionForRuntimeTest(
			t,
			"edge-b",
			"source-b",
			"output",
			"target-node",
			"input-b",
		),
	)

	edgeA := newEdgeRuntimeForTest(
		t,
		newEdgeDefinitionForRuntimeTest(
			t,
			"edge-a",
			"source-a",
			"output",
			"target-node",
			"input-a",
		),
	)

	executionContext :=
		newExecutionContextForContextTest(
			t,
			context.Background(),
			[]*EdgeRuntime{
				edgeB,
				edgeA,
			},
			nil,
		)

	nodeExecution, _ :=
		newRunningNodeExecutionForContextTest(
			t,
			"workflow-execution-1",
			"target-node",
		)

	nodeContext, err := NewNodeExecutionContext(
		executionContext,
		nodeExecution,
		[]workflow.EdgeID{
			workflow.EdgeID("edge-b"),
			workflow.EdgeID("edge-a"),
		},
	)
	if err != nil {
		t.Fatalf(
			"NewNodeExecutionContext() returned an unexpected error: %v",
			err,
		)
	}

	incomingEdges := nodeContext.IncomingEdges()

	if len(incomingEdges) != 2 {
		t.Fatalf(
			"len(IncomingEdges()) = %d, want 2",
			len(incomingEdges),
		)
	}

	if actual := incomingEdges[0].ID().String(); actual != "edge-a" {
		t.Fatalf(
			"IncomingEdges()[0].ID() = %q, want %q",
			actual,
			"edge-a",
		)
	}

	if actual := incomingEdges[1].ID().String(); actual != "edge-b" {
		t.Fatalf(
			"IncomingEdges()[1].ID() = %q, want %q",
			actual,
			"edge-b",
		)
	}

	incomingEdges[0] = IncomingEdge{}

	secondIncomingEdges := nodeContext.IncomingEdges()

	if actual := secondIncomingEdges[0].ID().String(); actual != "edge-a" {
		t.Fatalf(
			"incoming-edge registry changed through returned slice: got %q",
			actual,
		)
	}
}

func TestNodeExecutionContextReturnsAuthorizedIncomingEdge(
	t *testing.T,
) {
	edgeRuntime := newEdgeRuntimeForTest(
		t,
		newEdgeDefinitionForRuntimeTest(
			t,
			"edge-1",
			"source-node",
			"output",
			"target-node",
			"input",
		),
	)

	executionContext :=
		newExecutionContextForContextTest(
			t,
			context.Background(),
			[]*EdgeRuntime{
				edgeRuntime,
			},
			nil,
		)

	nodeExecution, _ :=
		newRunningNodeExecutionForContextTest(
			t,
			"workflow-execution-1",
			"target-node",
		)

	nodeContext, err := NewNodeExecutionContext(
		executionContext,
		nodeExecution,
		[]workflow.EdgeID{
			workflow.EdgeID("edge-1"),
		},
	)
	if err != nil {
		t.Fatalf(
			"NewNodeExecutionContext() returned an unexpected error: %v",
			err,
		)
	}

	incomingEdge, exists, err :=
		nodeContext.IncomingEdge(
			workflow.EdgeID(" edge-1 "),
		)
	if err != nil {
		t.Fatalf(
			"IncomingEdge() returned an unexpected error: %v",
			err,
		)
	}

	if !exists {
		t.Fatal(
			"IncomingEdge() exists = false",
		)
	}

	if actual := incomingEdge.ID().String(); actual != "edge-1" {
		t.Fatalf(
			"ID() = %q, want %q",
			actual,
			"edge-1",
		)
	}

	if actual := incomingEdge.SourceNodeID().String(); actual != "source-node" {
		t.Fatalf(
			"SourceNodeID() = %q, want %q",
			actual,
			"source-node",
		)
	}

	if actual := incomingEdge.SourceOutputPort(); actual != "output" {
		t.Fatalf(
			"SourceOutputPort() = %q, want %q",
			actual,
			"output",
		)
	}

	if actual := incomingEdge.TargetNodeID().String(); actual != "target-node" {
		t.Fatalf(
			"TargetNodeID() = %q, want %q",
			actual,
			"target-node",
		)
	}

	if actual := incomingEdge.TargetInputPort(); actual != "input" {
		t.Fatalf(
			"TargetInputPort() = %q, want %q",
			actual,
			"input",
		)
	}

	if incomingEdge.Cache() != edgeRuntime.Cache() {
		t.Fatal(
			"IncomingEdge.Cache() did not return the authorized edge cache",
		)
	}
}

func TestNodeExecutionContextAllowsAuthorizedCacheAccess(
	t *testing.T,
) {
	edgeRuntime := newEdgeRuntimeForTest(
		t,
		newEdgeDefinitionForRuntimeTest(
			t,
			"edge-1",
			"source-node",
			"output",
			"target-node",
			"input",
		),
	)

	executionContext :=
		newExecutionContextForContextTest(
			t,
			context.Background(),
			[]*EdgeRuntime{
				edgeRuntime,
			},
			nil,
		)

	nodeContext :=
		newNodeExecutionContextForTest(
			t,
			executionContext,
			"target-node",
			[]workflow.EdgeID{
				workflow.EdgeID("edge-1"),
			},
		)

	incomingEdge, exists, err :=
		nodeContext.IncomingEdge(
			workflow.EdgeID("edge-1"),
		)
	if err != nil {
		t.Fatalf(
			"IncomingEdge() returned an unexpected error: %v",
			err,
		)
	}

	if !exists {
		t.Fatal(
			"IncomingEdge() exists = false",
		)
	}

	value :=
		newRuntimeValueForExecutionContextTest(
			t,
			`{"cached":true}`,
		)

	if _, didEvict, err := incomingEdge.Cache().Set(
		"last-value",
		value,
	); err != nil {
		t.Fatalf(
			"Cache().Set() returned an unexpected error: %v",
			err,
		)
	} else if didEvict {
		t.Fatal(
			"Cache().Set() unexpectedly evicted an entry",
		)
	}

	actual, exists, err :=
		edgeRuntime.Cache().Get(
			"last-value",
		)
	if err != nil {
		t.Fatalf(
			"edge runtime Cache().Get() returned an unexpected error: %v",
			err,
		)
	}

	if !exists {
		t.Fatal(
			"edge runtime cache does not contain the value written through IncomingEdge",
		)
	}

	if actual.String() != `{"cached":true}` {
		t.Fatalf(
			"cached value = %q, want %q",
			actual.String(),
			`{"cached":true}`,
		)
	}
}

func TestNodeExecutionContextRejectsUnauthorizedIncomingEdge(
	t *testing.T,
) {
	authorizedEdge := newEdgeRuntimeForTest(
		t,
		newEdgeDefinitionForRuntimeTest(
			t,
			"edge-1",
			"source-1",
			"output",
			"target-node",
			"input",
		),
	)

	unauthorizedEdge := newEdgeRuntimeForTest(
		t,
		newEdgeDefinitionForRuntimeTest(
			t,
			"edge-2",
			"source-2",
			"output",
			"target-node",
			"input",
		),
	)

	executionContext :=
		newExecutionContextForContextTest(
			t,
			context.Background(),
			[]*EdgeRuntime{
				authorizedEdge,
				unauthorizedEdge,
			},
			nil,
		)

	nodeContext :=
		newNodeExecutionContextForTest(
			t,
			executionContext,
			"target-node",
			[]workflow.EdgeID{
				workflow.EdgeID("edge-1"),
			},
		)

	_, exists, err :=
		nodeContext.IncomingEdge(
			workflow.EdgeID("edge-2"),
		)

	if exists {
		t.Fatal(
			"IncomingEdge(edge-2) exists = true for an unauthorized edge",
		)
	}

	var accessError *AccessError
	if !errors.As(err, &accessError) {
		t.Fatalf(
			"error type = %T, want *AccessError",
			err,
		)
	}

	if accessError.Resource != "incoming edge" {
		t.Fatalf(
			"AccessError.Resource = %q, want %q",
			accessError.Resource,
			"incoming edge",
		)
	}

	if accessError.Identifier != "edge-2" {
		t.Fatalf(
			"AccessError.Identifier = %q, want %q",
			accessError.Identifier,
			"edge-2",
		)
	}

	if accessError.Reason == "" {
		t.Fatal(
			"AccessError.Reason must not be empty",
		)
	}
}

func TestNodeExecutionContextRejectsInvalidIncomingEdgeID(
	t *testing.T,
) {
	executionContext :=
		newExecutionContextForContextTest(
			t,
			context.Background(),
			nil,
			nil,
		)

	nodeContext :=
		newNodeExecutionContextForTest(
			t,
			executionContext,
			"target-node",
			nil,
		)

	_, _, err :=
		nodeContext.IncomingEdge(
			workflow.EdgeID(" "),
		)

	requireRuntimeValidationField(
		t,
		err,
		"edgeID",
	)
}

func TestNodeExecutionContextRejectsDuplicateIncomingEdges(
	t *testing.T,
) {
	edgeRuntime := newEdgeRuntimeForTest(
		t,
		newEdgeDefinitionForRuntimeTest(
			t,
			"edge-1",
			"source-node",
			"output",
			"target-node",
			"input",
		),
	)

	executionContext :=
		newExecutionContextForContextTest(
			t,
			context.Background(),
			[]*EdgeRuntime{
				edgeRuntime,
			},
			nil,
		)

	nodeExecution, _ :=
		newRunningNodeExecutionForContextTest(
			t,
			"workflow-execution-1",
			"target-node",
		)

	_, err := NewNodeExecutionContext(
		executionContext,
		nodeExecution,
		[]workflow.EdgeID{
			workflow.EdgeID("edge-1"),
			workflow.EdgeID(" edge-1 "),
		},
	)

	requireRuntimeValidationField(
		t,
		err,
		"incomingEdgeIDs",
	)
}

func TestNodeExecutionContextRejectsUnregisteredIncomingEdge(
	t *testing.T,
) {
	executionContext :=
		newExecutionContextForContextTest(
			t,
			context.Background(),
			nil,
			nil,
		)

	nodeExecution, _ :=
		newRunningNodeExecutionForContextTest(
			t,
			"workflow-execution-1",
			"target-node",
		)

	_, err := NewNodeExecutionContext(
		executionContext,
		nodeExecution,
		[]workflow.EdgeID{
			workflow.EdgeID("missing-edge"),
		},
	)

	requireRuntimeValidationField(
		t,
		err,
		"incomingEdgeIDs[0]",
	)
}

func TestNodeExecutionContextRejectsEdgeTargetingAnotherNode(
	t *testing.T,
) {
	edgeRuntime := newEdgeRuntimeForTest(
		t,
		newEdgeDefinitionForRuntimeTest(
			t,
			"edge-1",
			"source-node",
			"output",
			"different-target",
			"input",
		),
	)

	executionContext :=
		newExecutionContextForContextTest(
			t,
			context.Background(),
			[]*EdgeRuntime{
				edgeRuntime,
			},
			nil,
		)

	nodeExecution, _ :=
		newRunningNodeExecutionForContextTest(
			t,
			"workflow-execution-1",
			"target-node",
		)

	_, err := NewNodeExecutionContext(
		executionContext,
		nodeExecution,
		[]workflow.EdgeID{
			workflow.EdgeID("edge-1"),
		},
	)

	requireRuntimeValidationField(
		t,
		err,
		"incomingEdgeIDs[0]",
	)
}

func TestNodeExecutionContextReadsVariablesWithoutMutationAPI(
	t *testing.T,
) {
	executionContext :=
		newExecutionContextForContextTest(
			t,
			context.Background(),
			nil,
			map[string]RuntimeValue{
				"currency": newRuntimeValueForExecutionContextTest(
					t,
					`"TRY"`,
				),
			},
		)

	nodeContext :=
		newNodeExecutionContextForTest(
			t,
			executionContext,
			"target-node",
			nil,
		)

	actual, exists, err :=
		nodeContext.Variable("currency")
	if err != nil {
		t.Fatalf(
			"Variable() returned an unexpected error: %v",
			err,
		)
	}

	if !exists {
		t.Fatal(
			"Variable(currency) exists = false",
		)
	}

	if actual.String() != `"TRY"` {
		t.Fatalf(
			"Variable(currency) = %q, want %q",
			actual.String(),
			`"TRY"`,
		)
	}

	actual.raw[0] = '['

	second, exists, err :=
		nodeContext.Variable("currency")
	if err != nil {
		t.Fatalf(
			"second Variable() returned an unexpected error: %v",
			err,
		)
	}

	if !exists {
		t.Fatal(
			"second Variable(currency) exists = false",
		)
	}

	if second.String() != `"TRY"` {
		t.Fatalf(
			"variable changed through returned value: got %q",
			second.String(),
		)
	}

	snapshot := nodeContext.VariablesSnapshot()
	delete(
		snapshot,
		"currency",
	)

	if _, exists, err :=
		nodeContext.Variable("currency"); err != nil {
		t.Fatalf(
			"Variable(currency) after snapshot change returned an unexpected error: %v",
			err,
		)
	} else if !exists {
		t.Fatal(
			"variable was deleted through node-context snapshot",
		)
	}
}

func TestNodeExecutionContextProtectedMetadataIsIndependent(
	t *testing.T,
) {
	executionContext :=
		newExecutionContextForContextTest(
			t,
			context.Background(),
			nil,
			nil,
		)

	nodeContext :=
		newNodeExecutionContextForTest(
			t,
			executionContext,
			"target-node",
			nil,
		)

	metadata := nodeContext.ProtectedMetadata()

	if actual := metadata[ProtectedKeyCompanyID]; actual != "company-1" {
		t.Fatalf(
			"ProtectedMetadata()[companyID] = %q, want %q",
			actual,
			"company-1",
		)
	}

	metadata[ProtectedKeyCompanyID] =
		"changed-company"

	secondMetadata :=
		nodeContext.ProtectedMetadata()

	if actual := secondMetadata[ProtectedKeyCompanyID]; actual != "company-1" {
		t.Fatalf(
			"protected metadata changed through returned map: got %q",
			actual,
		)
	}
}

func TestNodeExecutionContextPropagatesCancellation(
	t *testing.T,
) {
	parent, cancel :=
		context.WithCancel(
			context.Background(),
		)

	executionContext :=
		newExecutionContextForContextTest(
			t,
			parent,
			nil,
			nil,
		)

	nodeContext :=
		newNodeExecutionContextForTest(
			t,
			executionContext,
			"target-node",
			nil,
		)

	cancel()

	select {
	case <-nodeContext.Done():
	default:
		t.Fatal(
			"Done() was not closed after parent cancellation",
		)
	}

	if !errors.Is(
		nodeContext.Err(),
		context.Canceled,
	) {
		t.Fatalf(
			"Err() = %v, want context.Canceled",
			nodeContext.Err(),
		)
	}
}

func TestNodeExecutionContextPropagatesDeadline(
	t *testing.T,
) {
	expectedDeadline := time.Now().
		Add(time.Hour).
		Round(0)

	parent, cancel := context.WithDeadline(
		context.Background(),
		expectedDeadline,
	)
	defer cancel()

	executionContext :=
		newExecutionContextForContextTest(
			t,
			parent,
			nil,
			nil,
		)

	nodeContext :=
		newNodeExecutionContextForTest(
			t,
			executionContext,
			"target-node",
			nil,
		)

	actualDeadline, exists :=
		nodeContext.Deadline()

	if !exists {
		t.Fatal(
			"Deadline() exists = false",
		)
	}

	if !actualDeadline.Equal(expectedDeadline) {
		t.Fatalf(
			"Deadline() = %v, want %v",
			actualDeadline,
			expectedDeadline,
		)
	}
}

func TestNodeExecutionContextDoesNotExposeQueueOrVariableMutation(
	t *testing.T,
) {
	incomingEdgeType :=
		reflect.TypeOf(IncomingEdge{})

	if _, exists :=
		incomingEdgeType.MethodByName("Queue"); exists {
		t.Fatal(
			"IncomingEdge must not expose a Queue method",
		)
	}

	nodeContextType :=
		reflect.TypeOf(&NodeExecutionContext{})

	for _, methodName := range []string{
		"SetVariable",
		"DeleteVariable",
		"Edge",
		"Edges",
	} {
		if _, exists :=
			nodeContextType.MethodByName(methodName); exists {
			t.Fatalf(
				"NodeExecutionContext must not expose method %q",
				methodName,
			)
		}
	}
}

func TestAccessErrorFormatting(
	t *testing.T,
) {
	err := &AccessError{
		Resource:   "incoming edge",
		Identifier: "edge-2",
		Reason:     "is not authorized for this node execution",
	}

	expected :=
		`runtime access denied for incoming edge "edge-2": is not authorized for this node execution`

	if actual := err.Error(); actual != expected {
		t.Fatalf(
			"Error() = %q, want %q",
			actual,
			expected,
		)
	}
}

func TestNilNodeExecutionContextMethodsAreSafe(
	t *testing.T,
) {
	var nodeContext *NodeExecutionContext

	if nodeContext.Context() != nil {
		t.Fatal(
			"nil Context() must return nil",
		)
	}

	if nodeContext.Done() != nil {
		t.Fatal(
			"nil Done() must return nil",
		)
	}

	if nodeContext.Err() != nil {
		t.Fatal(
			"nil Err() must return nil",
		)
	}

	if _, exists := nodeContext.Deadline(); exists {
		t.Fatal(
			"nil Deadline() exists = true",
		)
	}

	if nodeContext.NodeExecutionID().String() != "" {
		t.Fatal(
			"nil NodeExecutionID() must be empty",
		)
	}

	if nodeContext.NodeID().String() != "" {
		t.Fatal(
			"nil NodeID() must be empty",
		)
	}

	if !nodeContext.StartedAt().IsZero() {
		t.Fatal(
			"nil StartedAt() must be zero",
		)
	}

	if nodeContext.WorkflowExecutionID().String() != "" {
		t.Fatal(
			"nil WorkflowExecutionID() must be empty",
		)
	}

	if nodeContext.CompanyID().String() != "" {
		t.Fatal(
			"nil CompanyID() must be empty",
		)
	}

	if nodeContext.WorkflowID().String() != "" {
		t.Fatal(
			"nil WorkflowID() must be empty",
		)
	}

	if nodeContext.Mode().String() != "" {
		t.Fatal(
			"nil Mode() must be empty",
		)
	}

	if nodeContext.CorrelationID() != "" {
		t.Fatal(
			"nil CorrelationID() must be empty",
		)
	}

	if nodeContext.ProtectedMetadata() != nil {
		t.Fatal(
			"nil ProtectedMetadata() must return nil",
		)
	}

	if nodeContext.VariablesSnapshot() != nil {
		t.Fatal(
			"nil VariablesSnapshot() must return nil",
		)
	}

	if nodeContext.IncomingEdges() != nil {
		t.Fatal(
			"nil IncomingEdges() must return nil",
		)
	}

	_, _, err := nodeContext.Variable("key")

	requireRuntimeValidationField(
		t,
		err,
		"nodeExecutionContext",
	)

	_, _, err =
		nodeContext.IncomingEdge(
			workflow.EdgeID("edge-1"),
		)

	requireRuntimeValidationField(
		t,
		err,
		"nodeExecutionContext",
	)
}

type nodeContextTestKey string

func newNodeExecutionContextForTest(
	t *testing.T,
	executionContext *ExecutionContext,
	nodeID string,
	incomingEdgeIDs []workflow.EdgeID,
) *NodeExecutionContext {
	t.Helper()

	nodeExecution, _ :=
		newRunningNodeExecutionForContextTest(
			t,
			executionContext.
				WorkflowExecutionID().
				String(),
			nodeID,
		)

	nodeContext, err := NewNodeExecutionContext(
		executionContext,
		nodeExecution,
		incomingEdgeIDs,
	)
	if err != nil {
		t.Fatalf(
			"NewNodeExecutionContext() returned an unexpected error: %v",
			err,
		)
	}

	return nodeContext
}

func newPendingNodeExecutionForContextTest(
	t *testing.T,
	workflowExecutionID string,
	nodeID string,
) execution.NodeExecution {
	t.Helper()

	nodeExecution, err := execution.NewNodeExecution(
		execution.NodeExecutionID(
			" node-execution-1 ",
		),
		execution.WorkflowExecutionID(
			workflowExecutionID,
		),
		workflow.NodeID(nodeID),
		nodeContextTestTime(
			10,
			3,
		),
	)
	if err != nil {
		t.Fatalf(
			"execution.NewNodeExecution() returned an unexpected error: %v",
			err,
		)
	}

	return nodeExecution
}

func newRunningNodeExecutionForContextTest(
	t *testing.T,
	workflowExecutionID string,
	nodeID string,
) (
	execution.NodeExecution,
	time.Time,
) {
	t.Helper()

	nodeExecution :=
		newPendingNodeExecutionForContextTest(
			t,
			workflowExecutionID,
			nodeID,
		)

	if err := nodeExecution.MarkReady(
		nodeContextTestTime(
			10,
			4,
		),
	); err != nil {
		t.Fatalf(
			"MarkReady() returned an unexpected error: %v",
			err,
		)
	}

	startedAt :=
		nodeContextTestTime(
			10,
			5,
		)

	if err := nodeExecution.Start(
		startedAt,
	); err != nil {
		t.Fatalf(
			"Start() returned an unexpected error: %v",
			err,
		)
	}

	return nodeExecution, startedAt
}

func nodeContextTestTime(
	hour int,
	minute int,
) time.Time {
	return time.Date(
		2026,
		time.July,
		16,
		hour,
		minute,
		0,
		0,
		time.UTC,
	)
}

var _ NodeExecutor = NodeExecutorFunc(nil)

func TestNodeExecutorFuncImplementsExecutorContract(
	t *testing.T,
) {
	nodeContext := &NodeExecutionContext{}

	input, err := NewNodeInput(
		map[string][]Payload{
			"input": {
				newNodeInputPayloadForTest(
					t,
					"payload",
				),
			},
		},
	)
	if err != nil {
		t.Fatalf(
			"NewNodeInput() returned an unexpected error: %v",
			err,
		)
	}

	configuration := newNodeExecutorConfigurationForTest(
		t,
		`{"enabled":true}`,
	)

	expectedResult, err := NewNodeSuccessResult(
		nil,
		ContextChanges{},
	)
	if err != nil {
		t.Fatalf(
			"NewNodeSuccessResult() returned an unexpected error: %v",
			err,
		)
	}

	called := false

	executor := NodeExecutorFunc(
		func(
			actualNodeContext *NodeExecutionContext,
			actualInput NodeInput,
			actualConfiguration workflow.JSONObject,
		) (NodeResult, error) {
			called = true

			if actualNodeContext != nodeContext {
				t.Fatal(
					"executor received a different NodeExecutionContext pointer",
				)
			}

			if !actualInput.IsValid() {
				t.Fatal(
					"executor received an invalid NodeInput",
				)
			}

			if actualInput.TotalPayloadCount() != 1 {
				t.Fatalf(
					"TotalPayloadCount() = %d, want 1",
					actualInput.TotalPayloadCount(),
				)
			}

			if actualConfiguration.String() != `{"enabled":true}` {
				t.Fatalf(
					"configuration = %q, want %q",
					actualConfiguration.String(),
					`{"enabled":true}`,
				)
			}

			return expectedResult, nil
		},
	)

	var contract NodeExecutor = executor

	actualResult, err := contract.Execute(
		nodeContext,
		input,
		configuration,
	)
	if err != nil {
		t.Fatalf(
			"Execute() returned an unexpected error: %v",
			err,
		)
	}

	if !called {
		t.Fatal(
			"executor function was not called",
		)
	}

	if !actualResult.IsValid() {
		t.Fatal(
			"Execute() returned an invalid NodeResult",
		)
	}

	if !actualResult.IsSuccess() {
		t.Fatal(
			"Execute() result IsSuccess() = false",
		)
	}
}

func TestNodeExecutorFuncReturnsControlledFailureResult(
	t *testing.T,
) {
	failure := newRuntimeFailureForTest(
		t,
	)

	executor := NodeExecutorFunc(
		func(
			*NodeExecutionContext,
			NodeInput,
			workflow.JSONObject,
		) (NodeResult, error) {
			return NewNodeFailureResult(
				failure,
			)
		},
	)

	result, err := executor.Execute(
		nil,
		NodeInput{},
		workflow.JSONObject{},
	)
	if err != nil {
		t.Fatalf(
			"Execute() returned a Go error for a controlled failure: %v",
			err,
		)
	}

	if !result.IsFailure() {
		t.Fatal(
			"controlled failure result IsFailure() = false",
		)
	}

	actualFailure, exists := result.Failure()
	if !exists {
		t.Fatal(
			"Failure() exists = false",
		)
	}

	if actualFailure.Code() != failure.Code() {
		t.Fatalf(
			"failure code = %q, want %q",
			actualFailure.Code(),
			failure.Code(),
		)
	}
}

func TestNodeExecutorFuncReturnsTechnicalError(
	t *testing.T,
) {
	expectedError := errors.New(
		"unexpected executor failure",
	)

	executor := NodeExecutorFunc(
		func(
			*NodeExecutionContext,
			NodeInput,
			workflow.JSONObject,
		) (NodeResult, error) {
			return NodeResult{}, expectedError
		},
	)

	result, err := executor.Execute(
		nil,
		NodeInput{},
		workflow.JSONObject{},
	)

	if !errors.Is(err, expectedError) {
		t.Fatalf(
			"Execute() error = %v, want %v",
			err,
			expectedError,
		)
	}

	if result.IsValid() {
		t.Fatal(
			"technical-error path returned a valid NodeResult",
		)
	}
}

func TestNodeExecutorFuncForwardsConfigurationByValue(
	t *testing.T,
) {
	configuration := newNodeExecutorConfigurationForTest(
		t,
		`{"delay":"2s"}`,
	)

	executor := NodeExecutorFunc(
		func(
			_ *NodeExecutionContext,
			_ NodeInput,
			actualConfiguration workflow.JSONObject,
		) (NodeResult, error) {
			exposedBytes := actualConfiguration.Bytes()
			exposedBytes[0] = '['

			if actualConfiguration.String() != `{"delay":"2s"}` {
				t.Fatalf(
					"configuration changed through Bytes(): got %q",
					actualConfiguration.String(),
				)
			}

			return NewNodeSuccessResult(
				nil,
				ContextChanges{},
			)
		},
	)

	result, err := executor.Execute(
		nil,
		NodeInput{},
		configuration,
	)
	if err != nil {
		t.Fatalf(
			"Execute() returned an unexpected error: %v",
			err,
		)
	}

	if !result.IsSuccess() {
		t.Fatal(
			"Execute() result IsSuccess() = false",
		)
	}

	if configuration.String() != `{"delay":"2s"}` {
		t.Fatalf(
			"caller configuration changed during execution: got %q",
			configuration.String(),
		)
	}
}

func TestNilNodeExecutorFuncIsRejected(
	t *testing.T,
) {
	var executor NodeExecutorFunc

	_, err := executor.Execute(
		nil,
		NodeInput{},
		workflow.JSONObject{},
	)

	requireRuntimeValidationField(
		t,
		err,
		"nodeExecutor",
	)
}

func newNodeExecutorConfigurationForTest(
	t *testing.T,
	value string,
) workflow.JSONObject {
	t.Helper()

	configuration, err := workflow.NewJSONObject(
		[]byte(value),
	)
	if err != nil {
		t.Fatalf(
			"workflow.NewJSONObject(%q) returned an unexpected error: %v",
			value,
			err,
		)
	}

	return configuration
}

func TestNewNodeInputAllowsEmptyInput(
	t *testing.T,
) {
	input, err := NewNodeInput(nil)
	if err != nil {
		t.Fatalf(
			"NewNodeInput() returned an unexpected error: %v",
			err,
		)
	}

	if !input.IsEmpty() {
		t.Fatal(
			"IsEmpty() = false for an empty node input",
		)
	}

	if !input.IsValid() {
		t.Fatal(
			"IsValid() = false for an empty node input",
		)
	}

	if actual := input.PortCount(); actual != 0 {
		t.Fatalf(
			"PortCount() = %d, want 0",
			actual,
		)
	}

	if actual := input.TotalPayloadCount(); actual != 0 {
		t.Fatalf(
			"TotalPayloadCount() = %d, want 0",
			actual,
		)
	}

	if ports := input.Ports(); ports != nil {
		t.Fatalf(
			"Ports() = %#v, want nil",
			ports,
		)
	}

	payloads, exists, err := input.Payloads(
		"missing",
	)
	if err != nil {
		t.Fatalf(
			"Payloads(missing) returned an unexpected error: %v",
			err,
		)
	}

	if exists {
		t.Fatal(
			"Payloads(missing) exists = true",
		)
	}

	if payloads != nil {
		t.Fatalf(
			"Payloads(missing) = %#v, want nil",
			payloads,
		)
	}
}

func TestNewNodeInputNormalizesAndSortsPorts(
	t *testing.T,
) {
	input, err := NewNodeInput(
		map[string][]Payload{
			" right ": {
				newNodeInputPayloadForTest(
					t,
					"right-1",
				),
			},
			"left": {
				newNodeInputPayloadForTest(
					t,
					"left-1",
				),
				newNodeInputPayloadForTest(
					t,
					"left-2",
				),
			},
		},
	)
	if err != nil {
		t.Fatalf(
			"NewNodeInput() returned an unexpected error: %v",
			err,
		)
	}

	expectedPorts := []string{
		"left",
		"right",
	}

	if actual := input.Ports(); !reflect.DeepEqual(
		actual,
		expectedPorts,
	) {
		t.Fatalf(
			"Ports() = %#v, want %#v",
			actual,
			expectedPorts,
		)
	}

	if actual := input.PortCount(); actual != 2 {
		t.Fatalf(
			"PortCount() = %d, want 2",
			actual,
		)
	}

	if actual := input.TotalPayloadCount(); actual != 3 {
		t.Fatalf(
			"TotalPayloadCount() = %d, want 3",
			actual,
		)
	}

	leftPayloads, exists, err := input.Payloads(
		" left ",
	)
	if err != nil {
		t.Fatalf(
			"Payloads(left) returned an unexpected error: %v",
			err,
		)
	}

	if !exists {
		t.Fatal(
			"Payloads(left) exists = false",
		)
	}

	if len(leftPayloads) != 2 {
		t.Fatalf(
			"len(Payloads(left)) = %d, want 2",
			len(leftPayloads),
		)
	}

	if actual := nodeInputPayloadTextForTest(
		t,
		leftPayloads[0],
	); actual != "left-1" {
		t.Fatalf(
			"Payloads(left)[0] = %q, want %q",
			actual,
			"left-1",
		)
	}

	if actual := nodeInputPayloadTextForTest(
		t,
		leftPayloads[1],
	); actual != "left-2" {
		t.Fatalf(
			"Payloads(left)[1] = %q, want %q",
			actual,
			"left-2",
		)
	}
}

func TestNewNodeInputRejectsInvalidDefinitions(
	t *testing.T,
) {
	validPayload := newNodeInputPayloadForTest(
		t,
		"value",
	)

	t.Run("blank port", func(t *testing.T) {
		_, err := NewNodeInput(
			map[string][]Payload{
				" ": {
					validPayload,
				},
			},
		)

		requireRuntimeValidationField(
			t,
			err,
			"nodeInput.port",
		)
	})

	t.Run("duplicate normalized port", func(t *testing.T) {
		_, err := NewNodeInput(
			map[string][]Payload{
				"input": {
					validPayload,
				},
				" input ": {
					validPayload,
				},
			},
		)

		requireRuntimeValidationField(
			t,
			err,
			"nodeInput",
		)
	})

	t.Run("empty payload collection", func(t *testing.T) {
		_, err := NewNodeInput(
			map[string][]Payload{
				"input": {},
			},
		)

		requireRuntimeValidationField(
			t,
			err,
			"nodeInput.payloads",
		)
	})

	t.Run("invalid payload", func(t *testing.T) {
		_, err := NewNodeInput(
			map[string][]Payload{
				"input": {
					{},
				},
			},
		)

		requireRuntimeValidationField(
			t,
			err,
			"nodeInput.input[0]",
		)
	})
}

func TestNodeInputPayloadLookupValidatesPort(
	t *testing.T,
) {
	input, err := NewNodeInput(nil)
	if err != nil {
		t.Fatalf(
			"NewNodeInput() returned an unexpected error: %v",
			err,
		)
	}

	_, _, err = input.Payloads(" ")

	requireRuntimeValidationField(
		t,
		err,
		"nodeInput.port",
	)
}

func TestNodeInputCopiesConstructorData(
	t *testing.T,
) {
	payload, err := NewInlinePayload(
		ContentTypeTextPlain,
		[]byte("original"),
		map[string]string{
			"source": "original-source",
		},
		100,
	)
	if err != nil {
		t.Fatalf(
			"NewInlinePayload() returned an unexpected error: %v",
			err,
		)
	}

	sourcePayloads := []Payload{
		payload,
	}

	sourceInputs := map[string][]Payload{
		" input ": sourcePayloads,
	}

	input, err := NewNodeInput(sourceInputs)
	if err != nil {
		t.Fatalf(
			"NewNodeInput() returned an unexpected error: %v",
			err,
		)
	}

	payload.inlineData[0] = 'X'
	payload.metadata["source"] = "changed"

	sourcePayloads[0].inlineData[0] = 'Y'
	sourcePayloads[0].metadata["source"] = "changed-again"

	sourceInputs[" input "] = nil

	actualPayloads, exists, err := input.Payloads(
		"input",
	)
	if err != nil {
		t.Fatalf(
			"Payloads(input) returned an unexpected error: %v",
			err,
		)
	}

	if !exists {
		t.Fatal(
			"Payloads(input) exists = false",
		)
	}

	if actual := nodeInputPayloadTextForTest(
		t,
		actualPayloads[0],
	); actual != "original" {
		t.Fatalf(
			"stored payload = %q, want %q",
			actual,
			"original",
		)
	}

	if actual := actualPayloads[0].Metadata()["source"]; actual != "original-source" {
		t.Fatalf(
			"stored metadata source = %q, want %q",
			actual,
			"original-source",
		)
	}
}

func TestNodeInputReturnedValuesAreIndependent(
	t *testing.T,
) {
	input, err := NewNodeInput(
		map[string][]Payload{
			"input": {
				newNodeInputPayloadForTest(
					t,
					"original",
				),
			},
		},
	)
	if err != nil {
		t.Fatalf(
			"NewNodeInput() returned an unexpected error: %v",
			err,
		)
	}

	ports := input.Ports()
	ports[0] = "changed"

	secondPorts := input.Ports()

	if actual := secondPorts[0]; actual != "input" {
		t.Fatalf(
			"port changed through Ports() result: got %q",
			actual,
		)
	}

	payloads, exists, err := input.Payloads("input")
	if err != nil {
		t.Fatalf(
			"Payloads(input) returned an unexpected error: %v",
			err,
		)
	}

	if !exists {
		t.Fatal(
			"Payloads(input) exists = false",
		)
	}

	payloads[0].inlineData[0] = 'X'
	payloads[0].metadata["source"] = "changed"
	payloads[0] = Payload{}

	secondPayloads, exists, err := input.Payloads(
		"input",
	)
	if err != nil {
		t.Fatalf(
			"second Payloads(input) returned an unexpected error: %v",
			err,
		)
	}

	if !exists {
		t.Fatal(
			"second Payloads(input) exists = false",
		)
	}

	if actual := nodeInputPayloadTextForTest(
		t,
		secondPayloads[0],
	); actual != "original" {
		t.Fatalf(
			"payload changed through accessor result: got %q",
			actual,
		)
	}

	if actual := secondPayloads[0].Metadata()["source"]; actual != "node-input-test" {
		t.Fatalf(
			"metadata changed through accessor result: got %q",
			actual,
		)
	}
}

func TestNodeInputCloneCreatesIndependentCopy(
	t *testing.T,
) {
	input, err := NewNodeInput(
		map[string][]Payload{
			"input": {
				newNodeInputPayloadForTest(
					t,
					"original",
				),
			},
		},
	)
	if err != nil {
		t.Fatalf(
			"NewNodeInput() returned an unexpected error: %v",
			err,
		)
	}

	cloned := cloneNodeInput(input)

	cloned.ports[0] = "changed"
	cloned.inputs["input"][0].inlineData[0] = 'X'
	cloned.inputs["input"][0].metadata["source"] =
		"changed"

	if actual := input.Ports()[0]; actual != "input" {
		t.Fatalf(
			"original port changed through clone: got %q",
			actual,
		)
	}

	payloads, exists, err := input.Payloads("input")
	if err != nil {
		t.Fatalf(
			"Payloads(input) returned an unexpected error: %v",
			err,
		)
	}

	if !exists {
		t.Fatal(
			"Payloads(input) exists = false",
		)
	}

	if actual := nodeInputPayloadTextForTest(
		t,
		payloads[0],
	); actual != "original" {
		t.Fatalf(
			"original payload changed through clone: got %q",
			actual,
		)
	}

	if actual := payloads[0].Metadata()["source"]; actual != "node-input-test" {
		t.Fatalf(
			"original metadata changed through clone: got %q",
			actual,
		)
	}
}

func TestZeroNodeInputIsValidAndEmpty(
	t *testing.T,
) {
	var input NodeInput

	if !input.IsValid() {
		t.Fatal(
			"zero NodeInput IsValid() = false",
		)
	}

	if !input.IsEmpty() {
		t.Fatal(
			"zero NodeInput IsEmpty() = false",
		)
	}

	if input.PortCount() != 0 {
		t.Fatal(
			"zero NodeInput PortCount() must be 0",
		)
	}

	if input.TotalPayloadCount() != 0 {
		t.Fatal(
			"zero NodeInput TotalPayloadCount() must be 0",
		)
	}
}

func TestNodeInputDetectsMalformedInternalState(
	t *testing.T,
) {
	validPayload := newNodeInputPayloadForTest(
		t,
		"value",
	)

	tests := map[string]NodeInput{
		"port count mismatch": {
			inputs: map[string][]Payload{
				"input": {
					validPayload,
				},
			},
			ports: nil,
		},
		"duplicate stored port": {
			inputs: map[string][]Payload{
				"a": {
					validPayload,
				},
				"b": {
					validPayload,
				},
			},
			ports: []string{
				"a",
				"a",
			},
		},
		"non deterministic order": {
			inputs: map[string][]Payload{
				"a": {
					validPayload,
				},
				"b": {
					validPayload,
				},
			},
			ports: []string{
				"b",
				"a",
			},
		},
		"missing payload collection": {
			inputs: map[string][]Payload{
				"a": {
					validPayload,
				},
			},
			ports: []string{
				"b",
			},
		},
		"empty payload collection": {
			inputs: map[string][]Payload{
				"input": {},
			},
			ports: []string{
				"input",
			},
		},
		"invalid payload": {
			inputs: map[string][]Payload{
				"input": {
					{},
				},
			},
			ports: []string{
				"input",
			},
		},
	}

	for name, input := range tests {
		t.Run(name, func(t *testing.T) {
			if input.IsValid() {
				t.Fatal(
					"IsValid() = true for malformed NodeInput",
				)
			}
		})
	}
}

func newNodeInputPayloadForTest(
	t *testing.T,
	value string,
) Payload {
	t.Helper()

	payload, err := NewInlinePayload(
		ContentTypeTextPlain,
		[]byte(value),
		map[string]string{
			"source": "node-input-test",
		},
		1024,
	)
	if err != nil {
		t.Fatalf(
			"NewInlinePayload(%q) returned an unexpected error: %v",
			value,
			err,
		)
	}

	return payload
}

func nodeInputPayloadTextForTest(
	t *testing.T,
	payload Payload,
) string {
	t.Helper()

	data, exists := payload.InlineData()
	if !exists {
		t.Fatal(
			"payload does not contain inline data",
		)
	}

	return string(data)
}

func TestNodeResultStatusRecognizesSupportedValues(
	t *testing.T,
) {
	statuses := []NodeResultStatus{
		NodeResultStatusSucceeded,
		NodeResultStatusFailed,
	}

	for _, status := range statuses {
		t.Run(status.String(), func(t *testing.T) {
			if !status.IsValid() {
				t.Fatalf(
					"IsValid() = false for supported status %q",
					status,
				)
			}

			if status.String() == "" {
				t.Fatal(
					"String() returned an empty value",
				)
			}
		})
	}
}

func TestNodeResultStatusRejectsUnsupportedValues(
	t *testing.T,
) {
	statuses := map[string]NodeResultStatus{
		"zero":        "",
		"unsupported": "PARTIAL",
	}

	for name, status := range statuses {
		t.Run(name, func(t *testing.T) {
			if status.IsValid() {
				t.Fatalf(
					"IsValid() = true for unsupported status %q",
					status,
				)
			}
		})
	}
}

func TestNewNodeSuccessResultStoresRoutedOutputsAndContextChanges(
	t *testing.T,
) {
	contextChanges := newNodeResultContextChangesForTest(
		t,
	)

	result, err := NewNodeSuccessResult(
		map[string][]Payload{
			" right ": {
				newNodeInputPayloadForTest(
					t,
					"right-1",
				),
			},
			"left": {
				newNodeInputPayloadForTest(
					t,
					"left-1",
				),
				newNodeInputPayloadForTest(
					t,
					"left-2",
				),
			},
		},
		contextChanges,
	)
	if err != nil {
		t.Fatalf(
			"NewNodeSuccessResult() returned an unexpected error: %v",
			err,
		)
	}

	if !result.IsValid() {
		t.Fatal(
			"IsValid() = false for a valid success result",
		)
	}

	if actual := result.Status(); actual != NodeResultStatusSucceeded {
		t.Fatalf(
			"Status() = %q, want %q",
			actual,
			NodeResultStatusSucceeded,
		)
	}

	if !result.IsSuccess() {
		t.Fatal(
			"IsSuccess() = false",
		)
	}

	if result.IsFailure() {
		t.Fatal(
			"IsFailure() = true for a success result",
		)
	}

	if !result.HasRoutedOutputs() {
		t.Fatal(
			"HasRoutedOutputs() = false",
		)
	}

	if result.HasTerminalOutput() {
		t.Fatal(
			"HasTerminalOutput() = true for routed success",
		)
	}

	expectedPorts := []string{
		"left",
		"right",
	}

	if actual := result.OutputPorts(); !reflect.DeepEqual(
		actual,
		expectedPorts,
	) {
		t.Fatalf(
			"OutputPorts() = %#v, want %#v",
			actual,
			expectedPorts,
		)
	}

	if actual := result.TotalOutputPayloadCount(); actual != 3 {
		t.Fatalf(
			"TotalOutputPayloadCount() = %d, want 3",
			actual,
		)
	}

	leftPayloads, exists, err := result.OutputPayloads(
		" left ",
	)
	if err != nil {
		t.Fatalf(
			"OutputPayloads(left) returned an unexpected error: %v",
			err,
		)
	}

	if !exists {
		t.Fatal(
			"OutputPayloads(left) exists = false",
		)
	}

	if len(leftPayloads) != 2 {
		t.Fatalf(
			"len(OutputPayloads(left)) = %d, want 2",
			len(leftPayloads),
		)
	}

	if actual := nodeInputPayloadTextForTest(
		t,
		leftPayloads[0],
	); actual != "left-1" {
		t.Fatalf(
			"OutputPayloads(left)[0] = %q, want %q",
			actual,
			"left-1",
		)
	}

	if actual := nodeInputPayloadTextForTest(
		t,
		leftPayloads[1],
	); actual != "left-2" {
		t.Fatalf(
			"OutputPayloads(left)[1] = %q, want %q",
			actual,
			"left-2",
		)
	}

	actualChanges := result.ContextChanges()

	value, exists, err := actualChanges.SetValue(
		"currency",
	)
	if err != nil {
		t.Fatalf(
			"ContextChanges().SetValue(currency) returned an unexpected error: %v",
			err,
		)
	}

	if !exists {
		t.Fatal(
			"ContextChanges().SetValue(currency) exists = false",
		)
	}

	if actual := contextChangeStringValueForTest(
		t,
		value,
	); actual != "TRY" {
		t.Fatalf(
			"context change currency = %q, want %q",
			actual,
			"TRY",
		)
	}

	deletes, err := actualChanges.Deletes(
		"temporary",
	)
	if err != nil {
		t.Fatalf(
			"ContextChanges().Deletes(temporary) returned an unexpected error: %v",
			err,
		)
	}

	if !deletes {
		t.Fatal(
			"ContextChanges().Deletes(temporary) = false",
		)
	}

	if _, exists := result.TerminalOutput(); exists {
		t.Fatal(
			"TerminalOutput() exists = true for routed success",
		)
	}

	if _, exists := result.Failure(); exists {
		t.Fatal(
			"Failure() exists = true for success result",
		)
	}
}

func TestNewNodeSuccessResultAllowsContextOnlySuccess(
	t *testing.T,
) {
	contextChanges := newNodeResultContextChangesForTest(
		t,
	)

	result, err := NewNodeSuccessResult(
		nil,
		contextChanges,
	)
	if err != nil {
		t.Fatalf(
			"NewNodeSuccessResult() returned an unexpected error: %v",
			err,
		)
	}

	if !result.IsSuccess() {
		t.Fatal(
			"IsSuccess() = false",
		)
	}

	if result.HasRoutedOutputs() {
		t.Fatal(
			"HasRoutedOutputs() = true for context-only success",
		)
	}

	if result.HasTerminalOutput() {
		t.Fatal(
			"HasTerminalOutput() = true for context-only success",
		)
	}

	if result.OutputPorts() != nil {
		t.Fatal(
			"OutputPorts() must return nil for context-only success",
		)
	}

	if result.OutputsSnapshot() != nil {
		t.Fatal(
			"OutputsSnapshot() must return nil for context-only success",
		)
	}

	if result.TotalOutputPayloadCount() != 0 {
		t.Fatal(
			"TotalOutputPayloadCount() must return 0",
		)
	}

	if result.ContextChanges().IsEmpty() {
		t.Fatal(
			"ContextChanges() unexpectedly returned empty changes",
		)
	}
}

func TestNewNodeSuccessResultAllowsEmptySuccess(
	t *testing.T,
) {
	result, err := NewNodeSuccessResult(
		nil,
		ContextChanges{},
	)
	if err != nil {
		t.Fatalf(
			"NewNodeSuccessResult() returned an unexpected error: %v",
			err,
		)
	}

	if !result.IsValid() {
		t.Fatal(
			"IsValid() = false for empty success result",
		)
	}

	if !result.IsSuccess() {
		t.Fatal(
			"IsSuccess() = false",
		)
	}

	if result.HasRoutedOutputs() {
		t.Fatal(
			"HasRoutedOutputs() = true",
		)
	}

	if !result.ContextChanges().IsEmpty() {
		t.Fatal(
			"ContextChanges() must be empty",
		)
	}
}

func TestNewTerminalNodeSuccessResultStoresTerminalOutput(
	t *testing.T,
) {
	terminalPayload := newNodeInputPayloadForTest(
		t,
		"workflow-result",
	)

	contextChanges := newNodeResultContextChangesForTest(
		t,
	)

	result, err := NewTerminalNodeSuccessResult(
		terminalPayload,
		contextChanges,
	)
	if err != nil {
		t.Fatalf(
			"NewTerminalNodeSuccessResult() returned an unexpected error: %v",
			err,
		)
	}

	if !result.IsValid() {
		t.Fatal(
			"IsValid() = false for terminal success",
		)
	}

	if !result.IsSuccess() {
		t.Fatal(
			"IsSuccess() = false",
		)
	}

	if result.HasRoutedOutputs() {
		t.Fatal(
			"HasRoutedOutputs() = true for terminal success",
		)
	}

	if !result.HasTerminalOutput() {
		t.Fatal(
			"HasTerminalOutput() = false",
		)
	}

	if result.OutputPorts() != nil {
		t.Fatal(
			"OutputPorts() must return nil for terminal success",
		)
	}

	if result.OutputsSnapshot() != nil {
		t.Fatal(
			"OutputsSnapshot() must return nil for terminal success",
		)
	}

	actual, exists := result.TerminalOutput()
	if !exists {
		t.Fatal(
			"TerminalOutput() exists = false",
		)
	}

	if actualText := nodeInputPayloadTextForTest(
		t,
		actual,
	); actualText != "workflow-result" {
		t.Fatalf(
			"TerminalOutput() = %q, want %q",
			actualText,
			"workflow-result",
		)
	}

	if result.ContextChanges().IsEmpty() {
		t.Fatal(
			"terminal result ContextChanges() is unexpectedly empty",
		)
	}

	if _, exists := result.Failure(); exists {
		t.Fatal(
			"Failure() exists = true for terminal success",
		)
	}
}

func TestNewNodeFailureResultStoresControlledFailure(
	t *testing.T,
) {
	expectedFailure := newRuntimeFailureForTest(
		t,
	)

	result, err := NewNodeFailureResult(
		expectedFailure,
	)
	if err != nil {
		t.Fatalf(
			"NewNodeFailureResult() returned an unexpected error: %v",
			err,
		)
	}

	if !result.IsValid() {
		t.Fatal(
			"IsValid() = false for controlled failure result",
		)
	}

	if actual := result.Status(); actual != NodeResultStatusFailed {
		t.Fatalf(
			"Status() = %q, want %q",
			actual,
			NodeResultStatusFailed,
		)
	}

	if result.IsSuccess() {
		t.Fatal(
			"IsSuccess() = true for failed result",
		)
	}

	if !result.IsFailure() {
		t.Fatal(
			"IsFailure() = false",
		)
	}

	if result.HasRoutedOutputs() {
		t.Fatal(
			"HasRoutedOutputs() = true for failed result",
		)
	}

	if result.HasTerminalOutput() {
		t.Fatal(
			"HasTerminalOutput() = true for failed result",
		)
	}

	if result.OutputPorts() != nil {
		t.Fatal(
			"OutputPorts() must return nil for failed result",
		)
	}

	if result.OutputsSnapshot() != nil {
		t.Fatal(
			"OutputsSnapshot() must return nil for failed result",
		)
	}

	if !result.ContextChanges().IsEmpty() {
		t.Fatal(
			"failed result ContextChanges() must be empty",
		)
	}

	actualFailure, exists := result.Failure()
	if !exists {
		t.Fatal(
			"Failure() exists = false",
		)
	}

	if actualFailure.Category() != expectedFailure.Category() {
		t.Fatalf(
			"failure category = %q, want %q",
			actualFailure.Category(),
			expectedFailure.Category(),
		)
	}

	if actualFailure.Code() != expectedFailure.Code() {
		t.Fatalf(
			"failure code = %q, want %q",
			actualFailure.Code(),
			expectedFailure.Code(),
		)
	}

	if actualFailure.Message() != expectedFailure.Message() {
		t.Fatalf(
			"failure message = %q, want %q",
			actualFailure.Message(),
			expectedFailure.Message(),
		)
	}

	if actualFailure.Retryable() != expectedFailure.Retryable() {
		t.Fatalf(
			"failure retryable = %t, want %t",
			actualFailure.Retryable(),
			expectedFailure.Retryable(),
		)
	}
}

func TestNewNodeSuccessResultRejectsInvalidOutputs(
	t *testing.T,
) {
	validPayload := newNodeInputPayloadForTest(
		t,
		"value",
	)

	t.Run("blank output port", func(t *testing.T) {
		_, err := NewNodeSuccessResult(
			map[string][]Payload{
				" ": {
					validPayload,
				},
			},
			ContextChanges{},
		)

		requireRuntimeValidationField(
			t,
			err,
			"nodeResult.outputPort",
		)
	})

	t.Run("duplicate normalized output port", func(t *testing.T) {
		_, err := NewNodeSuccessResult(
			map[string][]Payload{
				"output": {
					validPayload,
				},
				" output ": {
					validPayload,
				},
			},
			ContextChanges{},
		)

		requireRuntimeValidationField(
			t,
			err,
			"nodeResult.outputs",
		)
	})

	t.Run("empty output collection", func(t *testing.T) {
		_, err := NewNodeSuccessResult(
			map[string][]Payload{
				"output": {},
			},
			ContextChanges{},
		)

		requireRuntimeValidationField(
			t,
			err,
			"nodeResult.outputs",
		)
	})

	t.Run("invalid output payload", func(t *testing.T) {
		_, err := NewNodeSuccessResult(
			map[string][]Payload{
				"output": {
					{},
				},
			},
			ContextChanges{},
		)

		requireRuntimeValidationField(
			t,
			err,
			"nodeResult.output[0]",
		)
	})
}

func TestNodeResultConstructorsRejectInvalidContextChanges(
	t *testing.T,
) {
	invalidChanges := ContextChanges{
		setValues: map[string]RuntimeValue{
			ProtectedKeyCompanyID: newContextChangeStringValueForTest(
				t,
				"changed-company",
			),
		},
		setOrder: []string{
			ProtectedKeyCompanyID,
		},
		deleteKeys: map[string]struct{}{},
	}

	t.Run("routed success", func(t *testing.T) {
		_, err := NewNodeSuccessResult(
			nil,
			invalidChanges,
		)

		requireRuntimeValidationField(
			t,
			err,
			"contextChanges",
		)
	})

	t.Run("terminal success", func(t *testing.T) {
		_, err := NewTerminalNodeSuccessResult(
			newNodeInputPayloadForTest(
				t,
				"terminal",
			),
			invalidChanges,
		)

		requireRuntimeValidationField(
			t,
			err,
			"contextChanges",
		)
	})
}

func TestTerminalResultRejectsInvalidPayload(
	t *testing.T,
) {
	_, err := NewTerminalNodeSuccessResult(
		Payload{},
		ContextChanges{},
	)

	requireRuntimeValidationField(
		t,
		err,
		"terminalOutput",
	)
}

func TestFailureResultRejectsInvalidFailure(
	t *testing.T,
) {
	_, err := NewNodeFailureResult(
		RuntimeFailure{},
	)

	requireRuntimeValidationField(
		t,
		err,
		"failure",
	)
}

func TestNodeResultOutputLookupHandlesMissingAndInvalidPorts(
	t *testing.T,
) {
	result, err := NewNodeSuccessResult(
		map[string][]Payload{
			"output": {
				newNodeInputPayloadForTest(
					t,
					"value",
				),
			},
		},
		ContextChanges{},
	)
	if err != nil {
		t.Fatalf(
			"NewNodeSuccessResult() returned an unexpected error: %v",
			err,
		)
	}

	payloads, exists, err := result.OutputPayloads(
		"missing",
	)
	if err != nil {
		t.Fatalf(
			"OutputPayloads(missing) returned an unexpected error: %v",
			err,
		)
	}

	if exists {
		t.Fatal(
			"OutputPayloads(missing) exists = true",
		)
	}

	if payloads != nil {
		t.Fatalf(
			"OutputPayloads(missing) = %#v, want nil",
			payloads,
		)
	}

	_, _, err = result.OutputPayloads(" ")

	requireRuntimeValidationField(
		t,
		err,
		"nodeResult.outputPort",
	)
}

func TestNodeSuccessResultCopiesConstructorData(
	t *testing.T,
) {
	payload, err := NewInlinePayload(
		ContentTypeTextPlain,
		[]byte("original"),
		map[string]string{
			"source": "original-source",
		},
		100,
	)
	if err != nil {
		t.Fatalf(
			"NewInlinePayload() returned an unexpected error: %v",
			err,
		)
	}

	outputPayloads := []Payload{
		payload,
	}

	outputs := map[string][]Payload{
		" output ": outputPayloads,
	}

	contextChanges := newNodeResultContextChangesForTest(
		t,
	)

	result, err := NewNodeSuccessResult(
		outputs,
		contextChanges,
	)
	if err != nil {
		t.Fatalf(
			"NewNodeSuccessResult() returned an unexpected error: %v",
			err,
		)
	}

	payload.inlineData[0] = 'X'
	payload.metadata["source"] = "changed"

	outputPayloads[0].inlineData[0] = 'Y'
	outputPayloads[0].metadata["source"] =
		"changed-again"

	outputs[" output "] = nil

	contextChanges.setValues["currency"] =
		newContextChangeStringValueForTest(
			t,
			"USD",
		)

	delete(
		contextChanges.deleteKeys,
		"temporary",
	)

	actualPayloads, exists, err :=
		result.OutputPayloads("output")
	if err != nil {
		t.Fatalf(
			"OutputPayloads(output) returned an unexpected error: %v",
			err,
		)
	}

	if !exists {
		t.Fatal(
			"OutputPayloads(output) exists = false",
		)
	}

	if actual := nodeInputPayloadTextForTest(
		t,
		actualPayloads[0],
	); actual != "original" {
		t.Fatalf(
			"stored output payload = %q, want %q",
			actual,
			"original",
		)
	}

	if actual := actualPayloads[0].Metadata()["source"]; actual != "original-source" {
		t.Fatalf(
			"stored output metadata = %q, want %q",
			actual,
			"original-source",
		)
	}

	actualChanges := result.ContextChanges()

	actualValue, exists, err :=
		actualChanges.SetValue("currency")
	if err != nil {
		t.Fatalf(
			"SetValue(currency) returned an unexpected error: %v",
			err,
		)
	}

	if !exists {
		t.Fatal(
			"SetValue(currency) exists = false",
		)
	}

	if actual := contextChangeStringValueForTest(
		t,
		actualValue,
	); actual != "TRY" {
		t.Fatalf(
			"stored currency = %q, want %q",
			actual,
			"TRY",
		)
	}

	deletes, err := actualChanges.Deletes(
		"temporary",
	)
	if err != nil {
		t.Fatalf(
			"Deletes(temporary) returned an unexpected error: %v",
			err,
		)
	}

	if !deletes {
		t.Fatal(
			"stored delete operation changed through constructor input",
		)
	}
}

func TestNodeResultReturnedRoutedValuesAreIndependent(
	t *testing.T,
) {
	result, err := NewNodeSuccessResult(
		map[string][]Payload{
			"output": {
				newNodeInputPayloadForTest(
					t,
					"original",
				),
			},
		},
		newNodeResultContextChangesForTest(
			t,
		),
	)
	if err != nil {
		t.Fatalf(
			"NewNodeSuccessResult() returned an unexpected error: %v",
			err,
		)
	}

	ports := result.OutputPorts()
	ports[0] = "changed"

	payloads, exists, err :=
		result.OutputPayloads("output")
	if err != nil {
		t.Fatalf(
			"OutputPayloads(output) returned an unexpected error: %v",
			err,
		)
	}

	if !exists {
		t.Fatal(
			"OutputPayloads(output) exists = false",
		)
	}

	payloads[0].inlineData[0] = 'X'
	payloads[0].metadata["source"] = "changed"
	payloads[0] = Payload{}

	snapshot := result.OutputsSnapshot()
	snapshotPayload := snapshot["output"][0]
	snapshotPayload.inlineData[0] = 'Y'
	snapshotPayload.metadata["source"] =
		"changed-again"

	snapshot["output"][0] = snapshotPayload
	delete(
		snapshot,
		"output",
	)

	changes := result.ContextChanges()

	changedValue :=
		changes.setValues["currency"]

	changedValue.raw[0] = '['

	changes.setValues["currency"] =
		changedValue

	delete(
		changes.deleteKeys,
		"temporary",
	)

	if actual := result.OutputPorts()[0]; actual != "output" {
		t.Fatalf(
			"output port changed through returned slice: got %q",
			actual,
		)
	}

	secondPayloads, exists, err :=
		result.OutputPayloads("output")
	if err != nil {
		t.Fatalf(
			"second OutputPayloads(output) returned an unexpected error: %v",
			err,
		)
	}

	if !exists {
		t.Fatal(
			"second OutputPayloads(output) exists = false",
		)
	}

	if actual := nodeInputPayloadTextForTest(
		t,
		secondPayloads[0],
	); actual != "original" {
		t.Fatalf(
			"output payload changed through accessor: got %q",
			actual,
		)
	}

	if actual := secondPayloads[0].Metadata()["source"]; actual != "node-input-test" {
		t.Fatalf(
			"output metadata changed through accessor: got %q",
			actual,
		)
	}

	secondChanges := result.ContextChanges()

	value, exists, err := secondChanges.SetValue(
		"currency",
	)
	if err != nil {
		t.Fatalf(
			"SetValue(currency) returned an unexpected error: %v",
			err,
		)
	}

	if !exists {
		t.Fatal(
			"SetValue(currency) exists = false",
		)
	}

	if actual := contextChangeStringValueForTest(
		t,
		value,
	); actual != "TRY" {
		t.Fatalf(
			"context change mutated through result accessor: got %q",
			actual,
		)
	}

	deletes, err := secondChanges.Deletes(
		"temporary",
	)
	if err != nil {
		t.Fatalf(
			"Deletes(temporary) returned an unexpected error: %v",
			err,
		)
	}

	if !deletes {
		t.Fatal(
			"context delete operation changed through result accessor",
		)
	}
}

func TestNodeResultReturnedTerminalOutputIsIndependent(
	t *testing.T,
) {
	result, err := NewTerminalNodeSuccessResult(
		newNodeInputPayloadForTest(
			t,
			"original",
		),
		ContextChanges{},
	)
	if err != nil {
		t.Fatalf(
			"NewTerminalNodeSuccessResult() returned an unexpected error: %v",
			err,
		)
	}

	terminalOutput, exists :=
		result.TerminalOutput()
	if !exists {
		t.Fatal(
			"TerminalOutput() exists = false",
		)
	}

	terminalOutput.inlineData[0] = 'X'
	terminalOutput.metadata["source"] = "changed"

	secondTerminalOutput, exists :=
		result.TerminalOutput()
	if !exists {
		t.Fatal(
			"second TerminalOutput() exists = false",
		)
	}

	if actual := nodeInputPayloadTextForTest(
		t,
		secondTerminalOutput,
	); actual != "original" {
		t.Fatalf(
			"terminal output changed through accessor: got %q",
			actual,
		)
	}

	if actual := secondTerminalOutput.Metadata()["source"]; actual != "node-input-test" {
		t.Fatalf(
			"terminal metadata changed through accessor: got %q",
			actual,
		)
	}
}

func TestNodeResultReturnedFailureIsIndependent(
	t *testing.T,
) {
	result, err := NewNodeFailureResult(
		newRuntimeFailureForTest(
			t,
		),
	)
	if err != nil {
		t.Fatalf(
			"NewNodeFailureResult() returned an unexpected error: %v",
			err,
		)
	}

	failure, exists := result.Failure()
	if !exists {
		t.Fatal(
			"Failure() exists = false",
		)
	}

	failure.details["service"] = "changed"
	delete(
		failure.details,
		"operation",
	)

	secondFailure, exists := result.Failure()
	if !exists {
		t.Fatal(
			"second Failure() exists = false",
		)
	}

	details := secondFailure.Details()

	if actual := details["service"]; actual != "billing" {
		t.Fatalf(
			"failure service changed through accessor: got %q",
			actual,
		)
	}

	if actual := details["operation"]; actual != "fetch-customer" {
		t.Fatalf(
			"failure operation changed through accessor: got %q",
			actual,
		)
	}
}

func TestNodeResultCloneCreatesIndependentCopies(
	t *testing.T,
) {
	t.Run("routed success", func(t *testing.T) {
		result, err := NewNodeSuccessResult(
			map[string][]Payload{
				"output": {
					newNodeInputPayloadForTest(
						t,
						"original",
					),
				},
			},
			newNodeResultContextChangesForTest(
				t,
			),
		)
		if err != nil {
			t.Fatalf(
				"NewNodeSuccessResult() returned an unexpected error: %v",
				err,
			)
		}

		cloned := cloneNodeResult(result)

		cloned.outputPorts[0] = "changed"
		cloned.outputs["output"][0].inlineData[0] =
			'X'

		changedValue :=
			cloned.contextChanges.
				setValues["currency"]

		changedValue.raw[0] = '['

		cloned.contextChanges.
			setValues["currency"] =
			changedValue

		if actual := result.OutputPorts()[0]; actual != "output" {
			t.Fatalf(
				"original output port changed through clone: got %q",
				actual,
			)
		}

		payloads, exists, err :=
			result.OutputPayloads("output")
		if err != nil {
			t.Fatalf(
				"OutputPayloads(output) returned an unexpected error: %v",
				err,
			)
		}

		if !exists {
			t.Fatal(
				"OutputPayloads(output) exists = false",
			)
		}

		if actual := nodeInputPayloadTextForTest(
			t,
			payloads[0],
		); actual != "original" {
			t.Fatalf(
				"original output changed through clone: got %q",
				actual,
			)
		}

		value, exists, err :=
			result.ContextChanges().
				SetValue("currency")
		if err != nil {
			t.Fatalf(
				"SetValue(currency) returned an unexpected error: %v",
				err,
			)
		}

		if !exists {
			t.Fatal(
				"SetValue(currency) exists = false",
			)
		}

		if actual := contextChangeStringValueForTest(
			t,
			value,
		); actual != "TRY" {
			t.Fatalf(
				"original context change changed through clone: got %q",
				actual,
			)
		}
	})

	t.Run("terminal success", func(t *testing.T) {
		result, err := NewTerminalNodeSuccessResult(
			newNodeInputPayloadForTest(
				t,
				"terminal",
			),
			ContextChanges{},
		)
		if err != nil {
			t.Fatalf(
				"NewTerminalNodeSuccessResult() returned an unexpected error: %v",
				err,
			)
		}

		cloned := cloneNodeResult(result)

		cloned.terminalOutput.inlineData[0] = 'X'

		actual, exists := result.TerminalOutput()
		if !exists {
			t.Fatal(
				"TerminalOutput() exists = false",
			)
		}

		if actualText := nodeInputPayloadTextForTest(
			t,
			actual,
		); actualText != "terminal" {
			t.Fatalf(
				"original terminal output changed through clone: got %q",
				actualText,
			)
		}
	})

	t.Run("failure", func(t *testing.T) {
		result, err := NewNodeFailureResult(
			newRuntimeFailureForTest(
				t,
			),
		)
		if err != nil {
			t.Fatalf(
				"NewNodeFailureResult() returned an unexpected error: %v",
				err,
			)
		}

		cloned := cloneNodeResult(result)

		cloned.failure.details["service"] =
			"changed"

		actual, exists := result.Failure()
		if !exists {
			t.Fatal(
				"Failure() exists = false",
			)
		}

		if actualService :=
			actual.Details()["service"]; actualService != "billing" {
			t.Fatalf(
				"original failure changed through clone: got %q",
				actualService,
			)
		}
	})
}

func TestZeroNodeResultIsInvalid(
	t *testing.T,
) {
	var result NodeResult

	if result.IsValid() {
		t.Fatal(
			"zero NodeResult IsValid() = true",
		)
	}

	if result.Status().IsValid() {
		t.Fatal(
			"zero NodeResult status must be invalid",
		)
	}

	if result.IsSuccess() {
		t.Fatal(
			"zero NodeResult IsSuccess() = true",
		)
	}

	if result.IsFailure() {
		t.Fatal(
			"zero NodeResult IsFailure() = true",
		)
	}

	if result.HasRoutedOutputs() {
		t.Fatal(
			"zero NodeResult HasRoutedOutputs() = true",
		)
	}

	if result.HasTerminalOutput() {
		t.Fatal(
			"zero NodeResult HasTerminalOutput() = true",
		)
	}

	if result.OutputPorts() != nil {
		t.Fatal(
			"zero NodeResult OutputPorts() must return nil",
		)
	}

	if result.OutputsSnapshot() != nil {
		t.Fatal(
			"zero NodeResult OutputsSnapshot() must return nil",
		)
	}

	if result.TotalOutputPayloadCount() != 0 {
		t.Fatal(
			"zero NodeResult TotalOutputPayloadCount() must return 0",
		)
	}

	if _, exists := result.TerminalOutput(); exists {
		t.Fatal(
			"zero NodeResult TerminalOutput() exists = true",
		)
	}

	if _, exists := result.Failure(); exists {
		t.Fatal(
			"zero NodeResult Failure() exists = true",
		)
	}

	if !result.ContextChanges().IsEmpty() {
		t.Fatal(
			"zero NodeResult ContextChanges() must be empty",
		)
	}
}

func TestNodeResultDetectsMalformedInternalState(
	t *testing.T,
) {
	validPayload := newNodeInputPayloadForTest(
		t,
		"value",
	)

	terminalPayload := newNodeInputPayloadForTest(
		t,
		"terminal",
	)

	validFailure := newRuntimeFailureForTest(
		t,
	)

	contextChanges := newNodeResultContextChangesForTest(
		t,
	)

	tests := map[string]NodeResult{
		"unsupported status": {
			status: NodeResultStatus(
				"PARTIAL",
			),
		},
		"success with failure": {
			status:  NodeResultStatusSucceeded,
			outputs: map[string][]Payload{},
			failure: &validFailure,
		},
		"success with routed and terminal output": {
			status: NodeResultStatusSucceeded,
			outputs: map[string][]Payload{
				"output": {
					validPayload,
				},
			},
			outputPorts: []string{
				"output",
			},
			terminalOutput: &terminalPayload,
		},
		"success with mismatched output order": {
			status: NodeResultStatusSucceeded,
			outputs: map[string][]Payload{
				"output": {
					validPayload,
				},
			},
			outputPorts: nil,
		},
		"success with non deterministic output order": {
			status: NodeResultStatusSucceeded,
			outputs: map[string][]Payload{
				"a": {
					validPayload,
				},
				"b": {
					validPayload,
				},
			},
			outputPorts: []string{
				"b",
				"a",
			},
		},
		"success with invalid context changes": {
			status:  NodeResultStatusSucceeded,
			outputs: map[string][]Payload{},
			contextChanges: ContextChanges{
				setValues: map[string]RuntimeValue{
					ProtectedKeyCompanyID: newContextChangeStringValueForTest(
						t,
						"changed",
					),
				},
				setOrder: []string{
					ProtectedKeyCompanyID,
				},
				deleteKeys: map[string]struct{}{},
			},
		},
		"failure without failure details": {
			status:  NodeResultStatusFailed,
			outputs: map[string][]Payload{},
		},
		"failure with routed outputs": {
			status: NodeResultStatusFailed,
			outputs: map[string][]Payload{
				"output": {
					validPayload,
				},
			},
			outputPorts: []string{
				"output",
			},
			failure: &validFailure,
		},
		"failure with terminal output": {
			status:         NodeResultStatusFailed,
			outputs:        map[string][]Payload{},
			terminalOutput: &terminalPayload,
			failure:        &validFailure,
		},
		"failure with context changes": {
			status:         NodeResultStatusFailed,
			outputs:        map[string][]Payload{},
			contextChanges: contextChanges,
			failure:        &validFailure,
		},
	}

	for name, result := range tests {
		t.Run(name, func(t *testing.T) {
			if result.IsValid() {
				t.Fatal(
					"IsValid() = true for malformed NodeResult",
				)
			}
		})
	}
}

func newNodeResultContextChangesForTest(
	t *testing.T,
) ContextChanges {
	t.Helper()

	changes, err := NewContextChanges(
		map[string]RuntimeValue{
			"currency": newContextChangeStringValueForTest(
				t,
				"TRY",
			),
		},
		[]string{
			"temporary",
		},
	)
	if err != nil {
		t.Fatalf(
			"NewContextChanges() returned an unexpected error: %v",
			err,
		)
	}

	return changes
}

func TestPayloadContentTypeNormalizesConcreteMediaType(
	t *testing.T,
) {
	contentType, err := NewContentType(
		"  Application/Problem+JSON  ",
	)
	if err != nil {
		t.Fatalf(
			"NewContentType() returned an unexpected error: %v",
			err,
		)
	}

	if actual := contentType.String(); actual != "application/problem+json" {
		t.Fatalf(
			"String() = %q, want %q",
			actual,
			"application/problem+json",
		)
	}

	if !contentType.IsValid() {
		t.Fatal(
			"IsValid() = false for a valid content type",
		)
	}

	if !contentType.IsJSON() {
		t.Fatal(
			"IsJSON() = false for a +json content type",
		)
	}

	if ContentTypeTextPlain.IsJSON() {
		t.Fatal(
			"IsJSON() = true for text/plain",
		)
	}
}

func TestPayloadContentTypeRejectsInvalidValues(
	t *testing.T,
) {
	tests := map[string]string{
		"blank":            "   ",
		"invalid":          "application",
		"wildcard type":    "*/json",
		"wildcard subtype": "application/*",
	}

	for name, value := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := NewContentType(value)

			requirePayloadValidationField(
				t,
				err,
				"contentType",
			)
		})
	}
}

func TestPayloadArtifactReferenceNormalizesAndCopiesValues(
	t *testing.T,
) {
	metadata := map[string]string{
		"source": " report-service ",
	}

	reference, err := NewArtifactReference(
		" artifact-1 ",
		" workflows/execution-1/output.json ",
		ContentTypeApplicationJSON,
		128,
		" sha256:abc ",
		metadata,
	)
	if err != nil {
		t.Fatalf(
			"NewArtifactReference() returned an unexpected error: %v",
			err,
		)
	}

	if actual := reference.ID(); actual != "artifact-1" {
		t.Fatalf(
			"ID() = %q, want %q",
			actual,
			"artifact-1",
		)
	}

	if actual := reference.Location(); actual != "workflows/execution-1/output.json" {
		t.Fatalf(
			"Location() = %q, want %q",
			actual,
			"workflows/execution-1/output.json",
		)
	}

	if actual := reference.ContentType(); actual != ContentTypeApplicationJSON {
		t.Fatalf(
			"ContentType() = %q, want %q",
			actual,
			ContentTypeApplicationJSON,
		)
	}

	if actual := reference.SizeBytes(); actual != 128 {
		t.Fatalf(
			"SizeBytes() = %d, want %d",
			actual,
			128,
		)
	}

	if actual := reference.Checksum(); actual != "sha256:abc" {
		t.Fatalf(
			"Checksum() = %q, want %q",
			actual,
			"sha256:abc",
		)
	}

	metadata["source"] = "changed-outside"

	actualMetadata := reference.Metadata()
	if actual := actualMetadata["source"]; actual != " report-service " {
		t.Fatalf(
			"Metadata()[source] = %q, want %q",
			actual,
			" report-service ",
		)
	}

	actualMetadata["source"] = "changed-through-getter"

	secondMetadata := reference.Metadata()
	if actual := secondMetadata["source"]; actual != " report-service " {
		t.Fatalf(
			"metadata changed through getter: got %q",
			actual,
		)
	}
}

func TestPayloadArtifactReferenceRejectsInvalidValues(
	t *testing.T,
) {
	t.Run("blank ID", func(t *testing.T) {
		_, err := NewArtifactReference(
			" ",
			"storage/output.json",
			ContentTypeApplicationJSON,
			1,
			"",
			nil,
		)

		requirePayloadValidationField(
			t,
			err,
			"artifact.id",
		)
	})

	t.Run("blank location", func(t *testing.T) {
		_, err := NewArtifactReference(
			"artifact-1",
			" ",
			ContentTypeApplicationJSON,
			1,
			"",
			nil,
		)

		requirePayloadValidationField(
			t,
			err,
			"artifact.location",
		)
	})

	t.Run("invalid content type", func(t *testing.T) {
		_, err := NewArtifactReference(
			"artifact-1",
			"storage/output.json",
			ContentType("application"),
			1,
			"",
			nil,
		)

		requirePayloadValidationField(
			t,
			err,
			"artifact.contentType",
		)
	})

	t.Run("negative size", func(t *testing.T) {
		_, err := NewArtifactReference(
			"artifact-1",
			"storage/output.json",
			ContentTypeApplicationJSON,
			-1,
			"",
			nil,
		)

		requirePayloadValidationField(
			t,
			err,
			"artifact.sizeBytes",
		)
	})

	t.Run("blank metadata key", func(t *testing.T) {
		_, err := NewArtifactReference(
			"artifact-1",
			"storage/output.json",
			ContentTypeApplicationJSON,
			1,
			"",
			map[string]string{
				" ": "value",
			},
		)

		requirePayloadValidationField(
			t,
			err,
			"artifact.metadata",
		)
	})

	t.Run("duplicate normalized metadata keys", func(t *testing.T) {
		_, err := NewArtifactReference(
			"artifact-1",
			"storage/output.json",
			ContentTypeApplicationJSON,
			1,
			"",
			map[string]string{
				"source":   "first",
				" source ": "second",
			},
		)

		requirePayloadValidationField(
			t,
			err,
			"artifact.metadata",
		)
	})
}

func TestPayloadInlineStoresAndCopiesData(
	t *testing.T,
) {
	originalData := []byte(
		`{"customerId":42}`,
	)
	originalMetadata := map[string]string{
		"source": "core.static-input",
	}

	payload, err := NewInlinePayload(
		ContentTypeApplicationJSON,
		originalData,
		originalMetadata,
		len(originalData),
	)
	if err != nil {
		t.Fatalf(
			"NewInlinePayload() returned an unexpected error: %v",
			err,
		)
	}

	if !payload.IsInline() {
		t.Fatal(
			"IsInline() = false for an inline payload",
		)
	}

	if payload.IsArtifact() {
		t.Fatal(
			"IsArtifact() = true for an inline payload",
		)
	}

	if actual := payload.ContentType(); actual != ContentTypeApplicationJSON {
		t.Fatalf(
			"ContentType() = %q, want %q",
			actual,
			ContentTypeApplicationJSON,
		)
	}

	originalData[0] = '['
	originalMetadata["source"] = "changed-outside"

	storedData, exists := payload.InlineData()
	if !exists {
		t.Fatal(
			"InlineData() exists = false for an inline payload",
		)
	}

	if actual := string(storedData); actual != `{"customerId":42}` {
		t.Fatalf(
			"InlineData() = %q, want %q",
			actual,
			`{"customerId":42}`,
		)
	}

	storedData[0] = '['

	secondData, exists := payload.InlineData()
	if !exists {
		t.Fatal(
			"second InlineData() exists = false",
		)
	}

	if actual := string(secondData); actual != `{"customerId":42}` {
		t.Fatalf(
			"payload data changed through getter: got %q",
			actual,
		)
	}

	actualMetadata := payload.Metadata()
	if actual := actualMetadata["source"]; actual != "core.static-input" {
		t.Fatalf(
			"Metadata()[source] = %q, want %q",
			actual,
			"core.static-input",
		)
	}

	actualMetadata["source"] = "changed-through-getter"

	secondMetadata := payload.Metadata()
	if actual := secondMetadata["source"]; actual != "core.static-input" {
		t.Fatalf(
			"payload metadata changed through getter: got %q",
			actual,
		)
	}

	if _, exists := payload.Artifact(); exists {
		t.Fatal(
			"Artifact() exists = true for an inline payload",
		)
	}
}

func TestPayloadInlineAcceptsEmptyContent(
	t *testing.T,
) {
	payload, err := NewInlinePayload(
		ContentTypeTextPlain,
		[]byte{},
		nil,
		1,
	)
	if err != nil {
		t.Fatalf(
			"NewInlinePayload() returned an unexpected error: %v",
			err,
		)
	}

	data, exists := payload.InlineData()
	if !exists {
		t.Fatal(
			"InlineData() exists = false for an empty inline payload",
		)
	}

	if len(data) != 0 {
		t.Fatalf(
			"len(InlineData()) = %d, want 0",
			len(data),
		)
	}
}

func TestPayloadInlineRejectsInvalidLimitsAndMetadata(
	t *testing.T,
) {
	t.Run("non-positive maximum", func(t *testing.T) {
		_, err := NewInlinePayload(
			ContentTypeTextPlain,
			[]byte("value"),
			nil,
			0,
		)

		requirePayloadValidationField(
			t,
			err,
			"maximumInlineBytes",
		)
	})

	t.Run("payload exceeds maximum", func(t *testing.T) {
		_, err := NewInlinePayload(
			ContentTypeTextPlain,
			[]byte("value"),
			nil,
			4,
		)

		requirePayloadValidationField(
			t,
			err,
			"payload.inlineData",
		)
	})

	t.Run("invalid content type", func(t *testing.T) {
		_, err := NewInlinePayload(
			ContentType("application"),
			[]byte("value"),
			nil,
			10,
		)

		requirePayloadValidationField(
			t,
			err,
			"payload.contentType",
		)
	})

	t.Run("duplicate normalized metadata keys", func(t *testing.T) {
		_, err := NewInlinePayload(
			ContentTypeTextPlain,
			[]byte("value"),
			map[string]string{
				"source":   "first",
				" source ": "second",
			},
			10,
		)

		requirePayloadValidationField(
			t,
			err,
			"payload.metadata",
		)
	})
}

func TestPayloadRejectsInvalidSourceCombinations(
	t *testing.T,
) {
	artifact := newArtifactReferenceForPayloadTest(t)

	t.Run("neither source exists", func(t *testing.T) {
		_, err := newPayload(
			ContentTypeApplicationJSON,
			nil,
			false,
			nil,
			nil,
			10,
		)

		requirePayloadValidationField(
			t,
			err,
			"payload.source",
		)
	})

	t.Run("both sources exist", func(t *testing.T) {
		_, err := newPayload(
			artifact.ContentType(),
			[]byte("value"),
			true,
			&artifact,
			nil,
			10,
		)

		requirePayloadValidationField(
			t,
			err,
			"payload.source",
		)
	})
}

func TestPayloadArtifactStoresAndCopiesReference(
	t *testing.T,
) {
	artifactMetadata := map[string]string{
		"artifact-source": "storage",
	}
	payloadMetadata := map[string]string{
		"execution": "execution-1",
	}

	artifact, err := NewArtifactReference(
		"artifact-1",
		"workflows/execution-1/output.bin",
		ContentTypeApplicationOctetStream,
		1024,
		"sha256:abc",
		artifactMetadata,
	)
	if err != nil {
		t.Fatalf(
			"NewArtifactReference() returned an unexpected error: %v",
			err,
		)
	}

	payload, err := NewArtifactPayload(
		artifact,
		payloadMetadata,
	)
	if err != nil {
		t.Fatalf(
			"NewArtifactPayload() returned an unexpected error: %v",
			err,
		)
	}

	if payload.IsInline() {
		t.Fatal(
			"IsInline() = true for an artifact payload",
		)
	}

	if !payload.IsArtifact() {
		t.Fatal(
			"IsArtifact() = false for an artifact payload",
		)
	}

	if _, exists := payload.InlineData(); exists {
		t.Fatal(
			"InlineData() exists = true for an artifact payload",
		)
	}

	artifactMetadata["artifact-source"] = "changed-outside"
	payloadMetadata["execution"] = "changed-outside"

	actualArtifact, exists := payload.Artifact()
	if !exists {
		t.Fatal(
			"Artifact() exists = false for an artifact payload",
		)
	}

	if actual := actualArtifact.ID(); actual != "artifact-1" {
		t.Fatalf(
			"Artifact().ID() = %q, want %q",
			actual,
			"artifact-1",
		)
	}

	actualArtifactMetadata := actualArtifact.Metadata()
	if actual := actualArtifactMetadata["artifact-source"]; actual != "storage" {
		t.Fatalf(
			"artifact metadata = %q, want %q",
			actual,
			"storage",
		)
	}

	actualArtifactMetadata["artifact-source"] = "changed-through-getter"

	secondArtifact, exists := payload.Artifact()
	if !exists {
		t.Fatal(
			"second Artifact() exists = false",
		)
	}

	if actual := secondArtifact.Metadata()["artifact-source"]; actual != "storage" {
		t.Fatalf(
			"artifact metadata changed through getter: got %q",
			actual,
		)
	}

	actualPayloadMetadata := payload.Metadata()
	if actual := actualPayloadMetadata["execution"]; actual != "execution-1" {
		t.Fatalf(
			"payload metadata = %q, want %q",
			actual,
			"execution-1",
		)
	}

	actualPayloadMetadata["execution"] = "changed-through-getter"

	if actual := payload.Metadata()["execution"]; actual != "execution-1" {
		t.Fatalf(
			"payload metadata changed through getter: got %q",
			actual,
		)
	}
}

func TestPayloadArtifactRejectsInvalidReference(
	t *testing.T,
) {
	_, err := NewArtifactPayload(
		ArtifactReference{},
		nil,
	)

	requirePayloadValidationField(
		t,
		err,
		"payload.contentType",
	)
}

func TestPayloadRejectsArtifactContentTypeMismatch(
	t *testing.T,
) {
	artifact := newArtifactReferenceForPayloadTest(t)

	_, err := newPayload(
		ContentTypeTextPlain,
		nil,
		false,
		&artifact,
		nil,
		0,
	)

	requirePayloadValidationField(
		t,
		err,
		"payload.contentType",
	)
}

func TestPayloadDecodeJSONReturnsTypedValue(
	t *testing.T,
) {
	type customerPayload struct {
		CustomerID int `json:"customerId"`
	}

	payload, err := NewInlinePayload(
		ContentTypeApplicationJSON,
		[]byte(`{"customerId":42}`),
		nil,
		100,
	)
	if err != nil {
		t.Fatalf(
			"NewInlinePayload() returned an unexpected error: %v",
			err,
		)
	}

	actual, err := DecodePayloadJSON[customerPayload](
		payload,
	)
	if err != nil {
		t.Fatalf(
			"DecodePayloadJSON() returned an unexpected error: %v",
			err,
		)
	}

	if actual.CustomerID != 42 {
		t.Fatalf(
			"CustomerID = %d, want %d",
			actual.CustomerID,
			42,
		)
	}
}

func TestPayloadDecodeJSONAcceptsJSONCompatibleMediaType(
	t *testing.T,
) {
	type problemPayload struct {
		Code string `json:"code"`
	}

	contentType, err := NewContentType(
		"application/problem+json",
	)
	if err != nil {
		t.Fatalf(
			"NewContentType() returned an unexpected error: %v",
			err,
		)
	}

	payload, err := NewInlinePayload(
		contentType,
		[]byte(`{"code":"INVALID_INPUT"}`),
		nil,
		100,
	)
	if err != nil {
		t.Fatalf(
			"NewInlinePayload() returned an unexpected error: %v",
			err,
		)
	}

	actual, err := DecodePayloadJSON[problemPayload](
		payload,
	)
	if err != nil {
		t.Fatalf(
			"DecodePayloadJSON() returned an unexpected error: %v",
			err,
		)
	}

	if actual.Code != "INVALID_INPUT" {
		t.Fatalf(
			"Code = %q, want %q",
			actual.Code,
			"INVALID_INPUT",
		)
	}
}

func TestPayloadDecodeJSONRejectsUnsupportedPayloads(
	t *testing.T,
) {
	type customerPayload struct {
		CustomerID int `json:"customerId"`
	}

	t.Run("artifact payload", func(t *testing.T) {
		artifact := newArtifactReferenceForPayloadTest(t)

		payload, err := NewArtifactPayload(
			artifact,
			nil,
		)
		if err != nil {
			t.Fatalf(
				"NewArtifactPayload() returned an unexpected error: %v",
				err,
			)
		}

		_, err = DecodePayloadJSON[customerPayload](
			payload,
		)

		requirePayloadValidationField(
			t,
			err,
			"payload",
		)
	})

	t.Run("non JSON content type", func(t *testing.T) {
		payload, err := NewInlinePayload(
			ContentTypeTextPlain,
			[]byte(`{"customerId":42}`),
			nil,
			100,
		)
		if err != nil {
			t.Fatalf(
				"NewInlinePayload() returned an unexpected error: %v",
				err,
			)
		}

		_, err = DecodePayloadJSON[customerPayload](
			payload,
		)

		requirePayloadValidationField(
			t,
			err,
			"payload.contentType",
		)
	})

	t.Run("invalid JSON", func(t *testing.T) {
		payload, err := NewInlinePayload(
			ContentTypeApplicationJSON,
			[]byte(`{"customerId":`),
			nil,
			100,
		)
		if err != nil {
			t.Fatalf(
				"NewInlinePayload() returned an unexpected error: %v",
				err,
			)
		}

		_, err = DecodePayloadJSON[customerPayload](
			payload,
		)

		var decodeError *DecodeError
		if !errors.As(err, &decodeError) {
			t.Fatalf(
				"error type = %T, want *DecodeError",
				err,
			)
		}

		if decodeError.Source != "payload" {
			t.Fatalf(
				"decode source = %q, want %q",
				decodeError.Source,
				"payload",
			)
		}

		if !strings.Contains(
			decodeError.Reason,
			"not valid JSON",
		) {
			t.Fatalf(
				"decode reason = %q, want invalid JSON reason",
				decodeError.Reason,
			)
		}
	})

	t.Run("incompatible target type", func(t *testing.T) {
		payload, err := NewInlinePayload(
			ContentTypeApplicationJSON,
			[]byte(`{"customerId":"invalid"}`),
			nil,
			100,
		)
		if err != nil {
			t.Fatalf(
				"NewInlinePayload() returned an unexpected error: %v",
				err,
			)
		}

		_, err = DecodePayloadJSON[customerPayload](
			payload,
		)

		var decodeError *DecodeError
		if !errors.As(err, &decodeError) {
			t.Fatalf(
				"error type = %T, want *DecodeError",
				err,
			)
		}

		if decodeError.Unwrap() == nil {
			t.Fatal(
				"DecodeError.Unwrap() = nil for an incompatible target type",
			)
		}
	})
}

func TestPayloadCloneCreatesIndependentCopy(
	t *testing.T,
) {
	t.Run("inline payload", func(t *testing.T) {
		payload, err := NewInlinePayload(
			ContentTypeApplicationJSON,
			[]byte(`{"value":1}`),
			map[string]string{
				"source": "original",
			},
			100,
		)
		if err != nil {
			t.Fatalf(
				"NewInlinePayload() returned an unexpected error: %v",
				err,
			)
		}

		cloned := clonePayload(payload)

		cloned.inlineData[0] = '['
		cloned.metadata["source"] = "changed"

		actualData, exists := payload.InlineData()
		if !exists {
			t.Fatal(
				"InlineData() exists = false",
			)
		}

		if actual := string(actualData); actual != `{"value":1}` {
			t.Fatalf(
				"original payload data changed: got %q",
				actual,
			)
		}

		if actual := payload.Metadata()["source"]; actual != "original" {
			t.Fatalf(
				"original payload metadata changed: got %q",
				actual,
			)
		}
	})

	t.Run("artifact payload", func(t *testing.T) {
		artifact, err := NewArtifactReference(
			"artifact-1",
			"storage/output.bin",
			ContentTypeApplicationOctetStream,
			10,
			"",
			map[string]string{
				"artifact": "original",
			},
		)
		if err != nil {
			t.Fatalf(
				"NewArtifactReference() returned an unexpected error: %v",
				err,
			)
		}

		payload, err := NewArtifactPayload(
			artifact,
			map[string]string{
				"payload": "original",
			},
		)
		if err != nil {
			t.Fatalf(
				"NewArtifactPayload() returned an unexpected error: %v",
				err,
			)
		}

		cloned := clonePayload(payload)

		cloned.artifact.metadata["artifact"] = "changed"
		cloned.metadata["payload"] = "changed"

		actualArtifact, exists := payload.Artifact()
		if !exists {
			t.Fatal(
				"Artifact() exists = false",
			)
		}

		if actual := actualArtifact.Metadata()["artifact"]; actual != "original" {
			t.Fatalf(
				"original artifact metadata changed: got %q",
				actual,
			)
		}

		if actual := payload.Metadata()["payload"]; actual != "original" {
			t.Fatalf(
				"original payload metadata changed: got %q",
				actual,
			)
		}
	})
}

func TestPayloadMetadataRenderingIsStableByValue(
	t *testing.T,
) {
	payload, err := NewInlinePayload(
		ContentTypeTextPlain,
		[]byte("value"),
		map[string]string{
			"source":      "input",
			"correlation": "correlation-1",
		},
		10,
	)
	if err != nil {
		t.Fatalf(
			"NewInlinePayload() returned an unexpected error: %v",
			err,
		)
	}

	expected := map[string]string{
		"source":      "input",
		"correlation": "correlation-1",
	}

	if actual := payload.Metadata(); !reflect.DeepEqual(
		actual,
		expected,
	) {
		t.Fatalf(
			"Metadata() = %#v, want %#v",
			actual,
			expected,
		)
	}
}

func newArtifactReferenceForPayloadTest(
	t *testing.T,
) ArtifactReference {
	t.Helper()

	reference, err := NewArtifactReference(
		"artifact-1",
		"storage/output.bin",
		ContentTypeApplicationOctetStream,
		64,
		"sha256:abc",
		nil,
	)
	if err != nil {
		t.Fatalf(
			"NewArtifactReference() returned an unexpected error: %v",
			err,
		)
	}

	return reference
}

func requirePayloadValidationField(
	t *testing.T,
	err error,
	expectedField string,
) {
	t.Helper()

	if err == nil {
		t.Fatalf(
			"expected a ValidationError for field %q, got nil",
			expectedField,
		)
	}

	var validationError *ValidationError
	if !errors.As(err, &validationError) {
		t.Fatalf(
			"error type = %T, want *ValidationError",
			err,
		)
	}

	if validationError.Field != expectedField {
		t.Fatalf(
			"validation field = %q, want %q",
			validationError.Field,
			expectedField,
		)
	}

	if strings.TrimSpace(validationError.Reason) == "" {
		t.Fatal(
			"validation reason must not be empty",
		)
	}
}

func TestNewEdgeQueueRejectsNonPositiveCapacity(
	t *testing.T,
) {
	tests := map[string]int{
		"zero":     0,
		"negative": -1,
	}

	for name, capacity := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := NewEdgeQueue(capacity)

			requireRuntimeValidationField(
				t,
				err,
				"queue.capacity",
			)
		})
	}
}

func TestEdgeQueueReportsCapacityAndInitialState(
	t *testing.T,
) {
	queue, err := NewEdgeQueue(3)
	if err != nil {
		t.Fatalf(
			"NewEdgeQueue() returned an unexpected error: %v",
			err,
		)
	}

	if actual := queue.Capacity(); actual != 3 {
		t.Fatalf(
			"Capacity() = %d, want %d",
			actual,
			3,
		)
	}

	if actual := queue.Len(); actual != 0 {
		t.Fatalf(
			"Len() = %d, want 0",
			actual,
		)
	}

	if snapshot := queue.Snapshot(); snapshot != nil {
		t.Fatalf(
			"Snapshot() = %#v, want nil",
			snapshot,
		)
	}
}

func TestEdgeQueueMaintainsFIFOOrder(
	t *testing.T,
) {
	queue := newEdgeQueueForTest(t, 3)

	for _, value := range []string{
		"A",
		"B",
		"C",
	} {
		evicted, didEvict, err := queue.Push(
			newTextPayloadForQueueTest(
				t,
				value,
			),
		)
		if err != nil {
			t.Fatalf(
				"Push(%q) returned an unexpected error: %v",
				value,
				err,
			)
		}

		if didEvict {
			t.Fatalf(
				"Push(%q) evicted %q before queue reached capacity",
				value,
				queuePayloadText(t, evicted),
			)
		}
	}

	if actual := queue.Len(); actual != 3 {
		t.Fatalf(
			"Len() = %d, want %d",
			actual,
			3,
		)
	}

	for _, expected := range []string{
		"A",
		"B",
		"C",
	} {
		actual, exists := queue.Pop()
		if !exists {
			t.Fatalf(
				"Pop() exists = false, want payload %q",
				expected,
			)
		}

		if actualText := queuePayloadText(
			t,
			actual,
		); actualText != expected {
			t.Fatalf(
				"Pop() = %q, want %q",
				actualText,
				expected,
			)
		}
	}

	if actual := queue.Len(); actual != 0 {
		t.Fatalf(
			"Len() after all Pop calls = %d, want 0",
			actual,
		)
	}
}

func TestEdgeQueuePeekReturnsOldestWithoutRemoval(
	t *testing.T,
) {
	queue := newEdgeQueueForTest(t, 2)

	pushQueuePayloadForTest(t, queue, "A")
	pushQueuePayloadForTest(t, queue, "B")

	first, exists := queue.Peek()
	if !exists {
		t.Fatal(
			"Peek() exists = false for a non-empty queue",
		)
	}

	if actual := queuePayloadText(t, first); actual != "A" {
		t.Fatalf(
			"Peek() = %q, want %q",
			actual,
			"A",
		)
	}

	if actual := queue.Len(); actual != 2 {
		t.Fatalf(
			"Len() after Peek() = %d, want %d",
			actual,
			2,
		)
	}

	second, exists := queue.Peek()
	if !exists {
		t.Fatal(
			"second Peek() exists = false",
		)
	}

	if actual := queuePayloadText(t, second); actual != "A" {
		t.Fatalf(
			"second Peek() = %q, want %q",
			actual,
			"A",
		)
	}
}

func TestEdgeQueuePopAndPeekHandleEmptyQueue(
	t *testing.T,
) {
	queue := newEdgeQueueForTest(t, 1)

	if _, exists := queue.Pop(); exists {
		t.Fatal(
			"Pop() exists = true for an empty queue",
		)
	}

	if _, exists := queue.Peek(); exists {
		t.Fatal(
			"Peek() exists = true for an empty queue",
		)
	}
}

func TestEdgeQueueDropOldestReturnsEvictedPayload(
	t *testing.T,
) {
	queue := newEdgeQueueForTest(t, 3)

	pushQueuePayloadForTest(t, queue, "A")
	pushQueuePayloadForTest(t, queue, "B")
	pushQueuePayloadForTest(t, queue, "C")

	evicted, didEvict, err := queue.Push(
		newTextPayloadForQueueTest(
			t,
			"D",
		),
	)
	if err != nil {
		t.Fatalf(
			"Push(D) returned an unexpected error: %v",
			err,
		)
	}

	if !didEvict {
		t.Fatal(
			"Push(D) didEvict = false for a full queue",
		)
	}

	if actual := queuePayloadText(
		t,
		evicted,
	); actual != "A" {
		t.Fatalf(
			"evicted payload = %q, want %q",
			actual,
			"A",
		)
	}

	requireQueueSnapshot(
		t,
		queue,
		[]string{
			"B",
			"C",
			"D",
		},
	)
}

func TestEdgeQueueCapacityOneDropsPreviousPayload(
	t *testing.T,
) {
	queue := newEdgeQueueForTest(t, 1)

	pushQueuePayloadForTest(t, queue, "A")

	evicted, didEvict, err := queue.Push(
		newTextPayloadForQueueTest(
			t,
			"B",
		),
	)
	if err != nil {
		t.Fatalf(
			"Push(B) returned an unexpected error: %v",
			err,
		)
	}

	if !didEvict {
		t.Fatal(
			"Push(B) didEvict = false for capacity-one queue",
		)
	}

	if actual := queuePayloadText(
		t,
		evicted,
	); actual != "A" {
		t.Fatalf(
			"evicted payload = %q, want %q",
			actual,
			"A",
		)
	}

	requireQueueSnapshot(
		t,
		queue,
		[]string{"B"},
	)
}

func TestEdgeQueueRejectsInvalidPayload(
	t *testing.T,
) {
	queue := newEdgeQueueForTest(t, 1)

	_, _, err := queue.Push(Payload{})
	if err == nil {
		t.Fatal(
			"Push() returned nil error for an invalid payload",
		)
	}

	if actual := queue.Len(); actual != 0 {
		t.Fatalf(
			"Len() after rejected payload = %d, want 0",
			actual,
		)
	}
}

func TestEdgeQueueSnapshotIsIndependent(
	t *testing.T,
) {
	queue := newEdgeQueueForTest(t, 2)

	payload, err := NewInlinePayload(
		ContentTypeTextPlain,
		[]byte("A"),
		map[string]string{
			"source": "original",
		},
		10,
	)
	if err != nil {
		t.Fatalf(
			"NewInlinePayload() returned an unexpected error: %v",
			err,
		)
	}

	if _, _, err := queue.Push(payload); err != nil {
		t.Fatalf(
			"Push() returned an unexpected error: %v",
			err,
		)
	}

	snapshot := queue.Snapshot()
	if len(snapshot) != 1 {
		t.Fatalf(
			"len(Snapshot()) = %d, want 1",
			len(snapshot),
		)
	}

	snapshot[0].inlineData[0] = 'X'
	snapshot[0].metadata["source"] = "changed"

	actual, exists := queue.Peek()
	if !exists {
		t.Fatal(
			"Peek() exists = false",
		)
	}

	if actualText := queuePayloadText(
		t,
		actual,
	); actualText != "A" {
		t.Fatalf(
			"queue payload changed through snapshot: got %q",
			actualText,
		)
	}

	if actualSource := actual.Metadata()["source"]; actualSource != "original" {
		t.Fatalf(
			"queue metadata changed through snapshot: got %q",
			actualSource,
		)
	}
}

func TestEdgeQueueReturnedPayloadsAreIndependent(
	t *testing.T,
) {
	queue := newEdgeQueueForTest(t, 2)

	pushQueuePayloadForTest(t, queue, "A")

	peeked, exists := queue.Peek()
	if !exists {
		t.Fatal(
			"Peek() exists = false",
		)
	}

	peeked.inlineData[0] = 'X'

	secondPeek, exists := queue.Peek()
	if !exists {
		t.Fatal(
			"second Peek() exists = false",
		)
	}

	if actual := queuePayloadText(
		t,
		secondPeek,
	); actual != "A" {
		t.Fatalf(
			"queue changed through Peek() result: got %q",
			actual,
		)
	}

	popped, exists := queue.Pop()
	if !exists {
		t.Fatal(
			"Pop() exists = false",
		)
	}

	popped.inlineData[0] = 'Y'

	if actual := queue.Len(); actual != 0 {
		t.Fatalf(
			"Len() after Pop() = %d, want 0",
			actual,
		)
	}
}

func TestNilEdgeQueueMethodsAreSafe(
	t *testing.T,
) {
	var queue *EdgeQueue

	_, _, err := queue.Push(
		newTextPayloadForQueueTest(
			t,
			"A",
		),
	)
	requireRuntimeValidationField(
		t,
		err,
		"queue",
	)

	if _, exists := queue.Pop(); exists {
		t.Fatal(
			"nil queue Pop() exists = true",
		)
	}

	if _, exists := queue.Peek(); exists {
		t.Fatal(
			"nil queue Peek() exists = true",
		)
	}

	if actual := queue.Len(); actual != 0 {
		t.Fatalf(
			"nil queue Len() = %d, want 0",
			actual,
		)
	}

	if actual := queue.Capacity(); actual != 0 {
		t.Fatalf(
			"nil queue Capacity() = %d, want 0",
			actual,
		)
	}

	if snapshot := queue.Snapshot(); snapshot != nil {
		t.Fatalf(
			"nil queue Snapshot() = %#v, want nil",
			snapshot,
		)
	}
}

func TestEdgeQueueConcurrentAccess(
	t *testing.T,
) {
	const (
		capacity       = 32
		writerCount    = 8
		readerCount    = 4
		popWorkerCount = 4
		operations     = 200
	)

	queue := newEdgeQueueForTest(
		t,
		capacity,
	)

	errorChannel := make(
		chan error,
		writerCount*operations,
	)

	var waitGroup sync.WaitGroup

	for writer := 0; writer < writerCount; writer++ {
		writerID := writer

		waitGroup.Add(1)

		go func() {
			defer waitGroup.Done()

			for operation := 0; operation < operations; operation++ {
				label := fmt.Sprintf(
					"writer-%d-item-%d",
					writerID,
					operation,
				)

				payload, err := NewInlinePayload(
					ContentTypeTextPlain,
					[]byte(label),
					nil,
					128,
				)
				if err != nil {
					errorChannel <- err
					continue
				}

				if _, _, err := queue.Push(payload); err != nil {
					errorChannel <- err
				}
			}
		}()
	}

	for reader := 0; reader < readerCount; reader++ {
		waitGroup.Add(1)

		go func() {
			defer waitGroup.Done()

			for operation := 0; operation < operations; operation++ {
				queue.Peek()
				queue.Snapshot()
				queue.Len()
				queue.Capacity()
			}
		}()
	}

	for worker := 0; worker < popWorkerCount; worker++ {
		waitGroup.Add(1)

		go func() {
			defer waitGroup.Done()

			for operation := 0; operation < operations; operation++ {
				queue.Pop()
			}
		}()
	}

	waitGroup.Wait()
	close(errorChannel)

	for err := range errorChannel {
		t.Errorf(
			"concurrent queue operation returned an error: %v",
			err,
		)
	}

	if actual := queue.Len(); actual > capacity {
		t.Fatalf(
			"Len() = %d, must not exceed capacity %d",
			actual,
			capacity,
		)
	}

	if actual := len(queue.Snapshot()); actual != queue.Len() {
		t.Fatalf(
			"len(Snapshot()) = %d, want current Len() %d",
			actual,
			queue.Len(),
		)
	}
}

func newEdgeQueueForTest(
	t *testing.T,
	capacity int,
) *EdgeQueue {
	t.Helper()

	queue, err := NewEdgeQueue(capacity)
	if err != nil {
		t.Fatalf(
			"NewEdgeQueue() returned an unexpected error: %v",
			err,
		)
	}

	return queue
}

func newTextPayloadForQueueTest(
	t *testing.T,
	value string,
) Payload {
	t.Helper()

	payload, err := NewInlinePayload(
		ContentTypeTextPlain,
		[]byte(value),
		nil,
		1024,
	)
	if err != nil {
		t.Fatalf(
			"NewInlinePayload(%q) returned an unexpected error: %v",
			value,
			err,
		)
	}

	return payload
}

func pushQueuePayloadForTest(
	t *testing.T,
	queue *EdgeQueue,
	value string,
) {
	t.Helper()

	_, didEvict, err := queue.Push(
		newTextPayloadForQueueTest(
			t,
			value,
		),
	)
	if err != nil {
		t.Fatalf(
			"Push(%q) returned an unexpected error: %v",
			value,
			err,
		)
	}

	if didEvict {
		t.Fatalf(
			"Push(%q) unexpectedly evicted an item",
			value,
		)
	}
}

func queuePayloadText(
	t *testing.T,
	payload Payload,
) string {
	t.Helper()

	data, exists := payload.InlineData()
	if !exists {
		t.Fatal(
			"payload does not contain inline data",
		)
	}

	return string(data)
}

func requireQueueSnapshot(
	t *testing.T,
	queue *EdgeQueue,
	expected []string,
) {
	t.Helper()

	snapshot := queue.Snapshot()
	if len(snapshot) != len(expected) {
		t.Fatalf(
			"len(Snapshot()) = %d, want %d",
			len(snapshot),
			len(expected),
		)
	}

	for index, expectedValue := range expected {
		actual := queuePayloadText(
			t,
			snapshot[index],
		)

		if actual != expectedValue {
			t.Fatalf(
				"Snapshot()[%d] = %q, want %q",
				index,
				actual,
				expectedValue,
			)
		}
	}
}

func requireRuntimeValidationField(
	t *testing.T,
	err error,
	expectedField string,
) {
	t.Helper()

	if err == nil {
		t.Fatalf(
			"expected a ValidationError for field %q, got nil",
			expectedField,
		)
	}

	var validationError *ValidationError
	if !errors.As(err, &validationError) {
		t.Fatalf(
			"error type = %T, want *ValidationError",
			err,
		)
	}

	if validationError.Field != expectedField {
		t.Fatalf(
			"validation field = %q, want %q",
			validationError.Field,
			expectedField,
		)
	}

	if strings.TrimSpace(validationError.Reason) == "" {
		t.Fatal(
			"validation reason must not be empty",
		)
	}
}

func TestFailureCategoryRecognizesSupportedValues(
	t *testing.T,
) {
	categories := []FailureCategory{
		FailureCategoryValidation,
		FailureCategoryExecution,
		FailureCategoryDependency,
		FailureCategoryTimeout,
		FailureCategoryCanceled,
		FailureCategoryInternal,
	}

	for _, category := range categories {
		t.Run(category.String(), func(t *testing.T) {
			if !category.IsValid() {
				t.Fatalf(
					"IsValid() = false for supported category %q",
					category,
				)
			}

			if category.String() == "" {
				t.Fatal(
					"String() returned an empty value",
				)
			}
		})
	}
}

func TestFailureCategoryRejectsUnsupportedValues(
	t *testing.T,
) {
	categories := map[string]FailureCategory{
		"zero":        "",
		"unsupported": "NETWORK",
	}

	for name, category := range categories {
		t.Run(name, func(t *testing.T) {
			if category.IsValid() {
				t.Fatalf(
					"IsValid() = true for unsupported category %q",
					category,
				)
			}
		})
	}
}

func TestNewRuntimeFailureNormalizesValues(
	t *testing.T,
) {
	failure, err := NewRuntimeFailure(
		FailureCategory(" dependency "),
		" UPSTREAM_UNAVAILABLE ",
		" Billing service is unavailable ",
		true,
		map[string]string{
			" service ": "billing",
			"attempt":   "1",
		},
	)
	if err != nil {
		t.Fatalf(
			"NewRuntimeFailure() returned an unexpected error: %v",
			err,
		)
	}

	if !failure.IsValid() {
		t.Fatal(
			"IsValid() = false for a valid runtime failure",
		)
	}

	if actual := failure.Category(); actual != FailureCategoryDependency {
		t.Fatalf(
			"Category() = %q, want %q",
			actual,
			FailureCategoryDependency,
		)
	}

	if actual := failure.Code(); actual != "UPSTREAM_UNAVAILABLE" {
		t.Fatalf(
			"Code() = %q, want %q",
			actual,
			"UPSTREAM_UNAVAILABLE",
		)
	}

	if actual := failure.Message(); actual != "Billing service is unavailable" {
		t.Fatalf(
			"Message() = %q, want %q",
			actual,
			"Billing service is unavailable",
		)
	}

	if !failure.Retryable() {
		t.Fatal(
			"Retryable() = false, want true",
		)
	}

	details := failure.Details()

	if actual := details["service"]; actual != "billing" {
		t.Fatalf(
			"Details()[service] = %q, want %q",
			actual,
			"billing",
		)
	}

	if actual := details["attempt"]; actual != "1" {
		t.Fatalf(
			"Details()[attempt] = %q, want %q",
			actual,
			"1",
		)
	}
}

func TestNewRuntimeFailurePreservesNonRetryableValue(
	t *testing.T,
) {
	failure, err := NewRuntimeFailure(
		FailureCategoryValidation,
		"INVALID_INPUT",
		"Input is invalid",
		false,
		nil,
	)
	if err != nil {
		t.Fatalf(
			"NewRuntimeFailure() returned an unexpected error: %v",
			err,
		)
	}

	if failure.Retryable() {
		t.Fatal(
			"Retryable() = true for a non-retryable failure",
		)
	}

	if details := failure.Details(); details != nil {
		t.Fatalf(
			"Details() = %#v, want nil",
			details,
		)
	}
}

func TestNewRuntimeFailureRejectsInvalidRequiredValues(
	t *testing.T,
) {
	tests := []struct {
		name     string
		category FailureCategory
		code     string
		message  string
		field    string
	}{
		{
			name:     "unsupported category",
			category: FailureCategory("NETWORK"),
			code:     "FAILURE",
			message:  "Failure occurred",
			field:    "failure.category",
		},
		{
			name:     "blank category",
			category: FailureCategory(" "),
			code:     "FAILURE",
			message:  "Failure occurred",
			field:    "failure.category",
		},
		{
			name:     "blank code",
			category: FailureCategoryExecution,
			code:     " ",
			message:  "Failure occurred",
			field:    "failure.code",
		},
		{
			name:     "blank message",
			category: FailureCategoryExecution,
			code:     "FAILURE",
			message:  " ",
			field:    "failure.message",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := NewRuntimeFailure(
				test.category,
				test.code,
				test.message,
				false,
				nil,
			)

			requireRuntimeValidationField(
				t,
				err,
				test.field,
			)
		})
	}
}

func TestNewRuntimeFailureRejectsInvalidDetails(
	t *testing.T,
) {
	tests := map[string]map[string]string{
		"blank key": {
			" ": "value",
		},
		"duplicate normalized key": {
			"service":   "billing",
			" service ": "payments",
		},
	}

	for name, details := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := NewRuntimeFailure(
				FailureCategoryDependency,
				"UPSTREAM_FAILURE",
				"Upstream dependency failed",
				true,
				details,
			)

			if err == nil {
				t.Fatal(
					"NewRuntimeFailure() returned nil error for invalid details",
				)
			}

			var validationError *ValidationError
			if !errors.As(err, &validationError) {
				t.Fatalf(
					"error type = %T, want *ValidationError",
					err,
				)
			}
		})
	}
}

func TestRuntimeFailureCopiesConstructorDetails(
	t *testing.T,
) {
	details := map[string]string{
		"service": "billing",
		"region":  "eu-central",
	}

	failure, err := NewRuntimeFailure(
		FailureCategoryDependency,
		"UPSTREAM_UNAVAILABLE",
		"Billing service is unavailable",
		true,
		details,
	)
	if err != nil {
		t.Fatalf(
			"NewRuntimeFailure() returned an unexpected error: %v",
			err,
		)
	}

	details["service"] = "changed"
	delete(
		details,
		"region",
	)
	details["new"] = "value"

	actual := failure.Details()

	if actual["service"] != "billing" {
		t.Fatalf(
			"stored service = %q, want %q",
			actual["service"],
			"billing",
		)
	}

	if actual["region"] != "eu-central" {
		t.Fatalf(
			"stored region = %q, want %q",
			actual["region"],
			"eu-central",
		)
	}

	if _, exists := actual["new"]; exists {
		t.Fatal(
			"failure details changed through constructor input map",
		)
	}
}

func TestRuntimeFailureReturnedDetailsAreIndependent(
	t *testing.T,
) {
	failure := newRuntimeFailureForTest(
		t,
	)

	details := failure.Details()
	details["service"] = "changed"
	delete(
		details,
		"operation",
	)
	details["new"] = "value"

	secondDetails := failure.Details()

	if actual := secondDetails["service"]; actual != "billing" {
		t.Fatalf(
			"service changed through Details() result: got %q",
			actual,
		)
	}

	if actual := secondDetails["operation"]; actual != "fetch-customer" {
		t.Fatalf(
			"operation changed through Details() result: got %q",
			actual,
		)
	}

	if _, exists := secondDetails["new"]; exists {
		t.Fatal(
			"new detail was added through Details() result",
		)
	}
}

func TestRuntimeFailureCloneCreatesIndependentCopy(
	t *testing.T,
) {
	failure := newRuntimeFailureForTest(
		t,
	)

	cloned := cloneRuntimeFailure(
		failure,
	)

	cloned.details["service"] = "changed"
	delete(
		cloned.details,
		"operation",
	)

	originalDetails := failure.Details()

	if actual := originalDetails["service"]; actual != "billing" {
		t.Fatalf(
			"original service changed through clone: got %q",
			actual,
		)
	}

	if actual := originalDetails["operation"]; actual != "fetch-customer" {
		t.Fatalf(
			"original operation changed through clone: got %q",
			actual,
		)
	}
}

func TestZeroRuntimeFailureIsInvalid(
	t *testing.T,
) {
	var failure RuntimeFailure

	if failure.IsValid() {
		t.Fatal(
			"zero RuntimeFailure IsValid() = true",
		)
	}

	if failure.Category().IsValid() {
		t.Fatal(
			"zero RuntimeFailure category must be invalid",
		)
	}

	if failure.Code() != "" {
		t.Fatal(
			"zero RuntimeFailure Code() must be empty",
		)
	}

	if failure.Message() != "" {
		t.Fatal(
			"zero RuntimeFailure Message() must be empty",
		)
	}

	if failure.Retryable() {
		t.Fatal(
			"zero RuntimeFailure Retryable() must be false",
		)
	}

	if failure.Details() != nil {
		t.Fatal(
			"zero RuntimeFailure Details() must return nil",
		)
	}
}

func TestRuntimeFailureDetectsMalformedInternalState(
	t *testing.T,
) {
	tests := map[string]RuntimeFailure{
		"invalid category": {
			category:  FailureCategory("NETWORK"),
			code:      "FAILURE",
			message:   "Failure occurred",
			retryable: false,
		},
		"blank code": {
			category:  FailureCategoryExecution,
			code:      " ",
			message:   "Failure occurred",
			retryable: false,
		},
		"blank message": {
			category:  FailureCategoryExecution,
			code:      "FAILURE",
			message:   " ",
			retryable: false,
		},
		"invalid details": {
			category:  FailureCategoryExecution,
			code:      "FAILURE",
			message:   "Failure occurred",
			retryable: false,
			details: map[string]string{
				" ": "value",
			},
		},
	}

	for name, failure := range tests {
		t.Run(name, func(t *testing.T) {
			if failure.IsValid() {
				t.Fatal(
					"IsValid() = true for malformed RuntimeFailure",
				)
			}
		})
	}
}

func newRuntimeFailureForTest(
	t *testing.T,
) RuntimeFailure {
	t.Helper()

	failure, err := NewRuntimeFailure(
		FailureCategoryDependency,
		"UPSTREAM_UNAVAILABLE",
		"Billing service is unavailable",
		true,
		map[string]string{
			"service":   "billing",
			"operation": "fetch-customer",
		},
	)
	if err != nil {
		t.Fatalf(
			"NewRuntimeFailure() returned an unexpected error: %v",
			err,
		)
	}

	return failure
}

func TestRuntimeValueAcceptsSupportedJSONRoots(
	t *testing.T,
) {
	tests := map[string]struct {
		input    string
		expected string
	}{
		"string": {
			input:    `  "TRY"  `,
			expected: `"TRY"`,
		},
		"number": {
			input:    `  500  `,
			expected: `500`,
		},
		"boolean": {
			input:    ` true `,
			expected: `true`,
		},
		"null": {
			input:    ` null `,
			expected: `null`,
		},
		"object": {
			input:    `  {"customerId":42}  `,
			expected: `{"customerId":42}`,
		},
		"array": {
			input:    `  ["A","B"]  `,
			expected: `["A","B"]`,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			value, err := NewRuntimeValue(
				[]byte(test.input),
			)
			if err != nil {
				t.Fatalf(
					"NewRuntimeValue() returned an unexpected error: %v",
					err,
				)
			}

			if !value.IsValid() {
				t.Fatal(
					"IsValid() = false for valid JSON",
				)
			}

			if actual := value.String(); actual != test.expected {
				t.Fatalf(
					"String() = %q, want %q",
					actual,
					test.expected,
				)
			}

			if actual := string(value.Bytes()); actual != test.expected {
				t.Fatalf(
					"Bytes() = %q, want %q",
					actual,
					test.expected,
				)
			}
		})
	}
}

func TestRuntimeValueRejectsEmptyAndInvalidJSON(
	t *testing.T,
) {
	tests := map[string]struct {
		input  []byte
		reason string
	}{
		"nil": {
			input:  nil,
			reason: "must not be empty",
		},
		"empty": {
			input:  []byte{},
			reason: "must not be empty",
		},
		"whitespace": {
			input:  []byte("   "),
			reason: "must not be empty",
		},
		"invalid JSON": {
			input:  []byte(`{"customerId":`),
			reason: "must contain valid JSON",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := NewRuntimeValue(
				test.input,
			)

			requireRuntimeValueValidationError(
				t,
				err,
				test.reason,
			)
		})
	}
}

func TestRuntimeValueCopiesInputAndOutputBytes(
	t *testing.T,
) {
	original := []byte(
		`{"value":1}`,
	)

	value, err := NewRuntimeValue(original)
	if err != nil {
		t.Fatalf(
			"NewRuntimeValue() returned an unexpected error: %v",
			err,
		)
	}

	original[0] = '['

	if actual := value.String(); actual != `{"value":1}` {
		t.Fatalf(
			"runtime value changed through constructor input: got %q",
			actual,
		)
	}

	exposed := value.Bytes()
	exposed[0] = '['

	if actual := value.String(); actual != `{"value":1}` {
		t.Fatalf(
			"runtime value changed through Bytes(): got %q",
			actual,
		)
	}
}

func TestRuntimeValueDecodeReturnsTypedValue(
	t *testing.T,
) {
	type customerVariable struct {
		CustomerID int    `json:"customerId"`
		Currency   string `json:"currency"`
	}

	value, err := NewRuntimeValue(
		[]byte(
			`{"customerId":42,"currency":"TRY"}`,
		),
	)
	if err != nil {
		t.Fatalf(
			"NewRuntimeValue() returned an unexpected error: %v",
			err,
		)
	}

	actual, err := DecodeRuntimeValue[customerVariable](
		value,
	)
	if err != nil {
		t.Fatalf(
			"DecodeRuntimeValue() returned an unexpected error: %v",
			err,
		)
	}

	if actual.CustomerID != 42 {
		t.Fatalf(
			"CustomerID = %d, want %d",
			actual.CustomerID,
			42,
		)
	}

	if actual.Currency != "TRY" {
		t.Fatalf(
			"Currency = %q, want %q",
			actual.Currency,
			"TRY",
		)
	}
}

func TestRuntimeValueDecodeSupportsScalarAndNull(
	t *testing.T,
) {
	t.Run("string", func(t *testing.T) {
		value, err := NewRuntimeValue(
			[]byte(`"TRY"`),
		)
		if err != nil {
			t.Fatalf(
				"NewRuntimeValue() returned an unexpected error: %v",
				err,
			)
		}

		actual, err := DecodeRuntimeValue[string](value)
		if err != nil {
			t.Fatalf(
				"DecodeRuntimeValue() returned an unexpected error: %v",
				err,
			)
		}

		if actual != "TRY" {
			t.Fatalf(
				"decoded value = %q, want %q",
				actual,
				"TRY",
			)
		}
	})

	t.Run("number", func(t *testing.T) {
		value, err := NewRuntimeValue(
			[]byte(`500`),
		)
		if err != nil {
			t.Fatalf(
				"NewRuntimeValue() returned an unexpected error: %v",
				err,
			)
		}

		actual, err := DecodeRuntimeValue[int](value)
		if err != nil {
			t.Fatalf(
				"DecodeRuntimeValue() returned an unexpected error: %v",
				err,
			)
		}

		if actual != 500 {
			t.Fatalf(
				"decoded value = %d, want %d",
				actual,
				500,
			)
		}
	})

	t.Run("null pointer", func(t *testing.T) {
		value, err := NewRuntimeValue(
			[]byte(`null`),
		)
		if err != nil {
			t.Fatalf(
				"NewRuntimeValue() returned an unexpected error: %v",
				err,
			)
		}

		actual, err := DecodeRuntimeValue[*string](
			value,
		)
		if err != nil {
			t.Fatalf(
				"DecodeRuntimeValue() returned an unexpected error: %v",
				err,
			)
		}

		if actual != nil {
			t.Fatalf(
				"decoded pointer = %#v, want nil",
				actual,
			)
		}
	})
}

func TestRuntimeValueDecodeRejectsIncompatibleTarget(
	t *testing.T,
) {
	type customerVariable struct {
		CustomerID int `json:"customerId"`
	}

	value, err := NewRuntimeValue(
		[]byte(`{"customerId":"invalid"}`),
	)
	if err != nil {
		t.Fatalf(
			"NewRuntimeValue() returned an unexpected error: %v",
			err,
		)
	}

	_, err = DecodeRuntimeValue[customerVariable](
		value,
	)
	if err == nil {
		t.Fatal(
			"DecodeRuntimeValue() returned nil error for an incompatible target",
		)
	}

	var decodeError *DecodeError
	if !errors.As(err, &decodeError) {
		t.Fatalf(
			"error type = %T, want *DecodeError",
			err,
		)
	}

	if decodeError.Source != "runtime value" {
		t.Fatalf(
			"decode source = %q, want %q",
			decodeError.Source,
			"runtime value",
		)
	}

	if !strings.Contains(
		decodeError.Reason,
		"incompatible",
	) {
		t.Fatalf(
			"decode reason = %q, want incompatible-target reason",
			decodeError.Reason,
		)
	}

	if decodeError.Unwrap() == nil {
		t.Fatal(
			"DecodeError.Unwrap() = nil for an incompatible target",
		)
	}
}

func TestRuntimeValueDecodeRejectsZeroValue(
	t *testing.T,
) {
	var value RuntimeValue

	if value.IsValid() {
		t.Fatal(
			"IsValid() = true for a zero RuntimeValue",
		)
	}

	_, err := DecodeRuntimeValue[string](value)

	requireRuntimeValueValidationError(
		t,
		err,
		"must contain valid JSON",
	)
}

func TestRuntimeValueCloneCreatesIndependentCopy(
	t *testing.T,
) {
	value, err := NewRuntimeValue(
		[]byte(`{"name":"Miletos"}`),
	)
	if err != nil {
		t.Fatalf(
			"NewRuntimeValue() returned an unexpected error: %v",
			err,
		)
	}

	cloned := cloneRuntimeValue(value)
	cloned.raw[0] = '['

	if actual := value.String(); actual != `{"name":"Miletos"}` {
		t.Fatalf(
			"original runtime value changed through clone: got %q",
			actual,
		)
	}
}

func TestRuntimeValueDecodeErrorFormatting(
	t *testing.T,
) {
	underlying := errors.New(
		"underlying decoder error",
	)

	decodeError := &DecodeError{
		Source: "runtime value",
		Reason: "content is incompatible with the target type",
		Err:    underlying,
	}

	expected := "runtime value decode failed: content is incompatible with the target type"

	if actual := decodeError.Error(); actual != expected {
		t.Fatalf(
			"Error() = %q, want %q",
			actual,
			expected,
		)
	}

	if !errors.Is(decodeError, underlying) {
		t.Fatal(
			"errors.Is() = false for wrapped decoder error",
		)
	}
}

func requireRuntimeValueValidationError(
	t *testing.T,
	err error,
	expectedReason string,
) {
	t.Helper()

	if err == nil {
		t.Fatal(
			"expected a ValidationError, got nil",
		)
	}

	var validationError *ValidationError
	if !errors.As(err, &validationError) {
		t.Fatalf(
			"error type = %T, want *ValidationError",
			err,
		)
	}

	if validationError.Field != "runtimeValue" {
		t.Fatalf(
			"validation field = %q, want %q",
			validationError.Field,
			"runtimeValue",
		)
	}

	if validationError.Reason != expectedReason {
		t.Fatalf(
			"validation reason = %q, want %q",
			validationError.Reason,
			expectedReason,
		)
	}
}
