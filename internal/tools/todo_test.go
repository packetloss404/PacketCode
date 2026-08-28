package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func writeTodos(t *testing.T, tool *TodoWriteTool, body string) ToolResult {
	t.Helper()
	res, err := tool.Execute(context.Background(), json.RawMessage(body))
	if err != nil {
		t.Fatalf("Execute returned a transport error: %v", err)
	}
	return res
}

// The list replaces wholesale. Incremental edits would need stable ids the
// model has to remember across turns, which is one more thing to get wrong.
func TestTodoWrite_ReplacesTheWholeList(t *testing.T) {
	store := NewTodoStore()
	tool := NewTodoWriteTool(store)

	writeTodos(t, tool, `{"todos":[
		{"content":"first","status":"completed"},
		{"content":"second","status":"in_progress"},
		{"content":"third","status":"pending"}]}`)
	if got := store.List(); len(got) != 3 {
		t.Fatalf("stored %d todos, want 3", len(got))
	}

	res := writeTodos(t, tool, `{"todos":[{"content":"only this","status":"pending"}]}`)
	if res.IsError {
		t.Fatalf("unexpected error result: %q", res.Content)
	}
	got := store.List()
	if len(got) != 1 || got[0].Content != "only this" {
		t.Fatalf("list was not replaced: %#v", got)
	}
}

// Malformed input is reported, never repaired. Coercing an unrecognised status
// would let the model believe it recorded something it did not.
func TestTodoWrite_RejectsMalformedListsWithoutStoring(t *testing.T) {
	cases := []struct {
		name string
		body string
		want string
	}{
		{"unknown status", `{"todos":[{"content":"a","status":"almost-done"}]}`, "unknown status"},
		{"empty content", `{"todos":[{"content":"   ","status":"pending"}]}`, "empty content"},
		{"two in progress", `{"todos":[{"content":"a","status":"in_progress"},{"content":"b","status":"in_progress"}]}`, "in_progress"},
		{"not json", `{"todos":`, "unexpected end"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store := NewTodoStore()
			tool := NewTodoWriteTool(store)
			// Seed a good list so we can prove a bad call leaves it alone.
			writeTodos(t, tool, `{"todos":[{"content":"keep me","status":"pending"}]}`)

			res := writeTodos(t, tool, tc.body)
			if !res.IsError {
				t.Fatalf("expected an error result, got %q", res.Content)
			}
			if !strings.Contains(res.Content, tc.want) {
				t.Fatalf("error %q should mention %q", res.Content, tc.want)
			}
			got := store.List()
			if len(got) != 1 || got[0].Content != "keep me" {
				t.Fatalf("a rejected write must not disturb the stored list: %#v", got)
			}
		})
	}
}

// The list is echoed into model context on every write, so it has to be bounded
// or it eats the context window it exists to help manage.
func TestTodoWrite_EnforcesBounds(t *testing.T) {
	tool := NewTodoWriteTool(NewTodoStore())

	items := make([]string, 0, MaxTodos+1)
	for i := 0; i <= MaxTodos; i++ {
		items = append(items, `{"content":"x","status":"pending"}`)
	}
	res := writeTodos(t, tool, `{"todos":[`+strings.Join(items, ",")+`]}`)
	if !res.IsError || !strings.Contains(res.Content, "too many todos") {
		t.Fatalf("an over-long list must be refused, got %q", res.Content)
	}

	long := strings.Repeat("y", MaxTodoContentBytes+1)
	res = writeTodos(t, tool, `{"todos":[{"content":"`+long+`","status":"pending"}]}`)
	if !res.IsError || !strings.Contains(res.Content, "limit") {
		t.Fatalf("an over-long entry must be refused, got %q", res.Content)
	}
}

// The rendered block goes to the model as well as the screen, so it must be
// legible unstyled and must carry the marker the renderer keys on.
func TestRenderTodoList_IsLegibleAndMarked(t *testing.T) {
	out := RenderTodoList([]TodoItem{
		{Content: "wire the registry", Status: TodoCompleted},
		{Content: "add the renderer", Status: TodoInProgress},
		{Content: "document it", Status: TodoPending},
	})
	if !IsTodoResult(out) {
		t.Fatalf("renderer marker missing from %q", out)
	}
	for _, want := range []string{"1/3 done", "[x] wire the registry", "[>] add the renderer", "[ ] document it"} {
		if !strings.Contains(out, want) {
			t.Fatalf("rendered list %q missing %q", out, want)
		}
	}
	if empty := RenderTodoList(nil); !IsTodoResult(empty) || !strings.Contains(empty, "cleared") {
		t.Fatalf("an emptied list should say so, got %q", empty)
	}
}

// It records intent only — no file, no command, no network — so gating it
// behind an approval prompt would train the user to approve reflexively.
func TestTodoWrite_NeedsNoApproval(t *testing.T) {
	if NewTodoWriteTool(nil).RequiresApproval() {
		t.Fatal("todo_write must not require approval")
	}
}

// A nil store is a programming slip, not a reason to panic on the first call.
func TestNewTodoWriteTool_NilStoreStillWorks(t *testing.T) {
	tool := NewTodoWriteTool(nil)
	res := writeTodos(t, tool, `{"todos":[{"content":"a","status":"pending"}]}`)
	if res.IsError {
		t.Fatalf("unexpected error: %q", res.Content)
	}
}

// Stores are per-session: a background job must not be able to rewrite the
// plan the user is watching in the foreground.
func TestTodoStores_AreIndependent(t *testing.T) {
	foreground, background := NewTodoStore(), NewTodoStore()
	writeTodos(t, NewTodoWriteTool(foreground), `{"todos":[{"content":"user work","status":"in_progress"}]}`)
	writeTodos(t, NewTodoWriteTool(background), `{"todos":[{"content":"job work","status":"pending"}]}`)

	if got := foreground.List(); len(got) != 1 || got[0].Content != "user work" {
		t.Fatalf("foreground list was disturbed by a background write: %#v", got)
	}
}

// List returns a copy; a renderer holding the slice must not be able to edit
// the store through it.
func TestTodoStore_ListReturnsACopy(t *testing.T) {
	store := NewTodoStore()
	store.Replace([]TodoItem{{Content: "original", Status: TodoPending}})

	got := store.List()
	got[0].Content = "mutated"

	if after := store.List(); after[0].Content != "original" {
		t.Fatalf("List exposed internal state: %q", after[0].Content)
	}
}
