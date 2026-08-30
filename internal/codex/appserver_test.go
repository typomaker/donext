package codex

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"strings"
	"testing"
	"time"
)

type fakeServer struct {
	conn net.Conn
	scan *bufio.Scanner
	t    *testing.T
}

func newFake(t *testing.T) (*fakeServer, transport) {
	t.Helper()
	client, server := net.Pipe()
	return &fakeServer{conn: server, scan: bufio.NewScanner(server), t: t}, client
}
func (f *fakeServer) request(method string) map[string]any {
	f.t.Helper()
	if !f.scan.Scan() {
		f.t.Fatalf("read request: %v", f.scan.Err())
	}
	var m map[string]any
	if err := json.Unmarshal(f.scan.Bytes(), &m); err != nil {
		f.t.Fatal(err)
	}
	if m["method"] != method {
		f.t.Fatalf("method=%v want=%s message=%s", m["method"], method, f.scan.Bytes())
	}
	return m
}
func (f *fakeServer) message() map[string]any {
	f.t.Helper()
	if !f.scan.Scan() {
		f.t.Fatalf("read message: %v", f.scan.Err())
	}
	var m map[string]any
	if err := json.Unmarshal(f.scan.Bytes(), &m); err != nil {
		f.t.Fatal(err)
	}
	return m
}
func (f *fakeServer) send(v any) {
	f.t.Helper()
	if err := json.NewEncoder(f.conn).Encode(v); err != nil {
		f.t.Fatal(err)
	}
}
func (f *fakeServer) respond(m map[string]any, result any) {
	f.send(map[string]any{"id": m["id"], "result": result})
}

func connect(t *testing.T, f *fakeServer, tr transport) *AppServer {
	t.Helper()
	result := make(chan struct {
		c   *AppServer
		err error
	}, 1)
	go func() {
		c, err := newAppServer(context.Background(), tr)
		result <- struct {
			c   *AppServer
			err error
		}{c, err}
	}()
	m := f.request("initialize")
	params := m["params"].(map[string]any)
	if params["clientInfo"].(map[string]any)["name"] != "donext" {
		t.Fatal("missing client info")
	}
	capabilities, ok := params["capabilities"].(map[string]any)
	if !ok || capabilities["experimentalApi"] != true {
		t.Fatalf("capabilities=%v want experimentalApi=true", params["capabilities"])
	}
	f.respond(m, map[string]any{"userAgent": "fake"})
	f.request("initialized")
	r := <-result
	if r.err != nil {
		t.Fatal(r.err)
	}
	return r.c
}

