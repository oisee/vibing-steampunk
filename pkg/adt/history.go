// Package adt provides ABAP Development Tools client functionality.
// history.go manages execution recording storage and retrieval.
package adt

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// RecordingIndex holds metadata for quick lookup without loading full recordings.
type RecordingIndex struct {
	ID          string    `json:"id"`
	SessionID   string    `json:"session_id"`
	Program     string    `json:"program"`
	Description string    `json:"description,omitempty"`
	StartTime   time.Time `json:"start_time"`
	EndTime     time.Time `json:"end_time,omitempty"`
	TotalSteps  int       `json:"total_steps"`
	IsComplete  bool      `json:"is_complete"`
	Tags        []string  `json:"tags,omitempty"`
	FilePath    string    `json:"file_path"`
	SizeBytes   int64     `json:"size_bytes"`
}

// HistoryManager manages execution recordings.
type HistoryManager struct {
	mu        sync.RWMutex
	storePath string
	index     []RecordingIndex
	cache     map[string]*ExecutionRecording // LRU cache for loaded recordings
	cacheSize int
}

// NewHistoryManager creates a new history manager.
func NewHistoryManager(storePath string) (*HistoryManager, error) {
	if err := os.MkdirAll(storePath, 0755); err != nil {
		return nil, fmt.Errorf("failed to create store path: %w", err)
	}

	hm := &HistoryManager{
		storePath: storePath,
		index:     make([]RecordingIndex, 0),
		cache:     make(map[string]*ExecutionRecording),
		cacheSize: 10, // Keep last 10 recordings in memory
	}

	// Load existing index
	if err := hm.loadIndex(); err != nil {
		// Index doesn't exist yet, that's OK
		_ = hm.rebuildIndex()
	}

	return hm, nil
}

// indexPath returns the path to the index file.
func (hm *HistoryManager) indexPath() string {
	return filepath.Join(hm.storePath, "index.json")
}

// loadIndex loads the recording index from disk.
func (hm *HistoryManager) loadIndex() error {
	data, err := os.ReadFile(hm.indexPath())
	if err != nil {
		return err
	}
	return json.Unmarshal(data, &hm.index)
}

