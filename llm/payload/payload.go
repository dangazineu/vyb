package payload

import (
	"fmt"
	"io/fs"
	"strings"
)

// fileEntry represents a file with its relative path and content.
type fileEntry struct {
	Path    string
	Content string
}

// BuildUserMessage constructs a Markdown-formatted string that includes the content of all files in scope.
// projectRoot represents the base directory for this project, and all file paths in the given filePaths parameter are relative to projectRoot.
func BuildUserMessage(projectRoot fs.FS, filePaths []string) (string, error) {
	var files []fileEntry
	for _, path := range filePaths {
		data, err := fs.ReadFile(projectRoot, path)
		if err != nil {
			return "", err
		}
		files = append(files, fileEntry{
			Path:    path,
			Content: string(data),
		})
	}
	markdown := buildPayload(files)
	return markdown, nil
}

// buildPayload constructs a Markdown payload from a slice of fileEntry.
// Each file is represented with an H1 header for its relative path, followed by a code block.
func buildPayload(files []fileEntry) string {
	var sb strings.Builder
	for _, f := range files {
		// Determine language based on the file extension.
		lang := getLanguageFromFilename(f.Path)
		sb.WriteString(fmt.Sprintf("# %s\n", f.Path))
		sb.WriteString(fmt.Sprintf("```%s\n", lang))
		sb.WriteString(f.Content)
		// Ensure a trailing newline before closing the code block.
		if !strings.HasSuffix(f.Content, "\n") {
			sb.WriteString("\n")
		}
		sb.WriteString("```\n\n")
	}
	return sb.String()
}

// getLanguageFromFilename returns a language identifier based on file extension.
func getLanguageFromFilename(filename string) string {
	if strings.HasSuffix(filename, ".go") {
		return "go"
	} else if strings.HasSuffix(filename, ".md") {
		return "markdown"
	} else if strings.HasSuffix(filename, ".json") {
		return "json"
	} else if strings.HasSuffix(filename, ".txt") {
		return "text"
	}
	// Default: no language specified.
	return ""
}

// WorkspaceChangeProposal is the interface for describing proposed changes.
type WorkspaceChangeProposal interface {
	GetDescription() string
	GetSummary() string
	GetProposals() []FileChangeProposal
}

// FileChangeProposal is the interface for describing a single file change.
type FileChangeProposal interface {
	GetFileName() string
	GetContent() string
	GetDelete() bool
}

//TODO(vyb): create an interface that matches module_summary_schema.json, just like WorkspaceChangeProposal matches workspace+change_proposal_schema.json
