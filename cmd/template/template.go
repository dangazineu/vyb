package template

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/cbroglie/mustache"
	"github.com/dangazineu/vyb/llm/openai"
	"github.com/dangazineu/vyb/llm/payload"
	"github.com/dangazineu/vyb/workspace/matcher"
	"github.com/dangazineu/vyb/workspace/project"
	"github.com/dangazineu/vyb/workspace/selector"
	"github.com/spf13/cobra"
)

var systemExclusionPatterns = []string{
	".git/",
	".gitignore",
	".vyb/",
	// the following files are excluded here just temporarily until a .vybignore logic is implemented
	"LICENSE",
	"go.sum",
}

type Definition struct {
	Name  string `yaml:"name"`
	Model string `yaml:"model"`

	// ArgExclusionPatterns specifies patterns for files that should be excluded as command arguments.
	ArgExclusionPatterns []string `yaml:"argExclusionPatterns"`

	// ArgInclusionPatterns specifies a list of matching patterns for files that can be used as command arguments.
	ArgInclusionPatterns []string `yaml:"argInclusionPatterns"`

	// RequestExclusionPatterns specifies patterns for files that should be excluded from the request payload.
	RequestExclusionPatterns []string `yaml:"requestExclusionPatterns"`
	// RequestInclusionPatterns specifies patterns for files that should be included in the request payload.
	RequestInclusionPatterns []string `yaml:"requestInclusionPatterns"`

	// ModificationExclusionPatterns specifies patterns for files that should never be modified when executing this command.
	ModificationExclusionPatterns []string `yaml:"modificationExclusionPatterns"`
	// ModificationInclusionPatterns specifies patterns for files that could be modified when executing this command.
	ModificationInclusionPatterns []string `yaml:"modificationInclusionPatterns"`

	// Prompt specifies the command-specific user prompt that should be included in the LLM request
	Prompt string `yaml:"prompt"`
	// TargetSpecificPrompt specifies additional instructions to be included in the user prompt, if a target is provided.
	TargetSpecificPrompt string `yaml:"targetSpecificPrompt"`
	// ShortDescription is a developer-provided description for the command.
	ShortDescription string `yaml:"shortDescription"`
	// LongDescription is a developer-provided description for the command.
	LongDescription string `yaml:"longDescription"`
}

// prepareExecutionContext extracts all the logic needed to prepare the file selection context.
func prepareExecutionContext(target *string) (absRoot string, relWorkDir string, relTarget *string, err error) {
	absWorkingDir, err := filepath.Abs(".")
	if err != nil {
		return "", "", nil, fmt.Errorf("failed to determine absolute path of working directory: %w", err)
	}
	distToRoot, err := project.FindDistanceToRoot(absWorkingDir)
	if err != nil {
		return "", "", nil, fmt.Errorf("unable to determine project distToRoot: %w", err)
	}

	absRoot, err = filepath.Abs(distToRoot)
	if err != nil {
		return "", "", nil, fmt.Errorf("failed to determine absolute path of project distToRoot %s: %w", distToRoot, err)
	}

	relWorkDir, err = filepath.Rel(absRoot, absWorkingDir)
	if err != nil {
		return "", "", nil, fmt.Errorf("failed to determine relative path: %w", err)
	}

	if target != nil {
		absTarget, err := filepath.Abs(*target)
		if err != nil {
			return "", "", nil, fmt.Errorf("failed to determine absolute path of target %s: %w", *target, err)
		}

		rel, err := filepath.Rel(absRoot, absTarget)
		if err != nil {
			return "", "", nil, fmt.Errorf("failed to determine relative path: %w", err)
		}

		if rel == "" || strings.HasPrefix(rel, "..") {
			return "", "", nil, fmt.Errorf("the target file %s is outside the project distToRoot %s", absTarget, absRoot)
		}

		info, err := os.Stat(absTarget)
		if err != nil || info.IsDir() {
			return "", "", nil, fmt.Errorf("the target %s is not a valid file", absTarget)
		}
		target = &rel
	}
	return absRoot, relWorkDir, target, nil
}

