package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
)

// TodoStatus is the lifecycle of a single todo entry.
//
// A string enum rather than a bool pair for the same reason ComputerPolicy's
// approval axis is: the set will grow, and an absent or unrecognised value must
// be reportable rather than decoding into a confident wrong answer.
type TodoStatus string

const (
	TodoPending    TodoStatus = "pending"
	TodoInProgress TodoStatus = "in_progress"
	TodoCompleted  TodoStatus = "completed"
)

const (
	// MaxTodos bounds the list. It exists because the list is echoed back into
	// model context on every write, so an unbounded list silently eats the
	// context window it was meant to help manage.
	MaxTodos = 100
	// MaxTodoContentBytes bounds one entry for the same reason.
	MaxTodoContentBytes = 500
	// todoResultPrefix marks the result so the conversation view can render it
	// as a block. The text stays readable as-is, because the model receives
	// this same string and must be able to act on it without a renderer.
	todoResultPrefix = "todos: "
)

// TodoItem is one entry in the list.
type TodoItem struct {
	Content string     `json:"content"`
	Status  TodoStatus `json:"status"`
}

// TodoStore holds one conversation's todo list.
//
// It is per-session state, not global: a background job tracking its own work
// must not be able to overwrite the foreground list, so each tool registry
// gets its own store.
type TodoStore struct {
	mu    sync.RWMutex
	items []TodoItem
}

// NewTodoStore returns an empty store.
func NewTodoStore() *TodoStore { return &TodoStore{} }

// Replace swaps the whole list. Replacement rather than incremental edits is
// deliberate: partial updates need stable ids, and an id the model has to
// remember across turns is one more thing for it to get wrong.
func (s *TodoStore) Replace(items []TodoItem) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.items = append(s.items[:0:0], items...)
}

// List returns a copy, so a caller rendering the list cannot mutate it.
func (s *TodoStore) List() []TodoItem {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]TodoItem(nil), s.items...)
}

// TodoWriteTool lets the agent record and update a working plan.
type TodoWriteTool struct {
	store *TodoStore
}

// NewTodoWriteTool builds the tool over store. store must not be nil; the
// caller owns it so the same list can also be rendered outside the tool.
func NewTodoWriteTool(store *TodoStore) *TodoWriteTool {
	if store == nil {
		store = NewTodoStore()
	}
	return &TodoWriteTool{store: store}
}

func (*TodoWriteTool) Name() string { return "todo_write" }

func (*TodoWriteTool) Description() string {
	return "Record the current task list. Send the COMPLETE list every time — it replaces the previous one. " +
		"Use it for multi-step work so progress is visible; skip it for single-step tasks. " +
		"Mark exactly one item in_progress while you work on it, and mark it completed as soon as it is done. " +
		"The list is rendered for the user automatically; do not repeat it back in prose."
}

func (*TodoWriteTool) Schema() json.RawMessage {
	return json.RawMessage(`{
  "type": "object",
  "properties": {
    "todos": {
      "type": "array",
      "description": "The complete task list, replacing any previous one.",
      "items": {
        "type": "object",
        "properties": {
          "content": {"type": "string", "description": "What the task is, in the imperative."},
          "status": {"type": "string", "enum": ["pending", "in_progress", "completed"]}
        },
        "required": ["content", "status"]
      }
    }
  },
  "required": ["todos"]
}`)
}

// RequiresApproval is false: the tool touches no file, runs no command, and
// reaches no network. It records intent only.
func (*TodoWriteTool) RequiresApproval() bool { return false }

type todoParams struct {
	Todos []TodoItem `json:"todos"`
}

func (t *TodoWriteTool) Execute(_ context.Context, params json.RawMessage) (ToolResult, error) {
	var p todoParams
	if err := json.Unmarshal(params, &p); err != nil {
		return ToolResult{Content: "todo_write: " + err.Error(), IsError: true}, nil
	}
	items, err := validateTodos(p.Todos)
	if err != nil {
		// A model error, not a tool failure: report it so the model can fix
		// its own call rather than aborting the turn.
		return ToolResult{Content: "todo_write: " + err.Error(), IsError: true}, nil
	}
	t.store.Replace(items)
	return ToolResult{
		Content:  RenderTodoList(items),
		Metadata: map[string]any{"todos": len(items), "completed": countTodos(items, TodoCompleted)},
	}, nil
}

// validateTodos rejects a malformed list rather than repairing it. Silently
// coercing an unrecognised status would let the model believe it had recorded
// something it had not, which is worse than a visible error it can retry.
func validateTodos(items []TodoItem) ([]TodoItem, error) {
	if len(items) > MaxTodos {
		return nil, fmt.Errorf("too many todos: %d (limit %d)", len(items), MaxTodos)
	}
	inProgress := 0
	out := make([]TodoItem, 0, len(items))
	for i, item := range items {
		content := strings.TrimSpace(item.Content)
		if content == "" {
			return nil, fmt.Errorf("todo %d has empty content", i+1)
		}
		if len(content) > MaxTodoContentBytes {
			return nil, fmt.Errorf("todo %d is %d bytes (limit %d)", i+1, len(content), MaxTodoContentBytes)
		}
		switch item.Status {
		case TodoPending, TodoInProgress, TodoCompleted:
		default:
			return nil, fmt.Errorf("todo %d has unknown status %q (want pending, in_progress, or completed)", i+1, item.Status)
		}
		if item.Status == TodoInProgress {
			inProgress++
		}
		out = append(out, TodoItem{Content: content, Status: item.Status})
	}
	// One task at a time is the whole point of the list. Several in_progress
	// entries describe an intention nobody can act on, and it is usually the
	// model forgetting to close the previous one.
	if inProgress > 1 {
		return nil, fmt.Errorf("%d todos are in_progress; exactly one may be", inProgress)
	}
	return out, nil
}

// RenderTodoList formats the list compactly. The same string goes to the model
// and to the renderer, so it has to be legible without styling.
func RenderTodoList(items []TodoItem) string {
	if len(items) == 0 {
		return todoResultPrefix + "list cleared"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%s%d/%d done", todoResultPrefix, countTodos(items, TodoCompleted), len(items))
	for _, item := range items {
		b.WriteString("\n" + todoMarker(item.Status) + " " + item.Content)
	}
	return b.String()
}

// IsTodoResult reports whether content came from todo_write, so a renderer can
// style it without having to know which tool produced it.
func IsTodoResult(content string) bool {
	return strings.HasPrefix(content, todoResultPrefix)
}

func todoMarker(status TodoStatus) string {
	switch status {
	case TodoCompleted:
		return "[x]"
	case TodoInProgress:
		return "[>]"
	default:
		return "[ ]"
	}
}

func countTodos(items []TodoItem, status TodoStatus) int {
	n := 0
	for _, item := range items {
		if item.Status == status {
			n++
		}
	}
	return n
}
