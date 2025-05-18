package summary

import (
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/tiktoken-go/tokenizer"
)

// NodeType represents the type of a node (folder or file).
type NodeType int

const (
	Folder NodeType = iota
	File
)

// Node is the public interface for both files and folders.
type Node interface {
	Name() string
	Type() NodeType
	Parent() Node
	Children() []Node
	TokenCount() int
}

// folderNode represents a folder.
type folderNode struct {
	name       string
	parent     Node
	children   []Node
	fsys       fs.FS
	tokenCount *int
}

// fileNode represents a file.
type fileNode struct {
	name       string
	parent     Node
	filePath   string
	fsys       fs.FS
	tokenCount *int
}

// Verify folderNode and fileNode implement Node.
var (
	_ Node = (*folderNode)(nil)
	_ Node = (*fileNode)(nil)
)

// Name returns the folder name.
func (f *folderNode) Name() string {
	return f.name
}

// Type returns the NodeType for a folder.
func (f *folderNode) Type() NodeType {
	return Folder
}

// Parent returns the parent node.
func (f *folderNode) Parent() Node {
	return f.parent
}

// Children returns the slice of child nodes.
func (f *folderNode) Children() []Node {
	return f.children
}

// TokenCount returns the aggregated token count of all children.
func (f *folderNode) TokenCount() int {
	if f.tokenCount != nil {
		return *f.tokenCount
	}

	sum := 0
	for _, c := range f.children {
		sum += c.TokenCount()
	}
	f.tokenCount = &sum
	return sum
}

// Name returns the file name.
func (f *fileNode) Name() string {
	return f.name
}

// Type returns the NodeType for a file.
func (f *fileNode) Type() NodeType {
	return File
}

// Parent returns the parent node.
func (f *fileNode) Parent() Node {
	return f.parent
}

// Children returns nil for a file.
func (f *fileNode) Children() []Node {
	return nil
}

// TokenCount lazily computes and returns the file's token count.
func (f *fileNode) TokenCount() int {
	if f.tokenCount != nil {
		return *f.tokenCount
	}

	content, err := fs.ReadFile(f.fsys, f.filePath)
	count := 0
	if err == nil {
		tCount, _ := getFileTokenCount(content)
		count = tCount
	}
	f.tokenCount = &count
	return count
}

// BuildTree constructs the hierarchy from a given fs.FS and list of path entries.
// It returns the Node representing the root folder.
func BuildTree(fsys fs.FS, pathEntries []string) (Node, error) {
	rootNode := &folderNode{
		name:   ".",
		parent: nil,
		fsys:   fsys,
	}

	for _, entry := range pathEntries {
		if entry == "" {
			continue
		}

		info, err := fs.Stat(fsys, entry)
		if err != nil {
			return nil, fmt.Errorf("failed to stat path %q: %w", entry, err)
		}

		relPath, err := filepath.Rel(".", entry)
		if err != nil {
			return nil, err
		}

		// Skip directories in the provided path list.
		if info.IsDir() {
			continue
		}

		fileN := &fileNode{
			name:     filepath.Base(entry),
			filePath: relPath,
			fsys:     fsys,
		}

		parentNode := findOrCreateParentNode(rootNode, relPath)
		if parentNode == nil {
			return nil, fmt.Errorf("failed to find or create parent node for %s", entry)
		}

		// parentNode must be folderNode.
		if pf, ok := parentNode.(*folderNode); ok {
			pf.children = append(pf.children, fileN)
			fileN.parent = pf
		} else {
			return nil, fmt.Errorf("parent node is not a folder for %s", entry)
		}
	}

	collapseFolders(rootNode)

	return rootNode, nil
}

// findOrCreateParentNode navigates from the root node down the path minus the last component.
func findOrCreateParentNode(root Node, relPath string) Node {
	parts := strings.Split(relPath, string(filepath.Separator))
	if len(parts) < 1 {
		return root
	}

	parentParts := parts[:len(parts)-1]
	if len(parentParts) == 0 {
		return root
	}

	return navigateOrCreate(root, parentParts)
}

// navigateOrCreate navigates down the tree from the given node, creating new Folder nodes as needed.
func navigateOrCreate(n Node, parts []string) Node {
	if len(parts) == 0 {
		return n
	}

	fn, ok := n.(*folderNode)
	if !ok {
		// The given node is a file, cannot navigate into it.
		return nil
	}

	chunk := parts[0]
	for _, c := range fn.children {
		if c.Name() == chunk && c.Type() == Folder {
			return navigateOrCreate(c, parts[1:])
		}
	}

	sub := &folderNode{
		name:   chunk,
		parent: fn,
		fsys:   fn.fsys,
	}
	fn.children = append(fn.children, sub)
	return navigateOrCreate(sub, parts[1:])
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

// collapseFolders performs in-place collapsing of folders that contain exactly one subfolder and no files.
func collapseFolders(n Node) {
	if n.Type() == File {
		return
	}
	folder := n.(*folderNode)

	for _, c := range folder.children {
		collapseFolders(c)
	}

	// Don't collapse the root node.
	if folder.parent == nil {
		return
	}

	// If we have exactly one child, it's a folder, and we have no other children,
	// we combine them.
	for {
		if len(folder.children) == 1 && folder.children[0].Type() == Folder {
			sub := folder.children[0].(*folderNode)
			folder.name = filepath.Join(folder.name, sub.name)
			folder.children = sub.children
			for _, gc := range folder.children {
				switch node := gc.(type) {
				case *folderNode:
					node.parent = folder
				case *fileNode:
					node.parent = folder
				}
			}
		} else {
			break
		}
	}
}
