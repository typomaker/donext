package codex

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"sync"
)

const maxMessageSize = 16 * 1024 * 1024

type transport interface {
	io.Reader
	io.Writer
	io.Closer
}

type processTransport struct {
	stdin  io.WriteCloser
	stdout io.ReadCloser
	wait   func() error
	kill   func() error
	once   sync.Once
	err    error
}

func (p *processTransport) ForceClose() error {
	if p.kill == nil {
		return p.Close()
	}
	return p.kill()
}

func (p *processTransport) Read(b []byte) (int, error)  { return p.stdout.Read(b) }
func (p *processTransport) Write(b []byte) (int, error) { return p.stdin.Write(b) }
func (p *processTransport) Close() error {
	p.once.Do(func() {
		if err := p.stdin.Close(); err != nil {
			p.err = err
		}
		if err := p.wait(); err != nil {
			p.err = err
		}
	})
	return p.err
}

// StartAppServer starts one `codex app-server --stdio` process and performs the
// initialize handshake. Diagnostics are copied from stderr without entering the
// protocol stream.
func StartAppServer(ctx context.Context, command string, stderr io.Writer) (*AppServer, error) {
	if command == "" {
		command = "codex"
	}
	if stderr == nil {
		stderr = io.Discard
	}
	cmd := exec.CommandContext(ctx, command, "app-server", "--stdio")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("app-server stdin: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("app-server stdout: %w", err)
	}
	errpipe, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("app-server stderr: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start app-server: %w", err)
	}
	go func() { _, _ = io.Copy(stderr, errpipe) }()
	t := &processTransport{stdin: stdin, stdout: stdout, wait: cmd.Wait, kill: cmd.Process.Kill}
	client, err := newAppServer(ctx, t)
	if err != nil {
		_ = t.Close()
		return nil, err
	}
	return client, nil
}

type response struct {
	result json.RawMessage
	err    error
}

type AppServer struct {
	t           transport
	writeMu     sync.Mutex
	mu          sync.Mutex
	nextID      int64
	pending     map[int64]chan response
	events      chan Event
	done        chan struct{}
	closeOnce   sync.Once
	closeErr    error
	terminalErr error
}

var _ Client = (*AppServer)(nil)

func (a *AppServer) ReadRateLimits(ctx context.Context) (RateLimits, error) {
	var out struct {
		RateLimits struct {
			Primary   *rateLimitWindow `json:"primary"`
			Secondary *rateLimitWindow `json:"secondary"`
		} `json:"rateLimits"`
	}
	if err := a.call(ctx, "account/rateLimits/read", map[string]any{}, &out); err != nil {
		return RateLimits{}, err
	}
	var limits RateLimits
	if out.RateLimits.Primary != nil {
		limits.Primary = toRateLimitWindow(out.RateLimits.Primary)
	}
	if out.RateLimits.Secondary != nil {
		limits.Secondary = toRateLimitWindow(out.RateLimits.Secondary)
	}
	return limits, nil
}

type rateLimitWindow struct {
	UsedPercent        int    `json:"usedPercent"`
	WindowDurationMins *int64 `json:"windowDurationMins"`
	ResetsAt           *int64 `json:"resetsAt"`
}

func toRateLimitWindow(window *rateLimitWindow) *RateLimitWindow {
	return &RateLimitWindow{UsedPercent: window.UsedPercent, WindowDurationMins: window.WindowDurationMins, ResetsAt: window.ResetsAt}
}

