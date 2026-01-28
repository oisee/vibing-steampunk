package adt

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNewHistoryManager(t *testing.T) {
	tmpDir := t.TempDir()

	hm, err := NewHistoryManager(tmpDir)
	if err != nil {
		t.Fatalf("NewHistoryManager failed: %v", err)
	}

	if hm == nil {
		t.Fatal("NewHistoryManager returned nil")
	}

	if hm.storePath != tmpDir {
		t.Errorf("expected store path %s, got %s", tmpDir, hm.storePath)
	}

	// Verify directory was created
	if _, err := os.Stat(tmpDir); os.IsNotExist(err) {
		t.Error("store directory was not created")
	}
}

func TestSaveAndLoadRecording(t *testing.T) {
	tmpDir := t.TempDir()
	hm, _ := NewHistoryManager(tmpDir)

	// Create a recording
	recorder := NewExecutionRecorder("test-session", "ZTEST")
	vars := map[string]VariableValue{
		"LV_TEST": {Name: "LV_TEST", Type: "STRING", Value: "hello"},
	}
	recorder.RecordFrame(CodeLocation{Program: "ZTEST", Line: 10}, "step_over", vars)
	recorder.Complete()

	// Save it
	err := hm.SaveRecording(recorder)
	if err != nil {
		t.Fatalf("SaveRecording failed: %v", err)
	}

	// Load it back
	recording := recorder.GetRecording()
	loaded, err := hm.LoadRecording(recording.ID)
	if err != nil {
		t.Fatalf("LoadRecording failed: %v", err)
	}

	if loaded.SessionID != "test-session" {
		t.Errorf("expected session ID 'test-session', got '%s'", loaded.SessionID)
	}
	if loaded.Program != "ZTEST" {
		t.Errorf("expected program 'ZTEST', got '%s'", loaded.Program)
	}
	if loaded.TotalSteps != 1 {
		t.Errorf("expected 1 step, got %d", loaded.TotalSteps)
	}
}

func TestLoadRecordingFromCache(t *testing.T) {
	tmpDir := t.TempDir()
	hm, _ := NewHistoryManager(tmpDir)

	recorder := NewExecutionRecorder("test-session", "ZTEST")
	recorder.RecordFrame(CodeLocation{Program: "ZTEST", Line: 1}, "entry", nil)
	recorder.Complete()
	hm.SaveRecording(recorder)

	recording := recorder.GetRecording()

	// First load populates cache
	_, err := hm.LoadRecording(recording.ID)
	if err != nil {
		t.Fatalf("First LoadRecording failed: %v", err)
	}

	// Second load should come from cache
	loaded, err := hm.LoadRecording(recording.ID)
	if err != nil {
		t.Fatalf("Second LoadRecording failed: %v", err)
	}

	if loaded.ID != recording.ID {
		t.Errorf("expected ID %s, got %s", recording.ID, loaded.ID)
	}
}

func TestLoadRecordingNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	hm, _ := NewHistoryManager(tmpDir)

	_, err := hm.LoadRecording("non-existent-id")
	if err == nil {
		t.Error("expected error for non-existent recording")
	}
}

func TestListRecordings(t *testing.T) {
	tmpDir := t.TempDir()
	hm, _ := NewHistoryManager(tmpDir)

	// Create multiple recordings
	for i := 0; i < 5; i++ {
		recorder := NewExecutionRecorder("session", "ZTEST")
		recorder.RecordFrame(CodeLocation{Program: "ZTEST", Line: i + 1}, "step", nil)
		recorder.Complete()
		hm.SaveRecording(recorder)
	}

	// List all
	all := hm.ListRecordings(RecordingFilter{})
	if len(all) != 5 {
		t.Errorf("expected 5 recordings, got %d", len(all))
	}

	// List with limit
	limited := hm.ListRecordings(RecordingFilter{Limit: 3})
	if len(limited) != 3 {
		t.Errorf("expected 3 recordings with limit, got %d", len(limited))
	}
}

