package summary

import (
	"testing"
	"testing/fstest"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

func TestBuildTree(t *testing.T) {
	memFS := fstest.MapFS{
		"dir1/file1.txt":           {Data: []byte("test file 1")},
		"dir1/dir2/file2.go":       {Data: []byte("package main\n\nfunc main() {}")},
		"dir3/dir4/dir5/file3.txt": {Data: []byte("some content")},
		"dir3/dir4/dir5/file4.txt": {Data: []byte("another file")},
		"dir3/file5.md":            {Data: []byte("# heading\ncontent")},
		"dir3/file6.md":            {Data: []byte("not included in the list of paths")},
	}

	rootNode, err := BuildTree(memFS, []string{
		"dir1/file1.txt",
		"dir1/dir2/file2.go",
		"dir3/dir4/dir5/file3.txt",
		"dir3/dir4/dir5/file4.txt",
		"dir3/file5.md",
	})
	if err != nil {
		t.Fatalf("error building tree: %v", err)
	}

	// We expect a root node named '.'
	if rootNode == nil {
		t.Fatal("root node is nil")
	}
	if rootNode.Name != "." {
		t.Errorf("expected root name '.', got '%s'", rootNode.Name)
	}
	if rootNode.Type != Folder {
		t.Errorf("expected root node to be Folder")
	}

	// Quick check token sums.
	expectedSum := 0
	for range memFS {
		expectedSum++
	}
	if rootNode.TokenCount() < expectedSum {
		t.Errorf("expected token count >= %d, got %d", expectedSum, rootNode.TokenCount())
	}

	// Use cmp.Diff to validate that the tree matches the expected data structure.
	// Ignore token counts and use a sorting function to ensure slices are always in the same order.
	wantRoot := &Node{
		Name: ".",
		Type: Folder,
		Children: []*Node{
			{
				Name: "dir1",
				Type: Folder,
				Children: []*Node{
					{
						Name: "dir2",
						Type: Folder,
						Children: []*Node{
							{
								Name: "file2.go",
								Type: File,
							},
						},
					},
					{
						Name: "file1.txt",
						Type: File,
					},
				},
			},
			{
				Name: "dir3",
				Type: Folder,
				Children: []*Node{
					{
						Name: "dir4/dir5",
						Type: Folder,
						Children: []*Node{
							{
								Name: "file3.txt",
								Type: File,
							},
							{
								Name: "file4.txt",
								Type: File,
							},
						},
					},
					{
						Name: "file5.md",
						Type: File,
					},
				},
			},
		},
	}

	opts := []cmp.Option{
		cmpopts.IgnoreFields(Node{}, "Parent", "tokenCount"),
		cmpopts.SortSlices(func(a, b *Node) bool {
			return a.Name < b.Name
		}),
	}

	if diff := cmp.Diff(wantRoot, rootNode, opts...); diff != "" {
		t.Errorf("tree structure mismatch (-want +got):\n%s", diff)
	}
}

func TestCollapseSingleChildFolders(t *testing.T) {
	dirLayout := fstest.MapFS{
		"dirA/dirB/dirC/fileA.txt": {Data: []byte("some data")},
		"dirA/dirB/ignored.txt":    {Data: []byte("this is ignored and should not be included in the final data structure")},
	}

	rootNode, err := BuildTree(dirLayout, []string{"dirA/dirB/dirC/fileA.txt"})
	if err != nil {
		t.Fatalf("unexpected error building tree: %v", err)
	}

	// We expect the single child folders to be collapsed into 'dirA/dirB/dirC'
	if len(rootNode.Children) != 1 {
		t.Fatalf("expected 1 child under root, got %d", len(rootNode.Children))
	}

	folder := rootNode.Children[0]
	if folder.Name != "dirA/dirB/dirC" {
		t.Errorf("folder name not collapsed properly, got: %s", folder.Name)
	}
}
