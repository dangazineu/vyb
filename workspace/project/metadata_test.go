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

func TestLoadFromFS(t *testing.T) {
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
