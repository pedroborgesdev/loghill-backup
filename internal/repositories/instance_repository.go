package repositories

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"time"

	"logtheater/internal/domain"
)

func (r *FileRepository) RegisterInstance(ctx context.Context, senderID string, instance domain.SenderInstance) error {
	d, err := r.dir(senderID)
	if err != nil {
		return err
	}
	release := r.acquireWrite(ctx, senderID)
	defer release()
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
	_, err := r.GetInstance(ctx, senderID, instanceID)
	if errors.Is(err, domain.ErrNotFound) {
		return false, nil
	}
	return err == nil, err
}

func (r *FileRepository) GetInstance(ctx context.Context, senderID, instanceID string) (domain.SenderInstance, error) {
	d, err := r.dir(senderID)
	if err != nil {
		return domain.SenderInstance{}, err
	}
	release := r.acquireRead(ctx, senderID)
	defer release()
	instances, err := readInstances(filepath.Join(d, "instances.json"))
	if err != nil {
		return domain.SenderInstance{}, err
	}
	for _, instance := range instances {
		if instance.ID == instanceID {
			return instance, nil
		}
	}
	return domain.SenderInstance{}, domain.ErrNotFound
}

func (r *FileRepository) InstanceCount(ctx context.Context, senderID string) (int, error) {
	d, err := r.dir(senderID)
	if err != nil {
		return 0, err
	}
	release := r.acquireRead(ctx, senderID)
	defer release()
	instances, err := readInstances(filepath.Join(d, "instances.json"))
	if err != nil {
		return 0, err
	}
	return len(instances), nil
}

// RegisteredInstances returns only executions registered in instances.json.
// Legacy logs without an instance_id do not represent an active process and are excluded.
func (r *FileRepository) RegisteredInstances(ctx context.Context, senderID string) ([]domain.SenderInstance, error) {
	d, err := r.dir(senderID)
	if err != nil {
		return nil, err
	}
	release := r.acquireRead(ctx, senderID)
	defer release()
	if _, err = r.readSender(filepath.Join(d, "sender.json")); err != nil {
		return nil, err
	}
	return readInstances(filepath.Join(d, "instances.json"))
}

func (r *FileRepository) DeleteInstance(ctx context.Context, senderID, instanceID string) error {
	d, err := r.dir(senderID)
	if err != nil {
		return err
	}
	release := r.acquireWrite(ctx, senderID)
	defer release()
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
	release := r.acquireWrite(ctx, senderID)
	defer release()
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
	release := r.acquireRead(ctx, senderID)
	defer release()
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
	items := make([]domain.SenderInstance, 0, len(stored.Items))
	for _, storedInstance := range stored.Items {
		instance := storedInstance.SenderInstance
		instance.TokenHash = storedInstance.TokenHash
		items = append(items, instance)
	}
	return items, nil
}

func writeInstancesAtomic(path string, instances []domain.SenderInstance) error {
	stored := make([]persistedInstance, 0, len(instances))
	for _, instance := range instances {
		stored = append(stored, persistedInstance{SenderInstance: instance, TokenHash: instance.TokenHash})
	}
	b, err := json.MarshalIndent(persistedInstances{Items: stored}, "", "  ")
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
