package repository

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
	"logtheater/internal/storage"
)

type FileRepository struct {
	root  string
	locks *storage.LockManager
}

type persistedSender struct {
	domain.Sender
	KeyHash string `json:"key_hash,omitempty"`
}

type persistedInstances struct {
	Items []domain.SenderInstance `json:"items"`
}

func New(root string) *FileRepository {
	return &FileRepository{root: filepath.Join(root, "senders"), locks: &storage.LockManager{}}
}
func (r *FileRepository) Init() error { return os.MkdirAll(r.root, 0750) }

func (r *FileRepository) RegisterInstance(ctx context.Context, senderID string, instance domain.SenderInstance) error {
	d, err := r.dir(senderID)
	if err != nil {
		return err
	}
	l := r.locks.Get(senderID)
	l.Lock()
	defer l.Unlock()
	if _, err = r.readSender(filepath.Join(d, "sender.json")); err != nil {
		return err
	}
	instances, err := readInstances(filepath.Join(d, "instances.json"))
	if err != nil {
		return err
	}
	for _, current := range instances {
		if current.ID == instance.ID {
			return domain.ErrConflict
		}
	}
	instances = append(instances, instance)
	instanceDir := filepath.Join(d, "instances", instance.ID)
	if err = os.MkdirAll(instanceDir, 0750); err != nil {
		return err
	}
	f, err := os.OpenFile(filepath.Join(instanceDir, "logs.txt"), os.O_CREATE|os.O_WRONLY, 0640)
	if err != nil {
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return writeInstancesAtomic(filepath.Join(d, "instances.json"), instances)
}

func (r *FileRepository) InstanceExists(ctx context.Context, senderID, instanceID string) (bool, error) {
	d, err := r.dir(senderID)
	if err != nil {
		return false, err
	}
	l := r.locks.Get(senderID)
	l.RLock()
	defer l.RUnlock()
	instances, err := readInstances(filepath.Join(d, "instances.json"))
	if err != nil {
		return false, err
	}
	for _, instance := range instances {
		if instance.ID == instanceID {
			return true, nil
		}
	}
	return false, nil
}

func (r *FileRepository) InstanceCount(ctx context.Context, senderID string) (int, error) {
	d, err := r.dir(senderID)
	if err != nil {
		return 0, err
	}
	l := r.locks.Get(senderID)
	l.RLock()
	defer l.RUnlock()
	instances, err := readInstances(filepath.Join(d, "instances.json"))
	if err != nil {
		return 0, err
	}
	return len(instances), nil
}

func (r *FileRepository) DeleteInstance(ctx context.Context, senderID, instanceID string) error {
	d, err := r.dir(senderID)
	if err != nil {
		return err
	}
	l := r.locks.Get(senderID)
	l.Lock()
	defer l.Unlock()
	senderPath := filepath.Join(d, "sender.json")
	sender, err := r.readSender(senderPath)
	if err != nil {
		return err
	}
	if instanceID == "legacy" {
		count, size, countErr := countLogFile(filepath.Join(d, "logs.txt"))
		if countErr != nil && !errors.Is(countErr, os.ErrNotExist) {
			return countErr
		}
		if err = os.Remove(filepath.Join(d, "logs.txt")); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		sender.LogLineCount = max(0, sender.LogLineCount-count)
		sender.LogFileSize = max(0, sender.LogFileSize-size)
		return r.writeSenderUnlocked(d, sender)
	}
	instances, err := readInstances(filepath.Join(d, "instances.json"))
	if err != nil {
		return err
	}
	index := -1
	var removed domain.SenderInstance
	for current := range instances {
		if instances[current].ID == instanceID {
			index, removed = current, instances[current]
			break
		}
	}
	if index < 0 {
		return domain.ErrNotFound
	}
	instanceDir := filepath.Join(d, "instances", instanceID)
	if err = os.RemoveAll(instanceDir); err != nil {
		return err
	}
	instances = append(instances[:index], instances[index+1:]...)
	if err = writeInstancesAtomic(filepath.Join(d, "instances.json"), instances); err != nil {
		return err
	}
	sender.LogLineCount = max(0, sender.LogLineCount-removed.LogLineCount)
	sender.LogFileSize = max(0, sender.LogFileSize-removed.LogFileSize)
	return r.writeSenderUnlocked(d, sender)
}

func (r *FileRepository) TouchInstance(ctx context.Context, senderID, instanceID string, at time.Time, healthcheck bool) error {
	if instanceID == "" {
		return nil
	}
	d, err := r.dir(senderID)
	if err != nil {
		return err
	}
	l := r.locks.Get(senderID)
	l.Lock()
	defer l.Unlock()
	instances, err := readInstances(filepath.Join(d, "instances.json"))
	if err != nil {
		return err
	}
	for index := range instances {
		if instances[index].ID != instanceID {
			continue
		}
		instances[index].LastActivityAt = &at
		if healthcheck {
			instances[index].LastHealthcheckAt = &at
		}
		return writeInstancesAtomic(filepath.Join(d, "instances.json"), instances)
	}
	return domain.ErrConflict
}

func (r *FileRepository) ListInstances(ctx context.Context, senderID string) ([]domain.SenderInstance, error) {
	d, err := r.dir(senderID)
	if err != nil {
		return nil, err
	}
	l := r.locks.Get(senderID)
	l.RLock()
	defer l.RUnlock()
	if _, err = r.readSender(filepath.Join(d, "sender.json")); err != nil {
		return nil, err
	}
	instances, err := readInstances(filepath.Join(d, "instances.json"))
	if err != nil {
		return nil, err
	}
	byID := make(map[string]*domain.SenderInstance, len(instances)+1)
	for index := range instances {
		byID[instances[index].ID] = &instances[index]
	}
	legacy := domain.SenderInstance{ID: "legacy", Legacy: true}
	path := filepath.Join(d, "logs.txt")
	f, openErr := os.Open(path)
	if openErr == nil {
		scan := bufio.NewScanner(f)
		scan.Buffer(make([]byte, 64*1024), 2*1024*1024)
		for scan.Scan() {
			if err = ctx.Err(); err != nil {
				_ = f.Close()
				return nil, err
			}
			var entry domain.LogEntry
			if json.Unmarshal(scan.Bytes(), &entry) != nil {
				continue
			}
			if entry.InstanceID != "" {
				continue
			}
			id := legacy.ID
			instance := byID[id]
			if instance == nil {
				instance = &legacy
				byID[id] = instance
			}
			instance.LogLineCount++
			instance.LogFileSize += int64(len(scan.Bytes()) + 1)
			if instance.LastActivityAt == nil || entry.Timestamp.After(*instance.LastActivityAt) {
				at := entry.Timestamp
				instance.LastActivityAt = &at
			}
		}
		err = scan.Err()
		_ = f.Close()
		if err != nil {
			return nil, err
		}
	} else if !errors.Is(openErr, os.ErrNotExist) {
		return nil, openErr
	}
	if legacy.LogLineCount > 0 {
		instances = append(instances, legacy)
	}
	sort.SliceStable(instances, func(i, j int) bool {
		left, right := instances[i].CreatedAt, instances[j].CreatedAt
		if instances[i].LastActivityAt != nil {
			left = *instances[i].LastActivityAt
		}
		if instances[j].LastActivityAt != nil {
			right = *instances[j].LastActivityAt
		}
		return left.After(right)
	})
	return instances, nil
}

func readInstances(path string) ([]domain.SenderInstance, error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return []domain.SenderInstance{}, nil
	}
	if err != nil {
		return nil, err
	}
	var stored persistedInstances
	if err = json.Unmarshal(b, &stored); err != nil {
		return nil, err
	}
	return stored.Items, nil
}

