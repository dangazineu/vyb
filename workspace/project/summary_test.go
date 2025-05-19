package project

import (
	"testing"
	"testing/fstest"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

// We'll reuse the NodeType constants for convenience in the tests
type NodeType int

const (
	Folder NodeType = iota
	F1le
)

// testNode is used to compare the final structure in tests.
type testNode struct {
	Name     string
	Type     NodeType
	Children []*testNode
}

func toTestNode(m *Module) *testNode {
	tn := &testNode{
		Name: m.Name,
		Type: Folder,
	}
	// For each submodule, treat them as children with Folder type
	for _, sm := range m.Modules {
		tn.Children = append(tn.Children, toTestNode(sm))
	}
	// For each file, treat it as a child with F1le type
	for _, f := range m.Files {
		tn.Children = append(tn.Children, &testNode{
			Name: f.Name,
			Type: F1le,
		})
	}
	return tn
}

// totalTokenCount sums up the token counts of all files (recursively)
func totalTokenCount(m *Module) int {
	sum := 0
	for _, f := range m.Files {
		sum += int(f.TokenCount)
	}
	for _, sm := range m.Modules {
		sum += totalTokenCount(sm)
	}
	return sum
}

func TestBuildTree(t *testing.T) {
	memFS := fstest.MapFS{
		"dir1/file1.txt":           {Data: []byte("test file 1")},
		"dir1/dir2/file2.go":       {Data: []byte("package main\n\nfunc main() {}")},
		"dir3/dir4/dir5/file3.txt": {Data: []byte("some content")},
		"dir3/dir4/dir5/file4.txt": {Data: []byte("another file")},
		"dir3/file5.md":            {Data: []byte("# heading\ncontent")},
		"dir3/file6.md":            {Data: []byte("not included in the list of paths")},
	}

	rm, err := BuildTree(memFS, []string{
		"dir1/file1.txt",
		"dir1/dir2/file2.go",
		"dir3/dir4/dir5/file3.txt",
		"dir3/dir4/dir5/file4.txt",
		"dir3/file5.md",
	})
	if err != nil {
		t.Fatalf("error building tree: %v", err)
	}

	if rm == nil {
		t.Fatal("root module is nil")
	}
	if rm.Name != "." {
		t.Errorf("expected root name '.' but got '%s'", rm.Name)
	}

	// Quick check token sums.
	expectedSum := 0
	for range memFS {
		expectedSum++
	}
	tcount := totalTokenCount(rm)
	if tcount < expectedSum {
		t.Errorf("expected token count >= %d, got %d", expectedSum, tcount)
	}

	gotRoot := toTestNode(rm)
	wantRoot := &testNode{
		Name: ".",
		Type: Folder,
		Children: []*testNode{
			{
				Name: "dir1",
				Type: Folder,
				Children: []*testNode{
					{
						Name: "dir2",
						Type: Folder,
						Children: []*testNode{
							{
								Name: "file2.go",
								Type: F1le,
							},
						},
					},
					{
						Name: "file1.txt",
						Type: F1le,
					},
				},
			},
			{
				Name: "dir3",
				Type: Folder,
				Children: []*testNode{
					{
						Name: "dir4/dir5",
						Type: Folder,
						Children: []*testNode{
							{
								Name: "file3.txt",
								Type: F1le,
							},
							{
								Name: "file4.txt",
								Type: F1le,
							},
						},
					},
					{
						Name: "file5.md",
						Type: F1le,
					},
				},
			},
		},
	}

	opts := []cmp.Option{
		cmpopts.SortSlices(func(a, b *testNode) bool {
			return a.Name < b.Name
		}),
	}

	if diff := cmp.Diff(wantRoot, gotRoot, opts...); diff != "" {
		t.Errorf("tree structure mismatch (-want +got):\n%s", diff)
	}
}

func TestCollapseSingleChildFolders(t *testing.T) {
	dirLayout := fstest.MapFS{
		"dirA/dirB/dirC/fileA.txt": {Data: []byte("some data")},
		"dirA/dirB/ignored.txt":    {Data: []byte("this is ignored and should not be included in the final data structure")},
	}

	rm, err := BuildTree(dirLayout, []string{"dirA/dirB/dirC/fileA.txt"})
	if err != nil {
		t.Fatalf("unexpected error building tree: %v", err)
	}

	gotRoot := toTestNode(rm)
	if len(gotRoot.Children) != 1 {
		t.Fatalf("expected 1 child under root, got %d", len(gotRoot.Children))
	}

	folder := gotRoot.Children[0]
	if folder.Name != "dirA/dirB/dirC" {
		t.Errorf("folder name not collapsed properly, got: %s", folder.Name)
	}
}
