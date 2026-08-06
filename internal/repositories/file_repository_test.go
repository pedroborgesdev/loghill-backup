package repositories

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"logtheater/internal/domain"
)

func TestTailLines(t *testing.T) {
	path := filepath.Join(t.TempDir(), "logs.txt")
	if err := os.WriteFile(path, []byte("one\ntwo\nthree\nfour\n"), 0600); err != nil {
		t.Fatal(err)
	}
	lines, err := tailLines(path, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(lines) != 2 || string(lines[0]) != "three" || string(lines[1]) != "four" {
		t.Fatalf("unexpected tail: %q", lines)
	}
}
func TestValidIDRejectsTraversal(t *testing.T) {
	for _, id := range []string{"../x", "a/b", `a\b`, ""} {
		if validID(id) {
			t.Fatalf("accepted %q", id)
		}
	}
}

func createRepositorySender(t *testing.T) (*FileRepository, domain.Sender) {
	t.Helper()
	repository := New(t.TempDir())
	if err := repository.Init(); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	sender := domain.Sender{ID: "worker-12345678", Name: "worker", Status: domain.StatusOnline, CreatedAt: now, UpdatedAt: now, LastActivityAt: &now}
	if err := repository.Create(context.Background(), sender); err != nil {
		t.Fatal(err)
	}
	return repository, sender
}

func appendAndUpdate(t *testing.T, repository *FileRepository, sender *domain.Sender, message string, limit domain.NumberUnitValue) {
	t.Helper()
	count, size, err := repository.Append(context.Background(), sender.ID, domain.LogEntry{Timestamp: time.Now(), Severity: domain.Info, Message: message}, limit)
	if err != nil {
		t.Fatal(err)
	}
	sender.LogLineCount = count
	sender.LogFileSize = size
	if err = repository.Update(context.Background(), *sender); err != nil {
		t.Fatal(err)
	}
}

func TestAppendAppliesLineLimitWithMargin(t *testing.T) {
	repository, sender := createRepositorySender(t)
	limit := domain.NumberUnitValue{Value: 3, Unit: domain.StorageLines}
	for _, message := range []string{"one", "two", "three", "four"} {
		appendAndUpdate(t, repository, &sender, message, limit)
	}
	page, err := repository.ListLogs(context.Background(), sender.ID, domain.LogFilters{Page: 1, PageSize: 10, Order: "asc"})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 2 || page.Items[0].Message != "three" || page.Items[1].Message != "four" {
		t.Fatalf("oldest entries were not removed: %+v", page.Items)
	}
}

func TestAppendAppliesMBLimitWithoutSplittingJSONLines(t *testing.T) {
	repository, sender := createRepositorySender(t)
	unlimited := domain.NumberUnitValue{Value: 0, Unit: domain.StorageMB}
	for index := 0; index < 4; index++ {
		appendAndUpdate(t, repository, &sender, strings.Repeat(string(rune('a'+index)), 300_000), unlimited)
	}
	appendAndUpdate(t, repository, &sender, strings.Repeat("z", 300_000), domain.NumberUnitValue{Value: 1, Unit: domain.StorageMB})
	if sender.LogFileSize > 1024*1024 {
		t.Fatalf("file remained over the configured limit: %d", sender.LogFileSize)
	}
	path, _ := repository.dir(sender.ID)
	file, err := os.Open(filepath.Join(path, "logs.txt"))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), 2*1024*1024)
	count := 0
	for scanner.Scan() {
		var entry domain.LogEntry
		if err = json.Unmarshal(scanner.Bytes(), &entry); err != nil {
			t.Fatalf("compaction split a JSON line: %v", err)
		}
		count++
	}
	if err = scanner.Err(); err != nil || count == 0 {
		t.Fatalf("unexpected compacted file: count=%d, err=%v", count, err)
	}
}

