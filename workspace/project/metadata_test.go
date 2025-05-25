package project

import (
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"testing"
	"testing/fstest"
	"time"
)

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
		Modules: &Module{
			Name: ".",
			Modules: []*Module{
				{
					Name: "folderA",
					Modules: []*Module{
						{
							Name: "folderA/folderB",
							Files: []*FileRef{
								{
									Name: "folderA/folderB/file2.md",
								},
							},
						},
					},
					Files: []*FileRef{
						{
							Name: "folderA/file1.txt",
						},
					},
				},
				{
					Name: "folderC",
					Files: []*FileRef{
						{
							Name: "folderC/foo.go",
						},
					},
				},
			},
		},
	}

	opts := []cmp.Option{
		// ignore MD5 on both FileRef and Module for structural comparison
		cmpopts.IgnoreFields(FileRef{}, "LastModified", "MD5", "TokenCount"),
		cmpopts.IgnoreFields(Module{}, "MD5", "TokenCount"),
		cmpopts.IgnoreUnexported(Module{}),
		cmpopts.EquateEmpty(),
		cmpopts.SortSlices(func(a, b *Module) bool { return a.Name < b.Name }),
		cmpopts.SortSlices(func(a, b *FileRef) bool { return a.Name < b.Name }),
	}

	if diff := cmp.Diff(want, meta, opts...); diff != "" {
		t.Errorf("metadata structure mismatch (-want +got):\n%s", diff)
	}

	// Validate files and modules have non-empty fields/hashes.
	checkNonEmptyFields(t, meta.Modules)
	checkModuleHashes(t, meta.Modules)
}

// checkNonEmptyFields validates that LastModified, MD5, and TokenCount are not empty on all files.
func checkNonEmptyFields(t *testing.T, mod *Module) {
	if mod == nil {
		return
	}
	for _, f := range mod.Files {
		if f.MD5 == "" {
			t.Errorf("F1le %s has empty MD5", f.Name)
		}
		if f.LastModified.IsZero() {
			t.Errorf("F1le %s has zero LastModified", f.Name)
		}
		if f.TokenCount < 0 {
			t.Errorf("F1le %s has negative TokenCount %d", f.Name, f.TokenCount)
		}
	}
	for _, child := range mod.Modules {
		checkNonEmptyFields(t, child)
	}
}

// checkModuleHashes walks the module tree ensuring every module has a non-empty
// MD5 value.
func checkModuleHashes(t *testing.T, m *Module) {
	if m == nil {
		return
	}
	if m.MD5 == "" {
		t.Errorf("Module %s has empty MD5", m.Name)
	}
	for _, sub := range m.Modules {
		checkModuleHashes(t, sub)
	}
}
