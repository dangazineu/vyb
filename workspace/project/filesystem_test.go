package project

import (
	"testing"
	"testing/fstest"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

func TestBuildTree(t *testing.T) {

	maxTokenCountPerModule = 5
	memFS := fstest.MapFS{
		"dir1/file1.txt":           {Data: []byte("test file 1")},
		"dir1/dir2/file2.go":       {Data: []byte("package main\n\nfunc main() {}")},
		"dir3/dir4/dir5/file3.txt": {Data: []byte("some content")},
		"dir3/dir4/dir5/file4.txt": {Data: []byte("another file")},
		"dir3/file5.md":            {Data: []byte("# heading\ncontent")},
		"dir3/file6.md":            {Data: []byte("not included in the list of paths")},
	}

	rm, err := buildModuleFromFS(memFS, []string{
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
	var expectedSum int64 = 0
	for range memFS {
		expectedSum++
	}
	if rm.TokenCount < expectedSum {
		t.Errorf("expected token count >= %d, got %d", expectedSum, rm.TokenCount)
	}

	// Expected module hierarchy.
	wantRoot := &Module{
		Name: ".",
		Modules: []*Module{
			{
				Name: "dir1",
				Modules: []*Module{
					{
						Name: "dir1/dir2",
						Files: []*FileRef{
							{Name: "dir1/dir2/file2.go"},
						},
					},
				},
				Files: []*FileRef{
					{Name: "dir1/file1.txt"},
				},
			},
			{
				Name: "dir3",
				Modules: []*Module{
					{
						Name: "dir3/dir4/dir5",
						Files: []*FileRef{
							{Name: "dir3/dir4/dir5/file3.txt"},
							{Name: "dir3/dir4/dir5/file4.txt"},
						},
					},
				},
				Files: []*FileRef{
					{Name: "dir3/file5.md"},
				},
			},
		},
	}

	opts := []cmp.Option{
		cmpopts.IgnoreFields(FileRef{}, "LastModified", "MD5", "TokenCount"),
		cmpopts.IgnoreFields(Module{}, "MD5", "TokenCount", "childrenMD5", "localTokenCount", "Annotation"),
		cmpopts.IgnoreUnexported(Module{}),
		cmpopts.EquateEmpty(),
		// Sort slices for deterministic comparison.
		cmpopts.SortSlices(func(a, b *Module) bool { return a.Name < b.Name }),
		cmpopts.SortSlices(func(a, b *FileRef) bool { return a.Name < b.Name }),
	}

	if diff := cmp.Diff(wantRoot, rm, opts...); diff != "" {
		t.Errorf("tree structure mismatch (-want +got):\n%s", diff)
	}
}

func TestCollapseSingleChildFolders(t *testing.T) {
	dirLayout := fstest.MapFS{
		"dirA/dirB/dirC/fileA.txt": {Data: []byte("some data")},
		"dirA/dirB/ignored.txt":    {Data: []byte("this file is ignored and should not be included in the final data structure")},
	}

	rm, err := buildModuleFromFS(dirLayout, []string{"dirA/dirB/dirC/fileA.txt"})
	if err != nil {
		t.Fatalf("unexpected error building tree: %v", err)
	}

	if len(rm.Modules) != 1 {
		t.Fatalf("expected 1 child under root, got %d", len(rm.Modules))
	}

	folder := rm.Modules[0]
	if folder.Name != "dirA/dirB/dirC" {
		t.Errorf("unexpected folder name after collapsing, got: %s", folder.Name)
	}

	if len(folder.Files) != 1 {
		t.Fatalf("expected 1 file inside collapsed folder, got %d", len(folder.Files))
	}
	if folder.Files[0].Name != "dirA/dirB/dirC/fileA.txt" {
		t.Errorf("unexpected file path, got %s", folder.Files[0].Name)
	}
}
