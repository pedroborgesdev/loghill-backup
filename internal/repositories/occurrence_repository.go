package repositories

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	"logtheater/internal/domain"
)

type eventOccurrenceSnapshot struct {
	Version int                              `json:"version"`
	Items   map[string]EventOccurrenceRecord `json:"items"`
}

func (r *FileRepository) GetEventOccurrence(ctx context.Context, senderID, occurrenceID string) (EventOccurrenceRecord, error) {
	dir, err := r.dir(senderID)
	if err != nil {
		return EventOccurrenceRecord{}, err
	}
	release, err := r.acquireRead(ctx, senderID)
	if err != nil {
		return EventOccurrenceRecord{}, err
	}
	defer release()
	snapshot, err := readEventOccurrences(filepath.Join(dir, "event-occurrences.json"))
	if err != nil {
		return EventOccurrenceRecord{}, err
	}
	record, ok := snapshot.Items[occurrenceID]
	if !ok {
		return EventOccurrenceRecord{}, os.ErrNotExist
	}
	return record, nil
}

func (r *FileRepository) SaveEventOccurrence(ctx context.Context, senderID string, record EventOccurrenceRecord) error {
	dir, err := r.dir(senderID)
	if err != nil {
		return err
	}
	release, err := r.acquireWrite(ctx, senderID)
	if err != nil {
		return err
	}
	defer release()
	path := filepath.Join(dir, "event-occurrences.json")
	snapshot, err := readEventOccurrences(path)
	if err != nil {
		return err
	}
	if existing, ok := snapshot.Items[record.ID]; ok {
		if existing.Fingerprint == record.Fingerprint {
			return nil
		}
		return domain.ErrEventOccurrenceConflict
	}
	snapshot.Items[record.ID] = record
	return writeEventOccurrences(path, snapshot)
}

func readEventOccurrences(path string) (eventOccurrenceSnapshot, error) {
	snapshot := eventOccurrenceSnapshot{Version: 1, Items: make(map[string]EventOccurrenceRecord)}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return snapshot, nil
	}
	if err != nil {
		return snapshot, err
	}
	if err = json.Unmarshal(data, &snapshot); err != nil {
		return snapshot, err
	}
	if snapshot.Version != 1 || snapshot.Items == nil {
		return snapshot, errors.New("invalid event occurrence snapshot")
	}
	return snapshot, nil
}

func writeEventOccurrences(path string, snapshot eventOccurrenceSnapshot) error {
	data, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	file, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o640)
	if err != nil {
		return err
	}
	if _, err = file.Write(data); err == nil {
		err = file.Sync()
	}
	closeErr := file.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, path)
}