func TestFullLifecycleAndRouting(t *testing.T) {
	f, tr := newFake(t)
	c := connect(t, f, tr)
	done := make(chan error, 1)
	go func() {
		thread, err := c.StartThread(context.Background(), ThreadOptions{CWD: "/repo", ProjectID: "project-1", ApprovalPolicy: "never", Sandbox: "read-only"})
		if err != nil {
			done <- err
			return
		}
		if thread != "th-1" {
			done <- errors.New("wrong thread")
			return
		}
		if err := c.NameThread(context.Background(), thread, "goal"); err != nil {
			done <- err
			return
		}
		turn, err := c.StartTurn(context.Background(), thread, "next goal")
		if err != nil {
			done <- err
			return
		}
		done <- c.InterruptTurn(context.Background(), thread, turn)
	}()
	m := f.request("thread/start")
	p := m["params"].(map[string]any)
	if p["cwd"] != "/repo" || p["projectId"] != "project-1" || p["ephemeral"] != false || p["approvalPolicy"] != "never" {
		t.Fatalf("params=%v", p)
	}
	f.respond(m, map[string]any{"thread": map[string]any{"id": "th-1"}})
	m = f.request("thread/name/set")
	f.respond(m, map[string]any{})
	m = f.request("turn/start")
	f.respond(m, map[string]any{"turn": map[string]any{"id": "tu-1"}})
	m = f.request("turn/interrupt")
	p = m["params"].(map[string]any)
	if p["threadId"] != "th-1" || p["turnId"] != "tu-1" {
		t.Fatalf("interrupt params=%v", p)
	}
	f.respond(m, map[string]any{})
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	f.send(map[string]any{"method": "unrecognized/future", "params": map[string]any{"x": 1}})
	f.send(map[string]any{"method": "item/completed", "params": map[string]any{"threadId": "th-1", "turnId": "tu-1", "completedAtMs": 1, "item": map[string]any{"id": "item-1", "type": "agentMessage", "text": "done"}}})
	f.send(map[string]any{"method": "turn/completed", "params": map[string]any{"threadId": "th-1", "turn": map[string]any{"id": "tu-1", "status": "interrupted"}}})
	e := <-c.Events()
	if e.Kind != AgentMessageCompleted || e.Text != "done" || e.TurnID != "tu-1" {
		t.Fatalf("event=%+v", e)
	}
	e = <-c.Events()
	if e.Kind != TurnCompleted || e.ThreadID != "th-1" || e.TurnID != "tu-1" || e.Status != "interrupted" {
		t.Fatalf("event=%+v", e)
	}
	_ = f.conn.Close()
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestListProjectsPaginates(t *testing.T) {
	f, tr := newFake(t)
	c := connect(t, f, tr)
	done := make(chan []Project, 1)
	go func() { projects, _ := c.ListProjects(context.Background()); done <- projects }()
	m := f.request("project/list")
	if m["params"].(map[string]any)["limit"] != float64(100) {
		t.Fatalf("params=%v", m["params"])
	}
	f.respond(m, map[string]any{"data": []any{map[string]any{"id": "p1", "name": "One", "roots": []any{map[string]any{"path": "/one"}}}}, "nextCursor": "next"})
	m = f.request("project/list")
	if m["params"].(map[string]any)["cursor"] != "next" {
		t.Fatalf("params=%v", m["params"])
	}
	f.respond(m, map[string]any{"data": []any{map[string]any{"id": "p2", "name": "Two", "roots": []any{map[string]any{"path": "/two"}}}}})
	projects := <-done
	if len(projects) != 2 || projects[0].ID != "p1" || projects[0].Roots[0] != "/one" || projects[1].ID != "p2" {
		t.Fatalf("projects=%+v", projects)
	}
	_ = f.conn.Close()
	_ = c.Close()
}

func TestReadRateLimits(t *testing.T) {
	f, tr := newFake(t)
	c := connect(t, f, tr)
	done := make(chan RateLimits, 1)
	go func() { limits, _ := c.ReadRateLimits(context.Background()); done <- limits }()
	m := f.request("account/rateLimits/read")
	f.respond(m, map[string]any{"rateLimits": map[string]any{"primary": map[string]any{"usedPercent": 20, "windowDurationMins": 300, "resetsAt": 1000}, "secondary": map[string]any{"usedPercent": 7, "windowDurationMins": 10080, "resetsAt": 2000}}})
	limits := <-done
	if limits.Primary == nil || limits.Primary.UsedPercent != 20 || limits.Primary.WindowDurationMins == nil || *limits.Primary.WindowDurationMins != 300 || limits.Secondary == nil || limits.Secondary.UsedPercent != 7 || limits.Secondary.ResetsAt == nil || *limits.Secondary.ResetsAt != 2000 {
		t.Fatalf("limits=%+v", limits)
	}
	_ = f.conn.Close()
	_ = c.Close()
}

func TestCorrelatesOutOfOrderResponses(t *testing.T) {
	f, tr := newFake(t)
	c := connect(t, f, tr)
	type got struct {
		id  string
		err error
	}
	one, two := make(chan got, 1), make(chan got, 1)
	go func() {
		id, err := c.StartThread(context.Background(), ThreadOptions{CWD: "/one"})
		one <- got{id, err}
	}()
	go func() {
		id, err := c.StartThread(context.Background(), ThreadOptions{CWD: "/two"})
		two <- got{id, err}
	}()
	a, b := f.request("thread/start"), f.request("thread/start")
	responseID := func(m map[string]any) string {
		return strings.TrimPrefix(m["params"].(map[string]any)["cwd"].(string), "/")
	}
	f.respond(b, map[string]any{"thread": map[string]any{"id": responseID(b)}})
	f.respond(a, map[string]any{"thread": map[string]any{"id": responseID(a)}})
	r1, r2 := <-one, <-two
	if r1.err != nil || r2.err != nil || r1.id != "one" || r2.id != "two" {
		t.Fatalf("results=%+v %+v", r1, r2)
	}
	_ = f.conn.Close()
	_ = c.Close()
}

func TestRPCErrorServerRequestAndEOF(t *testing.T) {
	f, tr := newFake(t)
	c := connect(t, f, tr)
	errCh := make(chan error, 1)
	go func() { _, err := c.StartThread(context.Background(), ThreadOptions{CWD: "/repo"}); errCh <- err }()
	m := f.request("thread/start")
	f.send(map[string]any{"id": m["id"], "error": map[string]any{"code": -32602, "message": "bad params"}})
	if err := <-errCh; err == nil || !strings.Contains(err.Error(), "bad params") {
		t.Fatalf("err=%v", err)
	}
	f.send(map[string]any{"id": 99, "method": "item/tool/requestUserInput", "params": map[string]any{"threadId": "th"}})
	e := <-c.Events()
	if e.Kind != ServerRequest || e.Method != "item/tool/requestUserInput" {
		t.Fatalf("event=%+v", e)
	}
	rejectErr := make(chan error, 1)
	go func() { rejectErr <- c.RejectRequest(context.Background(), e.RequestID, "not interactive") }()
	rejection := f.message()
	if rejection["id"] != float64(99) || rejection["error"].(map[string]any)["message"] != "not interactive" {
		t.Fatalf("rejection=%v", rejection)
	}
	if err := <-rejectErr; err != nil {
		t.Fatal(err)
	}
	go func() { _, err := c.StartThread(context.Background(), ThreadOptions{CWD: "/pending"}); errCh <- err }()
	f.request("thread/start")
	_ = f.conn.Close()
	if err := <-errCh; err == nil || !errors.Is(err, io.EOF) {
		t.Fatalf("EOF err=%v", err)
	}
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestMalformedProtocolFailsPendingRequest(t *testing.T) {
	f, tr := newFake(t)
	c := connect(t, f, tr)
	errCh := make(chan error, 1)
	go func() { _, err := c.StartThread(context.Background(), ThreadOptions{CWD: "/repo"}); errCh <- err }()
	f.request("thread/start")
	_, _ = io.WriteString(f.conn, "not-json\n")
	select {
	case err := <-errCh:
		if err == nil || !strings.Contains(err.Error(), "decode app-server message") {
			t.Fatalf("err=%v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("pending request was not failed")
	}
	_ = f.conn.Close()
	if err := c.Close(); err == nil {
		t.Fatal("expected protocol error")
	}
}
