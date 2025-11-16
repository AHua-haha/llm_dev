package context

import (
	"bytes"
	"encoding/json"
	"fmt"
	"llm_dev/model"

	"github.com/sashabaranov/go-openai"
	"github.com/sashabaranov/go-openai/jsonschema"
)

var appleDiff = openai.FunctionDefinition{
	Name:   "apply_diff",
	Strict: true,
	Description: `
Apply changes to a file using unified diff format.
	`,
	Parameters: jsonschema.Definition{
		Type:                 jsonschema.Object,
		AdditionalProperties: false,
		Properties: map[string]jsonschema.Definition{
			"file": {
				Type:        jsonschema.String,
				Description: "Path to the file to modify",
			},
			"diff": {
				Type:        jsonschema.String,
				Description: "Complete unified diff showing changes. Must include: --- and +++ headers, @@ hunk markers, context lines (space prefix), removed lines (- prefix), added lines (+ prefix). Include 3-5 lines of context before and after changes.",
			},
		},
		Required: []string{"file", "diff"},
	},
}

type BuildContextMgr struct {
}

func (mgr *BuildContextMgr) WriteContext(buf *bytes.Buffer) {
}

func (mgr *BuildContextMgr) GetToolDef() []model.ToolDef {
	applyDiffFunc := func(argsStr string) (string, error) {
		args := struct {
			File string
			Diff string
		}{}
		err := json.Unmarshal([]byte(argsStr), &args)
		if err != nil {
			return "", err
		}
		fmt.Printf("Diff content:\n%s\n", args.Diff)
		return "apply the edit success", nil
	}
	res := []model.ToolDef{
		{FunctionDefinition: appleDiff, Handler: applyDiffFunc},
	}
	return res
}
