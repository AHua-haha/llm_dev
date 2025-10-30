package agent

import (
	"llm_dev/database"
	"testing"
)

func TestBaseAgent_genRequest(t *testing.T) {
	t.Run("test base agent gen request", func(t *testing.T) {
		database.InitDB()
		defer database.CloseDB()
		model := NewModel("http://172.17.0.1:4000", "sk-1234")
		agent := NewBaseAgent("/root/workspace/llm_dev", *model)
		agent.newReq("summary what all the different module do in this project")
	})
}
