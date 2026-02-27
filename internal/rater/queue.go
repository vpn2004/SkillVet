package rater

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

type PendingUpload struct {
	SkillID    string  `json:"skill_id"`
	Score      float64 `json:"score"`
	Comment    string  `json:"comment"`
	ReportHash string  `json:"report_hash"`
	CreatedAt  string  `json:"created_at"`
}

type PendingUploadQueue struct {
	Items []PendingUpload `json:"items"`
}

func LoadPendingUploadQueue(path string) (PendingUploadQueue, error) {
	out := PendingUploadQueue{Items: []PendingUpload{}}
	buf, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return out, nil
		}
		return PendingUploadQueue{}, err
	}
	if len(buf) == 0 {
		return out, nil
	}
	if err := json.Unmarshal(buf, &out); err != nil {
		return PendingUploadQueue{}, err
	}
	if out.Items == nil {
		out.Items = []PendingUpload{}
	}
	return out, nil
}

func SavePendingUploadQueue(path string, q PendingUploadQueue) error {
	if q.Items == nil {
		q.Items = []PendingUpload{}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	buf, err := json.MarshalIndent(q, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, buf, 0o600)
}

func (q *PendingUploadQueue) Enqueue(item PendingUpload, maxItems int) {
	if q.Items == nil {
		q.Items = []PendingUpload{}
	}
	if item.CreatedAt == "" {
		item.CreatedAt = time.Now().UTC().Format(time.RFC3339)
	}

	for _, e := range q.Items {
		if e.SkillID == item.SkillID && e.ReportHash == item.ReportHash {
			return
		}
	}
	q.Items = append(q.Items, item)
	if maxItems > 0 && len(q.Items) > maxItems {
		q.Items = q.Items[len(q.Items)-maxItems:]
	}
}

func (q PendingUploadQueue) Drain(limit int, sender func(item PendingUpload) error) PendingUploadQueue {
	if limit <= 0 || limit > len(q.Items) {
		limit = len(q.Items)
	}
	kept := make([]PendingUpload, 0, len(q.Items))
	for i, item := range q.Items {
		if i < limit {
			if err := sender(item); err != nil {
				kept = append(kept, item)
			}
			continue
		}
		kept = append(kept, item)
	}
	return PendingUploadQueue{Items: kept}
}