func TestListRecordingsWithFilter(t *testing.T) {
	tmpDir := t.TempDir()
	hm, _ := NewHistoryManager(tmpDir)

	// Create recordings with different programs
	programs := []string{"ZTEST_A", "ZTEST_B", "ZPROD_A", "ZPROD_B"}
	for _, prog := range programs {
		recorder := NewExecutionRecorder("session", prog)
		recorder.RecordFrame(CodeLocation{Program: prog, Line: 1}, "step", nil)
		recorder.Complete()
		hm.SaveRecording(recorder)
	}

	// Filter by program
	filtered := hm.ListRecordings(RecordingFilter{Program: "ZTEST"})
	if len(filtered) != 2 {
		t.Errorf("expected 2 recordings matching ZTEST, got %d", len(filtered))
	}

	filteredProd := hm.ListRecordings(RecordingFilter{Program: "ZPROD"})
	if len(filteredProd) != 2 {
		t.Errorf("expected 2 recordings matching ZPROD, got %d", len(filteredProd))
	}
}

func TestRecordingFilterMinSteps(t *testing.T) {
	tmpDir := t.TempDir()
	hm, _ := NewHistoryManager(tmpDir)

	// Create recordings with different step counts
	for steps := 1; steps <= 5; steps++ {
		recorder := NewExecutionRecorder("session", "ZTEST")
		for i := 0; i < steps; i++ {
			recorder.RecordFrame(CodeLocation{Program: "ZTEST", Line: i + 1}, "step", nil)
		}
		recorder.Complete()
		hm.SaveRecording(recorder)
	}

	// Filter by minimum steps
	filtered := hm.ListRecordings(RecordingFilter{MinSteps: 3})
	if len(filtered) != 3 {
		t.Errorf("expected 3 recordings with >=3 steps, got %d", len(filtered))
	}
}

func TestDeleteRecording(t *testing.T) {
	tmpDir := t.TempDir()
	hm, _ := NewHistoryManager(tmpDir)

	recorder := NewExecutionRecorder("session", "ZTEST")
	recorder.RecordFrame(CodeLocation{Program: "ZTEST", Line: 1}, "step", nil)
	recorder.Complete()
	hm.SaveRecording(recorder)

	recording := recorder.GetRecording()

	// Delete it
	err := hm.DeleteRecording(recording.ID)
	if err != nil {
		t.Fatalf("DeleteRecording failed: %v", err)
	}

	// Verify it's gone
	_, err = hm.LoadRecording(recording.ID)
	if err == nil {
		t.Error("expected error loading deleted recording")
	}

	// Verify not in list
	all := hm.ListRecordings(RecordingFilter{})
	if len(all) != 0 {
		t.Errorf("expected 0 recordings after delete, got %d", len(all))
	}
}

func TestDeleteRecordingNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	hm, _ := NewHistoryManager(tmpDir)

	err := hm.DeleteRecording("non-existent-id")
	if err == nil {
		t.Error("expected error deleting non-existent recording")
	}
}

func TestGetRecordingStats(t *testing.T) {
	tmpDir := t.TempDir()
	hm, _ := NewHistoryManager(tmpDir)

	// Create recordings
	programs := []string{"ZTEST_A", "ZTEST_A", "ZTEST_B"}
	for _, prog := range programs {
		recorder := NewExecutionRecorder("session", prog)
		for i := 0; i < 5; i++ {
			recorder.RecordFrame(CodeLocation{Program: prog, Line: i + 1}, "step", nil)
		}
		recorder.Complete()
		hm.SaveRecording(recorder)
	}

	stats := hm.GetRecordingStats()

	if stats["total_recordings"].(int) != 3 {
		t.Errorf("expected 3 total recordings, got %v", stats["total_recordings"])
	}

	if stats["total_steps"].(int) != 15 {
		t.Errorf("expected 15 total steps, got %v", stats["total_steps"])
	}

	if stats["unique_programs"].(int) != 2 {
		t.Errorf("expected 2 unique programs, got %v", stats["unique_programs"])
	}

	programs_map := stats["programs"].(map[string]int)
	if programs_map["ZTEST_A"] != 2 {
		t.Errorf("expected ZTEST_A count 2, got %d", programs_map["ZTEST_A"])
	}
}

