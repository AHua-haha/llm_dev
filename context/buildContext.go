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
To help user with task, you can modify the codebase by running different actions;
To use the build system, you should strictly follow the instructions and action workflow, and examine the action context thoroughly.

# Instruction

You should follow the following instructions:
1.First examine the building context, if there are pending building plans, continue execute the pending building plan.
2.If NO pending building plan, examin the user's prompt, if the user specified a build task, you should load the relevant context and think about how to solve the task, then make build plan.
IMPORTANT: DO NOT make build plan if the user does not explicitly specified a build task.

# Action Workflow

Each action has three properties:
- Thought: your reasoning about what action to do next and the clearly definnd purpose of the action from the perspective of solving the task.
- Type: the type of the action.
- Result: the result of the executing the action.

Different type of actions:

- Edit: ecit the codebase files.

To execute an action, you should follow the following four phases.

- Phase 1: Declare an action
	You should examine the user's task and the previously executed actions thoroughly, then think about what action to do next to accomplish the task. then you should declare the Thought and Type of action.
- Phase 2: Load Context
	First load the file content that will be edited, then load the relevant context to perform the action.
- Phase 3: Execute an action
	execute the action, use the tool to execute the action.
- Phase 4: Examine result
	examine the result of executing the action.

NEVER edit the file without loading the file content.

# Principles

You should follow the good principles:
- When you want to modify existing code segment, it is good to keep the changes minial. Try to avoid changes of an entre code segment unless necessary.
- Always load the relevant context before executing action if necessary.
- Always imitate the existing code implementation.
- For loading context, first figure out the exact definition of the symbols, load the definition code, then find all the references of the definition to check out how to use it. You should follow these use examples.

`

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
				Description: "the type of the action",
			},
			"thought": {
				Type:        jsonschema.String,
				Description: "the reasoning about what action to do next and the clearly definnd purpose of the action from the perspective of solving the task",
			},
		},
		Required: []string{"type", "thought"},
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
	Type    string
	Thought string
	Result  string
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
		buf.WriteString(fmt.Sprintf("	Thought: %s\n", action.Thought))
		buf.WriteString(fmt.Sprintf("	Type: %s\n", action.Type))
		buf.WriteString(fmt.Sprintf("	Result: %s\n", action.Result))
		buf.WriteByte('\n')
	}
	buf.WriteString("{END OF EDIT ACTION}\n\n")
}

func (mgr *BuildContextMgr) addAction(action Action) {
	mgr.actions = append(mgr.actions, action)
}

func (mgr *BuildContextMgr) GetToolDef() []model.ToolDef {
	newActionHandler := func(argsStr string) (string, error) {
		args := struct {
			Type    string
			Thought string
		}{}
		err := json.Unmarshal([]byte(argsStr), &args)
		if err != nil {
			return "", err
		}
		mgr.addAction(Action{
			Type:    args.Type,
			Thought: args.Thought,
		})
		return "new action success", nil
	}
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
	// insert := func(argsStr string) (string, error) {
	// 	args := struct {
	// 		File    string
	// 		Line    uint
	// 		Content string
	// 	}{}
	// 	err := json.Unmarshal([]byte(argsStr), &args)
	// 	if err != nil {
	// 		return "", err
	// 	}
	// 	fmt.Printf("insert to file %s:%d\n```\n%s\n```\n", args.File, args.Line, args.Content)
	// 	res := fmt.Sprintf("File: %s\nLine: %d\n```\n%s\n```\n", args.File, args.Line, args.Content)
	// 	return res, nil
	// }
	// replace := func(argsStr string) (string, error) {
	// 	args := struct {
	// 		File      string
	// 		Startline uint
	// 		Endline   uint
	// 		Content   string
	// 	}{}
	// 	err := json.Unmarshal([]byte(argsStr), &args)
	// 	if err != nil {
	// 		return "", err
	// 	}
	// 	fmt.Printf("replace file %s:%d-%d\n```\n%s\n```\n", args.File, args.Startline, args.Endline, args.Content)
	// 	res := fmt.Sprintf("File: %s\nLine: %d-%d\n```\n%s\n```\n", args.File, args.Startline, args.Endline, args.Content)
	// 	return res, nil
	// }
	res := []model.ToolDef{
		{FunctionDefinition: newAction, Handler: newActionHandler},
		{FunctionDefinition: appleDiff, Handler: applyDiffFunc},
		// {FunctionDefinition: insertAction, Handler: insert},
		// {FunctionDefinition: replaceAction, Handler: replace},
	}
	return res

}
