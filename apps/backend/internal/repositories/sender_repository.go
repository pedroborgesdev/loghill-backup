package repositories

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"

	"logmate/internal/domain"
)

func (r *FileRepository) Create(ctx context.Context, s domain.Sender) error {
	d, err := r.dir(s.ID)
	if err != nil {
		return err
	}
	release, err := r.acquireWrite(ctx, s.ID)
	if err != nil {
		return err
	}
	defer release()
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
	release, err := r.acquireRead(ctx, id)
	if err != nil {
		return domain.Sender{}, err
	}
	defer release()
	return r.readSender(filepath.Join(d, "sender.json"))
}
func (r *FileRepository) Update(ctx context.Context, s domain.Sender) error {
	d, err := r.dir(s.ID)
	if err != nil {
		return err
	}
	release, err := r.acquireWrite(ctx, s.ID)
	if err != nil {
		return err
	}
	defer release()
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
func (r *FileRepository) DeleteLogs(ctx context.Context, id string) error {
	d, err := r.dir(id)
	if err != nil {
		return err
	}
	release, err := r.acquireWrite(ctx, id)
	if err != nil {
		return err
	}
	defer release()
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
	release, err := r.acquireWrite(ctx, id)
	if err != nil {
		return err
	}
	defer release()
	if _, err = os.Stat(filepath.Join(d, "sender.json")); errors.Is(err, os.ErrNotExist) {
		return domain.ErrNotFound
	} else if err != nil {
		return err
	}
	return os.RemoveAll(d)
}
