package context

import (
	"bytes"
	"encoding/json"
	"fmt"
	"llm_dev/model"

	"github.com/sashabaranov/go-openai"
	"github.com/sashabaranov/go-openai/jsonschema"
)

var prompt = `
To help user with task, you can modify the codebase using edit action.
To use the build system, you should strictly follow the instructions and action workflow, and examine the action context thoroughly.

# Instruction

You should follow the following instructions:
1.First examine the building context, if there are pending building plans, continue execute the pending building plan.
2.If NO pending building plan, examin the user's prompt, if the user specified a build task, you should load the relevant context and think about how to solve the task, then make build plan.
IMPORTANT: DO NOT make build plan if the user does not explicitly specified a build task.

# Action Workflow

Use case: you can trigger action to edit the codebase to help users complete build tasks. You should follow the following workflow to trigger action.

Each action edits one location in the codebase. Each action has four important properties:
- Type:  the type of the action, e.g. insert. replace. new file. delete.
- Location: the location the action is performed.
- Purpose: the clearly defined purpose of the action from the perspective of completing the task
- Content: the generated content for insert or replace.

<example>
- Type: replace
- Location: src/foo.go line 456-500
- Purpose: add debug message for variable name in line 467
- Content: ...
</example>

<example>
- Type: insert
- Location: src/foo.go line 78
- Purpose: add a new function GetRes to get the result of the http request.
- Content:"func GetRes(a hhtp.request) {}"
</example>


- Phase 1: Declare an action
	Declare a new action, you should declare the type, location and purpose of action.
- Phase 2: Load Context
	load the relevant context to perform the edit, and the used symbol in the edit.
- Phase 3: Execute an action
	execute the action, use the tool to execute the action.

Different Types of Action:
- Insert: insert new content after certain line in a file.
- Replace: replace certain range of code in a file.
- New file: create a new file with the file path.

`
var newAction = openai.FunctionDefinition{
	Name:   "declare_action",
	Strict: true,
	Description: `
Declare a new edit action, you should specify the type location and purpose of an action.
	`,
	Parameters: jsonschema.Definition{
		Type:                 jsonschema.Object,
		AdditionalProperties: false,
		Properties: map[string]jsonschema.Definition{
			"type": {
				Type:        jsonschema.String,
				Description: "the type of the action, e.g. insert replace new file",
			},
			"purpose": {
				Type:        jsonschema.String,
				Description: "the purpose of the action",
			},
			"location": {
				Type:        jsonschema.String,
				Description: "the location to perform the action",
			},
		},
		Required: []string{"type", "purpose", "location"},
	},
}

var insertAction = openai.FunctionDefinition{
	Name:   "insert_action",
	Strict: true,
	Description: `
Perform an insert action, insert content after certain line in some file.
	`,
	Parameters: jsonschema.Definition{
		Type:                 jsonschema.Object,
		AdditionalProperties: false,
		Properties: map[string]jsonschema.Definition{
			"file": {
				Type:        jsonschema.String,
				Description: "the file of the edit location",
			},
			"line": {
				Type:        jsonschema.Number,
				Description: "the line number of the insert location, the content will be inserted after the line",
			},
			"content": {
				Type:        jsonschema.String,
				Description: "the content to insert",
			},
		},
		Required: []string{"file", "line", "content"},
	},
}

var replaceAction = openai.FunctionDefinition{
	Name:   "replace_action",
	Strict: true,
	Description: `
Perform an replace action, replace certain block of the file.
	`,
	Parameters: jsonschema.Definition{
		Type:                 jsonschema.Object,
		AdditionalProperties: false,
		Properties: map[string]jsonschema.Definition{
			"file": {
				Type:        jsonschema.String,
				Description: "the file of the edit location",
			},
			"startline": {
				Type:        jsonschema.Number,
				Description: "the start line of the replaced block",
			},
			"endline": {
				Type:        jsonschema.Number,
				Description: "the end line of the replaced block",
			},
			"content": {
				Type:        jsonschema.String,
				Description: "the content to replace",
			},
		},
		Required: []string{"file", "line", "content"},
	},
}

type Action struct {
	Type     string
	Purpose  string
	Location string
}

type BuildContextMgr struct {
	actions []Action
}

func (mgr *BuildContextMgr) WriteContext(buf *bytes.Buffer) {
	buf.WriteString("{EDIT ACTION}\n\n")
	buf.WriteString(prompt)
	buf.WriteString("# Action Status\n\n")
	for i, action := range mgr.actions {
		buf.WriteString(fmt.Sprintf("- Action %d:\n", i))
		buf.WriteString(fmt.Sprintf("	Type: %s\n", action.Type))
		buf.WriteString(fmt.Sprintf("	Location: %s\n", action.Location))
		buf.WriteString(fmt.Sprintf("	Purpose: %s\n", action.Purpose))
		buf.WriteByte('\n')
	}
	buf.WriteString("{EDIT ACTION}\n\n")
}

func (mgr *BuildContextMgr) addAction(action Action) {
	mgr.actions = append(mgr.actions, action)
}

func (mgr *BuildContextMgr) GetToolDef() []model.ToolDef {
	newActionHandler := func(argsStr string) (string, error) {
		args := struct {
			Type     string
			Purpose  string
			Location string
		}{}
		err := json.Unmarshal([]byte(argsStr), &args)
		if err != nil {
			return "", err
		}
		mgr.addAction(Action{
			Type:     args.Type,
			Purpose:  args.Purpose,
			Location: args.Location,
		})
		return "new action success", nil
	}
	insert := func(argsStr string) (string, error) {
		args := struct {
			File    string
			Line    uint
			Content string
		}{}
		err := json.Unmarshal([]byte(argsStr), &args)
		if err != nil {
			return "", err
		}
		fmt.Printf("insert to file %s:%d\n```\n%s\n```\n", args.File, args.Line, args.Content)
		res := fmt.Sprintf("File: %s\nLine: %d\n```\n%s\n```\n", args.File, args.Line, args.Content)
		return res, nil
	}
	replace := func(argsStr string) (string, error) {
		args := struct {
			File      string
			Startline uint
			Endline   uint
			Content   string
		}{}
		err := json.Unmarshal([]byte(argsStr), &args)
		if err != nil {
			return "", err
		}
		fmt.Printf("replace file %s:%d-%d\n```\n%s\n```\n", args.File, args.Startline, args.Endline, args.Content)
		res := fmt.Sprintf("File: %s\nLine: %d-%d\n```\n%s\n```\n", args.File, args.Startline, args.Endline, args.Content)
		return res, nil
	}
	res := []model.ToolDef{
		{FunctionDefinition: newAction, Handler: newActionHandler},
		{FunctionDefinition: insertAction, Handler: insert},
		{FunctionDefinition: replaceAction, Handler: replace},
	}
	return res

}
