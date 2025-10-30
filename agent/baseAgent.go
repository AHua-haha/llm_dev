package agent

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	ctx "llm_dev/context"
	"llm_dev/model"

	"github.com/rs/zerolog/log"
	"github.com/sashabaranov/go-openai"
)

type Model struct {
	*openai.Client
	apikey  string
	baseUrl string
}

func NewModel(baseurl string, apikey string) *Model {
	cfg := openai.DefaultConfig(apikey)
	cfg.BaseURL = baseurl
	return &Model{
		Client:  openai.NewClientWithConfig(cfg),
		apikey:  apikey,
		baseUrl: baseurl,
	}
}

type history struct {
	userPrompt string
	resp       string
}
type BaseAgent struct {
	model Model

	currentUserPrompt string
	finished          bool

	historyCtx []history
	fileCtxMgr *ctx.FileContentCtxMgr

	toolHandlerMap map[string]model.ToolDef
}

func NewBaseAgent(codebase string, model Model) BaseAgent {
	agent := BaseAgent{
		model:      model,
		fileCtxMgr: ctx.NewFileCtxMgr(codebase),
	}
	return agent
}
func (agent *BaseAgent) registerTool(toolsDef []model.ToolDef) {
	for _, def := range toolsDef {
		_, exist := agent.toolHandlerMap[def.Name]
		if exist {
			log.Error().Any("tool", def.Name).Msg("tool already exist")
		} else {
			agent.toolHandlerMap[def.Name] = def
		}
	}
}

func (agent *BaseAgent) genRequest() (*openai.ChatCompletionRequest, error) {
	req := openai.ChatCompletionRequest{
		Model:  "openrouter/anthropic/claude-3-5-haiku",
		Stream: true,
	}

	var buf bytes.Buffer
	buf.WriteString(systemPompt)
	agent.fileCtxMgr.WriteFileTree(&buf)
	agent.fileCtxMgr.WriteUsedDefs(&buf)
	sysmsg := openai.ChatCompletionMessage{
		Role:    "system",
		Content: buf.String(),
	}
	usermsg := openai.ChatCompletionMessage{
		Role:    "user",
		Content: agent.currentUserPrompt,
	}
	req.Messages = []openai.ChatCompletionMessage{sysmsg, usermsg}
	for _, tool := range agent.toolHandlerMap {
		req.Tools = append(req.Tools, openai.Tool{
			Type:     openai.ToolTypeFunction,
			Function: &tool.FunctionDefinition,
		})
	}
	return &req, nil
}

func (agent *BaseAgent) handleResponse(stream *openai.ChatCompletionStream) {
	defer stream.Close()
	var err error
	var allToolCall []openai.FunctionCall
	var toolCall openai.FunctionCall
	for {
		res, e := stream.Recv()
		if e != nil {
			err = e
			break
		}
		d := res.Choices[0].Delta
		fmt.Print(d.Content)
		for _, call := range d.ToolCalls {
			if call.Function.Name != "" {
				toolCall.Name = call.Function.Name
				allToolCall = append(allToolCall, toolCall)
				toolCall = openai.FunctionCall{}
			}
			if call.Function.Arguments != "" {
				toolCall.Arguments += call.Function.Arguments
			}
		}
	}
	if !errors.Is(err, io.EOF) {
		agent.finished = true
		return
	}
	allToolCall = append(allToolCall, toolCall)
	agent.finished = true
}

func (agent *BaseAgent) newReq(userprompt string) {
	agent.currentUserPrompt = userprompt
	agent.finished = false
	agent.registerTool(agent.fileCtxMgr.GetToolDef())
	for {
		req, err := agent.genRequest()
		if err != nil {
			log.Error().Err(err).Msg("generate llm request failed")
			break
		}
		stream, err := agent.model.CreateChatCompletionStream(context.TODO(), *req)
		if err != nil {
			log.Error().Err(err).Msg("create chat completion stream failed")
			break
		}
		agent.handleResponse(stream)
		if agent.finished {
			break
		}
	}
}
