package main

import (
	"bufio"
	"fmt"
	"llm_dev/agent"
	"llm_dev/codebase/impl"
	"llm_dev/database"
	"os"
)

var sss string

func main() {

	database.InitDB()
	defer database.CloseDB()
	op := impl.BuildCodeBaseCtxOps{
		RootPath: "/root/workspace/llm_dev",
		Db:       database.GetDBClient().Database("llm_dev"),
	}
	op.RootPath = ""
	// op.GenAllUsedDefs()
	// op.SetMinPreFix()
	model := agent.NewModel("http://192.168.65.2:4000", "sk-1234")
	agent := agent.NewBaseAgent("/root/workspace/llm_dev", *model)
	for {
		reader := bufio.NewScanner(os.Stdin)
		fmt.Print("User Prompt> ")
		reader.Scan() // This will read a line of input from the user
		userprompt := reader.Text()

		agent.NewUserTask(userprompt)
	}
}
