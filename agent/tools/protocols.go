// Package tools provides team protocol implementations.
// Shutdown protocol and plan approval protocol, both using the same
// request_id correlation pattern.
//
// Shutdown FSM: pending -> approved | rejected
//
//	Lead                              Teammate
//	+---------------------+          +---------------------+
//	| shutdown_request     |          |                     |
//	| {                    | -------> | receives request    |
//	|   request_id: abc    |          | decides: approve?   |
//	| }                    |          |                     |
//	+---------------------+          +---------------------+
//	                                     |
//	+---------------------+          +-------v-------------+
//	| shutdown_response    | <------- | shutdown_response   |
//	| {                    |          | {                   |
//	|   request_id: abc    |          |   request_id: abc   |
//	|   approve: true      |          |   approve: true     |
//	| }                    |          | }                   |
//	+---------------------+          +---------------------+
//	        |
//	        v
//	status -> "shutdown", thread stops
//
// Plan approval FSM: pending -> approved | rejected
//
//	Teammate                          Lead
//	+---------------------+          +---------------------+
//	| plan_approval        |          |                     |
//	| submit: {plan:"..."}| -------> | reviews plan text   |
//	+---------------------+          | approve/reject?     |
//	                                 +---------------------+
//	                                     |
//	+---------------------+          +-------v-------------+
//	| plan_approval_resp   | <------- | plan_approval       |
//	| {approve: true}      |          | review: {req_id,    |
//	+---------------------+          |   approve: true}     |
//	                                 +---------------------+
//
// Key insight: "Same request_id correlation pattern, two domains."
package tools

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

// RequestStatus represents the status of a protocol request.
type RequestStatus string

const (
	StatusPending   RequestStatus = "pending"
	StatusApproved  RequestStatus = "approved"
	StatusRejected  RequestStatus = "rejected"
)

// ShutdownRequest represents a shutdown protocol request.
type ShutdownRequest struct {
	RequestID string        `json:"request_id"`
	Target    string        `json:"target"`
	Status    RequestStatus `json:"status"`
	CreatedAt float64       `json:"created_at"`
}

// PlanRequest represents a plan approval request.
type PlanRequest struct {
	RequestID string        `json:"request_id"`
	From      string        `json:"from"`
	Plan      string        `json:"plan"`
	Status    RequestStatus `json:"status"`
	CreatedAt float64       `json:"created_at"`
}

// RequestTracker tracks protocol requests by request_id.
type RequestTracker struct {
	shutdownRequests map[string]*ShutdownRequest
	planRequests     map[string]*PlanRequest
	mu               sync.RWMutex
}

// NewRequestTracker creates a new request tracker.
func NewRequestTracker() *RequestTracker {
	return &RequestTracker{
		shutdownRequests: make(map[string]*ShutdownRequest),
		planRequests:     make(map[string]*PlanRequest),
	}
}

// CreateShutdownRequest creates a new shutdown request.
func (t *RequestTracker) CreateShutdownRequest(target string) *ShutdownRequest {
	t.mu.Lock()
	defer t.mu.Unlock()

	req := &ShutdownRequest{
		RequestID: generateRequestID(),
		Target:    target,
		Status:    StatusPending,
		CreatedAt: float64(time.Now().UnixNano()) / 1e9,
	}
	t.shutdownRequests[req.RequestID] = req
	return req
}

// GetShutdownRequest retrieves a shutdown request by ID.
func (t *RequestTracker) GetShutdownRequest(requestID string) *ShutdownRequest {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.shutdownRequests[requestID]
}

// UpdateShutdownStatus updates the status of a shutdown request.
func (t *RequestTracker) UpdateShutdownStatus(requestID string, status RequestStatus) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if req, ok := t.shutdownRequests[requestID]; ok {
		req.Status = status
	}
}

// CreatePlanRequest creates a new plan approval request.
func (t *RequestTracker) CreatePlanRequest(from, plan string) *PlanRequest {
	t.mu.Lock()
	defer t.mu.Unlock()

	req := &PlanRequest{
		RequestID: generateRequestID(),
		From:      from,
		Plan:      plan,
		Status:    StatusPending,
		CreatedAt: float64(time.Now().UnixNano()) / 1e9,
	}
	t.planRequests[req.RequestID] = req
	return req
}

