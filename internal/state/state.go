package state

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"
)

var ErrLocked = errors.New("project is already running")

type State struct {
	Project   string    `json:"project"`
	Status    string    `json:"status"`
	ThreadID  string    `json:"thread_id,omitempty"`
	TurnID    string    `json:"turn_id,omitempty"`
	UpdatedAt time.Time `json:"updated_at"`
}

type Store struct {
	dir string
	now func() time.Time
}

func New(dir string) *Store {
	return &Store{dir: dir, now: time.Now}
}

func ProjectDir(repository string) string { return filepath.Join(repository, ".donext") }

func (s *Store) statePath(project string) string {
	return filepath.Join(s.dir, "state", project+".json")
}
func (s *Store) logPath(project string) string {
	return filepath.Join(s.dir, "logs", project+".lifecycle.log")
}
func (s *Store) lockPath(project string) string {
	return filepath.Join(s.dir, "locks", project+".lock")
}

type Lock struct {
	file *os.File
}

func (s *Store) Acquire(project string) (*Lock, bool, error) {
	if err := os.MkdirAll(filepath.Join(s.dir, "locks"), 0o700); err != nil {
		return nil, false, fmt.Errorf("create state directory: %w", err)
	}
	file, err := os.OpenFile(s.lockPath(project), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, false, fmt.Errorf("open project lock: %w", err)
	}
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = file.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return nil, false, ErrLocked
		}
		return nil, false, fmt.Errorf("lock project: %w", err)
	}
	previous, err := s.Read(project)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		_ = syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
		_ = file.Close()
		return nil, false, err
	}
	return &Lock{file: file}, err == nil && previous.Status == "running", nil
}

func (l *Lock) Close() error {
	if l == nil || l.file == nil {
		return nil
	}
	unlockErr := syscall.Flock(int(l.file.Fd()), syscall.LOCK_UN)
	closeErr := l.file.Close()
	if unlockErr != nil {
		return unlockErr
	}
	return closeErr
}

func (s *Store) Read(project string) (State, error) {
	data, err := os.ReadFile(s.statePath(project))
	if err != nil {
		return State{}, err
	}
	var current State
	if err := json.Unmarshal(data, &current); err != nil {
		return State{}, fmt.Errorf("decode state for project %q: %w", project, err)
	}
	return current, nil
}

// IsLocked reports whether another process currently owns the project's lock.
func (s *Store) IsLocked(project string) (bool, error) {
	if err := os.MkdirAll(filepath.Join(s.dir, "locks"), 0o700); err != nil {
		return false, fmt.Errorf("create state directory: %w", err)
	}
	file, err := os.OpenFile(s.lockPath(project), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return false, fmt.Errorf("open project lock: %w", err)
	}
	defer file.Close()
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return true, nil
		}
		return false, fmt.Errorf("probe project lock: %w", err)
	}
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_UN); err != nil {
		return false, fmt.Errorf("release project lock probe: %w", err)
	}
	return false, nil
}

// LogLifecycle appends one metadata-only lifecycle record to the project log.
func (s *Store) LogLifecycle(project, component, event string, fields map[string]string) error {
	if err := os.MkdirAll(filepath.Join(s.dir, "logs"), 0o700); err != nil {
		return fmt.Errorf("create state directory: %w", err)
	}
	parts := []string{s.now().UTC().Format(time.RFC3339Nano), "project=" + project, "component=" + component, "event=" + event}
	keys := make([]string, 0, len(fields))
	for key := range fields {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		value := strings.NewReplacer("\n", "_", "\r", "_", " ", "_").Replace(fields[key])
		parts = append(parts, key+"="+value)
	}
	file, err := os.OpenFile(s.logPath(project), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("open lifecycle log: %w", err)
	}
	defer file.Close()
	if _, err := io.WriteString(file, strings.Join(parts, " ")+"\n"); err != nil {
		return fmt.Errorf("append lifecycle log: %w", err)
	}
	return nil
}

func (s *Store) Write(current State) error {
	stateDir := filepath.Join(s.dir, "state")
	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		return fmt.Errorf("create state directory: %w", err)
	}
	current.UpdatedAt = s.now().UTC()
	data, err := json.MarshalIndent(current, "", "  ")
	if err != nil {
		return fmt.Errorf("encode state: %w", err)
	}
	data = append(data, '\n')
	tmp, err := os.CreateTemp(stateDir, ".state-*")
	if err != nil {
		return fmt.Errorf("create temporary state: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write temporary state: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("sync temporary state: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temporary state: %w", err)
	}
	path := s.statePath(current.Project)
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("replace state: %w", err)
	}
	dir, err := os.Open(stateDir)
	if err != nil {
		return fmt.Errorf("open state directory: %w", err)
	}
	defer dir.Close()
	if err := dir.Sync(); err != nil {
		return fmt.Errorf("sync state directory: %w", err)
	}
	return nil
}
