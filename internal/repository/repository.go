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

func New(root string) *FileRepository {
	return &FileRepository{root: filepath.Join(root, "senders"), locks: &storage.LockManager{}}
}
func (r *FileRepository) Init() error { return os.MkdirAll(r.root, 0750) }
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
	f, err := os.OpenFile(filepath.Join(d, "logs.txt"), os.O_APPEND|os.O_WRONLY, 0640)
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
	path := filepath.Join(d, "logs.txt")
	count := s.LogLineCount + 1
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
	return count, size, nil
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
	path := filepath.Join(d, "logs.txt")
	switch limit.Unit {
	case domain.StorageLines:
		return r.compactUnlocked(path, limit.Value)
	case domain.StorageMB:
		return r.compactBytesUnlocked(path, int64(limit.Value)*1024*1024)
	default:
		return 0, 0, domain.ErrInvalidSettings
	}
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
	err = os.Remove(filepath.Join(d, "logs.txt"))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
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
	f, err := os.Open(filepath.Join(d, "logs.txt"))
	if errors.Is(err, os.ErrNotExist) {
		return domain.LogPage{}, domain.ErrLogFileNotFound
	}
	if err != nil {
		return domain.LogPage{}, err
	}
	defer f.Close()
	items := []domain.LogEntry{}
	scan := bufio.NewScanner(f)
	scan.Buffer(make([]byte, 64*1024), 2*1024*1024)
	for scan.Scan() {
		select {
		case <-ctx.Done():
			return domain.LogPage{}, ctx.Err()
		default:
		}
		var e domain.LogEntry
		if json.Unmarshal(scan.Bytes(), &e) != nil {
			continue
		}
		if e.SenderID == "" {
			e.SenderID = id
		}
		if !match(e, filters) {
			continue
		}
		items = append(items, e)
	}
	if err = scan.Err(); err != nil {
		return domain.LogPage{}, err
	}
	if filters.Order == "desc" {
		for i, j := 0, len(items)-1; i < j; i, j = i+1, j-1 {
			items[i], items[j] = items[j], items[i]
		}
	}
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
func match(e domain.LogEntry, f domain.LogFilters) bool {
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
	f, err := os.Open(filepath.Join(d, "logs.txt"))
	if errors.Is(err, os.ErrNotExist) {
		if s.Status == domain.StatusExpired {
			return s, nil
		}
		f, err = os.OpenFile(filepath.Join(d, "logs.txt"), os.O_CREATE|os.O_WRONLY, 0640)
	}
	if err != nil {
		return s, err
	}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 2*1024*1024)
	var count int64
	for sc.Scan() {
		count++
	}
	_ = f.Close()
	if err = sc.Err(); err != nil {
		return s, err
	}
	info, err := os.Stat(filepath.Join(d, "logs.txt"))
	if err != nil {
		return s, err
	}
	s.LogLineCount = count
	s.LogFileSize = info.Size()
	return s, r.writeSenderUnlocked(d, s)
}
