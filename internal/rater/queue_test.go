package rater

import (
	"path/filepath"
	"testing"
)

func TestPendingUploadQueueDedupAndPersistence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pending.json")

	q, err := LoadPendingUploadQueue(path)
	if err != nil {
		t.Fatalf("LoadPendingUploadQueue failed: %v", err)
	}

	q.Enqueue(PendingUpload{SkillID: "a/b@1", ReportHash: "h1", Score: 80, Comment: "c1"}, 100)
	q.Enqueue(PendingUpload{SkillID: "a/b@1", ReportHash: "h1", Score: 80, Comment: "c1"}, 100)
	if len(q.Items) != 1 {
		t.Fatalf("expected dedup queue length 1, got %d", len(q.Items))
	}

	if err := SavePendingUploadQueue(path, q); err != nil {
		t.Fatalf("SavePendingUploadQueue failed: %v", err)
	}

	q2, err := LoadPendingUploadQueue(path)
	if err != nil {
		t.Fatalf("reload queue failed: %v", err)
	}
	if len(q2.Items) != 1 {
		t.Fatalf("expected 1 pending item after reload, got %d", len(q2.Items))
	}
}

func TestPendingUploadQueueDrain(t *testing.T) {
	q := PendingUploadQueue{}
	q.Enqueue(PendingUpload{SkillID: "a/1@1", ReportHash: "h1", Score: 80, Comment: "c"}, 100)
	q.Enqueue(PendingUpload{SkillID: "a/2@1", ReportHash: "h2", Score: 81, Comment: "c"}, 100)

	sent := 0
	remaining := q.Drain(10, func(item PendingUpload) error {
		sent++
		if item.SkillID == "a/2@1" {
			return assertErr{}
		}
		return nil
	})

	if sent != 2 {
		t.Fatalf("expected callback run 2 times, got %d", sent)
	}
	if len(remaining.Items) != 1 || remaining.Items[0].SkillID != "a/2@1" {
		t.Fatalf("expected failed item to remain, got %+v", remaining.Items)
	}
}

type assertErr struct{}

func (assertErr) Error() string { return "fail" }
