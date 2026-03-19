package tools

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRequestTracker_ShutdownRequest(t *testing.T) {
	tracker := NewRequestTracker()

	// Create shutdown request
	req := tracker.CreateShutdownRequest("teammate1")
	if req.RequestID == "" {
		t.Error("RequestID should not be empty")
	}
	if req.Target != "teammate1" {
		t.Errorf("Target = %s, want teammate1", req.Target)
	}
	if req.Status != StatusPending {
		t.Errorf("Status = %s, want pending", req.Status)
	}

	// Get shutdown request
	got := tracker.GetShutdownRequest(req.RequestID)
	if got == nil {
		t.Error("GetShutdownRequest returned nil")
	}
	if got.RequestID != req.RequestID {
		t.Errorf("GetShutdownRequest RequestID = %s, want %s", got.RequestID, req.RequestID)
	}

	// Update status
	tracker.UpdateShutdownStatus(req.RequestID, StatusApproved)
	got = tracker.GetShutdownRequest(req.RequestID)
	if got.Status != StatusApproved {
		t.Errorf("Status after update = %s, want approved", got.Status)
	}
}

func TestRequestTracker_PlanRequest(t *testing.T) {
	tracker := NewRequestTracker()

	// Create plan request
	req := tracker.CreatePlanRequest("teammate1", "Implement feature X")
	if req.RequestID == "" {
		t.Error("RequestID should not be empty")
	}
	if req.From != "teammate1" {
		t.Errorf("From = %s, want teammate1", req.From)
	}
	if req.Plan != "Implement feature X" {
		t.Errorf("Plan = %s, want 'Implement feature X'", req.Plan)
	}
	if req.Status != StatusPending {
		t.Errorf("Status = %s, want pending", req.Status)
	}

	// Get plan request
	got := tracker.GetPlanRequest(req.RequestID)
	if got == nil {
		t.Error("GetPlanRequest returned nil")
	}

	// Update status
	tracker.UpdatePlanStatus(req.RequestID, StatusRejected)
	got = tracker.GetPlanRequest(req.RequestID)
	if got.Status != StatusRejected {
		t.Errorf("Status after update = %s, want rejected", got.Status)
	}
}

func TestRequestTracker_ListPendingPlanRequests(t *testing.T) {
	tracker := NewRequestTracker()

	// Create multiple plan requests
	req1 := tracker.CreatePlanRequest("teammate1", "Plan 1")
	req2 := tracker.CreatePlanRequest("teammate2", "Plan 2")

	// Approve one
	tracker.UpdatePlanStatus(req1.RequestID, StatusApproved)

	// List pending
	pending := tracker.ListPendingPlanRequests()
	if len(pending) != 1 {
		t.Errorf("ListPendingPlanRequests returned %d items, want 1", len(pending))
	}
	if pending[0].RequestID != req2.RequestID {
		t.Errorf("Pending request ID = %s, want %s", pending[0].RequestID, req2.RequestID)
	}
}

func TestShutdownRequestHandler_Execute(t *testing.T) {
	// Create temp inbox dir
	tmpDir := t.TempDir()
	inboxDir := filepath.Join(tmpDir, "inbox")
	os.MkdirAll(inboxDir, 0755)

	bus := NewMessageBus(inboxDir)
	tracker := NewRequestTracker()
	handler := NewShutdownRequestHandler(tracker, bus)

	// Execute shutdown request
	result, err := handler.Execute(map[string]any{"teammate": "bob"})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if result == "" {
		t.Error("Result should not be empty")
	}

	// Check that request was tracked
	reqs := tracker.ListPendingPlanRequests()
	// There should be no plan requests (we created a shutdown request)
	if len(reqs) != 0 {
		t.Errorf("Expected 0 plan requests, got %d", len(reqs))
	}
}

