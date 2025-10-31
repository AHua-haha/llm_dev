package model

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"net/http"

	"github.com/sashabaranov/go-openai"
)

var httpClient = &http.Client{}

type StreamRes struct {
	content string
	err     error
}

type ToolHandler func(args string) (string, error)

type ToolDef struct {
	openai.FunctionDefinition
	Handler ToolHandler
}

func SendReq(req *http.Request) (<-chan StreamRes, error) {
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	outputChan := make(chan StreamRes, 10)
	go func() {
		defer resp.Body.Close()
		reader := bufio.NewReader(resp.Body)
		for {
			line, err := reader.ReadString('\n')
			outputChan <- StreamRes{
				content: line,
				err:     err,
			}
			if err != nil {
				break
			}
		}
		close(outputChan)
	}()
	return outputChan, nil
}

type ChatCompletionReq struct {
	openai.ChatCompletionRequest
}

type LiteLLMClient struct {
	baseUrl string
	ApiKey  string
}

func (client *LiteLLMClient) Completion(chatReq *ChatCompletionReq) (<-chan StreamRes, error) {
	url := client.baseUrl + "/chat/completions"
	jsonBody, err := json.Marshal(chatReq)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(context.TODO(), "POST", url, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, err
	}

	// Set required headers for streaming
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")
	httpReq.Header.Set("Cache-Control", "no-cache")
	httpReq.Header.Set("Connection", "keep-alive")
	httpReq.Header.Set("Authorization", "Bearer "+client.ApiKey)

	return SendReq(httpReq)
}
