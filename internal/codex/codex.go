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
	ListProjects(context.Context) ([]Project, error)
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
	ProjectID      string
	ApprovalPolicy string
	Sandbox        string
}

type Project struct {
	ID    string
	Name  string
	Roots []string
}

type EventKind string

const (
	TurnCompleted         EventKind = "turn_completed"
	AgentMessageCompleted EventKind = "agent_message_completed"
	ServerRequest         EventKind = "server_request"
)

// Event contains only lifecycle metadata. Params is retained for server
// requests so a later policy layer can inspect and answer them.
type Event struct {
	Kind      EventKind
	Method    string
	RequestID int64
	ThreadID  string
	TurnID    string
	Status    string
	Text      string
	Params    json.RawMessage
}