func TestShutdownResponseHandler_Execute(t *testing.T) {
	// Create temp inbox dir
	tmpDir := t.TempDir()
	inboxDir := filepath.Join(tmpDir, "inbox")
	os.MkdirAll(inboxDir, 0755)

	bus := NewMessageBus(inboxDir)
	tracker := NewRequestTracker()

	// Create a shutdown request first
	req := tracker.CreateShutdownRequest("bob")

	handler := NewShutdownResponseHandler(tracker, bus, "bob")

	// Execute response with approve
	result, err := handler.Execute(map[string]any{
		"request_id": req.RequestID,
		"approve":    true,
		"reason":     "Work complete",
	})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if result != "Shutdown approved" {
		t.Errorf("Result = %s, want 'Shutdown approved'", result)
	}

	// Check status was updated
	got := tracker.GetShutdownRequest(req.RequestID)
	if got.Status != StatusApproved {
		t.Errorf("Status = %s, want approved", got.Status)
	}
}

func TestPlanApprovalSubmitHandler_Execute(t *testing.T) {
	// Create temp inbox dir
	tmpDir := t.TempDir()
	inboxDir := filepath.Join(tmpDir, "inbox")
	os.MkdirAll(inboxDir, 0755)

	bus := NewMessageBus(inboxDir)
	tracker := NewRequestTracker()
	handler := NewPlanApprovalSubmitHandler(tracker, bus, "bob")

	// Execute plan submission
	result, err := handler.Execute(map[string]any{"plan": "Implement feature X"})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if result == "" {
		t.Error("Result should not be empty")
	}

	// Check that request was created
	pending := tracker.ListPendingPlanRequests()
	if len(pending) != 1 {
		t.Errorf("Expected 1 pending plan, got %d", len(pending))
	}
}

func TestPlanApprovalReviewHandler_Execute(t *testing.T) {
	// Create temp inbox dir
	tmpDir := t.TempDir()
	inboxDir := filepath.Join(tmpDir, "inbox")
	os.MkdirAll(inboxDir, 0755)

	bus := NewMessageBus(inboxDir)
	tracker := NewRequestTracker()

	// Create a plan request first
	req := tracker.CreatePlanRequest("bob", "Implement feature X")

	handler := NewPlanApprovalReviewHandler(tracker, bus)

	// Execute review with approve
	result, err := handler.Execute(map[string]any{
		"request_id": req.RequestID,
		"approve":    true,
		"feedback":   "Looks good",
	})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if result == "" {
		t.Error("Result should not be empty")
	}

	// Check status was updated
	got := tracker.GetPlanRequest(req.RequestID)
	if got.Status != StatusApproved {
		t.Errorf("Status = %s, want approved", got.Status)
	}
}

func TestPlanApprovalReviewHandler_UnknownRequest(t *testing.T) {
	bus := NewMessageBus("")
	tracker := NewRequestTracker()
	handler := NewPlanApprovalReviewHandler(tracker, bus)

	_, err := handler.Execute(map[string]any{
		"request_id": "nonexistent",
		"approve":    true,
	})
	if err == nil {
		t.Error("Expected error for unknown request_id")
	}
}

func TestCheckShutdownStatusHandler_Execute(t *testing.T) {
	tracker := NewRequestTracker()
	handler := NewCheckShutdownStatusHandler(tracker)

	// Create a shutdown request
	req := tracker.CreateShutdownRequest("bob")

	// Check status
	result, err := handler.Execute(map[string]any{"request_id": req.RequestID})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if result == "" {
		t.Error("Result should not be empty")
	}
}

func TestListPendingPlansHandler_Execute(t *testing.T) {
	tracker := NewRequestTracker()
	handler := NewListPendingPlansHandler(tracker)

	// No pending plans
	result, err := handler.Execute(map[string]any{})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if result != "No pending plan requests." {
		t.Errorf("Result = %s, want 'No pending plan requests.'", result)
	}

	// Add a plan
	tracker.CreatePlanRequest("bob", "Plan 1")
	result, err = handler.Execute(map[string]any{})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if result == "No pending plan requests." {
		t.Error("Should have pending plans")
	}
}

func TestGenerateRequestID(t *testing.T) {
	id1 := generateRequestID()
	id2 := generateRequestID()

	if id1 == "" {
		t.Error("Request ID should not be empty")
	}
	if len(id1) != 8 {
		t.Errorf("Request ID length = %d, want 8", len(id1))
	}
	if id1 == id2 {
		t.Error("Request IDs should be unique")
	}
}