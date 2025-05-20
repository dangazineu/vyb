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

 type ModuleContextRequest struct {
     FilePaths     []string
     ModuleContext *ModuleContext
     SubModules    []*ModuleContextRequest
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
         if req.ModuleContext != nil && (*req.ModuleContext).GetModuleName() != "" {
             if currentPrefix == "" || currentPrefix == "." {
                 currentPrefix = (*req.ModuleContext).GetModuleName()
             } else {
                 currentPrefix = filepath.Join(currentPrefix, (*req.ModuleContext).GetModuleName())
             }
         }

         // Only emit a module header if we have a non-empty path or some context text.
         var ctx ModuleContext
         if req.ModuleContext != nil {
             ctx = *req.ModuleContext
         }
         if currentPrefix != "" || (ctx != nil && (ctx.GetExternalContext() != "" || ctx.GetInternalContext() != "" || ctx.GetPublicContext() != "")) {
             writeModule(&sb, currentPrefix, ctx)
         }

         // Process files directly declared in this module.
         for _, relFile := range req.FilePaths {
             fullPath := relFile
             if currentPrefix != "" && currentPrefix != "." {
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

 func writeModule(sb *strings.Builder, path string, context ModuleContext) {
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

type ModuleContext interface {
	GetModuleName() string
	GetExternalContext() string
	GetInternalContext() string
	GetPublicContext() string
}