func TestCompareRecordings(t *testing.T) {
	tmpDir := t.TempDir()
	hm, _ := NewHistoryManager(tmpDir)

	// Create two similar recordings
	recorder1 := NewExecutionRecorder("session1", "ZTEST")
	recorder1.RecordFrame(CodeLocation{Program: "ZTEST", Line: 10}, "step", nil)
	recorder1.RecordFrame(CodeLocation{Program: "ZTEST", Line: 20}, "step", nil)
	recorder1.RecordFrame(CodeLocation{Program: "ZTEST", Line: 30}, "step", nil)
	recorder1.Complete()
	hm.SaveRecording(recorder1)
	id1 := recorder1.GetRecording().ID

	recorder2 := NewExecutionRecorder("session2", "ZTEST")
	recorder2.RecordFrame(CodeLocation{Program: "ZTEST", Line: 10}, "step", nil)
	recorder2.RecordFrame(CodeLocation{Program: "ZTEST", Line: 20}, "step", nil)
	recorder2.RecordFrame(CodeLocation{Program: "ZTEST", Line: 30}, "step", nil)
	recorder2.Complete()
	hm.SaveRecording(recorder2)
	id2 := recorder2.GetRecording().ID

	comparison, err := hm.CompareRecordings(id1, id2)
	if err != nil {
		t.Fatalf("CompareRecordings failed: %v", err)
	}

	if !comparison.PathsMatch {
		t.Error("expected paths to match")
	}
	if comparison.StepsCompared != 3 {
		t.Errorf("expected 3 steps compared, got %d", comparison.StepsCompared)
	}
}

func TestCompareRecordingsDifferentPaths(t *testing.T) {
	tmpDir := t.TempDir()
	hm, _ := NewHistoryManager(tmpDir)

	// Create two recordings with different paths
	recorder1 := NewExecutionRecorder("session1", "ZTEST")
	recorder1.RecordFrame(CodeLocation{Program: "ZTEST", Line: 10}, "step", nil)
	recorder1.RecordFrame(CodeLocation{Program: "ZTEST", Line: 20}, "step", nil) // Diverges here
	recorder1.Complete()
	hm.SaveRecording(recorder1)
	id1 := recorder1.GetRecording().ID

	recorder2 := NewExecutionRecorder("session2", "ZTEST")
	recorder2.RecordFrame(CodeLocation{Program: "ZTEST", Line: 10}, "step", nil)
	recorder2.RecordFrame(CodeLocation{Program: "ZTEST", Line: 25}, "step", nil) // Different line
	recorder2.Complete()
	hm.SaveRecording(recorder2)
	id2 := recorder2.GetRecording().ID

	comparison, err := hm.CompareRecordings(id1, id2)
	if err != nil {
		t.Fatalf("CompareRecordings failed: %v", err)
	}

	if comparison.PathsMatch {
		t.Error("expected paths to NOT match")
	}
}

func TestCompareRecordingsDifferentStepCount(t *testing.T) {
	tmpDir := t.TempDir()
	hm, _ := NewHistoryManager(tmpDir)

	recorder1 := NewExecutionRecorder("session1", "ZTEST")
	recorder1.RecordFrame(CodeLocation{Program: "ZTEST", Line: 10}, "step", nil)
	recorder1.RecordFrame(CodeLocation{Program: "ZTEST", Line: 20}, "step", nil)
	recorder1.Complete()
	hm.SaveRecording(recorder1)
	id1 := recorder1.GetRecording().ID

	recorder2 := NewExecutionRecorder("session2", "ZTEST")
	recorder2.RecordFrame(CodeLocation{Program: "ZTEST", Line: 10}, "step", nil)
	recorder2.Complete()
	hm.SaveRecording(recorder2)
	id2 := recorder2.GetRecording().ID

	comparison, err := hm.CompareRecordings(id1, id2)
	if err != nil {
		t.Fatalf("CompareRecordings failed: %v", err)
	}

	// Should have a step_count difference
	hasDiff := false
	for _, diff := range comparison.Differences {
		if diff.Type == "step_count" {
			hasDiff = true
			break
		}
	}
	if !hasDiff {
		t.Error("expected step_count difference")
	}
}