// saveIndex persists the recording index to disk.
func (hm *HistoryManager) saveIndex() error {
	data, err := json.MarshalIndent(hm.index, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(hm.indexPath(), data, 0644)
}

// rebuildIndex scans the store directory and rebuilds the index.
func (hm *HistoryManager) rebuildIndex() error {
	hm.mu.Lock()
	defer hm.mu.Unlock()

	hm.index = make([]RecordingIndex, 0)

	files, err := os.ReadDir(hm.storePath)
	if err != nil {
		return err
	}

	for _, file := range files {
		if !file.IsDir() && strings.HasSuffix(file.Name(), ".json") && file.Name() != "index.json" {
			filePath := filepath.Join(hm.storePath, file.Name())
			recording, err := hm.loadRecordingFromFile(filePath)
			if err != nil {
				continue // Skip invalid files
			}

			info, _ := file.Info()
			idx := RecordingIndex{
				ID:          recording.ID,
				SessionID:   recording.SessionID,
				Program:     recording.Program,
				Description: recording.Description,
				StartTime:   recording.StartTime,
				EndTime:     recording.EndTime,
				TotalSteps:  recording.TotalSteps,
				IsComplete:  recording.IsComplete,
				Tags:        recording.Tags,
				FilePath:    filePath,
				SizeBytes:   info.Size(),
			}
			hm.index = append(hm.index, idx)
		}
	}

	// Sort by start time descending (newest first)
	sort.Slice(hm.index, func(i, j int) bool {
		return hm.index[i].StartTime.After(hm.index[j].StartTime)
	})

	return hm.saveIndex()
}

// loadRecordingFromFile loads a recording from a JSON file.
func (hm *HistoryManager) loadRecordingFromFile(path string) (*ExecutionRecording, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return FromJSON(data)
}

// SaveRecording persists a recording to disk.
func (hm *HistoryManager) SaveRecording(recorder *ExecutionRecorder) error {
	hm.mu.Lock()
	defer hm.mu.Unlock()

	recording := recorder.GetRecording()
	data, err := json.MarshalIndent(recording, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to serialize recording: %w", err)
	}

	fileName := fmt.Sprintf("%s.json", recording.ID)
	filePath := filepath.Join(hm.storePath, fileName)

	if err := os.WriteFile(filePath, data, 0644); err != nil {
		return fmt.Errorf("failed to write recording: %w", err)
	}

	// Update index
	info, _ := os.Stat(filePath)
	idx := RecordingIndex{
		ID:          recording.ID,
		SessionID:   recording.SessionID,
		Program:     recording.Program,
		Description: recording.Description,
		StartTime:   recording.StartTime,
		EndTime:     recording.EndTime,
		TotalSteps:  recording.TotalSteps,
		IsComplete:  recording.IsComplete,
		Tags:        recording.Tags,
		FilePath:    filePath,
		SizeBytes:   info.Size(),
	}

	// Remove existing entry if any
	for i, existing := range hm.index {
		if existing.ID == recording.ID {
			hm.index = append(hm.index[:i], hm.index[i+1:]...)
			break
		}
	}

	// Add new entry at the beginning
	hm.index = append([]RecordingIndex{idx}, hm.index...)

	// Update cache
	hm.cache[recording.ID] = recording
	hm.evictCache()

	return hm.saveIndex()
}

// LoadRecording loads a recording by ID.
func (hm *HistoryManager) LoadRecording(id string) (*ExecutionRecording, error) {
	hm.mu.Lock()
	defer hm.mu.Unlock()

	// Check cache first
	if recording, exists := hm.cache[id]; exists {
		return recording, nil
	}

	// Find in index
	var filePath string
	for _, idx := range hm.index {
		if idx.ID == id {
			filePath = idx.FilePath
			break
		}
	}

	if filePath == "" {
		return nil, fmt.Errorf("recording not found: %s", id)
	}

	recording, err := hm.loadRecordingFromFile(filePath)
	if err != nil {
		return nil, err
	}

	// Add to cache
	hm.cache[id] = recording
	hm.evictCache()

	return recording, nil
}

// evictCache removes old entries if cache exceeds size limit.
func (hm *HistoryManager) evictCache() {
	if len(hm.cache) <= hm.cacheSize {
		return
	}

	// Find oldest entries to evict
	// Simple strategy: remove random entries until under limit
	for id := range hm.cache {
		if len(hm.cache) <= hm.cacheSize {
			break
		}
		delete(hm.cache, id)
	}
}

// ListRecordings returns all recording indexes, optionally filtered.
func (hm *HistoryManager) ListRecordings(filter RecordingFilter) []RecordingIndex {
	hm.mu.RLock()
	defer hm.mu.RUnlock()

	var result []RecordingIndex
	for _, idx := range hm.index {
		if filter.matches(idx) {
			result = append(result, idx)
		}
	}

	if filter.Limit > 0 && len(result) > filter.Limit {
		result = result[:filter.Limit]
	}

	return result
}

// RecordingFilter defines criteria for filtering recordings.
type RecordingFilter struct {
	Program    string
	SessionID  string
	Tags       []string
	StartAfter time.Time
	EndBefore  time.Time
	MinSteps   int
	Limit      int
}

// matches checks if a recording index matches the filter criteria.
func (f RecordingFilter) matches(idx RecordingIndex) bool {
	if f.Program != "" && !strings.Contains(idx.Program, f.Program) {
		return false
	}
	if f.SessionID != "" && idx.SessionID != f.SessionID {
		return false
	}
	if !f.StartAfter.IsZero() && idx.StartTime.Before(f.StartAfter) {
		return false
	}
	if !f.EndBefore.IsZero() && idx.EndTime.After(f.EndBefore) {
		return false
	}
	if f.MinSteps > 0 && idx.TotalSteps < f.MinSteps {
		return false
	}
	if len(f.Tags) > 0 {
		for _, filterTag := range f.Tags {
			found := false
			for _, tag := range idx.Tags {
				if tag == filterTag {
					found = true
					break
				}
			}
			if !found {
				return false
			}
		}
	}
	return true
}

// DeleteRecording removes a recording from storage.
func (hm *HistoryManager) DeleteRecording(id string) error {
	hm.mu.Lock()
	defer hm.mu.Unlock()

	var filePath string
	var indexPos int = -1
	for i, idx := range hm.index {
		if idx.ID == id {
			filePath = idx.FilePath
			indexPos = i
			break
		}
	}

	if filePath == "" {
		return fmt.Errorf("recording not found: %s", id)
	}

	// Delete file
	if err := os.Remove(filePath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete recording file: %w", err)
	}

	// Remove from index
	hm.index = append(hm.index[:indexPos], hm.index[indexPos+1:]...)

	// Remove from cache
	delete(hm.cache, id)

	return hm.saveIndex()
}

// GetRecordingStats returns aggregate statistics across all recordings.
func (hm *HistoryManager) GetRecordingStats() map[string]any {
	hm.mu.RLock()
	defer hm.mu.RUnlock()

	totalSteps := 0
	totalSize := int64(0)
	programs := make(map[string]int)

	for _, idx := range hm.index {
		totalSteps += idx.TotalSteps
		totalSize += idx.SizeBytes
		programs[idx.Program]++
	}

	return map[string]any{
		"total_recordings": len(hm.index),
		"total_steps":      totalSteps,
		"total_size_bytes": totalSize,
		"unique_programs":  len(programs),
		"programs":         programs,
	}
}

// SearchHistory searches recordings and frames for specific patterns.
func (hm *HistoryManager) SearchHistory(query HistoryQuery) []HistorySearchResult {
	hm.mu.RLock()
	defer hm.mu.RUnlock()

	var results []HistorySearchResult

	for _, idx := range hm.index {
		if query.RecordingFilter.Program != "" && !strings.Contains(idx.Program, query.RecordingFilter.Program) {
			continue
		}

		recording, err := hm.loadRecordingFromFile(idx.FilePath)
		if err != nil {
			continue
		}

		for stepNum, frame := range recording.Frames {
			if hm.frameMatchesQuery(frame, query, recording) {
				results = append(results, HistorySearchResult{
					RecordingID: idx.ID,
					StepNumber:  stepNum + 1,
					Location:    frame.Location,
					Timestamp:   frame.Timestamp,
					MatchType:   query.MatchType,
				})

				if query.Limit > 0 && len(results) >= query.Limit {
					return results
				}
			}
		}
	}

	return results
}

// frameMatchesQuery checks if a frame matches the search query.
func (hm *HistoryManager) frameMatchesQuery(frame ExecutionFrame, query HistoryQuery, recording *ExecutionRecording) bool {
	switch query.MatchType {
	case "variable_value":
		vars := frame.Variables
		if vars == nil {
			vars = frame.VariableDelta
		}
		if val, exists := vars[query.VariableName]; exists {
			valJSON, _ := json.Marshal(val.Value)
			targetJSON, _ := json.Marshal(query.TargetValue)
			return string(valJSON) == string(targetJSON)
		}
	case "variable_changed":
		vars := frame.Variables
		if vars == nil {
			vars = frame.VariableDelta
		}
		if val, exists := vars[query.VariableName]; exists {
			return val.IsChanged
		}
	case "location":
		return strings.Contains(frame.Location.Program, query.LocationPattern)
	case "checkpoint":
		stepNum := frame.StepNumber
		for name, step := range recording.Checkpoints {
			if step == stepNum && (query.CheckpointName == "" || strings.Contains(name, query.CheckpointName)) {
				return true
			}
		}
	}
	return false
}

// HistoryQuery defines a search query across recordings.
type HistoryQuery struct {
	RecordingFilter RecordingFilter
	MatchType       string // variable_value, variable_changed, location, checkpoint
	VariableName    string
	TargetValue     any
	LocationPattern string
	CheckpointName  string
	Limit           int
}

// HistorySearchResult represents a search match.
type HistorySearchResult struct {
	RecordingID string       `json:"recording_id"`
	StepNumber  int          `json:"step_number"`
	Location    CodeLocation `json:"location"`
	Timestamp   time.Time    `json:"timestamp"`
	MatchType   string       `json:"match_type"`
}

// CompareRecordings compares two recordings to find differences.
func (hm *HistoryManager) CompareRecordings(id1, id2 string) (*RecordingComparison, error) {
	rec1, err := hm.LoadRecording(id1)
	if err != nil {
		return nil, fmt.Errorf("failed to load recording %s: %w", id1, err)
	}

	rec2, err := hm.LoadRecording(id2)
	if err != nil {
		return nil, fmt.Errorf("failed to load recording %s: %w", id2, err)
	}

	comparison := &RecordingComparison{
		Recording1ID: id1,
		Recording2ID: id2,
		Differences:  make([]RecordingDiff, 0),
	}

	// Compare step counts
	if rec1.TotalSteps != rec2.TotalSteps {
		comparison.Differences = append(comparison.Differences, RecordingDiff{
			Type:        "step_count",
			Description: fmt.Sprintf("Different step counts: %d vs %d", rec1.TotalSteps, rec2.TotalSteps),
		})
	}

	// Compare execution paths
	maxSteps := rec1.TotalSteps
	if rec2.TotalSteps < maxSteps {
		maxSteps = rec2.TotalSteps
	}

	for i := 0; i < maxSteps; i++ {
		frame1 := rec1.Frames[i]
		frame2 := rec2.Frames[i]

		if frame1.Location.Program != frame2.Location.Program || frame1.Location.Line != frame2.Location.Line {
			comparison.Differences = append(comparison.Differences, RecordingDiff{
				Type:        "path_divergence",
				StepNumber:  i + 1,
				Description: fmt.Sprintf("Path diverged: %s:%d vs %s:%d", frame1.Location.Program, frame1.Location.Line, frame2.Location.Program, frame2.Location.Line),
			})
			break // Paths diverged, stop comparing
		}
	}

	comparison.StepsCompared = maxSteps
	comparison.PathsMatch = len(comparison.Differences) == 0 || comparison.Differences[0].Type != "path_divergence"

	return comparison, nil
}

// RecordingComparison holds the result of comparing two recordings.
type RecordingComparison struct {
	Recording1ID  string          `json:"recording1_id"`
	Recording2ID  string          `json:"recording2_id"`
	StepsCompared int             `json:"steps_compared"`
	PathsMatch    bool            `json:"paths_match"`
	Differences   []RecordingDiff `json:"differences"`
}

// RecordingDiff represents a single difference between recordings.
type RecordingDiff struct {
	Type        string `json:"type"` // step_count, path_divergence, variable_diff
	StepNumber  int    `json:"step_number,omitempty"`
	Description string `json:"description"`
}
