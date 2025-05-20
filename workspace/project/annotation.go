package project

import (
	"fmt"
	"os"

	"github.com/dangazineu/vyb/llm/openai"
	"github.com/dangazineu/vyb/llm/payload"
)

// Annotation holds context and summary for a Module.
type Annotation struct {
	//Context is an LLM-provided textual description of the context in which a given Module exists.
	Context string `yaml:"context"`
	//Summary is an LLM-provided textual description of the content that lives within a given Module.
	Summary string `yaml:"summary"`
}

// buildSummaries navigates the modules graph, starting from the leaf-most
// modules back to the root. For each module that has no Annotation, it calls
// a function to create an Annotation for it, and ensures no error was returned.
// The creation of annotations is performed in parallel using goroutines.
func buildSummaries(metadata *Metadata) error {
	if metadata == nil || metadata.Modules == nil {
		return nil
	}

	modules := collectModulesInPostOrder(metadata.Modules)

	errCh := make(chan error, len(modules))

	done := make(chan struct{})

	go func() {
		for _, mod := range modules {
			if mod.Annotation.Context == "" && mod.Annotation.Summary == "" {
				ann, err := createAnnotation(mod)
				if err != nil {
					errCh <- fmt.Errorf("failed to create annotation for module %q: %w", mod.Name, err)
					return
				}
				mod.Annotation = ann
			}
		}
		done <- struct{}{}
	}()

	<-done
	close(errCh)

	for e := range errCh {
		if e != nil {
			return e
		}
	}

	return nil
}

// collectModulesInPostOrder gathers modules in a post-order traversal (children first).
func collectModulesInPostOrder(root *Module) []*Module {
	var result []*Module
	var traverse func(*Module)

	traverse = func(m *Module) {
		for _, sub := range m.Modules {
			traverse(sub)
		}
		result = append(result, m)
	}

	traverse(root)
	return result
}

// createAnnotation calls OpenAI with the files contained in a given module, building a summary.
// The result is stored in Annotation.Context and Annotation.Summary.
// TODO(vyb): Implementation needed. Use selector.Select to gather all files from within each module,
// call openAI with a summarization prompt, parse the returned summary into the summary field in the annotation.
// Use payload.go to convert file contents into a prompt (add textual prompt in the developer message).
// Use o4-mini model
func createAnnotation(m *Module) (Annotation, error) {
	// Gather file paths from this module (including submodules) so we can build a user message.
	filePaths := gatherModuleFilePaths(m)
	if len(filePaths) == 0 {
		// No files: nothing to summarize.
		return Annotation{}, nil
	}

	// We assume the current working directory is the project root.
	rootFS := os.DirFS(".")

	// Build user message with all these files.
	userMsg, err := payload.BuildUserMessage(rootFS, filePaths)
	if err != nil {
		return Annotation{}, fmt.Errorf("failed to build user message: %w", err)
	}

	// We'll craft a short system message that instructs the LLM to summarize.
	// We rely on the existing JSON schema, so we place an empty proposals array.
	// We'll put our short text in 'summary' and a bit longer text in 'description'.
	systemMessage := `You are a summarizer. Please read the following code and produce a short and long description in JSON.\n` +
		`Your output must match this JSON schema, with "proposals" set to an empty array.\n` +
		`Use "summary" for a one-liner, and "description" for a paragraph.\n`

	proposal, err := openai.GetWorkspaceChangeProposals(systemMessage, userMsg)
	if err != nil {
		return Annotation{}, fmt.Errorf("failed to call openAI: %w", err)
	}

	ann := Annotation{
		Context: proposal.GetDescription(),
		Summary: proposal.GetSummary(),
	}

	return ann, nil
}

// gatherModuleFilePaths recursively visits the module and its children, collecting the relative paths.
// The module.Name stores a relative path, and each file's Name is the filename only.
// So we reconstruct full paths by combining the module's path with the file's name.
func gatherModuleFilePaths(m *Module) []string {
	var results []string
	var walk func(mod *Module, prefix string)

	walk = func(mod *Module, prefix string) {
		for _, f := range mod.Files {
			results = append(results, joinPath(prefix, f.Name))
		}
		for _, sub := range mod.Modules {
			newPrefix := joinPath(prefix, sub.Name)
			walk(sub, newPrefix)
		}
	}

	walk(m, m.Name)
	return results
}

func joinPath(prefix, name string) string {
	if prefix == "." || prefix == "" {
		return name
	}
	return prefix + string(os.PathSeparator) + name
}
