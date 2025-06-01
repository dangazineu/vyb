package project

import (
	"bufio"
	"crypto/md5"
	"fmt"
	"io"
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/tiktoken-go/tokenizer"
)

// buildModuleFromFS constructs a hierarchy of Modules and Files for the given path entries.
// It returns the Module representing the root folder.
func buildModuleFromFS(fsys fs.FS, pathEntries []string) (*Module, error) {
	// First, create a basic tree with empty token information so we can easily
	// attach files to the correct folder hierarchy.
	root := &Module{Name: ".", Modules: []*Module{}, Files: []*FileRef{}}

	for _, entry := range pathEntries {
		if entry == "" {
			continue
		}
		info, err := fs.Stat(fsys, entry)
		if err != nil {
			return nil, fmt.Errorf("failed to stat path %q: %w", entry, err)
		}
		// Ignore directories from the incoming list – we only care about files.
		if info.IsDir() {
			continue
		}

		fileRef, err := newFileRefFromFS(fsys, entry)
		if err != nil {
			return nil, fmt.Errorf("failed to build file object for %s: %w", entry, err)
		}
		
		parent := findOrCreateParentModule(root, entry)
		parent.Files = append(parent.Files, fileRef)
	}

	// Collapse trivial single-child folders first.
	collapseModules(root)

	// At this point, we already have all the FileRefs and their token counts.
	// Rebuild the tree using the newModule constructor, so module TokenCounts are computed.

	rebuilt := rebuildModule(root)

	// Now using the tree with token counts, collapse any modules that have fewer tokens than minTokenCountPerModule.
	collapseByTokens(rebuilt)

	// Return a fresh copy of the tree with updated per-module token counts.
	return rebuildModule(rebuilt), nil
}

// newFileRefFromFS creates a *project.FileRef with computed last-modified time, token count, and MD5.
func newFileRefFromFS(fsys fs.FS, relPath string) (*FileRef, error) {
	info, err := fs.Stat(fsys, relPath)
	if err != nil {
		return nil, fmt.Errorf("failed to stat file %s: %w", relPath, err)
	}

	content, err := fs.ReadFile(fsys, relPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file %s: %w", relPath, err)
	}

	tCount, _ := getFileTokenCount(content)

	hash, err := computeMd5(fsys, relPath)
	if err != nil {
		return nil, fmt.Errorf("failed to compute MD5 for %s: %w", relPath, err)
	}

	return newFileRef(relPath, info.ModTime(), int64(tCount), hash), nil
}

// findOrCreateParentModule navigates from the root module down the path minus the last component.
func findOrCreateParentModule(root *Module, relPath string) *Module {
	parts := strings.Split(relPath, string(filepath.Separator))
	if len(parts) < 1 {
		return root
	}

	parentParts := parts[:len(parts)-1]
	if len(parentParts) == 0 {
		return root
	}

	return navigateOrCreateModule(root, parentParts)
}

// navigateOrCreateModule navigates down the tree from the given module, creating new submodules as needed.
func navigateOrCreateModule(m *Module, parts []string) *Module {
	if len(parts) == 0 {
		return m
	}

	chunk := parts[0]

	// Compute the full path for this child module.
	var childFullName string
	if m.Name == "." {
		childFullName = chunk
	} else {
		childFullName = filepath.Join(m.Name, chunk)
	}

	// Try to find an existing submodule with this full name.
	for _, sub := range m.Modules {
		if sub.Name == childFullName {
			return navigateOrCreateModule(sub, parts[1:])
		}
	}

	// Create a new submodule.
	newSub := &Module{
		Name:    childFullName,
		Modules: []*Module{},
		Files:   []*FileRef{},
	}
	m.Modules = append(m.Modules, newSub)
	return navigateOrCreateModule(newSub, parts[1:])
}

// collapseModules performs in-place collapsing of modules that contain exactly one submodule and no files.
func collapseModules(m *Module) {
	// first collapse children
	for _, sub := range m.Modules {
		collapseModules(sub)
	}

	// Don't collapse the root module.
	if m.Name == "." {
		return
	}

	// If we have exactly one child module, no files, then merge.
	for {
		if len(m.Modules) == 1 && len(m.Files) == 0 {
			sub := m.Modules[0]
			m.Name = sub.Name // sub.Name already contains full path
			m.Modules = sub.Modules
			m.Files = sub.Files
		} else {
			break
		}
	}
}

// getFileTokenCount uses the tiktoken-go library to determine the token count.
func getFileTokenCount(content []byte) (int, error) {
	enc, err := tokenizer.Get(tokenizer.Cl100kBase)
	if err != nil {
		return 0, err
	}
	tokens, _, _ := enc.Encode(string(content))
	return len(tokens), nil
}

func computeMd5(fsys fs.FS, path string) (string, error) {
	f, err := fsys.Open(path)
	if err != nil {
		return "", err
	}

	defer f.Close()
	hasher := md5.New()
	_, err = io.Copy(hasher, bufio.NewReader(f))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", hasher.Sum(nil)), nil
}
