package agent

import (
	"fmt"
	"llm_dev/database"
	"testing"
)

func TestSysPrompt(t *testing.T) {
	t.Run("teest system prompt format", func(t *testing.T) {
		fmt.Print(systemPompt)
	})
}
func TestBaseAgent_genRequest(t *testing.T) {
	t.Run("test base agent gen request", func(t *testing.T) {
		database.InitDB()
		defer database.CloseDB()
		// op := impl.BuildCodeBaseCtxOps{
		// 	RootPath: "/root/workspace/llm_dev",
		// 	Db:       database.GetDBClient().Database("llm_dev"),
		// }
		// op.ExtractDefs()
		model := NewModel("http://192.168.65.2:4000", "sk-1234")
		agent := NewBaseAgent("/root/workspace/llm_dev", *model)
		for {
			var userPrompt string
			fmt.Print("User Prompt >")
			fmt.Scanln(&userPrompt)

			agent.NewUserTask(userPrompt)
		}
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
		ctx := NewAgentContext(nil, "", agent.fileCtxMgr)
		req := ctx.genRequest(systemPompt)
		for _, msg := range req.Messages {
			fmt.Printf("%s\n", msg.Role)
			fmt.Printf("%s\n", msg.Content)
		}
	})
}
