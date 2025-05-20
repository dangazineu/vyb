package schema

import (
	"encoding/json"
	"testing"
)

// TestGetResponseSchema loads the JSON schema from the embedded workspace_change_proposal_schema.json
// and prints its content for inspection.
func TestGetResponseSchema(t *testing.T) {
	schema := GetWorkspaceChangeProposalSchema()
	if schema.Name != "workspace_change_proposal" {
		t.Fatalf("Unexpected schema name: %s", schema.Name)
	}

	// Pretty print the schema object using JSON marshalling.
	b, err := json.MarshalIndent(schema, "", "  ")
	if err != nil {
		t.Fatalf("Error marshalling schema: %v", err)
	}
	t.Logf("Loaded JSON Schema:\n%s", string(b))
}
