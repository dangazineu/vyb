package summary

import (
	"testing"
	"testing/fstest"
)

func TestBuildTree(t *testing.T) {
	memFS := fstest.MapFS{
		"dir1/file1.txt":           {Data: []byte("test file 1")},
		"dir1/dir2/file2.go":       {Data: []byte("package main\n\nfunc main() {}")},
		"dir3/dir4/dir5/file3.txt": {Data: []byte("some content")},
		"dir3/dir4/dir5/file4.txt": {Data: []byte("another file")},
		"dir3/file5.md":            {Data: []byte("# heading\ncontent")},
	}

	rootNode, err := BuildTree(memFS, ".")
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
	for _, _ = range memFS {
		// tokens are not necessarily known exactly, but let's just ensure it's > 0.
		expectedSum += 1
	}
	if rootNode.TokenCount() < expectedSum {
		t.Errorf("expected token count >= %d, got %d", expectedSum, rootNode.TokenCount())
	}
}

func TestCollapseSingleChildFolders(t *testing.T) {
	// We'll create a structure with multiple nested single-child folders.
	dirLayout := fstest.MapFS{
		"root/dirA/dirB/dirC/fileA.txt": {Data: []byte("some data")},
	}

	rootNode, err := BuildTree(dirLayout, "root")
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
