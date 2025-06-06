package project

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Patch merges the receiver Metadata (the current state stored on disk)
// with *other* (a freshly generated snapshot of the workspace).
//
// Behaviour:
//   - The module hierarchy from *other* REPLACES the hierarchy in the
//     receiver – it is considered the source of truth regarding files
//     and token information.
//   - For every module that exists in both trees and whose MD5 checksum
//     remains the same, the Annotation present in the receiver is
//     preserved.
//   - If a module exists in the receiver but not in *other* it is
//     dropped (because the new snapshot no longer sees it).
//   - New modules that appear only in *other* are added with a nil
//     Annotation (it will be generated by annotate later on).
func (m *Metadata) Patch(other *Metadata) {
	if other == nil || other.Modules == nil {
		return // nothing to do
	}

	// Build a lookup map for the *current* (old) module tree so we can
	// quickly fetch annotations when MD5 matches.
	oldMap := map[string]*Module{}
	collectModuleMap(m.Modules, oldMap)

	// Replace the module tree with the fresh snapshot.
	m.Modules = other.Modules

	// Walk the new tree carrying over annotations when applicable.
	mergeAnnotations(m.Modules, oldMap)
}

// collectModuleMap traverses a module tree and records every module by
// its Name into dst.
func collectModuleMap(mod *Module, dst map[string]*Module) {
	if mod == nil {
		return
	}
	dst[mod.Name] = mod
	for _, child := range mod.Modules {
		collectModuleMap(child, dst)
	}
}

// mergeAnnotations walks the freshly generated module tree (fresh) and,
// using oldMap, copies annotations from the previous metadata when the
// module name exists and its MD5 hash is unchanged.
func mergeAnnotations(fresh *Module, oldMap map[string]*Module) {
	if fresh == nil {
		return
	}

	if old, ok := oldMap[fresh.Name]; ok {
		if old.MD5 == fresh.MD5 && old.Annotation != nil {
			fresh.Annotation = old.Annotation
		}
	}
	for _, child := range fresh.Modules {
		mergeAnnotations(child, oldMap)
	}
}

// Update refreshes the .vyb/metadata.yaml content to reflect the current
// workspace state while preserving valid annotations.
//
// Algorithm:
//  1. Load the stored metadata (with annotations).
//  2. Produce a fresh metadata snapshot from the file system.
//  3. Patch the stored metadata with the fresh snapshot.
//  4. Run annotate so missing/invalid annotations are regenerated.
//  5. Persist the updated metadata back to disk.
func Update(projectRoot string) error {
	// Ensure we have an absolute project root path.
	absRoot, err := filepath.Abs(projectRoot)
	if err != nil {
		return fmt.Errorf("failed to determine absolute project root: %w", err)
	}

	rootFS := os.DirFS(absRoot)

	// Step 1 – load existing metadata (with annotations).
	stored, err := loadStoredMetadata(rootFS)
	if err != nil {
		return err
	}

	// Step 2 – build a fresh snapshot.
	fresh, err := buildMetadata(rootFS)
	if err != nil {
		return err
	}

	// Step 3 – patch stored metadata with the fresh structure.
	stored.Patch(fresh)

	// Step 4 – (re)annotate modules missing or with invalid annotations.
	if err := annotate(stored, rootFS); err != nil {
		return err
	}

	// Step 5 – persist back to .vyb/metadata.yaml.
	data, err := yaml.Marshal(stored)
	if err != nil {
		return fmt.Errorf("failed to marshal updated metadata: %w", err)
	}

	metaFilePath := filepath.Join(absRoot, ".vyb", "metadata.yaml")
	if err := os.WriteFile(metaFilePath, data, 0644); err != nil {
		return fmt.Errorf("failed to write updated metadata.yaml: %w", err)
	}

	return nil
}
