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

// TODO(vyb): instead of a single Node struct, use two struct types, one to represent Folders, and another to represent Files. Make all field unexported, and only expose the relevant information through a shared common interface that is exported and implemented by both.

// Node represents either a file or folder in the hierarchy.
type Node struct {
	Name     string
	Type     NodeType
	Parent   *Node
	Children []*Node
	filePath string
	fsys     fs.FS
	// TODO(vyb): create a struct for tokenCount, so it starts as nil and only gets assigned when its value is computed, so you don't have to have two variables to accommodate the lazy loading logic.
	tokenCount         int
	tokenCountComputed bool
}

// TokenCount returns the number of tokens in this node.
// For a file, this includes the file's tokens, computed on-demand.
// For a folder, this is the sum of tokens of all descendants.
func (n *Node) TokenCount() int {
	if n.Type == File {
		if !n.tokenCountComputed {
			content, err := fs.ReadFile(n.fsys, n.filePath)
			if err == nil {
				tCount, err := getFileTokenCount(content)
				if err == nil {
					n.tokenCount = tCount
				}
			}
			n.tokenCountComputed = true
		}
		return n.tokenCount
	}

	sum := 0
	for _, child := range n.Children {
		sum += child.TokenCount()
	}
	n.tokenCount = sum
	return n.tokenCount
}

// BuildTree constructs the hierarchy from a given fs.FS and list of path entries.
// It returns the Node representing the root folder.
func BuildTree(fsys fs.FS, pathEntries []string) (*Node, error) {
	// The root folder always gets a node, regardless of the shape of the filesystem
	rootNode := &Node{
		Name:   ".",
		Type:   Folder,
		Parent: nil,
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

		if info.IsDir() {
			continue
		} else {
			n := &Node{
				Name:     filepath.Base(entry),
				Type:     File,
				filePath: relPath,
				fsys:     fsys,
			}

			parentNode := findOrCreateParentNode(rootNode, relPath)
			if parentNode == nil {
				return nil, fmt.Errorf("failed to find or create parent node for %s", entry)
			}
			parentNode.Children = append(parentNode.Children, n)
			n.Parent = parentNode
		}
	}

	// Collapse single-child folders.
	collapseFolders(rootNode)

	return rootNode, nil
}

// findOrCreateParentNode navigates from the root node down the path minus the last component.
// For example, if the path is "dir1/dir2/file.txt", the parent node is the node for "dir1/dir2".
func findOrCreateParentNode(rootNode *Node, relPath string) *Node {
	parts := strings.Split(relPath, string(filepath.Separator))
	if len(parts) < 1 {
		return rootNode
	}

	parentParts := parts[:len(parts)-1]
	if len(parentParts) == 0 {
		return rootNode
	}

	return navigateOrCreate(rootNode, parentParts)
}

// navigateOrCreate navigates down the tree from the given node, creating new Folder nodes as needed.
func navigateOrCreate(node *Node, parts []string) *Node {
	if len(parts) == 0 {
		return node
	}

	chunk := parts[0]
	var child *Node
	for _, c := range node.Children {
		if c.Name == chunk && c.Type == Folder {
			child = c
			break
		}
	}
	if child == nil {
		child = &Node{
			Name:   chunk,
			Type:   Folder,
			Parent: node,
		}
		node.Children = append(node.Children, child)
	}
	return navigateOrCreate(child, parts[1:])
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
func collapseFolders(n *Node) {
	if n.Type == File {
		return
	}

	for _, c := range n.Children {
		collapseFolders(c)
	}

	// Don't collapse the root node.
	if n.Parent == nil {
		return
	}

	// If we have exactly one child, that child is a folder, and we have no other files,
	// we combine them.
	for {
		if len(n.Children) == 1 && n.Children[0].Type == Folder {
			sub := n.Children[0]
			// Combine sub's name with n's.
			n.Name = filepath.Join(n.Name, sub.Name)

			// Move sub's children to n.
			n.Children = sub.Children
			for _, gc := range n.Children {
				gc.Parent = n
			}
		} else {
			break
		}
	}
}
