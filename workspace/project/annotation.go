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
// createAnnotation for it after all its submodules are annotated. The creation of
// annotations is performed in parallel using goroutines.
func buildSummaries(metadata *Metadata) error {
	if metadata == nil || metadata.Modules == nil {
		return nil
	}

	// Collect modules in post-order so children come before parents.
	modules := collectModulesInPostOrder(metadata.Modules)
	// Channel to collect errors from annotation goroutines.
	errCh := make(chan error, len(modules))
	// Create a done channel for each module to signal completion of annotation.
	dones := make(map[*Module]chan struct{})
	for _, m := range modules {
		dones[m] = make(chan struct{})
	}
	// Pre-close done channels for modules already annotated.
	for _, m := range modules {
		if m.Annotation != nil {
			close(dones[m])
		}
	}

	// Launch annotation tasks.
	for _, m := range modules {
		if m.Annotation != nil {
			continue
		}
		// Capture m for the goroutine.
		go func(mod *Module) {
			// Wait for all submodules to complete.
			for _, sub := range mod.Modules {
				<-dones[sub]
			}
			// Create annotation.
			ann, err := createAnnotation(mod)
			if err != nil {
				errCh <- fmt.Errorf("failed to create annotation for module %q: %w", mod.Name, err)
				// Signal done to avoid blocking parents.
				close(dones[mod])
				return
			}
			mod.Annotation = &ann
			close(dones[mod])
		}(m)
	}

	// Wait for root module to finish annotation.
	root := metadata.Modules
	<-dones[root]
	close(errCh)

	// Check for errors.
	for err := range errCh {
		if err != nil {
			return err
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
func createAnnotation(m *Module) (Annotation, error) {
	// Gather file paths from this module (including submodules).
	filePaths := gatherModuleFilePaths(m)
	if len(filePaths) == 0 {
		// No files: nothing to summarize.
		return Annotation{}, nil
	}

	// Use current working directory as project root.
	rootFS := os.DirFS(".")

	// Build user message with all these files.
	userMsg, err := payload.BuildUserMessage(rootFS, filePaths)
	if err != nil {
		return Annotation{}, fmt.Errorf("failed to build user message: %w", err)
	}

	// System prompt instructing the LLM to summarize code into JSON schema.
	systemMessage := `You are a summarizer. Please read the following code and produce a short and long description in JSON.
Your output must match this JSON schema, with "proposals" set to an empty array.
Use "summary" for a one-liner, and "description" for a paragraph.`

	// Call OpenAI to get the workspace change proposal containing summary and description.
	proposal, err := openai.GetWorkspaceChangeProposals(systemMessage, userMsg)
	if err != nil {
		return Annotation{}, fmt.Errorf("failed to call openAI: %w", err)
	}

	// Populate the Annotation: Context holds the paragraph (description), Summary holds the one-liner.
	ann := Annotation{
		Context: proposal.GetDescription(),
		Summary: proposal.GetSummary(),
	}
	return ann, nil
}

// gatherModuleFilePaths recursively visits the module and its children, collecting the relative paths.
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
