// Package codex defines the donext-facing Codex contract.
package codex

import (
	"context"
	"encoding/json"
)

// Client is deliberately expressed in donext domain terms. Implementations
// may use App Server or a fake without exposing protocol messages to callers.
type Client interface {
	ReadRateLimits(context.Context) (RateLimits, error)
	StartThread(context.Context, ThreadOptions) (string, error)
	NameThread(context.Context, string, string) error
	StartTurn(context.Context, string, string) (string, error)
	InterruptTurn(context.Context, string, string) error
	RejectRequest(context.Context, int64, string) error
	Events() <-chan Event
	ForceClose() error
	Close() error
}

type RateLimits struct {
	Primary   *RateLimitWindow
	Secondary *RateLimitWindow
}

type RateLimitWindow struct {
	UsedPercent        int
	WindowDurationMins *int64
	ResetsAt           *int64
}

type ThreadOptions struct {
	CWD            string
	ApprovalPolicy string
	Sandbox        string
}

type EventKind string

const (
	TurnCompleted         EventKind = "turn_completed"
	AgentMessageCompleted EventKind = "agent_message_completed"
	ReasoningCompleted    EventKind = "reasoning_completed"
	CommandStarted        EventKind = "command_started"
	FileChangeCompleted   EventKind = "file_change_completed"
	TokenUsageUpdated     EventKind = "token_usage_updated"
	ServerRequest         EventKind = "server_request"
)

type TokenUsage struct {
	InputTokens           int64
	CachedInputTokens     int64
	OutputTokens          int64
	ReasoningOutputTokens int64
	TotalTokens           int64
}

// Event exposes only the protocol data needed by the orchestrator. Params is
// retained for server requests so a later policy layer can inspect and answer them.
type Event struct {
	Kind          EventKind
	Method        string
	RequestID     int64
	ThreadID      string
	TurnID        string
	Status        string
	ErrorCode     string
	ErrorMessage  string
	Text          string
	Paths         []string
	LastUsage     TokenUsage
	TotalUsage    TokenUsage
	ContextWindow *int64
	Params        json.RawMessage
}
