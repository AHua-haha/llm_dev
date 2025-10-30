package agent

import (
	"fmt"
	"llm_dev/database"
	"testing"
)

func TestBaseAgent_genRequest(t *testing.T) {
	t.Run("test base agent gen request", func(t *testing.T) {
		database.InitDB()
		defer database.CloseDB()
		model := NewModel("http://172.17.0.1:4000", "sk-1234")
		agent := NewBaseAgent("/root/workspace/llm_dev", *model)
		agent.newReq("tell me how this project extract definition using treesitter")
	})
}

func TestTool(t *testing.T) {
	t.Run("test tool description", func(t *testing.T) {
		database.InitDB()
		defer database.CloseDB()
		model := NewModel("http://172.17.0.1:4000", "sk-1234")
		agent := NewBaseAgent("/root/workspace/llm_dev", *model)
		tools := agent.fileCtxMgr.GetToolDef()
		for _, tool := range tools {
			fmt.Printf("%s\n", tool.Name)
			fmt.Printf("%s\n", tool.Description)
		}
		req, _ := agent.genRequest()
		for _, msg := range req.Messages {
			fmt.Printf("%s\n", msg.Role)
			fmt.Printf("%s\n", msg.Content)
		}
	})
}
