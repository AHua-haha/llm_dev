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
		model := NewModel("http://192.168.65.2:4000", "sk-1234")
		agent := NewBaseAgent("/root/workspace/llm_dev", *model)
		req, _ := agent.genRequest()
		for _, msg := range req.Messages {
			fmt.Printf("%s\n", msg.Role)
			fmt.Printf("%s\n", msg.Content)
		}
	})
}
