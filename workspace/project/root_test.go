package project

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// createProjectStructure is a utility function that creates files with given content
// relative to the base directory. It creates directories as needed.
func createProjectStructure(base string, files map[string]string) error {
	for relPath, content := range files {
		fullPath := filepath.Join(base, relPath)
		dir := filepath.Dir(fullPath)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", dir, err)
		}
		if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
			return fmt.Errorf("failed to write file %s: %w", fullPath, err)
		}
	}
	return nil
}

func TestFindDistanceToRoot(t *testing.T) {
	tests := []struct {
		name          string
		files         map[string]string // files to create in temp dir
		pathToTest    string            // relative path from temp dir to call FindDistanceToRoot
		wantDistance  string            // expected relative distance (e.g., "." or ".." or "../../" etc)
		wantErrSubstr string            // expected error substring if any error is expected
	}{
		{
			name: "Path is project root",
			files: map[string]string{
				filepath.Join(".vyb", "metadata.yaml"): "root: .\n",
			},
			pathToTest:   ".", // project root itself
			wantDistance: ".",
		},
		{
			name: "Path is one level deeper than project root",
			files: map[string]string{
				filepath.Join(".vyb", "metadata.yaml"): "root: .\n",
				"sub/file.txt":                         "dummy",
			},
			pathToTest:   "sub",
			wantDistance: "..",
		},
		{
			name: "Path is two levels deeper than project root",
			files: map[string]string{
				filepath.Join(".vyb", "metadata.yaml"): "root: .\n",
				"sub/inner/file.txt":                   "dummy",
			},
			pathToTest:   "sub/inner",
			wantDistance: filepath.Join("..", ".."),
		},
		{
			name:          "Not within a valid project root",
			files:         map[string]string{}, // no metadata file anywhere
			pathToTest:    ".",
			wantErrSubstr: "is not within a valid project root",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			base := t.TempDir()
			if err := createProjectStructure(base, tc.files); err != nil {
				t.Fatalf("setup failed: %v", err)
			}
			testPath := filepath.Join(base, tc.pathToTest)
			got, err := FindDistanceToRoot(testPath)
			if tc.wantErrSubstr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tc.wantErrSubstr)
				}
				if !strings.Contains(err.Error(), tc.wantErrSubstr) {
					t.Fatalf("expected error to contain %q, but got: %v", tc.wantErrSubstr, err)
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if got != tc.wantDistance {
					t.Fatalf("expected distance %q, got %q", tc.wantDistance, got)
				}
			}
		})
	}
}

//func TestFindRoot(t *testing.T) {
//	// Table test cases for FindRoot.
//	// Each test case defines:
//	// - name: descriptive test name.
//	// - files: mapping of file paths to content to construct the project structure.
//	// - baseSubDir: subdirectory (relative to the created temp dir) from which to call FindRoot.
//	// - wantErrSubstr: if non-empty, the expected error message must contain this substring.
//	tests := []struct {
//		name          string
//		files         map[string]string
//		baseSubDir    string
//		wantErrSubstr string
//	}{
//		{
//			name: "Valid project: metadata root is '.'",
//			files: map[string]string{
//				filepath.Join(".vyb", "metadata.yaml"): "root: .\n",
//			},
//			baseSubDir: ".",
//		},
//		{
//			name: "Downward relative path is not allowed",
//			files: map[string]string{
//				filepath.Join(".vyb", "metadata.yaml"):                 "root: intermediate\n",
//				filepath.Join("intermediate", ".vyb", "metadata.yaml"): "root: .\n",
//			},
//			baseSubDir:    ".",
//			wantErrSubstr: "Project root is intermediate",
//		},
//		{
//			name:          "No metadata present",
//			files:         map[string]string{},
//			baseSubDir:    ".",
//			wantErrSubstr: "no .vyb/metadata.yaml",
//		},
//		{
//			name: "Wrong root in chain: recursive lookup returns wrong root",
//			files: map[string]string{
//				filepath.Join("project", ".vyb", "metadata.yaml"): "root: ../\n",
//				filepath.Join(".vyb", "metadata.yaml"):            "root: notvalid\n",
//			},
//			baseSubDir:    "project",
//			wantErrSubstr: "Project root is notvalid",
//		},
//	}
//
//	for _, tc := range tests {
//		tc := tc
//		t.Run(tc.name, func(t *testing.T) {
//			base := t.TempDir()
//			// Create the project structure based on the provided files map.
//			if err := createProjectStructure(base, tc.files); err != nil {
//				t.Fatalf("setup failed: %v", err)
//			}
//			// Determine the starting directory.
//			startDir := filepath.Join(base, tc.baseSubDir)
//			fsys, err := FindRoot(startDir)
//			if tc.wantErrSubstr != "" {
//				if err == nil {
//					t.Fatalf("expected error containing %q, got nil", tc.wantErrSubstr)
//				}
//				if !strings.Contains(err.Error(), tc.wantErrSubstr) {
//					t.Fatalf("expected error to contain %q, but got: %v", tc.wantErrSubstr, err)
//				}
//			} else {
//				if err != nil {
//					t.Fatalf("unexpected error: %v", err)
//				}
//				// Validate that fsys in fact is the project root by unmarshalling metadata.yaml into Metadata struct.
//				f, err := fsys.Open(".vyb/metadata.yaml")
//				if err != nil {
//					t.Fatalf("fsys.Open failed: %v", err)
//				}
//				data, err := io.ReadAll(f)
//				if err != nil {
//					t.Fatalf("failed to read metadata.yaml: %v", err)
//				}
//				f.Close()
//
//				var m Metadata
//				if err := yaml.Unmarshal(data, &m); err != nil {
//					t.Fatalf("failed to unmarshal metadata.yaml: %v", err)
//				}
//				if m.Root != "." {
//					t.Fatalf("expected metadata.yaml to have root \".\", got %q", m.Root)
//				}
//			}
//		})
//	}
//}

// TestFindRootDoesntFollowRelativePath verifies that FindRoot returns an error when the metadata's
// relative path does not refer to an allowed upward (parent) directory.
// According to the specification, only relative paths like "../", "../../", etc. are allowed.
//func TestFindRootDoesntFollowRelativePath(t *testing.T) {
//	base := t.TempDir()
//	files := map[string]string{
//		filepath.Join(".vyb", "metadata.yaml"):                 "root: intermediate\n",
//		filepath.Join("intermediate", ".vyb", "metadata.yaml"): "root: .\n",
//	}
//	if err := createProjectStructure(base, files); err != nil {
//		t.Fatalf("setup failed: %v", err)
//	}
//	_, err := FindRoot(base)
//	if err == nil {
//		t.Fatalf("expected error due to downward relative path, got nil")
//	}
//	var wrongRoot WrongRootError
//	if !errors.As(err, &wrongRoot) && !strings.Contains(err.Error(), "Project root is intermediate") {
//		t.Fatalf("expected WrongRootError, got: %v", err)
//	}
//}
