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

type AgentContext struct {
	userPrompt     string
	finished       bool
	fileCtxMgr     *ctx.FileContentCtxMgr
	toolHandlerMap map[string]model.ToolDef
}

func NewAgentContext(userprompt string, fileCtxMgr *ctx.FileContentCtxMgr) *AgentContext {
	ctx := AgentContext{
		userPrompt:     userprompt,
		finished:       false,
		fileCtxMgr:     fileCtxMgr,
		toolHandlerMap: make(map[string]model.ToolDef),
	}
	ctx.registerTool(fileCtxMgr.GetToolDef())
	return &ctx
}
func (ctx *AgentContext) done() bool {
	return ctx.finished
}
func (ctx *AgentContext) writeContext(buf *bytes.Buffer) {
	ctx.fileCtxMgr.WriteFileTree(buf)
	ctx.fileCtxMgr.WriteUsedDefs(buf)
}

func (ctx *AgentContext) toolCall(toolCall openai.FunctionCall) error {
	def, exist := ctx.toolHandlerMap[toolCall.Name]
	if !exist {
		return fmt.Errorf("%s tool does not exist", toolCall.Name)
	}
	def.Handler(toolCall.Arguments)
	log.Info().Any("toolcall", toolCall).Msg("execute tool call")
	return nil
}

func (ctx *AgentContext) registerTool(tools []model.ToolDef) {
	for _, def := range tools {
		_, exist := ctx.toolHandlerMap[def.Name]
		if exist {
			log.Error().Any("tool", def.Name).Msg("tool already exist")
		} else {
			ctx.toolHandlerMap[def.Name] = def
		}
	}
}

type history struct {
	userPrompt string
	resp       string
}
type BaseAgent struct {
	model Model

	historyCtx []history
	fileCtxMgr *ctx.FileContentCtxMgr
}

func NewBaseAgent(codebase string, model Model) BaseAgent {
	agent := BaseAgent{
		model:      model,
		fileCtxMgr: ctx.NewFileCtxMgr(codebase),
	}
	return agent
}

func (agent *BaseAgent) genRequest(ctx *AgentContext) (*openai.ChatCompletionRequest, error) {
	req := openai.ChatCompletionRequest{
		Model:  "openrouter/anthropic/claude-3-5-haiku",
		Stream: true,
	}

	var buf bytes.Buffer
	buf.WriteString(systemPompt)
	ctx.writeContext(&buf)
	sysmsg := openai.ChatCompletionMessage{
		Role:    "system",
		Content: buf.String(),
	}
	usermsg := openai.ChatCompletionMessage{
		Role:    "user",
		Content: ctx.userPrompt,
	}
	req.Messages = []openai.ChatCompletionMessage{sysmsg, usermsg}
	for _, tool := range ctx.toolHandlerMap {
		req.Tools = append(req.Tools, openai.Tool{
			Type:     openai.ToolTypeFunction,
			Function: &tool.FunctionDefinition,
		})
	}
	return &req, nil
}

func (agent *BaseAgent) handleResponse(stream *openai.ChatCompletionStream, ctx *AgentContext) {
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
	allToolCall = append(allToolCall, toolCall)
	if !errors.Is(err, io.EOF) {
		ctx.finished = true
		return
	}
	for _, toolCall := range allToolCall {
		ctx.toolCall(toolCall)
	}
}

func (agent *BaseAgent) newReq(userprompt string) {
	ctx := NewAgentContext(userprompt, agent.fileCtxMgr)
	for {
		req, err := agent.genRequest(ctx)
		if err != nil {
			log.Error().Err(err).Msg("generate llm request failed")
			break
		}
		stream, err := agent.model.CreateChatCompletionStream(context.TODO(), *req)
		if err != nil {
			log.Error().Err(err).Msg("create chat completion stream failed")
			break
		}
		agent.handleResponse(stream, ctx)
		if ctx.done() {
			break
		}
	}
}