func TestRecordingFilterMatches(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name     string
		filter   RecordingFilter
		idx      RecordingIndex
		expected bool
	}{
		{
			name:     "empty filter matches all",
			filter:   RecordingFilter{},
			idx:      RecordingIndex{Program: "ZTEST", SessionID: "s1", TotalSteps: 5},
			expected: true,
		},
		{
			name:     "program filter matches",
			filter:   RecordingFilter{Program: "ZTEST"},
			idx:      RecordingIndex{Program: "ZTEST_001"},
			expected: true,
		},
		{
			name:     "program filter no match",
			filter:   RecordingFilter{Program: "ZPROD"},
			idx:      RecordingIndex{Program: "ZTEST"},
			expected: false,
		},
		{
			name:     "session ID filter matches",
			filter:   RecordingFilter{SessionID: "session-123"},
			idx:      RecordingIndex{SessionID: "session-123"},
			expected: true,
		},
		{
			name:     "session ID filter no match",
			filter:   RecordingFilter{SessionID: "session-123"},
			idx:      RecordingIndex{SessionID: "session-456"},
			expected: false,
		},
		{
			name:     "min steps filter matches",
			filter:   RecordingFilter{MinSteps: 10},
			idx:      RecordingIndex{TotalSteps: 15},
			expected: true,
		},
		{
			name:     "min steps filter no match",
			filter:   RecordingFilter{MinSteps: 10},
			idx:      RecordingIndex{TotalSteps: 5},
			expected: false,
		},
		{
			name:     "start after filter matches",
			filter:   RecordingFilter{StartAfter: now.Add(-1 * time.Hour)},
			idx:      RecordingIndex{StartTime: now},
			expected: true,
		},
		{
			name:     "start after filter no match",
			filter:   RecordingFilter{StartAfter: now},
			idx:      RecordingIndex{StartTime: now.Add(-1 * time.Hour)},
			expected: false,
		},
		{
			name:     "tags filter matches",
			filter:   RecordingFilter{Tags: []string{"production"}},
			idx:      RecordingIndex{Tags: []string{"production", "error"}},
			expected: true,
		},
		{
			name:     "tags filter no match",
			filter:   RecordingFilter{Tags: []string{"production"}},
			idx:      RecordingIndex{Tags: []string{"development"}},
			expected: false,
		},
		{
			name:     "multiple filters all match",
			filter:   RecordingFilter{Program: "ZTEST", MinSteps: 5},
			idx:      RecordingIndex{Program: "ZTEST_001", TotalSteps: 10},
			expected: true,
		},
		{
			name:     "multiple filters one fails",
			filter:   RecordingFilter{Program: "ZTEST", MinSteps: 20},
			idx:      RecordingIndex{Program: "ZTEST_001", TotalSteps: 10},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.filter.matches(tt.idx)
			if result != tt.expected {
				t.Errorf("filter.matches() = %v, expected %v", result, tt.expected)
			}
		})
	}
}