func writeInstancesAtomic(path string, instances []domain.SenderInstance) error {
	b, err := json.MarshalIndent(persistedInstances{Items: instances}, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0640)
	if err != nil {
		return err
	}
	if _, err = f.Write(b); err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, path)
}
func validID(id string) bool {
	if id == "" || strings.ContainsAny(id, `/\`) || strings.Contains(id, "..") {
		return false
	}
	for _, c := range id {
		if !(c == '-' || c == '_' || c >= 'a' && c <= 'z' || c >= '0' && c <= '9') {
			return false
		}
	}
	return true
}
func (r *FileRepository) dir(id string) (string, error) {
	if !validID(id) {
		return "", domain.ErrNotFound
	}
	return filepath.Join(r.root, id), nil
}
func (r *FileRepository) Create(ctx context.Context, s domain.Sender) error {
	d, err := r.dir(s.ID)
	if err != nil {
		return err
	}
	l := r.locks.Get(s.ID)
	l.Lock()
	defer l.Unlock()
	if err = os.Mkdir(d, 0750); err != nil {
		if errors.Is(err, os.ErrExist) {
			return domain.ErrConflict
		}
		return err
	}
	f, err := os.OpenFile(filepath.Join(d, "logs.txt"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0640)
	if err == nil {
		err = f.Close()
	}
	if err != nil {
		_ = os.RemoveAll(d)
		return err
	}
	if err = r.writeSenderUnlocked(d, s); err != nil {
		_ = os.RemoveAll(d)
	}
	return err
}
func (r *FileRepository) Get(ctx context.Context, id string) (domain.Sender, error) {
	d, err := r.dir(id)
	if err != nil {
		return domain.Sender{}, err
	}
	l := r.locks.Get(id)
	l.RLock()
	defer l.RUnlock()
	return r.readSender(filepath.Join(d, "sender.json"))
}
func (r *FileRepository) Update(ctx context.Context, s domain.Sender) error {
	d, err := r.dir(s.ID)
	if err != nil {
		return err
	}
	l := r.locks.Get(s.ID)
	l.Lock()
	defer l.Unlock()
	return r.writeSenderUnlocked(d, s)
}
func (r *FileRepository) writeSenderUnlocked(d string, s domain.Sender) error {
	b, err := json.MarshalIndent(persistedSender{Sender: s, KeyHash: s.KeyHash}, "", "  ")
	if err != nil {
		return err
	}
	tmp := filepath.Join(d, "sender.json.tmp")
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0640)
	if err != nil {
		return err
	}
	if _, err = f.Write(b); err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, filepath.Join(d, "sender.json"))
}
func (r *FileRepository) readSender(path string) (domain.Sender, error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return domain.Sender{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.Sender{}, err
	}
	var stored persistedSender
	if err = json.Unmarshal(b, &stored); err != nil {
		return stored.Sender, err
	}
	stored.Sender.KeyHash = stored.KeyHash
	return stored.Sender, nil
}
func (r *FileRepository) All(ctx context.Context) ([]domain.Sender, error) {
	entries, err := os.ReadDir(r.root)
	if err != nil {
		return nil, err
	}
	out := make([]domain.Sender, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		s, er := r.Get(ctx, e.Name())
		if er == nil {
			out = append(out, s)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out, nil
}
func (r *FileRepository) Append(ctx context.Context, id string, entry domain.LogEntry, limit domain.NumberUnitValue) (int64, int64, error) {
	d, err := r.dir(id)
	if err != nil {
		return 0, 0, err
	}
	l := r.locks.Get(id)
	l.Lock()
	defer l.Unlock()
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
	l := r.locks.Get(id)
	l.Lock()
	defer l.Unlock()
	return r.compactUnlocked(filepath.Join(d, "logs.txt"), keep)
}

func (r *FileRepository) CompactByLimit(ctx context.Context, id string, limit domain.NumberUnitValue) (int64, int64, error) {
	d, err := r.dir(id)
	if err != nil {
		return 0, 0, err
	}
	l := r.locks.Get(id)
	l.Lock()
	defer l.Unlock()
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
func (r *FileRepository) DeleteLogs(ctx context.Context, id string) error {
	d, err := r.dir(id)
	if err != nil {
		return err
	}
	l := r.locks.Get(id)
	l.Lock()
	defer l.Unlock()
	paths, err := r.logPathsUnlocked(d, "")
	if err != nil {
		return err
	}
	for _, path := range paths {
		if removeErr := os.Remove(path); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
			return removeErr
		}
	}
	instances, err := readInstances(filepath.Join(d, "instances.json"))
	if err != nil {
		return err
	}
	for index := range instances {
		instances[index].LogLineCount, instances[index].LogFileSize = 0, 0
	}
	return writeInstancesAtomic(filepath.Join(d, "instances.json"), instances)
}

func (r *FileRepository) Delete(ctx context.Context, id string) error {
	d, err := r.dir(id)
	if err != nil {
		return err
	}
	l := r.locks.Get(id)
	l.Lock()
	defer l.Unlock()
	if _, err = os.Stat(filepath.Join(d, "sender.json")); errors.Is(err, os.ErrNotExist) {
		return domain.ErrNotFound
	} else if err != nil {
		return err
	}
	return os.RemoveAll(d)
}
func (r *FileRepository) ListLogs(ctx context.Context, id string, filters domain.LogFilters) (domain.LogPage, error) {
	d, err := r.dir(id)
	if err != nil {
		return domain.LogPage{}, err
	}
	l := r.locks.Get(id)
	l.RLock()
	defer l.RUnlock()
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
func (r *FileRepository) Repair(ctx context.Context, s domain.Sender) (domain.Sender, error) {
	d, err := r.dir(s.ID)
	if err != nil {
		return s, err
	}
	l := r.locks.Get(s.ID)
	l.Lock()
	defer l.Unlock()
	_ = os.Remove(filepath.Join(d, "logs.txt.tmp"))
	_ = os.Remove(filepath.Join(d, "sender.json.tmp"))
	_ = os.Remove(filepath.Join(d, "instances.json.tmp"))
	rootPath := filepath.Join(d, "logs.txt")
	f, err := os.Open(rootPath)
	if errors.Is(err, os.ErrNotExist) {
		if s.Status == domain.StatusExpired {
			return s, nil
		}
		f, err = os.OpenFile(rootPath, os.O_CREATE|os.O_WRONLY, 0640)
	}
	if err != nil {
		return s, err
	}
	instances, err := readInstances(filepath.Join(d, "instances.json"))
	if err != nil {
		_ = f.Close()
		return s, err
	}
	known := make(map[string]int, len(instances))
	for index := range instances {
		known[instances[index].ID] = index
	}
	legacyLines := make([][]byte, 0)
	instanceLines := make(map[string][][]byte)
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 2*1024*1024)
	for sc.Scan() {
		line := append([]byte(nil), sc.Bytes()...)
		var entry domain.LogEntry
		if json.Unmarshal(line, &entry) == nil && entry.InstanceID != "" {
			if _, exists := known[entry.InstanceID]; exists {
				instanceLines[entry.InstanceID] = append(instanceLines[entry.InstanceID], line)
				continue
			}
		}
		legacyLines = append(legacyLines, line)
	}
	_ = f.Close()
	if err = sc.Err(); err != nil {
		return s, err
	}
	legacyCount, legacySize, err := writeLinesAtomic(rootPath, legacyLines)
	if err != nil {
		return s, err
	}
	totalCount, totalSize := legacyCount, legacySize
	for index := range instances {
		instanceDir := filepath.Join(d, "instances", instances[index].ID)
		if err = os.MkdirAll(instanceDir, 0750); err != nil {
			return s, err
		}
		path := filepath.Join(instanceDir, "logs.txt")
		lines, migrating := instanceLines[instances[index].ID]
		var count, size int64
		if migrating {
			count, size, err = writeLinesAtomic(path, lines)
		} else {
			count, size, err = countLogFile(path)
		}
		if err != nil {
			return s, err
		}
		instances[index].LogLineCount, instances[index].LogFileSize = count, size
		if len(lines) > 0 {
			var last domain.LogEntry
			if json.Unmarshal(lines[len(lines)-1], &last) == nil {
				at := last.Timestamp
				instances[index].LastActivityAt = &at
			}
		}
		totalCount, totalSize = totalCount+count, totalSize+size
	}
	if err = writeInstancesAtomic(filepath.Join(d, "instances.json"), instances); err != nil {
		return s, err
	}
	s.LogLineCount, s.LogFileSize = totalCount, totalSize
	return s, r.writeSenderUnlocked(d, s)
}

func countLogFile(path string) (int64, int64, error) {
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		created, createErr := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, 0640)
		if createErr != nil {
			return 0, 0, createErr
		}
		return 0, 0, created.Close()
	}
	if err != nil {
		return 0, 0, err
	}
	scan := bufio.NewScanner(f)
	scan.Buffer(make([]byte, 64*1024), 2*1024*1024)
	var count int64
	for scan.Scan() {
		count++
	}
	if err = scan.Err(); err != nil {
		_ = f.Close()
		return 0, 0, err
	}
	if err = f.Close(); err != nil {
		return 0, 0, err
	}
	info, err := os.Stat(path)
	if err != nil {
		return 0, 0, err
	}
	return count, info.Size(), nil
}
