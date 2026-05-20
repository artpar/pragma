package applypatch

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/artpar/pragma/internal/tool"
)

type testState struct {
	workDir string
}

func (s testState) WorkDir() string { return s.workDir }

func TestApplyPatchUpdateAddDelete(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n\nfunc main() {\n\tprintln(\"old\")\n}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "old.txt"), []byte("remove me\n"), 0644); err != nil {
		t.Fatal(err)
	}

	patch := `*** Begin Patch
*** Update File: main.go
@@
 func main() {
-	println("old")
+	println("new")
 }
*** Add File: added.txt
+created
*** Delete File: old.txt
*** End Patch`

	result, err := ApplyPatchText(context.Background(), patch, dir, testState{workDir: dir})
	if err != nil {
		t.Fatalf("ApplyPatchText: %v", err)
	}
	if !strings.Contains(result.Content, "main.go") || !strings.Contains(result.Content, "added.txt") {
		t.Fatalf("result content did not list changed files: %s", result.Content)
	}
	mainData, err := os.ReadFile(filepath.Join(dir, "main.go"))
	if err != nil {
		t.Fatal(err)
	}
	if got := string(mainData); !strings.Contains(got, `println("new")`) {
		t.Fatalf("main.go not updated:\n%s", got)
	}
	addedData, err := os.ReadFile(filepath.Join(dir, "added.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if got := string(addedData); got != "created\n" {
		t.Fatalf("added.txt = %q", got)
	}
	if _, err := os.Stat(filepath.Join(dir, "old.txt")); !os.IsNotExist(err) {
		t.Fatalf("old.txt still exists or stat failed with unexpected error: %v", err)
	}
}

func TestApplyPatchRejectsStaleUpdateWithoutMutation(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "main.go")
	original := "package main\n\nfunc main() {\n\tprintln(\"current\")\n}\n"
	if err := os.WriteFile(path, []byte(original), 0644); err != nil {
		t.Fatal(err)
	}

	patch := `*** Begin Patch
*** Update File: main.go
@@
 func main() {
-	println("old")
+	println("new")
 }
*** End Patch`

	_, err := ApplyPatchText(context.Background(), patch, dir, testState{workDir: dir})
	if err == nil {
		t.Fatal("expected stale update to fail")
	}
	data, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if got := string(data); got != original {
		t.Fatalf("file mutated after failed patch:\n%s", got)
	}
}

func TestToolInvokeRequiresPatchJSON(t *testing.T) {
	dir := t.TempDir()
	input, _ := json.Marshal(Input{Patch: `*** Begin Patch
*** Add File: x.txt
+x
*** End Patch`})
	result, err := (&Tool{}).Invoke(context.Background(), input, testState{workDir: dir})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if result.Content == "" {
		t.Fatal("expected tool content")
	}
}

func TestExtractShellApplyPatchRejectsMixedCommands(t *testing.T) {
	_, _, ok, err := ExtractShellApplyPatch("echo before; apply_patch <<'EOF'\n*** Begin Patch\n*** End Patch\nEOF")
	if err == nil {
		t.Fatal("expected mixed command to fail")
	}
	if ok {
		t.Fatal("mixed command should not be ok")
	}
}

func TestExtractShellApplyPatch(t *testing.T) {
	patch, workDir, ok, err := ExtractShellApplyPatch("cd subdir && apply_patch <<'EOF'\n*** Begin Patch\n*** Add File: x.txt\n+x\n*** End Patch\nEOF")
	if err != nil {
		t.Fatalf("ExtractShellApplyPatch: %v", err)
	}
	if !ok {
		t.Fatal("expected ok")
	}
	if workDir != "subdir" {
		t.Fatalf("workDir = %q", workDir)
	}
	if !strings.Contains(patch, "*** Add File: x.txt") {
		t.Fatalf("patch = %q", patch)
	}
}

var _ tool.StateSnapshot = testState{}
