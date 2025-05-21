package payload

import (
	"testing"
	"testing/fstest"
)

func context(name string) *ModuleContext {
	return &ModuleContext{Name: name}
}
func pcontext(name string) *ModuleContext { return context(name) }

func TestBuildModuleContextUserMessage(t *testing.T) {
	// Files arranged in a nested module hierarchy:
	//  - root.txt (root module / no module name)
	//  - moduleA/a.go
	//  - moduleA/subB/b.md
	mfs := fstest.MapFS{
		"root.txt":          &fstest.MapFile{Data: []byte("root")},
		"moduleA/a.go":      &fstest.MapFile{Data: []byte("package foo\n")},
		"moduleA/subB/b.md": &fstest.MapFile{Data: []byte("Markdown content")},
	}

	// Construct the ModuleContextRequest tree that mirrors the hierarchy.
	req := &ModuleContextRequest{
		FilePaths: []string{"root.txt"},
		SubModules: []*ModuleContextRequest{
			{
				ModuleCtx: pcontext("moduleA"),
				FilePaths: []string{"moduleA/a.go"},
				SubModules: []*ModuleContextRequest{
					{
						ModuleCtx: pcontext("moduleA/subB"),
						FilePaths: []string{"moduleA/subB/b.md"},
					},
				},
			},
		},
	}

	got, err := BuildModuleContextUserMessage(mfs, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Expected payload manually constructed to include module headers.
	expected := "# root.txt\n" +
		"```text\nroot\n```\n\n" +
		"# moduleA\n" +
		"# moduleA/a.go\n```go\npackage foo\n```\n\n" +
		"# moduleA/subB\n" +
		"# moduleA/subB/b.md\n```markdown\nMarkdown content\n```\n\n"

	if got != expected {
		t.Errorf("payload mismatch.\nGot:\n%s\nExpected:\n%s", got, expected)
	}
}

func TestBuildModuleContextUserMessage_FileNotFound(t *testing.T) {
	// Empty filesystem – any file access should fail.
	mfs := fstest.MapFS{}

	req := &ModuleContextRequest{
		FilePaths: []string{"does_not_exist.txt"},
	}

	if _, err := BuildModuleContextUserMessage(mfs, req); err == nil {
		t.Fatalf("expected error for missing file, got nil")
	}
}
