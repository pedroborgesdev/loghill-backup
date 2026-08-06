package repositories

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"logtheater/internal/domain"
)

func (r *FileRepository) Append(ctx context.Context, id string, entry domain.LogEntry, limit domain.NumberUnitValue) (int64, int64, error) {
	d, err := r.dir(id)
	if err != nil {
		return 0, 0, err
	}
	release := r.acquireWrite(ctx, id)
	defer release()
	s, err := r.readSender(filepath.Join(d, "sender.json"))
	if err != nil {
		return 0, 0, err
	}
	if s.Status == domain.StatusExpired {
		return 0, 0, domain.ErrExpired
	}
	line, err := json.Marshal(entry)
	if err != nil {
		return 0, 0, err
	}
	line = append(line, '\n')
	path := filepath.Join(d, "logs.txt")
	instances, err := readInstances(filepath.Join(d, "instances.json"))
	if err != nil {
		return 0, 0, err
	}
	instanceIndex := -1
	var oldCount, oldSize int64
	if entry.InstanceID != "" {
		for index := range instances {
			if instances[index].ID == entry.InstanceID {
				instanceIndex = index
				oldCount, oldSize = instances[index].LogLineCount, instances[index].LogFileSize
				break
			}
		}
		if instanceIndex < 0 {
			return 0, 0, domain.ErrConflict
		}
		path = filepath.Join(d, "instances", entry.InstanceID, "logs.txt")
	} else {
		oldCount, oldSize = s.LogLineCount, s.LogFileSize
		for _, instance := range instances {
			oldCount -= instance.LogLineCount
			oldSize -= instance.LogFileSize
		}
		if oldCount < 0 {
			oldCount = 0
		}
		if oldSize < 0 {
			oldSize = 0
		}
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0640)
	if errors.Is(err, os.ErrNotExist) {
		return 0, 0, domain.ErrLogFileNotFound
	}
	if err != nil {
		return 0, 0, err
	}
	if _, err = f.Write(line); err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return 0, 0, err
	}
	count := oldCount + 1
	info, err := os.Stat(path)
	if err != nil {
		return 0, 0, err
	}
	size := info.Size()
	if limit.Value > 0 {
		switch limit.Unit {
		case domain.StorageLines:
			if count > int64(limit.Value) {
				count, size, err = r.compactUnlocked(path, int(compactionTarget(int64(limit.Value))))
			}
		case domain.StorageMB:
			maximum := int64(limit.Value) * 1024 * 1024
			if size > maximum {
				count, size, err = r.compactBytesUnlocked(path, compactionTarget(maximum))
			}
		default:
			err = domain.ErrInvalidSettings
		}
		if err != nil {
			return 0, 0, err
		}
	}
	if instanceIndex >= 0 {
		instances[instanceIndex].LogLineCount = count
		instances[instanceIndex].LogFileSize = size
		at := time.Now()
		instances[instanceIndex].LastActivityAt = &at
		if err = writeInstancesAtomic(filepath.Join(d, "instances.json"), instances); err != nil {
			return 0, 0, err
		}
	}
	return s.LogLineCount + count - oldCount, s.LogFileSize + size - oldSize, nil
}

func compactionTarget(limit int64) int64 {
	margin := limit / 20
	if margin < 1 {
		margin = 1
	}
	if margin > limit {
		return 0
	}
	return limit - margin
}

func (r *FileRepository) Compact(ctx context.Context, id string, keep int) (int64, int64, error) {
	d, err := r.dir(id)
	if err != nil {
		return 0, 0, err
	}
	release := r.acquireWrite(ctx, id)
	defer release()
	return r.compactUnlocked(filepath.Join(d, "logs.txt"), keep)
}

