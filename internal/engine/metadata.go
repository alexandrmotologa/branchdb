package engine

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// BranchRecord holds persistent lineage metadata.
type BranchRecord struct {
	Name          string    `json:"name"`
	DatabaseName  string    `json:"database_name"`
	ParentBranch  string    `json:"parent_branch"`
	CreatedAt     time.Time `json:"created_at"`
	LastQueriedAt time.Time `json:"last_queried_at,omitempty"`
}

// SnapshotRecord holds persistent snapshot metadata.
type SnapshotRecord struct {
	Name         string    `json:"name"`
	BranchName   string    `json:"branch_name"`
	DatabaseName string    `json:"database_name"`
	CreatedAt    time.Time `json:"created_at"`
}

// metadataPayload represents the combined on-disk structure.
type metadataPayload struct {
	Branches  map[string]BranchRecord   `json:"branches"`
	Snapshots map[string]SnapshotRecord `json:"snapshots"`
}

// MetadataStore manages the persistent record of branch lineages and snapshots.
type MetadataStore struct {
	dir       string
	filePath  string
	records   map[string]BranchRecord
	snapshots map[string]SnapshotRecord
	mu        sync.RWMutex
}

// NewMetadataStore initializes a metadata store at the given directory.
func NewMetadataStore(dataDir string) (*MetadataStore, error) {
	if dataDir == "" {
		dataDir = ".branchdb"
	}

	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return nil, err
	}

	filePath := filepath.Join(dataDir, "metadata.json")
	store := &MetadataStore{
		dir:       dataDir,
		filePath:  filePath,
		records:   make(map[string]BranchRecord),
		snapshots: make(map[string]SnapshotRecord),
	}

	if err := store.load(); err != nil {
		store.records = make(map[string]BranchRecord)
		store.snapshots = make(map[string]SnapshotRecord)
	}

	return store, nil
}

func (s *MetadataStore) load() error {
	data, err := os.ReadFile(s.filePath)
	if err != nil {
		return err
	}

	// Try modern combined structure
	var payload metadataPayload
	if err := json.Unmarshal(data, &payload); err == nil && (payload.Branches != nil || payload.Snapshots != nil) {
		if payload.Branches != nil {
			s.records = payload.Branches
		} else {
			s.records = make(map[string]BranchRecord)
		}
		if payload.Snapshots != nil {
			s.snapshots = payload.Snapshots
		} else {
			s.snapshots = make(map[string]SnapshotRecord)
		}
		return nil
	}

	// Fallback to legacy structure (direct map of branches)
	var legacyRecs map[string]BranchRecord
	if err := json.Unmarshal(data, &legacyRecs); err == nil {
		s.records = legacyRecs
		s.snapshots = make(map[string]SnapshotRecord)
		return nil
	}

	return err
}

func (s *MetadataStore) save() error {
	payload := metadataPayload{
		Branches:  s.records,
		Snapshots: s.snapshots,
	}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.filePath, data, 0644)
}

// RecordBranch logs or updates branch creation metadata.
func (s *MetadataStore) RecordBranch(name, dbName, parentBranch string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	rec, exists := s.records[name]
	if !exists {
		rec = BranchRecord{
			Name:         name,
			DatabaseName: dbName,
			ParentBranch: parentBranch,
			CreatedAt:    time.Now().UTC(),
		}
	} else {
		if parentBranch != "" {
			rec.ParentBranch = parentBranch
		}
	}
	s.records[name] = rec
	return s.save()
}

// TouchBranch updates the last queried timestamp for a branch.
func (s *MetadataStore) TouchBranch(name string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	rec, exists := s.records[name]
	if exists {
		rec.LastQueriedAt = time.Now().UTC()
		s.records[name] = rec
		_ = s.save()
	}
}

// TouchDatabase updates the last queried timestamp using database name.
func (s *MetadataStore) TouchDatabase(dbName string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for k, rec := range s.records {
		if rec.DatabaseName == dbName {
			rec.LastQueriedAt = time.Now().UTC()
			s.records[k] = rec
			_ = s.save()
			return
		}
	}
}

// RemoveBranch deletes branch lineage records.
func (s *MetadataStore) RemoveBranch(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.records, name)
	// Clean up related snapshots
	for k, snap := range s.snapshots {
		if snap.BranchName == name {
			delete(s.snapshots, k)
		}
	}
	return s.save()
}

// GetRecord looks up metadata for a given branch name.
func (s *MetadataStore) GetRecord(name string) (BranchRecord, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rec, ok := s.records[name]
	return rec, ok
}

// AllRecords returns a copy of all stored branch records.
func (s *MetadataStore) AllRecords() map[string]BranchRecord {
	s.mu.RLock()
	defer s.mu.RUnlock()

	res := make(map[string]BranchRecord, len(s.records))
	for k, v := range s.records {
		res[k] = v
	}
	return res
}

// RecordSnapshot registers a newly created checkpoint database.
func (s *MetadataStore) RecordSnapshot(snapshotName, branchName, dbName string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := branchName + ":" + snapshotName
	s.snapshots[key] = SnapshotRecord{
		Name:         snapshotName,
		BranchName:   branchName,
		DatabaseName: dbName,
		CreatedAt:    time.Now().UTC(),
	}
	return s.save()
}

// RemoveSnapshot deletes snapshot metadata.
func (s *MetadataStore) RemoveSnapshot(snapshotName, branchName string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := branchName + ":" + snapshotName
	delete(s.snapshots, key)
	return s.save()
}

// GetSnapshot retrieves snapshot metadata.
func (s *MetadataStore) GetSnapshot(snapshotName, branchName string) (SnapshotRecord, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	key := branchName + ":" + snapshotName
	rec, ok := s.snapshots[key]
	return rec, ok
}

// ListSnapshots returns all snapshots, optionally filtered by branch.
func (s *MetadataStore) ListSnapshots(branchName string) []SnapshotRecord {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []SnapshotRecord
	for _, snap := range s.snapshots {
		if branchName == "" || snap.BranchName == branchName {
			result = append(result, snap)
		}
	}
	return result
}
