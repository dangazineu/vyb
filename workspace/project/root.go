package project

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// isAllowedRelativePath returns true if the provided relative path is allowed to be followed.
// Only upward relative paths (e.g., "..", "../", "../../", etc) are allowed.
func isAllowedRelativePath(rel string) bool {
	// Clean the path and check if it starts with ".."
	clean := filepath.Clean(rel)
	return clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator))
}

// FindRoot inspects the .vyb/metadata.yaml file under the given path and returns an fs.FS
// that points to the project root as configured in the metadata. It returns an error if no
// configuration is found or if the metadata's root field indicates a different project root.
//func FindRoot(projectRoot string) (fs.FS, error) {
//	return findRoot(projectRoot, true)
//}

// FindDistanceToRoot returns the relative distance between the given path and the project root,
// as long as the project root is either the given path or one of its parents.
// For example, if the project root is "parent" and the path is "parent/child", the return value is "..".
// If the path is exactly the project root, it returns ".".
// If the given path is not within the project root, it returns an empty string and an error.
func FindDistanceToRoot(path string) (string, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("failed to get absolute path for %s: %w", path, err)
	}

	// Ascend from absPath to find the project root.
	// The project root is identified by a .vyb/metadata.yaml
	curr := absPath
	var projectRoot string
	found := false
	for {
		metaPath := filepath.Join(curr, ".vyb", "metadata.yaml")
		data, err := os.ReadFile(metaPath)
		if err == nil {
			var m Metadata
			err := yaml.Unmarshal(data, &m)
			if err != nil {
				return "", fmt.Errorf("project root has invalid metadata: %w", err)
			}
			projectRoot = curr
			found = true
			break
		}
		parent := filepath.Dir(curr)
		if parent == curr {
			break
		}
		curr = parent
	}
	if !found {
		return "", fmt.Errorf("given path %s is not within a valid project root", path)
	}

	// Compute the relative path from the given path to the project root.
	// This must be a series of ".." components if absPath is a subdirectory of projectRoot.
	rel, err := filepath.Rel(absPath, projectRoot)
	if err != nil {
		return "", fmt.Errorf("error computing relative path: %w", err)
	}

	if rel == "." {
		return ".", nil
	}

	// Ensure the relative path consists solely of ".." segments.
	parts := strings.Split(rel, string(os.PathSeparator))
	for _, p := range parts {
		if p != ".." {
			return "", fmt.Errorf("given path %s is not within the project root %s", path, projectRoot)
		}
	}

	return rel, nil
}

// findRoot inspects the .vyb/metadata.yaml file under the given path and returns an fs.FS
// that points to the project root as configured in the metadata.
//   - If the given path has a .vyb/metadata.yaml, and its Metadata.Root value is ".",
//     it returns an fs.FS pointing to the given path.
//   - If the given path has a .vyb/metadata.yaml with a Metadata.Root value not equal to ".",
//     and follow is true, it only follows the relative path if it is within the parent hierarchy
//     (i.e.: only "../", "../../", "../../../", etc). Otherwise, it returns a WrongRootError.
//   - If no .vyb/metadata.yaml is found, or if follow is false and the Metadata.Root is not ".",
//     it returns an error.
//func findRoot(projectRoot string, follow bool) (fs.FS, error) {
//	metaPath := filepath.Join(projectRoot, ".vyb", "metadata.yaml")
//	data, err := os.ReadFile(metaPath)
//	if err == nil {
//		var m Metadata
//		if err := yaml.Unmarshal(data, &m); err != nil {
//			return nil, fmt.Errorf("failed to unmarshal metadata.yaml: %w", err)
//		}
//		if m.Root == "." {
//			return os.DirFS(projectRoot), nil
//		} else {
//			if follow {
//				if !isAllowedRelativePath(m.Root) {
//					return nil, newWrongRootErr(m.Root)
//				}
//				newRoot := filepath.Join(projectRoot, m.Root)
//				newRoot = filepath.Clean(newRoot)
//				return findRoot(newRoot, false)
//			}
//			return nil, newWrongRootErr(m.Root)
//		}
//	}
//
//	// If metadata.yaml not found and follow is true, move to parent directory.
//	if follow {
//		parent := filepath.Dir(projectRoot)
//		if parent == projectRoot {
//			return nil, fmt.Errorf("no .vyb/metadata.yaml found, reached filesystem root")
//		}
//		return findRoot(parent, true)
//	}
//	return nil, fmt.Errorf("no .vyb/metadata.yaml found in %s", projectRoot)
//}
