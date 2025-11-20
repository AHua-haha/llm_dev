package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"llm_dev/codebase/impl"
	ctx "llm_dev/context"
	"llm_dev/database"
	"llm_dev/model"
	"os"
	"strings"

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
	userPrompt    string
	history       []openai.ChatCompletionMessage
	finished      bool
	finalResponse string

	ctxMgr []ctx.ContextMgr

	toolHandlerMap map[string]model.ToolDef
}

func NewAgentContext(userprompt string, ctxMgr ...ctx.ContextMgr) *AgentContext {
	ctx := AgentContext{
		userPrompt:     userprompt,
		finished:       false,
		toolHandlerMap: make(map[string]model.ToolDef),
		ctxMgr:         ctxMgr,
	}
	for _, mgr := range ctxMgr {
		ctx.registerTool(mgr.GetToolDef())
	}
	return &ctx
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
	req.Messages = append(req.Messages, sysmsg, usermsg)
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
	for _, mgr := range ctx.ctxMgr {
		mgr.WriteContext(buf)
	}
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
	model   Model
	root    string
	buildOp *impl.BuildCodeBaseCtxOps

	historyContextMgr ctx.HistoryContextMgr
}

func NewBaseAgent(codebase string, model Model) BaseAgent {
	buildOp := &impl.BuildCodeBaseCtxOps{
		RootPath: codebase,
		Db:       database.GetDBClient().Database("llm_dev"),
	}
	agent := BaseAgent{
		model:   model,
		root:    codebase,
		buildOp: buildOp,
		historyContextMgr: ctx.HistoryContextMgr{
			BuildOps: buildOp,
		},
	}
	return agent
}

type AggregateChunk struct {
	msg       openai.ChatCompletionMessage
	toolCalls map[int]openai.ToolCall
}

func (acc *AggregateChunk) addChunk(delta openai.ChatCompletionStreamChoiceDelta) {
	if delta.Content != "" {
		acc.msg.Content += delta.Content
	}
	for _, toolCall := range delta.ToolCalls {
		index := *toolCall.Index
		value, exist := acc.toolCalls[index]
		if !exist {
			value = toolCall
		} else {
			value.Function.Arguments += toolCall.Function.Arguments
		}
		acc.toolCalls[index] = value
	}
}
func (acc *AggregateChunk) res() openai.ChatCompletionMessage {
	acc.msg.Role = "assistant"
	for _, toolcall := range acc.toolCalls {
		acc.msg.ToolCalls = append(acc.msg.ToolCalls, toolcall)
	}
	return acc.msg
}

func (agent *BaseAgent) handleResponse(stream *openai.ChatCompletionStream, ctx *AgentContext) {
	var err error
	aggregate := AggregateChunk{
		toolCalls: make(map[int]openai.ToolCall),
	}
	var finishReason openai.FinishReason
	fmt.Printf("RESP:\n")
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
	fmt.Print("END OF RESP\n\n")
	if !errors.Is(err, io.EOF) {
		ctx.finished = true
		return
	}
	file, _ := os.OpenFile("context.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	resp := aggregate.res()
	file.WriteString(fmt.Sprintf("RESP:\n%s\n\n", resp.Content))
	ctx.addMessage(resp)
	for _, toolCall := range resp.ToolCalls {
		msg, err := ctx.toolCall(toolCall)
		file.WriteString(fmt.Sprintf("%s, %s\n", toolCall.Function.Name, toolCall.Function.Arguments))
		file.WriteString(fmt.Sprintf("TOOL CALL RESULT:\n%s\n", msg.Content))
		if err != nil {
			log.Error().Err(err).Any("toolcall", toolCall).Msg("tool call failed")
		} else {
			log.Info().Any("tool call", toolCall).Any("result", msg.Content).Msg("run tool call success")
			ctx.addMessage(msg)
		}
	}
	if strings.HasPrefix(resp.Content, "FINAL RESPONSE") {
		ctx.finalResponse = resp.Content
	}
	var buf bytes.Buffer
	ctx.writeContext(&buf)
	file.WriteString("CONTEXT:\n")
	file.Write(buf.Bytes())
	file.Close()

	if finishReason == openai.FinishReasonStop {
		ctx.finished = true
	}
}

func (agent *BaseAgent) NewUserTask(userprompt string) {
	callGraphMgr := ctx.NewCallGraphMgr(agent.root, agent.buildOp)
	filectxMgr := ctx.NewFileCtxMgr(agent.root, agent.buildOp)
	outlineCtxMgr := ctx.NewOutlineCtxMgr(agent.root, agent.buildOp)
	buildContextMgr := ctx.BuildContextMgr{}
	taskCtxMgr := ctx.NewTaskCtxMgr(userprompt)
	readMgr := ctx.ReadContextMgr{
		Root: agent.root,
	}
	agentCtx := NewAgentContext(userprompt, &callGraphMgr, &outlineCtxMgr, &buildContextMgr, &agent.historyContextMgr, &filectxMgr, &readMgr, &taskCtxMgr)
	for {
		// var buf bytes.Buffer
		// // ctx.fileCtxMgr.WriteUsedDefs(&buf)
		// ctx.fileCtxMgr.WriteAutoLoadCtx(&buf)
		// fmt.Print(buf.String())
		req := agentCtx.genRequest(systemPompt)
		stream, err := agent.model.CreateChatCompletionStream(context.TODO(), req)
		if err != nil {
			log.Error().Err(err).Msg("create chat completion stream failed")
			break
		}
		defer stream.Close()
		agent.handleResponse(stream, agentCtx)
		if agentCtx.done() {
			break
		}
	}
	agent.historyContextMgr.RecordUserTask(userprompt, agentCtx.finalResponse, taskCtxMgr.Records)
}

func DebugMsg(msg *openai.ChatCompletionRequest) {
	for _, elem := range msg.Messages {
		fmt.Printf("ROLE: %s\n", elem.Role)
		fmt.Printf("CONTENT:\n%s\n", elem.Content)
	}
	for _, tool := range msg.Tools {
		funcdef := tool.Function
		fmt.Printf("NAME: %s\n", funcdef.Name)
		fmt.Printf("DESC:\n%s\n", funcdef.Description)
		b, _ := json.MarshalIndent(funcdef.Parameters, "", "  ")
		fmt.Printf("PARA:\n%s\n", string(b))
	}
}
