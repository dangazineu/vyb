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

// Node represents either a file or folder in the hierarchy.
type Node struct {
	Name       string
	Type       NodeType
	Parent     *Node
	Children   []*Node
	tokenCount int // computed lazily for folders if needed
}

// TokenCount returns the number of tokens in this node.
// For a file, this includes the file's tokens.
// For a folder, this is the sum of tokens of all descendants.
func (n *Node) TokenCount() int {
	if n.Type == File {
		return n.tokenCount
	}

	sum := 0
	for _, child := range n.Children {
		sum += child.TokenCount()
	}
	n.tokenCount = sum
	return n.tokenCount
}

// BuildTree constructs the hierarchy from a given fs.FS and root path.
// It returns the Node representing the root folder.
func BuildTree(fsys fs.FS, rootPath string) (*Node, error) {
	rootNode := &Node{
		Name:   rootPath,
		Type:   Folder,
		Parent: nil,
	}

	// Walk the entire fs to list files.
	err := fs.WalkDir(fsys, rootPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		// Skip the root folder itself.
		if path == rootPath {
			return nil
		}

		relPath, err := filepath.Rel(rootPath, path)
		if err != nil {
			return err
		}

		// Build nodes as we discover them.
		if d.IsDir() {
			n := &Node{
				Name:     d.Name(),
				Type:     Folder,
				Children: []*Node{},
			}
			// Insert into parent's children.
			parentNode := findOrCreateParentNode(rootNode, relPath)
			if parentNode == nil {
				return fmt.Errorf("failed to find or create parent node for %s", path)
			}
			parentNode.Children = append(parentNode.Children, n)
			n.Parent = parentNode
		} else {
			fileBytes, err := fs.ReadFile(fsys, path)
			if err != nil {
				return err
			}
			n := &Node{
				Name:     d.Name(),
				Type:     File,
				Children: nil,
				// We'll compute token count from the content.
			}
			n.tokenCount, err = getFileTokenCount(fileBytes)
			if err != nil {
				return err
			}

			parentNode := findOrCreateParentNode(rootNode, relPath)
			if parentNode == nil {
				return fmt.Errorf("failed to find or create parent node for %s", path)
			}
			parentNode.Children = append(parentNode.Children, n)
			n.Parent = parentNode
		}
		return nil
	})

	if err != nil {
		return nil, err
	}

	// Collapse single-child folders.
	collapseFolders(rootNode)

	return rootNode, nil
}

// findOrCreateParentNode navigates from the root node down the path minus the last component.
// For example, if the path is "dir1/dir2/file.txt", the parent node is the node for "dir1/dir2".
func findOrCreateParentNode(rootNode *Node, relPath string) *Node {
	// The parent path in the hierarchy is everything except the last segment.
	// If there's only one segment, the parent is the root.
	parts := strings.Split(relPath, string(filepath.Separator))
	if len(parts) < 1 {
		return rootNode
	}
	parentParts := parts[:len(parts)-1]
	// If there are no parent parts, then the parent is root.
	if len(parentParts) == 0 {
		return rootNode
	}
	// We must navigate down the tree to find or create that folder chain.
	return navigateOrCreate(rootNode, parentParts)
}

// navigateOrCreate navigates down the tree from the given node, creating new Folder nodes as needed.
func navigateOrCreate(node *Node, parts []string) *Node {
	if len(parts) == 0 {
		return node
	}

	chunk := parts[0]
	// Find child node matching chunk.
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

	// Recurse on children first.
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
