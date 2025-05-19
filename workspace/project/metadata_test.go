package project

import (
	"fmt"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"io/fs"
	"testing"
	"testing/fstest"
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
			t.Errorf("expected root '.', got '%s'", meta.Root)
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
		"folderA/file1.txt":        {Data: []byte("this is file1")},
		"folderA/folderB/file2.md": {Data: []byte("this is file2")},
		"folderC/foo.go":           {Data: []byte("package main\nfunc main(){}")},
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

	if meta.Root != "." {
		t.Errorf("expected root to be '.' but got %q", meta.Root)
	}

	// We expect a top-level module named '.'
	if meta.Modules.Name != "." {
		t.Errorf("expected top-level module name '.' but got %q", meta.Modules.Name)
	}

	// TODO(vyb): replace all the logic from here to the end of this test function, by using cmp.Diff (see examples in other test functions in the project).
	// We'll define a helper to find a submodule by name
	findModule := func(mods []*Module, name string) *Module {
		for _, m := range mods {
			if m.Name == name {
				return m
			}
		}
		return nil
	}

	// We expect 2 submodules: folderA, folderC
	if len(meta.Modules.Modules) != 2 {
		t.Fatalf("expected 2 submodules, got %d", len(meta.Modules.Modules))
	}

	folderA := findModule(meta.Modules.Modules, "folderA")
	folderC := findModule(meta.Modules.Modules, "folderC")

	if folderA == nil {
		t.Error("missing folderA submodule")
	}
	if folderC == nil {
		t.Error("missing folderC submodule")
	}

	// Check folderA
	if folderA != nil {
		if folderA.Name != "folderA" {
			t.Errorf("expected folderA name, got %q", folderA.Name)
		}
		if len(folderA.Files) != 1 {
			t.Errorf("expected 1 file in folderA, got %d", len(folderA.Files))
		} else {
			if folderA.Files[0].Name != "file1.txt" {
				t.Errorf("expected file1.txt in folderA, got %q", folderA.Files[0].Name)
			}
		}
		if len(folderA.Modules) != 1 {
			t.Errorf("expected 1 child module in folderA, got %d", len(folderA.Modules))
		} else {
			folderB := folderA.Modules[0]
			if folderB.Name != "folderB" {
				t.Errorf("expected folderB name, got %q", folderB.Name)
			}

			if len(folderB.Files) != 1 {
				t.Errorf("expected 1 file in folderB, got %d", len(folderB.Files))
			} else {
				if folderB.Files[0].Name != "file2.md" {
					t.Errorf("expected file2.md in folderB, got %q", folderB.Files[0].Name)
				}
			}
		}
	}

	// Check folderC
	if folderC != nil {
		if folderC.Name != "folderC" {
			t.Errorf("expected folderC name, got %q", folderC.Name)
		}
		if len(folderC.Files) != 1 {
			t.Errorf("expected 1 file in folderC, got %d", len(folderC.Files))
		} else {
			if folderC.Files[0].Name != "foo.go" {
				t.Errorf("expected foo.go in folderC, got %q", folderC.Files[0].Name)
			}
		}
		if len(folderC.Modules) != 0 {
			t.Errorf("expected no child modules in folderC, got %d", len(folderC.Modules))
		}
	}
}
