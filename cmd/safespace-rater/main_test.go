package main

import (
	"path/filepath"
	"testing"

	"github.com/vpn2004/SkillVet/internal/rater"
)

func TestDefaultPendingQueuePath(t *testing.T) {
	path := defaultPendingQueuePath()
	if path == "" {
		t.Fatalf("expected non-empty pending queue path")
	}
	if filepath.Base(path) != "pending-uploads.json" {
		t.Fatalf("unexpected pending queue file name: %s", path)
	}
}

func TestRetryPendingUploads(t *testing.T) {
	queue := rater.PendingUploadQueue{Items: []rater.PendingUpload{
		{SkillID: "openclaw/a@1", Score: 80, Comment: "ok", ReportHash: "h1"},
		{SkillID: "openclaw/b@1", Score: 81, Comment: "err", ReportHash: "h2"},
	}}

	submitted, failed, remaining := retryPendingUploads(queue, 10, func(item rater.PendingUpload) error {
		if item.SkillID == "openclaw/b@1" {
			return assertErr{}
		}
		return nil
	})

	if submitted != 1 {
		t.Fatalf("expected submitted=1, got %d", submitted)
	}
	if failed != 1 {
		t.Fatalf("expected failed=1, got %d", failed)
	}
	if len(remaining.Items) != 1 || remaining.Items[0].SkillID != "openclaw/b@1" {
		t.Fatalf("unexpected remaining queue: %+v", remaining.Items)
	}
}

type assertErr struct{}

func (assertErr) Error() string { return "fail" }
