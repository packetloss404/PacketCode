package app

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strconv"
	"testing"
)

// Every verb handleSlashCommand dispatches must be reserved in the registry.
//
// A verb the switch consumes but the registry does not know about is a name a
// skill or a command file can claim: it registers, it shows up in /help and
// autocomplete with the author's description, and typing it runs the builtin
// instead — with no collision reported anywhere, because nothing knew there
// was a collision. `workflow` was exactly that for as long as `workflows` was
// the only spelling in SlashCommands.
//
// Reading the switch out of the source is the only way to check this: the case
// list is not a value any test can reach at runtime, and a hand-maintained
// copy of it would drift the same way the original two lists did.
func TestBuiltinRegistryReservesEveryDispatchedVerb(t *testing.T) {
	verbs := dispatchedSlashVerbs(t)
	if len(verbs) < 20 {
		t.Fatalf("only found %d dispatched verbs; the switch was probably not located", len(verbs))
	}
	reg := NewBuiltinSlashRegistry()
	for _, verb := range verbs {
		cmd, ok := reg.Lookup(verb)
		if !ok {
			t.Errorf("handleSlashCommand dispatches %q but the registry does not reserve it; "+
				"add it to SlashCommands, or to builtinAliases if it is an alias", verb)
			continue
		}
		if !cmd.Builtin {
			t.Errorf("%q is dispatched by handleSlashCommand but registered as a non-builtin", verb)
		}
	}
}

// dispatchedSlashVerbs returns the case values of handleSlashCommand's switch.
func dispatchedSlashVerbs(t *testing.T) []string {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "app.go", nil, 0)
	if err != nil {
		t.Fatalf("parse app.go: %v", err)
	}

	var out []string
	ast.Inspect(file, func(n ast.Node) bool {
		fn, ok := n.(*ast.FuncDecl)
		if !ok || fn.Name.Name != "handleSlashCommand" || fn.Body == nil {
			return true
		}
		for _, stmt := range fn.Body.List {
			sw, ok := stmt.(*ast.SwitchStmt)
			if !ok || sw.Tag == nil {
				continue
			}
			if ident, ok := sw.Tag.(*ast.Ident); !ok || ident.Name != "cmd" {
				continue
			}
			for _, c := range sw.Body.List {
				clause, ok := c.(*ast.CaseClause)
				if !ok {
					continue
				}
				for _, expr := range clause.List {
					lit, ok := expr.(*ast.BasicLit)
					if !ok || lit.Kind != token.STRING {
						continue
					}
					verb, err := strconv.Unquote(lit.Value)
					if err != nil {
						t.Fatalf("unquote %s: %v", lit.Value, err)
					}
					out = append(out, verb)
				}
			}
		}
		return false
	})
	return out
}
