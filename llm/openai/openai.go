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

// GetModuleContext calls the LLM and returns a parsed ModuleContext value.
func GetModuleContext(systemMessage, userMessage string) (*payload.ModuleContextResponse, error) {
	openaiResp, err := callOpenAI(systemMessage, userMessage, schema.GetModuleContextSchema(), "o4-mini")
	if err != nil {
		return nil, err
	}
	var ctx payload.ModuleContextResponse
	if err := json.Unmarshal([]byte(openaiResp.Choices[0].Message.Content), &ctx); err != nil {
		return nil, err
	}
	return &ctx, nil
}

// GetWorkspaceChangeProposals sends the given messages to the OpenAI API and
// returns the structured workspace change proposal.
func GetWorkspaceChangeProposals(systemMessage, userMessage string) (*payload.WorkspaceChangeProposal, error) {
	// as of May 2025, using "o3" model requires verifying your ID with OpenAI.
	model := "o3"

	openaiResp, err := callOpenAI(systemMessage, userMessage, schema.GetWorkspaceChangeProposalSchema(), model)
	if err != nil {
		return nil, err
	}

	var proposal payload.WorkspaceChangeProposal
	if err := json.Unmarshal([]byte(openaiResp.Choices[0].Message.Content), &proposal); err != nil {
		return nil, err
	}
	return &proposal, nil
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
