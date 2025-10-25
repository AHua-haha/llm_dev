package model

import (
	"fmt"
	"testing"

	"github.com/sashabaranov/go-openai"
)

func TestLiteLLMClient_Completion(t *testing.T) {
	t.Run("test litellm client", func(t *testing.T) {
		client := LiteLLMClient{
			baseUrl: "http://192.168.65.2:4000",
			ApiKey:  "sk-1234",
		}
		var chatReq ChatCompletionReq
		chatReq.Model = "openrouter/anthropic/claude-3-5-haiku"
		msg := openai.ChatCompletionMessage{
			Role:    "user",
			Content: "who are you",
		}
		chatReq.Messages = append(chatReq.Messages, msg)
		chatReq.Stream = true

		res, err := client.Completion(&chatReq)
		if err != nil {
			fmt.Printf("err: %v\n", err)
			return
		}
		for chunk := range res {
			if chunk.err != nil {
				fmt.Printf("chunk.err: %v\n", chunk.err)
				break
			}
			fmt.Printf("chunk.content: %v\n", chunk.content)
		}
	})
}
