package op

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

// TestGlossaryMarkdownInSync regenerates glossary.md with UPDATE_GLOSSARY=1
// and otherwise fails when the checked-in file drifts from Glossary/Examples.
func TestGlossaryMarkdownInSync(t *testing.T) {
	var b strings.Builder
	b.WriteString("# CMS Open Payments glossary\n\nPlain words mapped to the local SQLite columns (`openpayments-pp-cli schema --json`) and the CMS source fields they come from. `openpayments-pp-cli ask \"<question>\"` prints this glossary with worked examples.\n\n")
	b.WriteString("| Plain word | Local column | CMS field | Meaning |\n|---|---|---|---|\n")
	for _, g := range Glossary {
		esc := func(s string) string { return strings.ReplaceAll(s, "|", "\\|") }
		fmt.Fprintf(&b, "| %s | `%s` | `%s` | %s |\n", esc(g.Term), esc(g.Column), esc(g.Source), esc(g.Meaning))
	}
	b.WriteString("\n## Worked questions\n\n")
	for i, e := range Examples {
		fmt.Fprintf(&b, "%d. **%s**\n   ```bash\n   %s\n   ```\n", i+1, e.Question, e.Command)
		if e.SQL != "" {
			fmt.Fprintf(&b, "   SQL: `%s`\n", e.SQL)
		}
	}
	want := b.String()
	path := "../../glossary.md"
	if os.Getenv("UPDATE_GLOSSARY") == "1" {
		if err := os.WriteFile(path, []byte(want), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("glossary.md missing; run UPDATE_GLOSSARY=1 go test ./internal/op -run Glossary: %v", err)
	}
	if string(got) != want {
		t.Fatal("glossary.md is stale; run UPDATE_GLOSSARY=1 go test ./internal/op -run Glossary")
	}
}