type wireMessage struct {
	ID     *int64          `json:"id,omitempty"`
	Method string          `json:"method,omitempty"`
	Params json.RawMessage `json:"params,omitempty"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

func (e *rpcError) Error() string {
	return fmt.Sprintf("app-server RPC error %d: %s", e.Code, e.Message)
}

func newAppServer(ctx context.Context, t transport) (*AppServer, error) {
	a := &AppServer{t: t, pending: make(map[int64]chan response), events: make(chan Event, 32), done: make(chan struct{})}
	go a.readLoop()
	var initialized json.RawMessage
	if err := a.call(ctx, "initialize", map[string]any{"clientInfo": map[string]string{"name": "donext", "version": "0.1.0"}}, &initialized); err != nil {
		_ = a.Close()
		return nil, fmt.Errorf("initialize app-server: %w", err)
	}
	if err := a.notify("initialized", map[string]any{}); err != nil {
		_ = a.Close()
		return nil, fmt.Errorf("notify initialized: %w", err)
	}
	return a, nil
}

func (a *AppServer) StartThread(ctx context.Context, o ThreadOptions) (string, error) {
	params := map[string]any{"cwd": o.CWD, "ephemeral": false}
	if o.ProjectID != "" {
		params["projectId"] = o.ProjectID
	}
	if o.ApprovalPolicy != "" {
		params["approvalPolicy"] = o.ApprovalPolicy
	}
	if o.Sandbox != "" {
		params["sandbox"] = o.Sandbox
	}
	var out struct {
		Thread struct {
			ID string `json:"id"`
		} `json:"thread"`
	}
	if err := a.call(ctx, "thread/start", params, &out); err != nil {
		return "", err
	}
	if out.Thread.ID == "" {
		return "", errors.New("thread/start returned an empty thread ID")
	}
	return out.Thread.ID, nil
}

func (a *AppServer) ListProjects(ctx context.Context) ([]Project, error) {
	var projects []Project
	var cursor string
	for {
		params := map[string]any{"limit": 100}
		if cursor != "" {
			params["cursor"] = cursor
		}
		var out struct {
			Data []struct {
				ID    string `json:"id"`
				Name  string `json:"name"`
				Roots []struct {
					Path string `json:"path"`
				} `json:"roots"`
			} `json:"data"`
			NextCursor *string `json:"nextCursor"`
		}
		if err := a.call(ctx, "project/list", params, &out); err != nil {
			return nil, err
		}
		for _, item := range out.Data {
			project := Project{ID: item.ID, Name: item.Name}
			for _, root := range item.Roots {
				project.Roots = append(project.Roots, root.Path)
			}
			projects = append(projects, project)
		}
		if out.NextCursor == nil || *out.NextCursor == "" {
			return projects, nil
		}
		cursor = *out.NextCursor
	}
}

func (a *AppServer) NameThread(ctx context.Context, threadID, name string) error {
	return a.call(ctx, "thread/name/set", map[string]string{"threadId": threadID, "name": name}, nil)
}

func (a *AppServer) StartTurn(ctx context.Context, threadID, prompt string) (string, error) {
	params := map[string]any{"threadId": threadID, "input": []map[string]string{{"type": "text", "text": prompt}}}
	var out struct {
		Turn struct {
			ID string `json:"id"`
		} `json:"turn"`
	}
	if err := a.call(ctx, "turn/start", params, &out); err != nil {
		return "", err
	}
	if out.Turn.ID == "" {
		return "", errors.New("turn/start returned an empty turn ID")
	}
	return out.Turn.ID, nil
}

func (a *AppServer) InterruptTurn(ctx context.Context, threadID, turnID string) error {
	return a.call(ctx, "turn/interrupt", map[string]string{"threadId": threadID, "turnId": turnID}, nil)
}

func (a *AppServer) RejectRequest(_ context.Context, requestID int64, message string) error {
	return a.write(map[string]any{"id": requestID, "error": map[string]any{"code": -32001, "message": message}})
}

func (a *AppServer) ForceClose() error {
	if t, ok := a.t.(interface{ ForceClose() error }); ok {
		return t.ForceClose()
	}
	return a.t.Close()
}

func (a *AppServer) Events() <-chan Event { return a.events }

func (a *AppServer) Close() error {
	a.closeOnce.Do(func() { a.closeErr = a.t.Close() })
	<-a.done
	a.mu.Lock()
	err := a.terminalErr
	closeErr := a.closeErr
	a.mu.Unlock()
	if closeErr != nil {
		return fmt.Errorf("close app-server: %w", closeErr)
	}
	if errors.Is(err, io.EOF) {
		return nil
	}
	return err
}

func (a *AppServer) call(ctx context.Context, method string, params any, out any) error {
	a.mu.Lock()
	if a.terminalErr != nil {
		err := a.terminalErr
		a.mu.Unlock()
		return err
	}
	a.nextID++
	id := a.nextID
	ch := make(chan response, 1)
	a.pending[id] = ch
	a.mu.Unlock()
	if err := a.write(map[string]any{"id": id, "method": method, "params": params}); err != nil {
		a.mu.Lock()
		delete(a.pending, id)
		a.mu.Unlock()
		return err
	}
	select {
	case r := <-ch:
		if r.err != nil {
			return r.err
		}
		if out != nil && len(r.result) != 0 && string(r.result) != "null" {
			if err := json.Unmarshal(r.result, out); err != nil {
				return fmt.Errorf("decode %s response: %w", method, err)
			}
		}
		return nil
	case <-ctx.Done():
		a.mu.Lock()
		delete(a.pending, id)
		a.mu.Unlock()
		return ctx.Err()
	}
}

func (a *AppServer) notify(method string, params any) error {
	return a.write(map[string]any{"method": method, "params": params})
}
func (a *AppServer) write(v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	b = append(b, '\n')
	a.writeMu.Lock()
	defer a.writeMu.Unlock()
	if _, err := a.t.Write(b); err != nil {
		return fmt.Errorf("write app-server message: %w", err)
	}
	return nil
}

func (a *AppServer) readLoop() {
	s := bufio.NewScanner(a.t)
	s.Buffer(make([]byte, 64*1024), maxMessageSize)
	for s.Scan() {
		var msg wireMessage
		if err := json.Unmarshal(s.Bytes(), &msg); err != nil {
			a.fail(fmt.Errorf("decode app-server message: %w", err))
			return
		}
		if msg.ID != nil && msg.Method == "" {
			a.deliverResponse(*msg.ID, msg)
			continue
		}
		if msg.Method != "" {
			a.route(msg)
		}
	}
	err := s.Err()
	if err == nil {
		err = io.EOF
	}
	a.fail(fmt.Errorf("app-server stream closed: %w", err))
}

func (a *AppServer) deliverResponse(id int64, msg wireMessage) {
	a.mu.Lock()
	ch := a.pending[id]
	delete(a.pending, id)
	a.mu.Unlock()
	if ch == nil {
		return
	}
	if msg.Error != nil {
		ch <- response{err: msg.Error}
	} else {
		ch <- response{result: msg.Result}
	}
}

func (a *AppServer) route(msg wireMessage) {
	// A method plus id is a request initiated by the server, not a notification.
	if msg.ID != nil {
		var scope struct {
			ThreadID string `json:"threadId"`
			TurnID   string `json:"turnId"`
		}
		_ = json.Unmarshal(msg.Params, &scope)
		a.emit(Event{Kind: ServerRequest, Method: msg.Method, RequestID: *msg.ID, ThreadID: scope.ThreadID, TurnID: scope.TurnID, Params: msg.Params})
		return
	}
	if msg.Method == "item/completed" {
		var p struct {
			ThreadID string `json:"threadId"`
			TurnID   string `json:"turnId"`
			Item     struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"item"`
		}
		if json.Unmarshal(msg.Params, &p) == nil && p.Item.Type == "agentMessage" {
			a.emit(Event{Kind: AgentMessageCompleted, Method: msg.Method, ThreadID: p.ThreadID, TurnID: p.TurnID, Text: p.Item.Text})
		}
		return
	}
	if msg.Method != "turn/completed" {
		return
	}
	var p struct {
		ThreadID string `json:"threadId"`
		Turn     struct {
			ID     string `json:"id"`
			Status string `json:"status"`
		} `json:"turn"`
	}
	if json.Unmarshal(msg.Params, &p) != nil {
		return
	}
	a.emit(Event{Kind: TurnCompleted, Method: msg.Method, ThreadID: p.ThreadID, TurnID: p.Turn.ID, Status: p.Turn.Status})
}

func (a *AppServer) emit(e Event) {
	select {
	case a.events <- e:
	case <-a.done:
	}
}

func (a *AppServer) fail(err error) {
	a.mu.Lock()
	if a.terminalErr == nil {
		a.terminalErr = err
	}
	for id, ch := range a.pending {
		ch <- response{err: err}
		delete(a.pending, id)
	}
	a.mu.Unlock()
	close(a.done)
	close(a.events)
}
