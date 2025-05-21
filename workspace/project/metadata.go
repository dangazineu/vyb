package project

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/dangazineu/vyb/workspace/selector"
)

// Metadata represents the project-specific metadata file. Only one Metadata
// file should exist within a given vyb project, and it should be located in
// the .vyb/ directory under the project root directory.
type Metadata struct {
	Modules *Module `yaml:"modules"`
}

// Module represents a hierarchical grouping of information within a vyb
// project structure.
//
// Name now stores the *full* relative path of the module from the workspace
// root – e.g. "dirA/dirB".  The root module has Name equal to ".".
type Module struct {
	Name       string      `yaml:"name"`
	Modules    []*Module   `yaml:"modules"`
	Files      []*FileRef  `yaml:"files"`
	Annotation *Annotation `yaml:"annotation,omitempty"`
}

type FileRef struct {
	// Name holds the full relative path to the file from the workspace root.
	Name         string    `yaml:"name"`
	LastModified time.Time `yaml:"last_modified"`
	TokenCount   int64     `yaml:"token_count"`
	MD5          string    `yaml:"md5"`
}

var systemExclusionPatterns = []string{
	".git/",
	".gitignore",
	".vyb/",
	"LICENSE",
	"go.sum",
}

// Create creates the project metadata configuration at the project root.
// Returns an error if the metadata cannot be created, or if it already exists.
// If a ".vyb" folder exists in the root directory or any of its subdirectories,
// this function returns an error.
func Create(projectRoot string) error {

	rootFS := os.DirFS(projectRoot)
	existingFolders, err := findAllConfigWithinRoot(rootFS)
	if err != nil {
		return err
	}
	if len(existingFolders) > 0 {
		return fmt.Errorf("failed to create a project configuration because there is already a configuration within the given root: %s", existingFolders[0])
	}

	configDir := filepath.Join(projectRoot, ".vyb")
	if err := os.Mkdir(configDir, 0755); err != nil {
		return fmt.Errorf("failed to create .vyb directory: %w", err)
	}

	metadata, err := buildMetadata(rootFS)
	if err != nil {
		return fmt.Errorf("failed to build metadata: %w", err)
	}

	err = annotate(metadata, rootFS)
	if err != nil {
		return fmt.Errorf("failed to annotate metadata: %w", err)
	}

	data, err := yaml.Marshal(metadata)
	if err != nil {
		return fmt.Errorf("failed to marshal metadata.yaml: %w", err)
	}

	metaFilePath := filepath.Join(configDir, "metadata.yaml")
	if err := os.WriteFile(metaFilePath, data, 0644); err != nil {
		return fmt.Errorf("failed to write metadata.yaml: %w", err)
	}

	return nil
}

// buildMetadata builds a metadata representation for the files available in
// the given filesystem
func buildMetadata(fsys fs.FS) (*Metadata, error) {
	selected, err := selector.Select(fsys, "", nil, systemExclusionPatterns, []string{"*"})
	if err != nil {
		return nil, fmt.Errorf("failed during file selection: %w", err)
	}

	rootModule, err := BuildTree(fsys, selected)
	if err != nil {
		return nil, fmt.Errorf("failed to build summary module tree: %w", err)
	}

	metadata := &Metadata{
		Modules: rootModule,
	}
	return metadata, nil
}

// loadStoredMetadata reads the .vyb/metadata.yaml in the given fs.FS.
// It parses its contents into a Metadata struct. If the file is
// not found or if parsing fails, it returns an error.
func loadStoredMetadata(fsys fs.FS) (*Metadata, error) {
	data, err := fs.ReadFile(fsys, ".vyb/metadata.yaml")
	if err != nil {
		return nil, fmt.Errorf("failed to read metadata file .vyb/metadata.yaml: %w", err)
	}

	var meta Metadata
	if err := yaml.Unmarshal(data, &meta); err != nil {
		return nil, fmt.Errorf("failed to unmarshal metadata from .vyb/metadata.yaml: %w", err)
	}

	return &meta, nil
}

// WrongRootError is returned by Remove when the current directory is not a
// valid project root.
type WrongRootError struct {
	Root *string
}

func (w WrongRootError) Error() string {
	if w.Root == nil {
		return "Error: Folder has no project configuration. Project root is unknown."
	}
	return fmt.Sprintf("Error: Removal attempted on a folder that is not configured as the project root. Project root is %s", *w.Root)
}

func newWrongRootErr(root string) *WrongRootError {
	return &WrongRootError{
		Root: &root,
	}
}

// Remove removes the metadata folder directly under a given project root
// directory. projectRoot must point to a directory with a .vyb directory under
// it, otherwise the operation fails.
func Remove(projectRoot string) error {
	absPath, err := filepath.Abs(projectRoot)
	if err != nil {
		return fmt.Errorf("failed to determine absolute path of project root: %w", err)
	}

	configDir := filepath.Join(absPath, ".vyb")
	info, err := os.Stat(configDir)
	if err != nil {
		if os.IsNotExist(err) {
			return newWrongRootErr(absPath)
		}
		return fmt.Errorf("failed to stat .vyb directory: %w", err)
	}

	if !info.IsDir() {
		return newWrongRootErr(absPath)
	}

	if err := os.RemoveAll(configDir); err != nil {
		return fmt.Errorf("failed to remove .vyb directory: %w", err)
	}

	return nil
}

// findAllConfigWithinRoot recursively scans the provided file system for directories named
// ".vyb". It returns a slice of paths (relative to the provided file system's root) where
// ".vyb" directories are found.
func findAllConfigWithinRoot(projectRoot fs.FS) ([]string, error) {
	var matches []string
	err := fs.WalkDir(projectRoot, ".", func(path string, info fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() && info.Name() == ".vyb" {
			matches = append(matches, path)
			return fs.SkipDir
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("error walking the file tree: %w", err)
	}
	sort.Strings(matches)
	return matches, nil
}
