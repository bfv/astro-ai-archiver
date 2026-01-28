package tools

import (
	"testing"
	"time"
)

// TestBeginScanEndScan tests the scan state management functions
func TestBeginScanEndScan(t *testing.T) {
	// Reset state before test
	scanMutex.Lock()
	scanState.Scanning = false
	scanState.LastScanTime = time.Time{}
	scanMutex.Unlock()

	// Test: BeginScan should succeed when no scan is running
	if !BeginScan() {
		t.Fatal("BeginScan() should return true when no scan is in progress")
	}

	// Test: Scanning should be true after BeginScan
	state := GetScanState()
	if !state.Scanning {
		t.Error("Expected Scanning to be true after BeginScan()")
	}

	// Test: BeginScan should fail when a scan is already running
	if BeginScan() {
		t.Error("BeginScan() should return false when a scan is already in progress")
	}

	// Test: EndScan should mark scan as complete
	EndScan()
	state = GetScanState()
	if state.Scanning {
		t.Error("Expected Scanning to be false after EndScan()")
	}

	// Test: LastScanTime should be set after EndScan
	if state.LastScanTime.IsZero() {
		t.Error("Expected LastScanTime to be set after EndScan()")
	}

	// Test: BeginScan should succeed again after EndScan
	if !BeginScan() {
		t.Error("BeginScan() should return true after previous scan completed")
	}
	EndScan()
}

// TestConcurrentScans tests that concurrent scan attempts are properly blocked
func TestConcurrentScans(t *testing.T) {
	// Reset state before test
	scanMutex.Lock()
	scanState.Scanning = false
	scanState.LastScanTime = time.Time{}
	scanMutex.Unlock()

	// Start first scan
	if !BeginScan() {
		t.Fatal("First BeginScan() should succeed")
	}

	// Try to start concurrent scans
	for i := 0; i < 5; i++ {
		if BeginScan() {
			t.Errorf("Concurrent BeginScan() attempt %d should have been blocked", i+1)
		}
	}

	// End scan
	EndScan()

	// Verify state is back to not scanning
	state := GetScanState()
	if state.Scanning {
		t.Error("Expected Scanning to be false after EndScan()")
	}
}

// TestGetScanState tests the GetScanState function
func TestGetScanState(t *testing.T) {
	// Reset state before test
	scanMutex.Lock()
	scanState.Scanning = false
	testTime := time.Now()
	scanState.LastScanTime = testTime
	scanMutex.Unlock()

	// Get state
	state := GetScanState()

	// Verify it returns a copy, not a reference
	if state.Scanning {
		t.Error("Expected Scanning to be false")
	}

	if state.LastScanTime != testTime {
		t.Errorf("Expected LastScanTime to be %v, got %v", testTime, state.LastScanTime)
	}

	// Modify returned state should not affect internal state
	state.Scanning = true
	state.LastScanTime = time.Time{}

	// Check internal state is unchanged
	actualState := GetScanState()
	if actualState.Scanning {
		t.Error("Modifying returned state should not affect internal state")
	}
	if actualState.LastScanTime != testTime {
		t.Error("Modifying returned state should not affect internal LastScanTime")
	}
}