func (r *FileRepository) CompactByLimit(ctx context.Context, id string, limit domain.NumberUnitValue) (int64, int64, error) {
	d, err := r.dir(id)
	if err != nil {
		return 0, 0, err
	}
	release := r.acquireWrite(ctx, id)
	defer release()
	instances, err := readInstances(filepath.Join(d, "instances.json"))
	if err != nil {
		return 0, 0, err
	}
	compact := func(path string) (int64, int64, error) {
		var count, size int64
		var compactErr error
		switch limit.Unit {
		case domain.StorageLines:
			count, size, compactErr = r.compactUnlocked(path, limit.Value)
		case domain.StorageMB:
			count, size, compactErr = r.compactBytesUnlocked(path, int64(limit.Value)*1024*1024)
		default:
			return 0, 0, domain.ErrInvalidSettings
		}
		if errors.Is(compactErr, domain.ErrLogFileNotFound) {
			return 0, 0, nil
		}
		return count, size, compactErr
	}
	totalCount, totalSize, err := compact(filepath.Join(d, "logs.txt"))
	if err != nil {
		return 0, 0, err
	}
	for index := range instances {
		count, size, compactErr := compact(filepath.Join(d, "instances", instances[index].ID, "logs.txt"))
		if compactErr != nil {
			return 0, 0, compactErr
		}
		instances[index].LogLineCount, instances[index].LogFileSize = count, size
		totalCount, totalSize = totalCount+count, totalSize+size
	}
	if err = writeInstancesAtomic(filepath.Join(d, "instances.json"), instances); err != nil {
		return 0, 0, err
	}
	return totalCount, totalSize, nil
}

func (r *FileRepository) compactUnlocked(path string, keep int) (int64, int64, error) {
	lines, err := tailLines(path, keep)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return 0, 0, domain.ErrLogFileNotFound
		}
		return 0, 0, err
	}
	return writeLinesAtomic(path, lines)
}

func (r *FileRepository) compactBytesUnlocked(path string, keepBytes int64) (int64, int64, error) {
	lines, err := tailBytes(path, keepBytes)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return 0, 0, domain.ErrLogFileNotFound
		}
		return 0, 0, err
	}
	return writeLinesAtomic(path, lines)
}

func writeLinesAtomic(path string, lines [][]byte) (int64, int64, error) {
	tmp := path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0640)
	if err != nil {
		return 0, 0, err
	}
	for _, line := range lines {
		if _, err = f.Write(append(line, '\n')); err != nil {
			break
		}
	}
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		_ = os.Remove(tmp)
		return 0, 0, err
	}
	if err = os.Rename(tmp, path); err != nil {
		return 0, 0, err
	}
	info, err := os.Stat(path)
	if err != nil {
		return 0, 0, err
	}
	return int64(len(lines)), info.Size(), nil
}

