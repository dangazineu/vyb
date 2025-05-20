package schema

import (
	"embed"
	"encoding/json"
)

//go:embed schemas/*
var embedded embed.FS

// GetWorkspaceChangeProposalSchema reads configuration files from the embedded directory and parses the JSON schema.
func GetWorkspaceChangeProposalSchema() StructuredOutputSchema {
	data, _ := embedded.ReadFile("schemas/workspace_change_proposal_schema.json")
	var resp StructuredOutputSchema
	// this file is embedded, so ignore the error
	_ = json.Unmarshal(data, &resp)
	return resp
}

type StructuredOutputSchema struct {
	Schema JSONSchema `json:"schema,omitempty"`
	Name   string     `json:"name,omitempty"`
	Strict bool       `json:"strict,omitempty"`
}

type JSONSchema struct {
	Description          string                 `json:"description,omitempty"`
	Type                 string                 `json:"type,omitempty"`
	Properties           map[string]*JSONSchema `json:"properties,omitempty"`
	Items                *JSONSchema            `json:"items,omitempty"`
	Required             []string               `json:"required,omitempty"`
	AdditionalProperties bool                   `json:"additionalProperties"`
}
