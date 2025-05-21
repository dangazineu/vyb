package payload

import (
	"fmt"
	"io/fs"
	"path/filepath"
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

// ---------------------
//  Data abstractions
// ---------------------

// WorkspaceChangeProposal is a concrete description of proposed workspace
// changes coming from the LLM.
type WorkspaceChangeProposal struct {
	Description string               `json:"description"`
	Summary     string               `json:"summary"`
	Proposals   []FileChangeProposal `json:"proposals"`
}

// GetDescription returns the long description used by the git commit body.
func (w *WorkspaceChangeProposal) GetDescription() string { return w.Description }

// GetSummary returns the short summary used by the git commit title.
func (w *WorkspaceChangeProposal) GetSummary() string { return w.Summary }

// GetProposals exposes the slice of individual file changes.
func (w *WorkspaceChangeProposal) GetProposals() []FileChangeProposal { return w.Proposals }

// FileChangeProposal represents a single file modification.
type FileChangeProposal struct {
	FileName string `json:"file_name"`
	Content  string `json:"content"`
	Delete   bool   `json:"delete"`
}

// GetFileName returns the affected file path.
func (f *FileChangeProposal) GetFileName() string { return f.FileName }

// GetContent returns the new file content (ignored when Delete == true).
func (f *FileChangeProposal) GetContent() string { return f.Content }

// GetDelete indicates whether the file must be deleted.
func (f *FileChangeProposal) GetDelete() bool { return f.Delete }

// ModuleContext captures contextual information for a module.
type ModuleContext struct {
	Name            string `json:"name,omitempty"`
	ExternalContext string `json:"external_context,omitempty"`
	InternalContext string `json:"internal_context,omitempty"`
	PublicContext   string `json:"public_context,omitempty"`
}

func (m *ModuleContext) GetModuleName() string      { return m.Name }
func (m *ModuleContext) GetExternalContext() string { return m.ExternalContext }
func (m *ModuleContext) GetInternalContext() string { return m.InternalContext }
func (m *ModuleContext) GetPublicContext() string   { return m.PublicContext }

type ModuleContextRequest struct {
	FilePaths  []string
	ModuleCtx  *ModuleContext
	SubModules []*ModuleContextRequest
}

type ModuleContextResponse struct {
	Modules []ModuleContext `json:"modules"`
}

// BuildModuleContextUserMessage constructs a Markdown-formatted string that
// includes the content of all files referenced by the provided
// ModuleContextRequest tree.  `projectRoot` is expected to be an fs.FS rooted
// at the workspace root, and every file path contained in the request is
// interpreted as relative to the module to which it belongs.
//
// If any referenced file cannot be read this function returns an error.
func BuildModuleContextUserMessage(projectRoot fs.FS, request *ModuleContextRequest) (string, error) {
	if projectRoot == nil {
		return "", fmt.Errorf("projectRoot fs.FS must not be nil")
	}
	if request == nil {
		return "", fmt.Errorf("ModuleContextRequest must not be nil")
	}

	var sb strings.Builder

	// Recursively walk the ModuleContextRequest tree collecting file entries.
	var walk func(req *ModuleContextRequest, modulePrefix string) error
	walk = func(req *ModuleContextRequest, modulePrefix string) error {
		if req == nil {
			return nil
		}

		// Compute this module's absolute (from project root) path.
		currentPrefix := modulePrefix
		if req.ModuleCtx != nil && req.ModuleCtx.GetModuleName() != "" {
			// When module names already hold the full path we simply adopt it.
			currentPrefix = req.ModuleCtx.GetModuleName()
		}

		// Only emit a module header if we have a path or some context text.
		if currentPrefix != "" && currentPrefix != "." || (req.ModuleCtx != nil && (req.ModuleCtx.GetExternalContext() != "" || req.ModuleCtx.GetInternalContext() != "" || req.ModuleCtx.GetPublicContext() != "")) {
			writeModule(&sb, currentPrefix, req.ModuleCtx)
		}

		// Process files declared in this module.
		for _, relFile := range req.FilePaths {
			fullPath := relFile
			if currentPrefix != "" && currentPrefix != "." && !strings.HasPrefix(relFile, currentPrefix+string(filepath.Separator)) {
				fullPath = filepath.Join(currentPrefix, relFile)
			}

			data, err := fs.ReadFile(projectRoot, fullPath)
			if err != nil {
				return fmt.Errorf("failed to read file %s: %w", fullPath, err)
			}
			writeFile(&sb, fullPath, string(data))
		}

		// Recurse into sub-modules.
		for _, sub := range req.SubModules {
			if err := walk(sub, currentPrefix); err != nil {
				return err
			}
		}
		return nil
	}

	if err := walk(request, ""); err != nil {
		return "", err
	}

	return sb.String(), nil
}

// buildPayload constructs a Markdown payload from a slice of fileEntry.
// Each file is represented with an H1 header for its relative path, followed by a code block.
func buildPayload(files []fileEntry) string {
	var sb strings.Builder
	for _, f := range files {
		writeFile(&sb, f.Path, f.Content)
	}
	return sb.String()
}

func writeModule(sb *strings.Builder, path string, context *ModuleContext) {
	if sb == nil {
		return
	}
	if path == "" && (context == nil || (context.GetExternalContext() == "" && context.GetInternalContext() == "" && context.GetPublicContext() == "")) {
		return
	}
	sb.WriteString(fmt.Sprintf("# %s\n", path))
	if context != nil {
		if context.GetExternalContext() != "" {
			sb.WriteString("# External Context\n")
			sb.WriteString(fmt.Sprintf("%s\n", context.GetExternalContext()))
		}
		if context.GetInternalContext() != "" {
			sb.WriteString("# Internal Context\n")
			sb.WriteString(fmt.Sprintf("%s\n", context.GetInternalContext()))
		}
		if context.GetPublicContext() != "" {
			sb.WriteString("# Public Context\n")
			sb.WriteString(fmt.Sprintf("%s\n", context.GetPublicContext()))
		}
	}
}

func writeFile(sb *strings.Builder, filepath, content string) {
	if sb == nil {
		return
	}
	lang := getLanguageFromFilename(filepath)
	sb.WriteString(fmt.Sprintf("# %s\n", filepath))
	sb.WriteString(fmt.Sprintf("```%s\n", lang))
	sb.WriteString(content)
	// Ensure a trailing newline before closing the code block.
	if !strings.HasSuffix(content, "\n") {
		sb.WriteString("\n")
	}
	sb.WriteString("```\n\n")
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