func tailLines(path string, n int) ([][]byte, error) {
	if n <= 0 {
		return [][]byte{}, nil
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	pos := info.Size()
	buf := make([]byte, 64*1024)
	data := []byte{}
	for pos > 0 && bytes.Count(data, []byte{'\n'}) <= n {
		size := int64(len(buf))
		if pos < size {
			size = pos
		}
		pos -= size
		if _, err = f.ReadAt(buf[:size], pos); err != nil && err != io.EOF {
			return nil, err
		}
		data = append(append([]byte{}, buf[:size]...), data...)
	}
	data = bytes.TrimSuffix(data, []byte{'\n'})
	if len(data) == 0 {
		return [][]byte{}, nil
	}
	raw := bytes.Split(data, []byte{'\n'})
	if len(raw) > n {
		raw = raw[len(raw)-n:]
	}
	return raw, nil
}

func tailBytes(path string, maximum int64) ([][]byte, error) {
	if maximum <= 0 {
		return [][]byte{}, nil
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	end := info.Size()
	if end == 0 {
		return [][]byte{}, nil
	}
	last := []byte{0}
	if _, err = f.ReadAt(last, end-1); err != nil && err != io.EOF {
		return nil, err
	}
	if last[0] == '\n' {
		end--
	}
	position := end
	buffer := make([]byte, 64*1024)
	data := []byte{}
	complete := []byte{}
	for position > 0 {
		size := int64(len(buffer))
		if position < size {
			size = position
		}
		position -= size
		if _, err = f.ReadAt(buffer[:size], position); err != nil && err != io.EOF {
			return nil, err
		}
		data = append(append([]byte{}, buffer[:size]...), data...)
		if position == 0 {
			complete = data
		} else if newline := bytes.IndexByte(data, '\n'); newline >= 0 {
			complete = data[newline+1:]
		}
		if int64(len(complete)) >= maximum {
			break
		}
	}
	if len(complete) == 0 {
		return [][]byte{}, nil
	}
	raw := bytes.Split(complete, []byte{'\n'})
	used := int64(0)
	start := len(raw)
	for index := len(raw) - 1; index >= 0; index-- {
		cost := int64(len(raw[index]) + 1)
		if used+cost > maximum {
			break
		}
		used += cost
		start = index
	}
	return raw[start:], nil
}
func (r *FileRepository) ListLogs(ctx context.Context, id string, filters domain.LogFilters) (domain.LogPage, error) {
	d, err := r.dir(id)
	if err != nil {
		return domain.LogPage{}, err
	}
	release := r.acquireRead(ctx, id)
	defer release()
	items := []domain.LogEntry{}
	paths, err := r.logPathsUnlocked(d, filters.InstanceID)
	if err != nil {
		return domain.LogPage{}, err
	}
	for _, path := range paths {
		f, openErr := os.Open(path)
		if errors.Is(openErr, os.ErrNotExist) {
			continue
		}
		if openErr != nil {
			return domain.LogPage{}, openErr
		}
		scan := bufio.NewScanner(f)
		scan.Buffer(make([]byte, 64*1024), 2*1024*1024)
		for scan.Scan() {
			if err = ctx.Err(); err != nil {
				_ = f.Close()
				return domain.LogPage{}, err
			}
			var e domain.LogEntry
			if json.Unmarshal(scan.Bytes(), &e) != nil {
				continue
			}
			if e.SenderID == "" {
				e.SenderID = id
			}
			if match(e, filters) {
				items = append(items, e)
			}
		}
		err = scan.Err()
		_ = f.Close()
		if err != nil {
			return domain.LogPage{}, err
		}
	}
	sort.SliceStable(items, func(i, j int) bool {
		if filters.Order == "desc" {
			return items[i].Timestamp.After(items[j].Timestamp)
		}
		return items[i].Timestamp.Before(items[j].Timestamp)
	})
	total := len(items)
	start := (filters.Page - 1) * filters.PageSize
	if start > total {
		start = total
	}
	end := start + filters.PageSize
	if end > total {
		end = total
	}
	pages := (total + filters.PageSize - 1) / filters.PageSize
	return domain.LogPage{Sender: id, Items: items[start:end], Pagination: domain.Pagination{Page: filters.Page, PageSize: filters.PageSize, Returned: end - start, Total: int64(total), TotalPages: pages}}, nil
}

func (r *FileRepository) logPathsUnlocked(senderDir, instanceID string) ([]string, error) {
	if instanceID == "legacy" {
		return []string{filepath.Join(senderDir, "logs.txt")}, nil
	}
	if instanceID != "" {
		return []string{filepath.Join(senderDir, "instances", instanceID, "logs.txt")}, nil
	}
	paths := []string{filepath.Join(senderDir, "logs.txt")}
	instances, err := readInstances(filepath.Join(senderDir, "instances.json"))
	if err != nil {
		return nil, err
	}
	for _, instance := range instances {
		paths = append(paths, filepath.Join(senderDir, "instances", instance.ID, "logs.txt"))
	}
	return paths, nil
}
func match(e domain.LogEntry, f domain.LogFilters) bool {
	if f.InstanceID != "" {
		if f.InstanceID == "legacy" && e.InstanceID != "" {
			return false
		}
		if f.InstanceID != "legacy" && e.InstanceID != f.InstanceID {
			return false
		}
	}
	if len(f.Severities) > 0 && !f.Severities[e.Severity] {
		return false
	}
	if f.Start != nil && e.Timestamp.Before(*f.Start) {
		return false
	}
	if f.End != nil && e.Timestamp.After(*f.End) {
		return false
	}
	if f.EventMode == "with" && e.Event == "" {
		return false
	}
	if f.EventMode == "without" && e.Event != "" {
		return false
	}
	if f.EventKey != "" && e.Event != f.EventKey {
		return false
	}
	if f.Search != "" {
		q := strings.ToLower(f.Search)
		b, _ := json.Marshal(e.Metadata)
		if !strings.Contains(strings.ToLower(e.Message), q) && !strings.Contains(strings.ToLower(e.Event), q) && !strings.Contains(strings.ToLower(string(b)), q) {
			return false
		}
	}
	return true
}
