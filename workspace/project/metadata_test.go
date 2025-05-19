package project

import (
	"fmt"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"io/fs"
	"testing"
	"testing/fstest"
	"time"
)

func Test_selectForRemoval(t *testing.T) {
	f := fstest.MapFS{
		"root/.vyb/metadata.yaml":           {Data: []byte("root: .")},
		"root/dir1/.vyb/metadata.yaml":      {Data: []byte("root: ../")},
		"root/dir1/dir2/.vyb/metadata.yaml": {Data: []byte("root: ../../")},
		"root/dir3/foo.txt":                 {Data: []byte("...")},
		"root/dir3/dir4/.vyb/metadata.yaml": {Data: []byte("root: ../../")},
	}

	tests := []struct {
		baseDir      string
		validateRoot bool
		wantErr      *WrongRootError
		want         []string
		explanation  string
	}{
		{
			baseDir:      "root/dir3",
			validateRoot: true,
			wantErr:      &WrongRootError{},
			explanation:  "validateRoot is true and no config in given root",
		},
		{
			baseDir:      "root/dir1",
			validateRoot: true,
			wantErr:      newWrongRootErr("../"),
			explanation:  "validateRoot is true and config in given root says project root is in another path",
		},
		{
			baseDir:      "root",
			validateRoot: true,
			want:         []string{".vyb", "dir1/.vyb", "dir1/dir2/.vyb", "dir3/dir4/.vyb"},
			explanation:  "validateRoot is true and config in given root says project root is the given root",
		},
		{
			baseDir:      "root/dir3",
			validateRoot: false,
			want:         []string{"dir4/.vyb"},
			explanation:  "validateRoot is false and no config in given root",
		},
		{
			baseDir:      "root/dir1",
			validateRoot: false,
			want:         []string{".vyb", "dir2/.vyb"},
			explanation:  "validateRoot is false and config in given root says project root is another path",
		},
		{
			baseDir:      "root",
			validateRoot: false,
			want:         []string{".vyb", "dir1/.vyb", "dir1/dir2/.vyb", "dir3/dir4/.vyb"},
			explanation:  "validateRoot is false and config in given root says project root is the given root",
		},
	}

	for i, tc := range tests {
		t.Run(fmt.Sprintf("TestRemove[%d]", i), func(t *testing.T) {
			tcfs, _ := fs.Sub(f, tc.baseDir)
			got, gotErr := findAllConfigWithinRoot(tcfs, tc.validateRoot)

			if tc.wantErr != nil {
				if diff := cmp.Diff(*tc.wantErr, gotErr, cmpopts.EquateEmpty()); diff != "" {
					t.Fatalf("(-want, +got):\n%s", diff)
				}
			} else {
				if diff := cmp.Diff(tc.want, got, cmpopts.EquateEmpty()); diff != "" {
					t.Fatalf("(-want, +got):\n%s", diff)
				}
			}
		})
	}
}

func Test_loadStoredMetadata(t *testing.T) {
	t.Run("Success case", func(t *testing.T) {
		memFS := fstest.MapFS{
			".vyb/metadata.yaml": {
				Data: []byte("root: .\nmodules:\n  name: hello\n"),
			},
		}

		meta, err := loadStoredMetadata(memFS)
		if err != nil {
			t.Fatalf("loadStoredMetadata returned an error: %v", err)
		}
		if meta.Root != "." {
			t.Errorf("expected root '.' , got '%s'", meta.Root)
		}
	})

	t.Run("File not found", func(t *testing.T) {
		memFS := fstest.MapFS{}
		_, err := loadStoredMetadata(memFS)
		if err == nil {
			t.Fatal("expected error for missing metadata.yaml, got nil")
		}
	})
}

func Test_buildMetadata(t *testing.T) {
	memFS := fstest.MapFS{
		"folderA/file1.txt":        {Data: []byte("this is file1"), ModTime: time.Now()},
		"folderA/folderB/file2.md": {Data: []byte("this is file2"), ModTime: time.Now()},
		"folderC/foo.go":           {Data: []byte("package main\nfunc main(){}"), ModTime: time.Now()},
		".git/ignored":             {Data: []byte("should be excluded")},
		"go.sum":                   {Data: []byte("should be excluded")},
	}

	meta, err := buildMetadata(memFS)
	if err != nil {
		t.Fatalf("buildMetadata returned error: %v", err)
	}

	if meta == nil {
		t.Fatal("buildMetadata returned nil metadata")
	}

	want := &Metadata{
		Root: ".",
		Modules: &Module{
			Name: ".",
			Modules: []*Module{
				{
					Name: "folderA",
					Modules: []*Module{
						{
							Name: "folderB",
							Files: []*File{
								{
									Name: "file2.md",
								},
							},
						},
					},
					Files: []*File{
						{
							Name: "file1.txt",
						},
					},
				},
				{
					Name: "folderC",
					Files: []*File{
						{
							Name: "foo.go",
						},
					},
				},
			},
		},
	}

	// Use cmp with custom sorting for modules and files.
	opts := []cmp.Option{
		// We ignore them in structural comparison but will check them below.
		cmpopts.IgnoreFields(File{}, "LastModified", "MD5", "TokenCount"),
		cmpopts.EquateEmpty(),
		cmpopts.SortSlices(func(a, b *Module) bool {
			return a.Name < b.Name
		}),
		cmpopts.SortSlices(func(a, b *File) bool {
			return a.Name < b.Name
		}),
	}

	if diff := cmp.Diff(want, meta, opts...); diff != "" {
		t.Errorf("metadata structure mismatch (-want +got):\n%s", diff)
	}

	checkNonEmptyFields(t, meta.Modules)
}

// checkNonEmptyFields validates that LastModified, MD5, and TokenCount are not empty on all files.
func checkNonEmptyFields(t *testing.T, mod *Module) {
	if mod == nil {
		return
	}
	for _, f := range mod.Files {
		if f.MD5 == "" {
			t.Errorf("File %s has empty MD5", f.Name)
		}
		if f.LastModified.IsZero() {
			t.Errorf("File %s has zero LastModified", f.Name)
		}
		if f.TokenCount < 0 {
			t.Errorf("File %s has negative TokenCount %d", f.Name, f.TokenCount)
		}
	}
	for _, child := range mod.Modules {
		checkNonEmptyFields(t, child)
	}
}