// GetPlanRequest retrieves a plan request by ID.
func (t *RequestTracker) GetPlanRequest(requestID string) *PlanRequest {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.planRequests[requestID]
}

// UpdatePlanStatus updates the status of a plan request.
func (t *RequestTracker) UpdatePlanStatus(requestID string, status RequestStatus) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if req, ok := t.planRequests[requestID]; ok {
		req.Status = status
	}
}

// ListPendingPlanRequests lists all pending plan requests.
func (t *RequestTracker) ListPendingPlanRequests() []*PlanRequest {
	t.mu.RLock()
	defer t.mu.RUnlock()

	var pending []*PlanRequest
	for _, req := range t.planRequests {
		if req.Status == StatusPending {
			pending = append(pending, req)
		}
	}
	return pending
}

// generateRequestID generates a short request ID.
func generateRequestID() string {
	b := make([]byte, 4)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// -- Protocol Message Types --

// ShutdownRequestMessage represents a shutdown request message.
type ShutdownRequestMessage struct {
	Type      string  `json:"type"`
	From      string  `json:"from"`
	RequestID string  `json:"request_id"`
	Timestamp float64 `json:"timestamp"`
}

// ShutdownResponseMessage represents a shutdown response message.
type ShutdownResponseMessage struct {
	Type      string  `json:"type"`
	From      string  `json:"from"`
	RequestID string  `json:"request_id"`
	Approve   bool    `json:"approve"`
	Reason    string  `json:"reason,omitempty"`
	Timestamp float64 `json:"timestamp"`
}

// PlanApprovalRequestMessage represents a plan approval request from teammate.
type PlanApprovalRequestMessage struct {
	Type      string  `json:"type"`
	From      string  `json:"from"`
	RequestID string  `json:"request_id"`
	Plan      string  `json:"plan"`
	Timestamp float64 `json:"timestamp"`
}

// PlanApprovalResponseMessage represents a plan approval response from lead.
type PlanApprovalResponseMessage struct {
	Type      string  `json:"type"`
	From      string  `json:"from"`
	RequestID string  `json:"request_id"`
	Approve   bool    `json:"approve"`
	Feedback  string  `json:"feedback,omitempty"`
	Timestamp float64 `json:"timestamp"`
}

// -- Tool Definitions --

// ShutdownRequestDefinition returns the tool definition for shutdown_request (lead -> teammate).
func ShutdownRequestDefinition() Definition {
	return Definition{
		Name:        "shutdown_request",
		Description: "Request a teammate to shut down gracefully. Returns a request_id for tracking.",
		InputSchema: InputSchema{
			Type: "object",
			Properties: map[string]Property{
				"teammate": {Type: "string", Description: "Name of the teammate to shut down"},
			},
			Required: []string{"teammate"},
		},
	}
}

// ShutdownResponseDefinition returns the tool definition for shutdown_response (teammate -> lead).
func ShutdownResponseDefinition() Definition {
	return Definition{
		Name:        "shutdown_response",
		Description: "Respond to a shutdown request. Approve to shut down, reject to keep working.",
		InputSchema: InputSchema{
			Type: "object",
			Properties: map[string]Property{
				"request_id": {Type: "string", Description: "The request ID from the shutdown request"},
				"approve":    {Type: "boolean", Description: "Whether to approve the shutdown"},
				"reason":     {Type: "string", Description: "Optional reason for the response"},
			},
			Required: []string{"request_id", "approve"},
		},
	}
}

// PlanApprovalSubmitDefinition returns the tool definition for submitting a plan (teammate -> lead).
func PlanApprovalSubmitDefinition() Definition {
	return Definition{
		Name:        "plan_approval_submit",
		Description: "Submit a plan for lead approval before major work.",
		InputSchema: InputSchema{
			Type: "object",
			Properties: map[string]Property{
				"plan": {Type: "string", Description: "The plan text to submit for approval"},
			},
			Required: []string{"plan"},
		},
	}
}

// PlanApprovalReviewDefinition returns the tool definition for reviewing a plan (lead -> teammate).
func PlanApprovalReviewDefinition() Definition {
	return Definition{
		Name:        "plan_approval_review",
		Description: "Approve or reject a teammate's plan. Provide request_id + approve + optional feedback.",
		InputSchema: InputSchema{
			Type: "object",
			Properties: map[string]Property{
				"request_id": {Type: "string", Description: "The request ID of the plan to review"},
				"approve":    {Type: "boolean", Description: "Whether to approve the plan"},
				"feedback":   {Type: "string", Description: "Optional feedback for the teammate"},
			},
			Required: []string{"request_id", "approve"},
		},
	}
}

// CheckShutdownStatusDefinition returns the tool definition for checking shutdown status.
func CheckShutdownStatusDefinition() Definition {
	return Definition{
		Name:        "check_shutdown_status",
		Description: "Check the status of a shutdown request by request_id.",
		InputSchema: InputSchema{
			Type: "object",
			Properties: map[string]Property{
				"request_id": {Type: "string", Description: "The request ID to check"},
			},
			Required: []string{"request_id"},
		},
	}
}

// ListPendingPlansDefinition returns the tool definition for listing pending plan requests.
func ListPendingPlansDefinition() Definition {
	return Definition{
		Name:        "list_pending_plans",
		Description: "List all pending plan approval requests from teammates.",
		InputSchema: InputSchema{Type: "object"},
	}
}

// -- Tool Handlers --

// ShutdownRequestHandler handles shutdown_request tool calls (lead -> teammate).
type ShutdownRequestHandler struct {
	tracker *RequestTracker
	bus     *MessageBus
}

// NewShutdownRequestHandler creates a new shutdown_request handler.
func NewShutdownRequestHandler(tracker *RequestTracker, bus *MessageBus) *ShutdownRequestHandler {
	return &ShutdownRequestHandler{tracker: tracker, bus: bus}
}

// Execute implements Handler.
func (h *ShutdownRequestHandler) Execute(input map[string]any) (string, error) {
	teammate, _ := input["teammate"].(string)
	if teammate == "" {
		return "", fmt.Errorf("teammate is required")
	}

	req := h.tracker.CreateShutdownRequest(teammate)

	// Send shutdown request message to teammate
	msg := map[string]any{
		"type":       "shutdown_request",
		"from":       "lead",
		"request_id": req.RequestID,
		"timestamp":  req.CreatedAt,
	}
	msgJSON, _ := json.Marshal(msg)
	h.bus.Send("lead", teammate, string(msgJSON), "shutdown_request")

	return fmt.Sprintf("Shutdown request %s sent to '%s' (status: pending)", req.RequestID, teammate), nil
}

// ShutdownResponseHandler handles shutdown_response tool calls (teammate -> lead).
type ShutdownResponseHandler struct {
	tracker *RequestTracker
	bus     *MessageBus
	sender  string
}

// NewShutdownResponseHandler creates a new shutdown_response handler.
func NewShutdownResponseHandler(tracker *RequestTracker, bus *MessageBus, sender string) *ShutdownResponseHandler {
	return &ShutdownResponseHandler{tracker: tracker, bus: bus, sender: sender}
}

// Execute implements Handler.
func (h *ShutdownResponseHandler) Execute(input map[string]any) (string, error) {
	requestID, _ := input["request_id"].(string)
	approve, _ := input["approve"].(bool)
	reason, _ := input["reason"].(string)

	if requestID == "" {
		return "", fmt.Errorf("request_id is required")
	}

	status := StatusApproved
	if !approve {
		status = StatusRejected
	}
	h.tracker.UpdateShutdownStatus(requestID, status)

	// Send response to lead
	msg := map[string]any{
		"type":       "shutdown_response",
		"from":       h.sender,
		"request_id": requestID,
		"approve":    approve,
		"reason":     reason,
		"timestamp":  float64(time.Now().UnixNano()) / 1e9,
	}
	msgJSON, _ := json.Marshal(msg)
	h.bus.Send(h.sender, "lead", string(msgJSON), "shutdown_response")

	result := "rejected"
	if approve {
		result = "approved"
	}
	return fmt.Sprintf("Shutdown %s", result), nil
}

// PlanApprovalSubmitHandler handles plan_approval_submit tool calls (teammate -> lead).
type PlanApprovalSubmitHandler struct {
	tracker *RequestTracker
	bus     *MessageBus
	sender  string
}

// NewPlanApprovalSubmitHandler creates a new plan_approval_submit handler.
func NewPlanApprovalSubmitHandler(tracker *RequestTracker, bus *MessageBus, sender string) *PlanApprovalSubmitHandler {
	return &PlanApprovalSubmitHandler{tracker: tracker, bus: bus, sender: sender}
}

// Execute implements Handler.
func (h *PlanApprovalSubmitHandler) Execute(input map[string]any) (string, error) {
	plan, _ := input["plan"].(string)
	if plan == "" {
		return "", fmt.Errorf("plan is required")
	}

	req := h.tracker.CreatePlanRequest(h.sender, plan)

	// Send plan approval request to lead
	msg := map[string]any{
		"type":       "plan_approval_request",
		"from":       h.sender,
		"request_id": req.RequestID,
		"plan":       plan,
		"timestamp":  req.CreatedAt,
	}
	msgJSON, _ := json.Marshal(msg)
	h.bus.Send(h.sender, "lead", string(msgJSON), "plan_approval_request")

	return fmt.Sprintf("Plan submitted (request_id=%s). Waiting for lead approval.", req.RequestID), nil
}

// PlanApprovalReviewHandler handles plan_approval_review tool calls (lead -> teammate).
type PlanApprovalReviewHandler struct {
	tracker *RequestTracker
	bus     *MessageBus
}

// NewPlanApprovalReviewHandler creates a new plan_approval_review handler.
func NewPlanApprovalReviewHandler(tracker *RequestTracker, bus *MessageBus) *PlanApprovalReviewHandler {
	return &PlanApprovalReviewHandler{tracker: tracker, bus: bus}
}

// Execute implements Handler.
func (h *PlanApprovalReviewHandler) Execute(input map[string]any) (string, error) {
	requestID, _ := input["request_id"].(string)
	approve, _ := input["approve"].(bool)
	feedback, _ := input["feedback"].(string)

	if requestID == "" {
		return "", fmt.Errorf("request_id is required")
	}

	req := h.tracker.GetPlanRequest(requestID)
	if req == nil {
		return "", fmt.Errorf("unknown plan request_id '%s'", requestID)
	}

	status := StatusApproved
	if !approve {
		status = StatusRejected
	}
	h.tracker.UpdatePlanStatus(requestID, status)

	// Send response to teammate
	msg := map[string]any{
		"type":       "plan_approval_response",
		"from":       "lead",
		"request_id": requestID,
		"approve":    approve,
		"feedback":   feedback,
		"timestamp":  float64(time.Now().UnixNano()) / 1e9,
	}
	msgJSON, _ := json.Marshal(msg)
	h.bus.Send("lead", req.From, string(msgJSON), "plan_approval_response")

	return fmt.Sprintf("Plan %s for '%s'", status, req.From), nil
}

// CheckShutdownStatusHandler handles check_shutdown_status tool calls.
type CheckShutdownStatusHandler struct {
	tracker *RequestTracker
}

// NewCheckShutdownStatusHandler creates a new check_shutdown_status handler.
func NewCheckShutdownStatusHandler(tracker *RequestTracker) *CheckShutdownStatusHandler {
	return &CheckShutdownStatusHandler{tracker: tracker}
}

// Execute implements Handler.
func (h *CheckShutdownStatusHandler) Execute(input map[string]any) (string, error) {
	requestID, _ := input["request_id"].(string)
	if requestID == "" {
		return "", fmt.Errorf("request_id is required")
	}

	req := h.tracker.GetShutdownRequest(requestID)
	if req == nil {
		return fmt.Sprintf(`{"error": "not found"}`), nil
	}

	data, _ := json.MarshalIndent(req, "", "  ")
	return string(data), nil
}

// ListPendingPlansHandler handles list_pending_plans tool calls.
type ListPendingPlansHandler struct {
	tracker *RequestTracker
}

// NewListPendingPlansHandler creates a new list_pending_plans handler.
func NewListPendingPlansHandler(tracker *RequestTracker) *ListPendingPlansHandler {
	return &ListPendingPlansHandler{tracker: tracker}
}

// Execute implements Handler.
func (h *ListPendingPlansHandler) Execute(input map[string]any) (string, error) {
	pending := h.tracker.ListPendingPlanRequests()
	if len(pending) == 0 {
		return "No pending plan requests.", nil
	}

	data, _ := json.MarshalIndent(pending, "", "  ")
	return string(data), nil
}