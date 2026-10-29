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
	Name         string    `json:"name"`
	DatabaseName string    `json:"database_name"`
	ParentBranch string    `json:"parent_branch"`
	CreatedAt    time.Time `json:"created_at"`
}

// MetadataStore manages the persistent record of branch lineages.
type MetadataStore struct {
	dir      string
	filePath string
	records  map[string]BranchRecord
	mu       sync.RWMutex
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
		dir:      dataDir,
		filePath: filePath,
		records:  make(map[string]BranchRecord),
	}

	if err := store.load(); err != nil {
		// If file doesn't exist, start fresh
		store.records = make(map[string]BranchRecord)
	}

	return store, nil
}

func (s *MetadataStore) load() error {
	data, err := os.ReadFile(s.filePath)
	if err != nil {
		return err
	}

	var recs map[string]BranchRecord
	if err := json.Unmarshal(data, &recs); err != nil {
		return err
	}

	s.records = recs
	return nil
}

func (s *MetadataStore) save() error {
	data, err := json.MarshalIndent(s.records, "", "  ")
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

// RemoveBranch deletes branch lineage records.
func (s *MetadataStore) RemoveBranch(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.records, name)
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
