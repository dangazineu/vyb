package project

import (
	"fmt"
	"github.com/dangazineu/vyb/llm/payload"
	"os"

	"github.com/dangazineu/vyb/llm/openai"
)

// Annotation holds context and summary for a Module.
type Annotation struct {
	//ExternalContext is an LLM-provided textual description of the context in which a given Module exists.
	ExternalContext string `yaml:"external-context"`
	//InternalContext is an LLM-provided textual description of the content that lives within a given Module.
	InternalContext string `yaml:"internal-context"`
	//PublicContext is an LLM-provided textual description of content that his Module exposes for other modules to use.
	PublicContext string `yaml:"public-context"`
}

// annotate navigates the modules graph, starting from the leaf-most
// modules back to the root. For each module that has no Annotation, it calls
// createAnnotation for it after all its submodules are annotated. The creation of
// annotations is performed in parallel using goroutines.
func annotate(metadata *Metadata) error {
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

	// TODO(vyb): pass the right parameters to payload.BuildModuleContextUserMessage.
	userMsg, _ := payload.BuildModuleContextUserMessage(nil, nil)

	// System prompt instructing the LLM to summarize code into JSON schema.
	systemMessage := `You are a summarizer. Please read the following code and produce a short and long description in JSON.
Your output must match this JSON schema, with "proposals" set to an empty array.
Use "summary" for a one-liner, and "description" for a paragraph.`

	// Call OpenAI to get the workspace change proposal containing summary and description.
	context, err := openai.GetModuleContext(systemMessage, userMsg)
	if err != nil {
		return Annotation{}, fmt.Errorf("failed to call openAI: %w", err)
	}

	// Populate the Annotation: ExternalContext holds the paragraph (description), InternalContext holds the one-liner.
	ann := Annotation{
		ExternalContext: context.GetExternalContext(),
		InternalContext: context.GetInternalContext(),
		PublicContext:   context.GetPublicContext(),
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