func execute(cmd *cobra.Command, args []string, def *Definition) error {
	if len(def.ArgInclusionPatterns) == 0 && len(args) > 0 {
		return fmt.Errorf("command \"%s\" expects no arguments, but got %v", cmd.Use, args)
	}

	var target *string
	if len(args) > 0 {
		target = &args[0]
	}

	absRoot, relWorkDir, relTarget, err := prepareExecutionContext(target)
	if err != nil {
		return err
	}

	rootFS := os.DirFS(absRoot)

	if relTarget != nil {
		if !matcher.IsIncluded(rootFS, *relTarget, append(systemExclusionPatterns, def.ArgExclusionPatterns...), def.ArgInclusionPatterns) {
			return fmt.Errorf("command \"%s\" does not support given target %s", cmd.Use, *relTarget)
		}
	}

	files, err := selector.Select(rootFS, relWorkDir, relTarget, append(systemExclusionPatterns, def.ArgExclusionPatterns...), def.ArgInclusionPatterns)
	if err != nil {
		return err
	}

	fmt.Printf("The following files will be included in the request:\n")
	for _, file := range files {
		if relTarget != nil && file == *relTarget {
			fmt.Printf("  %s <-- TARGET\n", file)
		} else {
			fmt.Printf("  %s\n", file)
		}
	}

	userMsg, err := payload.BuildUserMessage(rootFS, files)
	if err != nil {
		return err
	}

	promptGeneralInstructions, _ := embedded.ReadFile("embedded/prompts/instructions.md.mustache")
	tmpl, err := mustache.ParseString(string(promptGeneralInstructions))
	if err != nil {
		return err
	}

	rendered, err := tmpl.Render(def)
	if err != nil {
		return err
	}

	systemMessage := rendered

	proposal, err := openai.GetWorkspaceChangeProposals(systemMessage, userMsg)
	if err != nil {
		return err
	}

	// Validate that every file in the proposal is allowed to be modified.
	invalidFiles := []string{}
	for _, prop := range proposal.Proposals {
		if !matcher.IsIncluded(rootFS, prop.FileName, append(systemExclusionPatterns, def.ModificationExclusionPatterns...), def.ModificationInclusionPatterns) {
			invalidFiles = append(invalidFiles, prop.FileName)
		}
	}
	if len(invalidFiles) > 0 {
		return fmt.Errorf("change proposal contains modifications to unallowed files: %v", invalidFiles)
	}

	if err := applyProposals(absRoot, proposal.Proposals); err != nil {
		return err
	}

	fmt.Printf("Change summary: %s\n\n", proposal.Summary)
	fmt.Printf("Change description: %s\n\n", proposal.Description)
	fmt.Printf("Changed files: \n")
	for _, file := range proposal.Proposals {
		fmt.Printf("  %s -- delete? %v\n", file.FileName, file.Delete)
	}

	return nil
}

// applyProposals applies all file modifications as proposed by the LLM.
func applyProposals(absRoot string, proposals []payload.FileChangeProposal) error {
	for _, prop := range proposals {
		absPath := filepath.Join(absRoot, prop.FileName)
		if prop.Delete {
			if err := os.Remove(absPath); err != nil && !os.IsNotExist(err) {
				return fmt.Errorf("failed to delete file %s: %w", absPath, err)
			}
			fmt.Printf("Deleted file: %s\n", prop.FileName)
		} else {
			dir := filepath.Dir(absPath)
			if err := os.MkdirAll(dir, 0755); err != nil {
				return fmt.Errorf("failed to create directory %s: %w", dir, err)
			}
			if err := os.WriteFile(absPath, []byte(prop.Content), 0644); err != nil {
				return fmt.Errorf("failed to write to file %s: %w", absPath, err)
			}
			fmt.Printf("Modified file: %s\n", prop.FileName)
		}
	}
	return nil
}

func Register(rootCmd *cobra.Command) error {
	// Register subcommands.
	defs := load()
	for _, def := range defs {
		rootCmd.AddCommand(&cobra.Command{
			Use:   def.Name,
			Long:  def.LongDescription,
			Short: def.ShortDescription,
			RunE: func(cmd *cobra.Command, args []string) error {
				return execute(cmd, args, def)
			},
		})
	}
	return nil
}
