package openai

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/dangazineu/vyb/llm/openai/internal/schema"
	"github.com/dangazineu/vyb/llm/payload"
	"io"
	"net/http"
	"os"
)

// message represents a single message in the chat conversation.
type message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// request defines the request payload sent to the OpenAI API.
type request struct {
	Model          string         `json:"model"`
	Messages       []message      `json:"messages"`
	ResponseFormat responseFormat `json:"response_format"`
}

type responseFormat struct {
	Type       string                        `json:"type"`
	JSONSchema schema.StructuredOutputSchema `json:"json_schema"`
}

// openaiResponse defines the expected response structure from the OpenAI API.
type openaiResponse struct {
	Choices []struct {
		Message message `json:"message"`
	} `json:"choices"`
}

type moduleContext struct {
	Name            string `json:"name,omitempty"`
	ExternalContext string `json:"external_context,omitempty"`
	InternalContext string `json:"internal_context,omitempty"`
	PublicContext   string `json:"public_context,omitempty"`
}

// Ensure moduleContext implements payload.ModuleContext.
var _ payload.ModuleContext = (*moduleContext)(nil)

func (m *moduleContext) GetModuleName() string {
	return m.Name
}

// GetExternalContext returns the external context of the module.
func (m *moduleContext) GetExternalContext() string {
	return m.ExternalContext
}

// GetInternalContext returns the internal context of the module.
func (m *moduleContext) GetInternalContext() string {
	return m.InternalContext
}

// GetPublicContext returns the public context of the module.
func (m *moduleContext) GetPublicContext() string {
	return m.PublicContext
}

// workspaceChangeProposal is an unexported struct backing the WorkspaceChangeProposal interface.
type workspaceChangeProposal struct {
	Description string               `json:"description,omitempty"`
	Summary     string               `json:"summary,omitempty"`
	Proposals   []fileChangeProposal `json:"proposals,omitempty"`
}

// fileChangeProposal is an unexported struct backing the FileChangeProposal interface.
type fileChangeProposal struct {
	Filename string `json:"file_name,omitempty"`
	Content  string `json:"content,omitempty"`
	Delete   bool   `json:"delete,omitempty"`
}

// Ensure workspaceChangeProposal implements WorkspaceChangeProposal.
var _ payload.WorkspaceChangeProposal = (*workspaceChangeProposal)(nil)

// Ensure fileChangeProposal implements FileChangeProposal.
var _ payload.FileChangeProposal = (*fileChangeProposal)(nil)

func (w *workspaceChangeProposal) GetDescription() string {
	return w.Description
}

func (w *workspaceChangeProposal) GetSummary() string {
	return w.Summary
}

func (w *workspaceChangeProposal) GetProposals() []payload.FileChangeProposal {
	fp := make([]payload.FileChangeProposal, len(w.Proposals))
	for i := range w.Proposals {
		fp[i] = &w.Proposals[i]
	}
	return fp
}

func (f *fileChangeProposal) GetFileName() string {
	return f.Filename
}

func (f *fileChangeProposal) GetContent() string {
	return f.Content
}

func (f *fileChangeProposal) GetDelete() bool {
	return f.Delete
}

func GetModuleContext(systemMessage, userMessage string) (payload.ModuleContext, error) {
	openaiResp, err := callOpenAI(systemMessage, userMessage, schema.GetModuleContextSchema(), "o4-mini")
	if err != nil {
		return nil, err
	}
	var context moduleContext
	if err := json.Unmarshal([]byte(openaiResp.Choices[0].Message.Content), &context); err != nil {
		return nil, err
	}

	return &context, nil
}

// GetWorkspaceChangeProposals sends the given developer and user messages to the OpenAI API using the specified model.
// It returns the content of the first message from the API's response.
func GetWorkspaceChangeProposals(systemMessage, userMessage string) (payload.WorkspaceChangeProposal, error) {
	// as of May 2025, using "o3" model requires verifying your ID with OpenAI.
	// See https://platform.openai.com/docs/models/o3 for more details.
	// Switch to "o1" if you have problems running this model -- vyb works better with "o1" than with any "o#-mini"
	model := "o3"

	openaiResp, err := callOpenAI(systemMessage, userMessage, schema.GetWorkspaceChangeProposalSchema(), model)

	if err != nil {
		return nil, err
	}
	var proposedChanges workspaceChangeProposal
	if err := json.Unmarshal([]byte(openaiResp.Choices[0].Message.Content), &proposedChanges); err != nil {
		return nil, err
	}

	return &proposedChanges, nil
}

func callOpenAI(systemMessage, userMessage string, structuredOutput schema.StructuredOutputSchema, model string) (*openaiResponse, error) {
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		return nil, errors.New("OPENAI_API_KEY is not set")
	}

	// Construct request payload.
	reqPayload := request{
		Model: model,
		Messages: []message{
			{
				Role:    "system",
				Content: systemMessage,
			},
			{
				Role:    "user",
				Content: userMessage,
			},
		},
		ResponseFormat: responseFormat{
			Type:       "json_schema",
			JSONSchema: structuredOutput,
		},
	}

	reqBytes, err := json.MarshalIndent(reqPayload, "", "  ")
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("POST", "https://api.openai.com/v1/chat/completions", bytes.NewBuffer(reqBytes))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", apiKey))

	fmt.Printf("About to call OpenAI\n")
	client := &http.Client{}
	resp, err := client.Do(req)
	fmt.Printf("Fininshed calling OpenAI\n")
	if err != nil {
		fmt.Printf("Got an error back %v\n", err)
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		fmt.Printf("Response code %d, aborting\nOpenAI API error: %s\n", resp.StatusCode, string(bodyBytes))
		return nil, fmt.Errorf("OpenAI API error: %s", string(bodyBytes))
	}

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Printf("Error when reading response body%v\n", err)
		return nil, err
	}

	var openaiResp openaiResponse
	if err := json.Unmarshal(respBytes, &openaiResp); err != nil {
		return nil, err
	}

	if len(openaiResp.Choices) == 0 {
		return nil, errors.New("no choices returned from OpenAI")
	}

	return &openaiResp, nil
}