func TestInstanceLogLimitsAreIndependent(t *testing.T) {
	repository, sender := createRepositorySender(t)
	now := time.Now()
	instances := []string{"ins_11111111111111111111111111111111", "ins_22222222222222222222222222222222"}
	for _, id := range instances {
		if err := repository.RegisterInstance(context.Background(), sender.ID, domain.SenderInstance{ID: id, CreatedAt: now}); err != nil {
			t.Fatal(err)
		}
	}
	limit := domain.NumberUnitValue{Value: 1, Unit: domain.StorageMB}
	for round := 0; round < 5; round++ {
		for index, id := range instances {
			message := strings.Repeat(string(rune('a'+index)), 300_000)
			count, size, err := repository.Append(context.Background(), sender.ID, domain.LogEntry{Timestamp: now.Add(time.Duration(round) * time.Second), InstanceID: id, Severity: domain.Info, Message: message}, limit)
			if err != nil {
				t.Fatal(err)
			}
			sender.LogLineCount, sender.LogFileSize = count, size
			if err = repository.Update(context.Background(), sender); err != nil {
				t.Fatal(err)
			}
		}
	}
	dir, _ := repository.dir(sender.ID)
	for _, id := range instances {
		info, err := os.Stat(filepath.Join(dir, "instances", id, "logs.txt"))
		if err != nil {
			t.Fatal(err)
		}
		if info.Size() > 1024*1024 {
			t.Fatalf("instance %s exceeded its own limit: %d", id, info.Size())
		}
		page, err := repository.ListLogs(context.Background(), sender.ID, domain.LogFilters{InstanceID: id, Page: 1, PageSize: 10, Order: "asc"})
		if err != nil || len(page.Items) == 0 {
			t.Fatalf("instance %s logs unavailable: items=%d err=%v", id, len(page.Items), err)
		}
		for _, entry := range page.Items {
			if entry.InstanceID != id {
				t.Fatalf("instance %s received log from %s", id, entry.InstanceID)
			}
		}
	}
}

func TestCompactByLimitPreservesNewestEntries(t *testing.T) {
	repository, sender := createRepositorySender(t)
	for _, message := range []string{"one", "two", "three", "four"} {
		appendAndUpdate(t, repository, &sender, message, domain.NumberUnitValue{Value: 0, Unit: domain.StorageLines})
	}
	count, _, err := repository.CompactByLimit(context.Background(), sender.ID, domain.NumberUnitValue{Value: 2, Unit: domain.StorageLines})
	if err != nil || count != 2 {
		t.Fatalf("unexpected compaction: count=%d, err=%v", count, err)
	}
	count, size, err := repository.CompactByLimit(context.Background(), sender.ID, domain.NumberUnitValue{Value: 0, Unit: domain.StorageMB})
	if err != nil || count != 0 || size != 0 {
		t.Fatalf("zero preservation did not empty the file: count=%d size=%d err=%v", count, size, err)
	}
}

func TestCompactByMBPreservesCompleteNewestEntries(t *testing.T) {
	repository, sender := createRepositorySender(t)
	unlimited := domain.NumberUnitValue{Value: 0, Unit: domain.StorageMB}
	for index := 0; index < 4; index++ {
		appendAndUpdate(t, repository, &sender, strings.Repeat(string(rune('a'+index)), 300_000), unlimited)
	}
	count, size, err := repository.CompactByLimit(context.Background(), sender.ID, domain.NumberUnitValue{Value: 1, Unit: domain.StorageMB})
	if err != nil {
		t.Fatal(err)
	}
	if count != 3 || size > 1024*1024 {
		t.Fatalf("unexpected MB preservation: count=%d size=%d", count, size)
	}
	page, err := repository.ListLogs(context.Background(), sender.ID, domain.LogFilters{Page: 1, PageSize: 10, Order: "asc"})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 3 || page.Items[2].Message[0] != 'd' {
		t.Fatalf("newest complete entries were not preserved")
	}
}
