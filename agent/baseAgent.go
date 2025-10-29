package agent

import (
	"bytes"
	"context"
	ctx "llm_dev/context"

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
}

func NewBaseAgent(codebase string, model Model) BaseAgent {
	agent := BaseAgent{
		model:      model,
		fileCtxMgr: ctx.NewFileCtxMgr(codebase),
	}
	return agent
}

func (agent *BaseAgent) genRequest() (*openai.ChatCompletionRequest, error) {
	req := openai.ChatCompletionRequest{
		Model: "",
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
	return &req, nil
}

func (agent *BaseAgent) handleResponse(stream *openai.ChatCompletionStream) {
}

func (agent *BaseAgent) newReq(userprompt string) {
	agent.currentUserPrompt = userprompt
	agent.finished = false
	for {
		req, err := agent.genRequest()
		if err == nil {
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
