package project

import (
	"fmt"
	"github.com/dangazineu/vyb/llm/openai"
	"github.com/dangazineu/vyb/llm/payload"
	"io/fs"
)

// Annotation holds context and summary for a Module.
// ExternalContext is an LLM-provided textual description of the context in which a given Module exists.
// InternalContext is an LLM-provided textual description of the content that lives within a given Module.
// PublicContext is an LLM-provided textual description of content that his Module exposes for other modules to use.
type Annotation struct {
	ExternalContext string `yaml:"external-context"`
	InternalContext string `yaml:"internal-context"`
	PublicContext   string `yaml:"public-context"`
}

// annotate navigates the modules graph, starting from the leaf-most
// modules back to the root. For each module that has no Annotation, it calls
// annotateModule for it after all its submodules are annotated. The creation of
// annotations is performed in parallel using goroutines.
func annotate(metadata *Metadata, sysfs fs.FS) error {
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
			err := annotateModule(mod, sysfs)
			if err != nil {
				errCh <- fmt.Errorf("failed to create annotation for module %q: %w", mod.Name, err)
				// Signal done to avoid blocking parents.
				close(dones[mod])
				return
			}
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

// buildModuleContextRequest converts a *Module hierarchy to a *payload.ModuleContextRequest tree.
func buildModuleContextRequest(m *Module) *payload.ModuleContextRequest {
	if m == nil {
		return nil
	}

	// Collect file paths relative to this module (just the file names).
	var paths []string
	for _, f := range m.Files {
		paths = append(paths, f.Name)
	}

	// Recursively process sub-modules.
	var subs []*payload.ModuleContextRequest
	for _, sm := range m.Modules {
		subs = append(subs, buildModuleContextRequest(sm))
	}

	// For the root module (name == ".") we omit the ModuleContext so we don’t get a "# ." header.
	var ctxPtr *payload.ModuleContext
	if m.Name != "." {
		ctxPtr = &payload.ModuleContext{Name: m.Name}
	}

	return &payload.ModuleContextRequest{
		FilePaths:  paths,
		ModuleCtx:  ctxPtr,
		SubModules: subs,
	}
}

// annotateModule calls OpenAI with the files contained in a given module, building a summary.
func annotateModule(m *Module, sysfs fs.FS) error {
	// Build the ModuleContextRequest tree starting from this module.
	req := buildModuleContextRequest(m)

	// Construct user message including the files for this module.
	userMsg, err := payload.BuildModuleContextUserMessage(sysfs, req)
	if err != nil {
		return fmt.Errorf("failed to build user message: %w", err)
	}

	// System prompt instructing the LLM to summarize code into JSON schema.
	systemMessage := `You are a prompt engineer, structuring information about an application's code base 
so context can be provided to an LLM in the most efficient way. 
The user message contains information about one or more modules in the application.
A module is a folder with files, and possibly other folders within it. 

Module information includes:

- External context: a description of the context in which the module exists. 
This is used when an LLM prompt is constructed from within the module, 
and doesn't include any additional file or information external to the module, only the "External Context";

- Internal context: a description of the content that lives within the module. 
This is used when an LLM prompt is constructed from a sub-module of this given module, 
and the prompt is too large to include all files within the module. 
So instead of providing all the file contents, the "Internal Context" is used as a summary;  

- Public context: a description of content that this module exposes for other modules to use. 
This should encapsulate not only the contents of the module, but the contents of all its sub-modules.
This is used when the LLM prompt is constructed from a module outside of the hierarchy of this given module.
The "Public Context" can include snippets of interfaces, script parameters, 
or any useful information for the LLM to understand the module.

Each type of context should be as descriptive as possible, using around one thousand LLM tokens, each.

When reviewing this information, you may see modules that only have context, but no file contents, 
and modules that only have file contents but no context. That is because you are building context iteratively, 
and you'll only see files for a few modules at a time. Use the context and file contents that are given to you to 
enrich existing context information and build context where it's missing.
Only change existing context when you believe you have additional information that will materially improve the 
existing context's usability by an LLM. Otherwise, keep the existing context to avoid unnecessary churn. 

Only include in the response the modules that you have changed. But if you change at least one of the context fields
of a given module, include all of them in the response.`

	// Call OpenAI to get the workspace change proposal containing summary and description.
	context, err := openai.GetModuleContext(systemMessage, userMsg)
	if err != nil {
		return fmt.Errorf("failed to call openAI: %w", err)
	}

	// Build a lookup map of module name -> *Module for the subtree rooted at m.
	moduleMap := make(map[string]*Module)
	var walk func(*Module)
	walk = func(mod *Module) {
		moduleMap[mod.Name] = mod
		for _, child := range mod.Modules {
			walk(child)
		}
	}
	walk(m)

	// Iterate over LLM-provided contexts and update corresponding modules.
	for _, mc := range context.Modules {
		if mod, ok := moduleMap[mc.Name]; ok {
			mod.Annotation = &Annotation{
				ExternalContext: mc.ExternalContext,
				InternalContext: mc.InternalContext,
				PublicContext:   mc.PublicContext,
			}
		}
	}

	return nil
}