func TestRebuildIndex(t *testing.T) {
	tmpDir := t.TempDir()
	hm, _ := NewHistoryManager(tmpDir)

	// Save some recordings
	for i := 0; i < 3; i++ {
		recorder := NewExecutionRecorder("session", "ZTEST")
		recorder.RecordFrame(CodeLocation{Program: "ZTEST", Line: i + 1}, "step", nil)
		recorder.Complete()
		hm.SaveRecording(recorder)
	}

	// Delete index file to simulate corruption
	indexPath := filepath.Join(tmpDir, "index.json")
	os.Remove(indexPath)

	// Create new manager - should rebuild index
	hm2, err := NewHistoryManager(tmpDir)
	if err != nil {
		t.Fatalf("NewHistoryManager failed after index deletion: %v", err)
	}

	// Verify index was rebuilt
	all := hm2.ListRecordings(RecordingFilter{})
	if len(all) != 3 {
		t.Errorf("expected 3 recordings after rebuild, got %d", len(all))
	}
}

func TestCacheEviction(t *testing.T) {
	tmpDir := t.TempDir()
	hm, _ := NewHistoryManager(tmpDir)
	hm.cacheSize = 3 // Small cache for testing

	// Create more recordings than cache size
	for i := 0; i < 5; i++ {
		recorder := NewExecutionRecorder("session", "ZTEST")
		recorder.RecordFrame(CodeLocation{Program: "ZTEST", Line: i + 1}, "step", nil)
		recorder.Complete()
		hm.SaveRecording(recorder)
	}

	// Cache should only have cacheSize entries
	if len(hm.cache) > hm.cacheSize {
		t.Errorf("cache size %d exceeds limit %d", len(hm.cache), hm.cacheSize)
	}
}

func TestSearchHistoryVariableValue(t *testing.T) {
	tmpDir := t.TempDir()
	hm, _ := NewHistoryManager(tmpDir)

	recorder := NewExecutionRecorder("session", "ZTEST")
	vars1 := map[string]VariableValue{
		"LV_STATUS": {Name: "LV_STATUS", Type: "STRING", Value: "INIT"},
	}
	recorder.RecordFrame(CodeLocation{Program: "ZTEST", Line: 1}, "step", vars1)

	vars2 := map[string]VariableValue{
		"LV_STATUS": {Name: "LV_STATUS", Type: "STRING", Value: "DONE"},
	}
	recorder.RecordFrame(CodeLocation{Program: "ZTEST", Line: 2}, "step", vars2)
	recorder.Complete()
	hm.SaveRecording(recorder)

	// Search for variable value
	query := HistoryQuery{
		MatchType:    "variable_value",
		VariableName: "LV_STATUS",
		TargetValue:  "DONE",
	}

	results := hm.SearchHistory(query)
	if len(results) == 0 {
		t.Error("expected to find variable value match")
	}
}

func TestSearchHistoryLocation(t *testing.T) {
	tmpDir := t.TempDir()
	hm, _ := NewHistoryManager(tmpDir)

	recorder := NewExecutionRecorder("session", "ZTEST")
	recorder.RecordFrame(CodeLocation{Program: "ZTEST_MAIN", Line: 1}, "step", nil)
	recorder.RecordFrame(CodeLocation{Program: "ZTEST_HELPER", Line: 1}, "step", nil)
	recorder.Complete()
	hm.SaveRecording(recorder)

	query := HistoryQuery{
		MatchType:       "location",
		LocationPattern: "HELPER",
	}

	results := hm.SearchHistory(query)
	if len(results) != 1 {
		t.Errorf("expected 1 location match, got %d", len(results))
	}
}

func TestSearchHistoryWithLimit(t *testing.T) {
	tmpDir := t.TempDir()
	hm, _ := NewHistoryManager(tmpDir)

	recorder := NewExecutionRecorder("session", "ZTEST")
	for i := 0; i < 10; i++ {
		recorder.RecordFrame(CodeLocation{Program: "ZTEST", Line: i + 1}, "step", nil)
	}
	recorder.Complete()
	hm.SaveRecording(recorder)

	query := HistoryQuery{
		MatchType:       "location",
		LocationPattern: "ZTEST",
		Limit:           5,
	}

	results := hm.SearchHistory(query)
	if len(results) != 5 {
		t.Errorf("expected 5 results with limit, got %d", len(results))
	}
}
