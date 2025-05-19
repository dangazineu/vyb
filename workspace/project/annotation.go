package project

import (
	"fmt"
	"math/rand"
	"sync"
)

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

	var wg sync.WaitGroup
	errCh := make(chan error, len(modules))

	for _, mod := range modules {
		if mod.Annotation.Context == "" && mod.Annotation.Summary == "" {
			wg.Add(1)
			go func(m *Module) {
				defer wg.Done()
				ann, err := createAnnotation(m)
				if err != nil {
					errCh <- fmt.Errorf("failed to create annotation for module %q: %w", m.Name, err)
					return
				}
				m.Annotation = ann
			}(mod)
		}
	}

	wg.Wait()
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

// createAnnotation calls OpenAI with all files in a given module, and asks it to summarize the contents of the module, then adds the result in the Summary field of the annotation.
func createAnnotation(m *Module) (Annotation, error) {
	//TODO(vyb): implement this function according to the description.
}
