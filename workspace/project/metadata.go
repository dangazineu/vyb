package project

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"time"

	"gopkg.in/yaml.v3"
)

// Metadata represents project-specific metadata.
type Metadata struct {
	// Root determines the relative position of a given Metadata file's .vyb directory and the project root
	Root string `yaml:"root"`

	Modules *Module `yaml:"modules"`
}

// Module represents a hierarchical grouping of information within a vyb project structure. For now, this translates to
// directories under the metadata's parent directory, but this could be changed to represent sets of files or any other
// logical grouping.
type Module struct {
	Name    string    `yaml:"name"`
	Modules []*Module `yaml:"modules"`
	Files   []*File   `yaml:"files"`
}

type File struct {
	Name         string    `yaml:"name"`
	LastModified time.Time `yaml:"last_modified"`
	TokenCount   int64     `yaml:"token_count"`
	MD5          string    `yaml:"md5"`
}

// ConfigFoundError is returned when a project configuration is already found.
// The error indicates that a project configuration already exists. Remove or update the existing
// configuration if necessary.
type ConfigFoundError struct{}

func (e ConfigFoundError) Error() string {
	return "project configuration already exists; remove the existing .vyb folder or update the configuration if necessary"
}

// Create creates the project metadata configuration at the project root.
// Returns an error if the metadata cannot be created, or if it already exists.
// If a ".vyb" folder exists in the root directory or any of its subdirectories,
// this function returns an error.
func Create(projectRoot string) error {

	existingFolders, err := findAllConfigWithinRoot(os.DirFS(projectRoot), false)
	if err != nil {
		return err
	}
	if len(existingFolders) > 0 {
		// Replaced generic error with custom error type.
		return ConfigFoundError{}
	}

	// Create the .vyb directory in the project root.
	configDir := filepath.Join(projectRoot, ".vyb")
	if err := os.Mkdir(configDir, 0755); err != nil {
		return fmt.Errorf("failed to create .vyb directory: %w", err)
	}

	// Write a minimal metadata.yaml file.
	metaContent := "root: .\n"
	metaFilePath := filepath.Join(configDir, "metadata.yaml")
	if err := os.WriteFile(metaFilePath, []byte(metaContent), 0644); err != nil {
		return fmt.Errorf("failed to write metadata.yaml: %w", err)
	}
	return nil
}

// WrongRootError is returned by Remove when the current directory is not a valid project root.
type WrongRootError struct {
	Root *string
}

func (w WrongRootError) Error() string {
	if w.Root == nil {
		return "Error: Removal attempted on a folder with no configuration. Root is unknown."
	}
	return fmt.Sprintf("Error: Removal attempted on a folder that is not configured as the project root. Project root is %s", *w.Root)
}

func newWrongRootErr(root string) *WrongRootError {
	return &WrongRootError{
		Root: &root,
	}
}

// Remove deletes all metadata folders and files, directly and indirectly under a given project root directory.
// It deletes every ".vyb" folder under the project root recursively. Returns an error if any deletion fails.
// When validateRoot is set to true, only performs the removal if a valid Metadata file is stored in a `.vyb` folder
// under the given root directory, and it represents a project root (i.e.: `Root` value is `.`).
func Remove(projectRoot string, validateRoot bool) error {

	absPath, err := filepath.Abs(projectRoot)
	if err != nil {
		return err
	}

	rootFS := os.DirFS(absPath)

	toDelete, err := findAllConfigWithinRoot(rootFS, validateRoot)
	if err != nil {
		return err
	}

	// Remove each found .vyb directory.
	for _, d := range toDelete {
		d = filepath.Join(projectRoot, d)
		if err := os.RemoveAll(d); err != nil {
			return fmt.Errorf("failed to remove %s: %w", d, err)
		}
	}
	return nil
}

// findAllConfigWithinRoot recursively scans the provided file system for directories named ".vyb".
// If validateRoot is true, it ensures that a ".vyb/metadata.yaml" file exists in the root of the provided file system
// and that its Metadata.Root value is exactly ".".
// It returns a slice of paths (relative to the provided file system's root) where ".vyb" directories are found.
func findAllConfigWithinRoot(projectRoot fs.FS, validateRoot bool) ([]string, error) {
	// If validateRoot is true, ensure that there is a .vyb/metadata.yaml file in the project root
	// and that its root field is exactly ".".
	metaPath := filepath.Join(".vyb", "metadata.yaml")

	if validateRoot {
		data, err := fs.ReadFile(projectRoot, metaPath)
		if err != nil {
			return nil, WrongRootError{Root: nil}
		}
		var m Metadata
		if err := yaml.Unmarshal(data, &m); err != nil {
			return nil, fmt.Errorf("failed to unmarshal metadata.yaml: %w", err)
		}
		if m.Root != "." {
			return nil, WrongRootError{Root: &m.Root}
		}
	}

	// Recursively find all directories named ".vyb" under the current working directory.
	var toDelete []string
	err := fs.WalkDir(projectRoot, ".", func(path string, info fs.DirEntry, err error) error {
		if err != nil {
			// Log error and skip this path.
			return nil
		}
		if info.IsDir() && info.Name() == ".vyb" {
			toDelete = append(toDelete, path)
			return fs.SkipDir // Skip processing contents of this directory.
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("error walking the file tree: %w", err)
	}
	sort.Strings(toDelete)
	return toDelete, nil
}
