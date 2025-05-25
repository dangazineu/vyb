package project

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
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

func newModule(name string, modules []*Module, files []*FileRef, annotation *Annotation) *Module {
	return &Module{
		Name:            name,
		Modules:         modules,
		Files:           files,
		Annotation:      annotation,
		MD5:             computeHashFromAnnotation(annotation), // Annotation MD5 filled later when annotation exists.
		childrenMD5:     computeHashFromChildren(modules, files),
		localTokenCount: computeTokenCountFromChildren(nil, files),
		TokenCount:      computeTokenCountFromChildren(modules, files),
	}
}

// Module represents a hierarchical grouping of information within a vyb
// project structure.
type Module struct {
	// Name stores the *full* relative path of the module from the workspace
	// root – e.g. "dirA/dirB".  The root module has Name equal to ".".
	Name            string      `yaml:"name"`
	Modules         []*Module   `yaml:"modules"`
	Files           []*FileRef  `yaml:"files"`
	Annotation      *Annotation `yaml:"annotation,omitempty"`
	TokenCount      int64       `yaml:"token_count"`
	MD5             string      `yaml:"md5"`
	childrenMD5     string      `yaml:"-"`
	localTokenCount int64       `yaml:"-"`
}

//func (m *Module) update() {
//	for _, mod := range m.Modules {
//		mod.update()
//	}
//	m.childrenMD5 = computeHashFromChildren(m.Modules, m.Files)
//
//
//}

func (m *Module) NeedsAnnotationUpdate() bool {
	for _, mod := range m.Modules {
		if mod.NeedsAnnotationUpdate() {
			return true
		}
	}
	if m.childrenMD5 != computeHashFromChildren(m.Modules, m.Files) {
		return true
	}
	if m.Annotation == nil {
		return true
	}
	if m.MD5 != computeHashFromBytes([]byte(m.Annotation.PublicContext)) {
		return true
	}
	return false
}

//func (m *Module) TokenCount() int64 {
//	if m.localTokenCount == nil {
//		var cnt int64
//		for _, file := range m.Files {
//			cnt += file.TokenCount
//		}
//		m.localTokenCount = &cnt
//	}
//	return *m.localTokenCount
//}

func computeTokenCountFromChildren(modules []*Module, files []*FileRef) int64 {
	var count int64
	for _, m := range modules {
		count += m.TokenCount
	}
	for _, f := range files {
		count += f.TokenCount
	}
	return count
}

func computeHashFromChildren(modules []*Module, files []*FileRef) string {
	var hashes []string
	for _, m := range modules {
		hashes = append(hashes, m.MD5)
	}
	for _, f := range files {
		hashes = append(hashes, f.MD5)
	}
	sort.Strings(hashes)
	return computeHashFromBytes([]byte(strings.Join(hashes, "")))
}

var emptyHash = computeHashFromBytes([]byte(""))

func computeHashFromAnnotation(annotation *Annotation) string {
	if annotation == nil {
		return emptyHash
	}
	return computeHashFromBytes([]byte(annotation.PublicContext))
}

func computeHashFromBytes(bytes []byte) string {
	h := md5.Sum(bytes)
	return hex.EncodeToString(h[:])
}

type FileRef struct {
	// Name holds the full relative path to the file from the workspace root.
	Name         string    `yaml:"name"`
	LastModified time.Time `yaml:"last_modified"`
	TokenCount   int64     `yaml:"token_count"`
	MD5          string    `yaml:"md5"`
}

func newFileRef(name string, lastModified time.Time, tokenCount int64, md5 string) *FileRef {
	return &FileRef{
		Name:         name,
		LastModified: lastModified,
		TokenCount:   tokenCount,
		MD5:          md5,
	}
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

	rootModule, err := buildModuleFromFS(fsys, selected)
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

// -------------------- internal helpers --------------------

var minTokenCountPerModule int64 = 10000
var maxTokenCountPerModule int64 = 100000

// collapseByTokens walks the tree bottom-up, merging children whose cumulative
// token counts are < minTokenCountPerModule into their parent when this does not push the
// parent direct token count above maxTokenCountPerModule.
//
// The function mutates the provided module tree.
func collapseByTokens(m *Module) {
	// Recurse first so children are already processed.
	for _, child := range m.Modules {
		collapseByTokens(child)
	}

	// Don't collapse the root module.
	if m.Name == "." {
		return
	}

	// Iterate over children and merge the small ones.
	for i := 0; i < len(m.Modules); {
		child := m.Modules[i]

		if child.localTokenCount < minTokenCountPerModule {
			// Can we merge? Check direct token limit for parent.
			if m.localTokenCount+child.localTokenCount <= maxTokenCountPerModule {
				// Adopt child's files.
				m.Files = append(m.Files, child.Files...)
				// Remove child and adopt its sub-modules.
				m.Modules = append(m.Modules[:i], m.Modules[i+1:]...)
				m.Modules = append(m.Modules, child.Modules...)
				m.localTokenCount += child.localTokenCount
				// Do NOT advance i – re-evaluate new item in same index.
				continue
			}
		}
		i++
	}
}

// rebuildModule converts a pre-existing *Module hierarchy into a new
// tree where each node is produced via newModule so token counts and hashes
// are accurate.
func rebuildModule(old *Module) *Module {
	if old == nil {
		return nil
	}
	// Convert children first.
	var children []*Module
	for _, c := range old.Modules {
		children = append(children, rebuildModule(c))
	}
	return newModule(old.Name, children, old.Files, old.Annotation)
}
