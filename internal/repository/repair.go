package repository

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	"logtheater/internal/domain"
)

func (r *FileRepository) Repair(ctx context.Context, s domain.Sender) (domain.Sender, error) {
	d, err := r.dir(s.ID)
	if err != nil {
		return s, err
	}
	release := r.acquireWrite(ctx, s.ID)
	defer release()
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
