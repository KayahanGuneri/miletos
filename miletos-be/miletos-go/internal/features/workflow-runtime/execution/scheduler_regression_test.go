package execution

import (
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestOutboxDispatcherIsSoleNodeQueuePublisher(t *testing.T) {
	t.Helper()

	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve scheduler regression test path")
	}

	executionRoot := filepath.Dir(currentFile)
	publications := make([]string, 0)

	err := filepath.WalkDir(
		executionRoot,
		func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() {
				return nil
			}
			if filepath.Ext(path) != ".go" ||
				strings.HasSuffix(path, "_test.go") {
				return nil
			}

			content, err := os.ReadFile(path)
			if err != nil {
				return err
			}

			source := string(content)
			for offset := 0; ; {
				index := strings.Index(
					source[offset:],
					".queue.Push(",
				)
				if index < 0 {
					break
				}

				publications = append(
					publications,
					filepath.Clean(path),
				)
				offset += index + len(".queue.Push(")
			}

			return nil
		},
	)
	if err != nil {
		t.Fatalf(
			"scan production execution package: %v",
			err,
		)
	}

	if len(publications) != 1 {
		t.Fatalf(
			"production queue publication sites = %#v, want exactly one",
			publications,
		)
	}

	if filepath.Base(publications[0]) != "outbox_dispatcher.go" {
		t.Fatalf(
			"production queue publisher = %q, want outbox_dispatcher.go",
			publications[0],
		)
	}

	schedulerPath := filepath.Join(
		executionRoot,
		"scheduler.go",
	)

	schedulerSource, err := os.ReadFile(schedulerPath)
	if err != nil {
		t.Fatalf(
			"read scheduler.go: %v",
			err,
		)
	}

	schedulerText := string(schedulerSource)

	if strings.Contains(
		schedulerText,
		"func (scheduler *Scheduler) push(",
	) {
		t.Fatal("Scheduler.push direct-publication method still exists")
	}

	if strings.Contains(
		schedulerText,
		"scheduler.queue.Push(",
	) {
		t.Fatal("scheduler.go still publishes directly to the queue")
	}
}
