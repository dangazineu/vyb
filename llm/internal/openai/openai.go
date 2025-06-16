package openai

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/vybdev/vyb/config"
	"github.com/vybdev/vyb/llm/internal/openai/internal/schema"
	"io"
	"net/http"
	"os"

	"github.com/vybdev/vyb/llm/payload"
	"time"
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

type openaiErrorResponse struct {
	OpenAIError struct {
		Message string `json:"message"`
		Type    string `json:"type"`
		Param   string `json:"param"`
		Code    string `json:"code"`
	} `json:"error"`
}

func (o openaiErrorResponse) Error() string {
	return fmt.Sprintf("OpenAI API error: %s", o.OpenAIError.Message)
}

// -----------------------------------------------------------------------------
//
//	Model resolver
//
// -----------------------------------------------------------------------------
// mapModel converts a generic (family,size) pair into a concrete OpenAI model
// string.  The mapping is local to this provider so business-level code never
// depends on provider-specific identifiers.
func mapModel(fam config.ModelFamily, sz config.ModelSize) (string, error) {
	switch fam {
	case config.ModelFamilyGPT:
		switch sz {
		case config.ModelSizeLarge:
			return "GPT-4.1", nil
		case config.ModelSizeSmall:
			return "GPT-4.1-mini", nil
		}
	case config.ModelFamilyReasoning:
		switch sz {
		case config.ModelSizeLarge:
			return "o3", nil
		case config.ModelSizeSmall:
			return "o4-mini", nil
		}
	}
	return "", fmt.Errorf("openai: unsupported model mapping for family=%s size=%s", fam, sz)
}

// GetModuleContext calls the LLM and returns a parsed ModuleSelfContainedContext
// value using the model derived from family/size.
func GetModuleContext(systemMessage string, request *payload.ModuleContextRequest) (*payload.ModuleSelfContainedContext, error) {
	_ = request // TODO(vyb): serialize request payload
	model := "o4-mini"
	userMessage := ""
	openaiResp, err := callOpenAI(systemMessage, userMessage, schema.GetModuleContextSchema(), model)
	if err != nil {
		var openAIErrResp openaiErrorResponse
		if errors.As(err, &openAIErrResp) {
			if openAIErrResp.OpenAIError.Code == "rate_limit_exceeded" {
				fmt.Printf("Rate limit exceeded, retrying after 30s\n")
				<-time.After(30 * time.Second)
				return GetModuleContext(systemMessage, request)
			}
		}
		return nil, err
	}
	var ctx payload.ModuleSelfContainedContext
	if err := json.Unmarshal([]byte(openaiResp.Choices[0].Message.Content), &ctx); err != nil {
		return nil, err
	}
	return &ctx, nil
}

// GetWorkspaceChangeProposals sends the given messages to the OpenAI API and
// returns the structured workspace change proposal.
func GetWorkspaceChangeProposals(fam config.ModelFamily, sz config.ModelSize, systemMessage string, request *payload.WorkspaceChangeRequest) (*payload.WorkspaceChangeProposal, error) {
	_ = request // TODO(vyb): serialize request payload
	model, err := mapModel(fam, sz)
	if err != nil {
		return nil, err
	}

	userMessage := ""
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

// callOpenAI sends a request to OpenAI, returns the parsed response, and logs
// the request/response pair to a uniquely-named JSON file in the OS temp dir.
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

		var errorResp openaiErrorResponse
		if err := json.Unmarshal(bodyBytes, &errorResp); err != nil {
			fmt.Printf("Response code %d, aborting\nOpenAI API error: %s\n", resp.StatusCode, string(bodyBytes))
			return nil, fmt.Errorf("OpenAI API error: %s", string(bodyBytes))
		}

		return nil, errorResp
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

	// ------------------------------------------------------------
	// Persist request and response to a unique temp-file for debug.
	// ------------------------------------------------------------
	logEntry := struct {
		Request  json.RawMessage `json:"request"`
		Response json.RawMessage `json:"response"`
	}{
		Request:  reqBytes,
		Response: respBytes,
	}

	if logBytes, err := json.MarshalIndent(logEntry, "", "  "); err == nil {
		if f, err := os.CreateTemp("", "vyb-openai-*.json"); err == nil {
			if _, wErr := f.Write(logBytes); wErr == nil {
				_ = f.Close()
			} else {
				fmt.Printf("error writing OpenAI log file: %v\n", wErr)
			}
			fmt.Printf("Wrote OpenAI log file to %s\n", f.Name())
		} else {
			fmt.Printf("error creating OpenAI log file: %v\n", err)
		}
	} else {
			fmt.Printf("error marshalling OpenAI log entry: %v\n", err)
	}

	return &openaiResp, nil
}

// GetModuleExternalContexts calls the LLM and returns a list of external
// context strings – one per module.
func GetModuleExternalContexts(systemMessage string, request *payload.ExternalContextsRequest) (*payload.ModuleExternalContextResponse, error) {
	_ = request // TODO(vyb): serialize request payload
	model := "o4-mini"
	userMessage := ""
	openaiResp, err := callOpenAI(systemMessage, userMessage, schema.GetModuleExternalContextSchema(), model)
	if err != nil {
		return nil, err
	}

	var resp payload.ModuleExternalContextResponse
	if err := json.Unmarshal([]byte(openaiResp.Choices[0].Message.Content), &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}
