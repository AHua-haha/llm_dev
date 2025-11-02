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
	history        []openai.ChatCompletionMessage
	preTaskHistory []openai.ChatCompletionMessage
	finished       bool
	fileCtxMgr     *ctx.FileContentCtxMgr
	toolHandlerMap map[string]model.ToolDef
}

func NewAgentContext(preHistory []openai.ChatCompletionMessage, userprompt string, fileCtxMgr *ctx.FileContentCtxMgr) *AgentContext {
	ctx := AgentContext{
		userPrompt:     userprompt,
		finished:       false,
		fileCtxMgr:     fileCtxMgr,
		toolHandlerMap: make(map[string]model.ToolDef),
		preTaskHistory: preHistory,
	}
	ctx.registerTool(fileCtxMgr.GetToolDef())
	return &ctx
}
func (ctx *AgentContext) getResult() []openai.ChatCompletionMessage {
	usermsg := openai.ChatCompletionMessage{
		Role:    "user",
		Content: ctx.userPrompt,
	}
	res := []openai.ChatCompletionMessage{usermsg}
	historyLen := len(ctx.history)
	if historyLen != 0 {
		res = append(res, ctx.history[historyLen-1])
	}
	return res
}
func (ctx *AgentContext) genRequest(sysPrompt string) openai.ChatCompletionRequest {
	req := openai.ChatCompletionRequest{
		Model:  "openrouter/anthropic/claude-sonnet-4",
		Stream: true,
	}

	var buf bytes.Buffer
	buf.WriteString(sysPrompt)
	ctx.writeContext(&buf)
	req.Messages = []openai.ChatCompletionMessage{}
	sysmsg := openai.ChatCompletionMessage{
		Role:    "system",
		Content: buf.String(),
	}
	usermsg := openai.ChatCompletionMessage{
		Role:    "user",
		Content: ctx.userPrompt,
	}
	req.Messages = append(req.Messages, sysmsg)
	req.Messages = append(req.Messages, ctx.preTaskHistory...)
	req.Messages = append(req.Messages, usermsg)
	req.Messages = append(req.Messages, ctx.history...)
	for _, tool := range ctx.toolHandlerMap {
		req.Tools = append(req.Tools, openai.Tool{
			Type:     openai.ToolTypeFunction,
			Function: &tool.FunctionDefinition,
		})
	}
	return req
}
func (ctx *AgentContext) addMessage(msg openai.ChatCompletionMessage) {
	ctx.history = append(ctx.history, msg)
}
func (ctx *AgentContext) done() bool {
	return ctx.finished
}
func (ctx *AgentContext) writeContext(buf *bytes.Buffer) {
	ctx.fileCtxMgr.WriteFileTree(buf)
	ctx.fileCtxMgr.WriteUsedDefs(buf)
	ctx.fileCtxMgr.WriteAutoLoadCtx(buf)
}

func (ctx *AgentContext) toolCall(toolCall openai.ToolCall) (openai.ChatCompletionMessage, error) {
	res := openai.ChatCompletionMessage{
		Role:       "tool",
		ToolCallID: toolCall.ID,
	}

	def, exist := ctx.toolHandlerMap[toolCall.Function.Name]
	if !exist {
		return res, fmt.Errorf("%s tool does not exist", toolCall.Function.Name)
	}
	resStr, err := def.Handler(toolCall.Function.Arguments)
	if err != nil {
		return res, err
	}
	res.Content = resStr
	return res, nil
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

type BaseAgent struct {
	model Model

	history    []openai.ChatCompletionMessage
	fileCtxMgr *ctx.FileContentCtxMgr
}

func NewBaseAgent(codebase string, model Model) BaseAgent {
	agent := BaseAgent{
		model:      model,
		fileCtxMgr: ctx.NewFileCtxMgr(codebase),
	}
	return agent
}

type AggregateChunk struct {
	msg       openai.ChatCompletionMessage
	toolCalls map[int]openai.ToolCall
}

func (self *AggregateChunk) addChunk(delta openai.ChatCompletionStreamChoiceDelta) {
	if delta.Content != "" {
		self.msg.Content += delta.Content
	}
	for _, toolCall := range delta.ToolCalls {
		index := *toolCall.Index
		value, exist := self.toolCalls[index]
		if !exist {
			value = toolCall
		} else {
			value.Function.Arguments += toolCall.Function.Arguments
		}
		self.toolCalls[index] = value
	}
}
func (self *AggregateChunk) res() openai.ChatCompletionMessage {
	self.msg.Role = "assistant"
	for _, toolcall := range self.toolCalls {
		self.msg.ToolCalls = append(self.msg.ToolCalls, toolcall)
	}
	return self.msg
}

func (agent *BaseAgent) handleResponse(stream *openai.ChatCompletionStream, ctx *AgentContext) {
	defer stream.Close()
	var err error
	aggregate := AggregateChunk{
		toolCalls: make(map[int]openai.ToolCall),
	}
	var finishReason openai.FinishReason
	for {
		res, e := stream.Recv()
		if e != nil {
			err = e
			break
		}
		finishReason = res.Choices[0].FinishReason
		d := res.Choices[0].Delta
		if d.Content != "" {
			fmt.Print(d.Content)
		}
		aggregate.addChunk(d)
	}
	fmt.Print("\n\n")
	if !errors.Is(err, io.EOF) {
		ctx.finished = true
		return
	}
	resp := aggregate.res()
	ctx.addMessage(resp)
	for _, toolCall := range resp.ToolCalls {
		msg, err := ctx.toolCall(toolCall)
		ctx.addMessage(msg)
		if err != nil {
			log.Error().Err(err).Any("toolcall", toolCall).Msg("tool call failed")
		} else {
			log.Info().Any("tool call", toolCall).Any("result", msg.Content).Msg("run tool call success")
		}
	}
	if finishReason == openai.FinishReasonStop {
		ctx.finished = true
	}
}

func (agent *BaseAgent) NewUserTask(userprompt string) {
	ctx := NewAgentContext(agent.history, userprompt, agent.fileCtxMgr)
	for {
		var buf bytes.Buffer
		// ctx.fileCtxMgr.WriteUsedDefs(&buf)
		ctx.fileCtxMgr.WriteAutoLoadCtx(&buf)
		fmt.Print(buf.String())
		req := ctx.genRequest(systemPompt)
		stream, err := agent.model.CreateChatCompletionStream(context.TODO(), req)
		if err != nil {
			log.Error().Err(err).Msg("create chat completion stream failed")
			break
		}
		agent.handleResponse(stream, ctx)
		if ctx.done() {
			break
		}
	}
	agent.history = append(agent.history, ctx.getResult()...)
}
