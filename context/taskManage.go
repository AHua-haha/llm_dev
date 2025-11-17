package context

import (
	"bytes"
	"encoding/json"
	"fmt"
	"llm_dev/model"

	"github.com/sashabaranov/go-openai"
	"github.com/sashabaranov/go-openai/jsonschema"
)

var taskPrompt = `
You are given a user's task, you should help user finish the task.

Your job:
- Understand the user's goal.
- Break it into clear, ordered tasks.
- Use the available tools to complete those tasks.
- Keep the project in a working state (run tests when appropriate).
- Record conclusion and result of your analyze or yout action or you reasoning.

# Tool Usage Guidelines

Good workflow examples to Identify the relevant context use tools.
- from top down, use 'get_directory_overview' tool to get the used definition of a directory. Get a overall understanding of the directory and how the directory is used and what in the directory is used.
- Based on the used definition in directory, search for relevant context from the used definition.
- Use 'load_file_context' tool to load all the definitions in a file, identify which definition is relevant.
- Then use 'load_definition_context' tool to load the complete implementation of the definition.
- Analyze the functionality of definitions, use 'find_reference' tool to examine where the definition is used and how the definition is used, analyze what the definition is used for.
- Analyze definition implementation details, use 'find_used_definition' tool to examine the exact definition used within one function.

# Record Conclusion and Result

IMPORTANT: You should record all conclusinos and results that is crucial to complete the task while solving the task.
IMPORTANT: Once you get a conclusion or result, you should record it IMMEDIATELY use tools.

You MUST record conclusion and result in this fotmat:
- Type: the type of the conclusion, e.g. analyze, build, plain text.
- Statement: the concise and straightforward statement of the conclusion.
- References: the location of referenced code in the codebase, e.g. src/test.go:22, src/common/utils.go:56-189

There are three different types of conclusion and result, Analyze, Build, Plain Text.
1. Analyze: the conclusion of analyze and understand the codebase that is crucial to colve the task.
2. Build: the result of editing the codebase that is crucial to colve the task.
3. Plain Text: other conclusion that is crucial to solve the task.

Guidelines:
- Make each conclusion minimal, each conclusion should be about exact one point.
- Make conclusion concise and short and straightforward.

`
var createTask = openai.FunctionDefinition{
	Name:   "create_task",
	Strict: true,
	Description: `
Create a new task
`,
	Parameters: jsonschema.Definition{
		Type:                 jsonschema.Object,
		AdditionalProperties: false,
		Properties: map[string]jsonschema.Definition{
			"content": {
				Type:        jsonschema.String,
				Description: "the content of the task, what this task do",
			},
		},
		Required: []string{"content"},
	},
}
var finishTask = openai.FunctionDefinition{
	Name:   "finish_task",
	Strict: true,
	Description: `
Finish the current working task.
`,
	Parameters: jsonschema.Definition{
		Type:                 jsonschema.Object,
		AdditionalProperties: false,
		Properties: map[string]jsonschema.Definition{
			"id": {
				Type:        jsonschema.Number,
				Description: "the id of the task",
			},
		},
		Required: []string{"id"},
	},
}
var record = openai.FunctionDefinition{
	Name:   "record_conclusion",
	Strict: true,
	Description: `
Record conclusinos and results that is crucial to complete the task
`,
	Parameters: jsonschema.Definition{
		Type:                 jsonschema.Object,
		AdditionalProperties: false,
		Properties: map[string]jsonschema.Definition{
			"type": {
				Type:        jsonschema.String,
				Description: "the type of the conclusion or result, e.g. analyze, build, plain text",
				Enum:        []string{"Analyze", "Build", "Plain Text"},
			},
			"statement": {
				Type:        jsonschema.String,
				Description: "the concise and straightforward statement of the conclusion",
			},
			"references": {
				Type:        jsonschema.Array,
				Description: "array of locations of referenced code in the codebase",
				Items: &jsonschema.Definition{
					Type:        jsonschema.String,
					Description: "the location of the code line or block, e.g. src/test.go:22, src/utils.go:678, src/common/impl.go:224-445",
				},
			},
		},
		Required: []string{"type", "statement", "references"},
	},
}

type TaskStatus string

const (
	Progress  TaskStatus = "In Progress"
	Completed TaskStatus = "Completed"
)

type Task struct {
	ID      uint
	Content string
	Status  TaskStatus
}

func (t *Task) toString() string {
	return fmt.Sprintf("(%-11s) Task %d: %s", t.Status, t.ID, t.Content)
}

type Conclusion struct {
	Type       string
	Statement  string
	References []string
}

func (c *Conclusion) toString() string {
	return fmt.Sprintf("Type: %s, Statement: %s, References: %v", c.Type, c.Statement, c.References)
}

type TaskContextMgr struct {
	UserTask    string
	TaskList    []*Task
	CurrentTask *Task
	Records     []Conclusion
}

func (mgr *TaskContextMgr) finishTask(id uint) string {
	if mgr.CurrentTask == nil || mgr.CurrentTask.ID != id {
		return fmt.Sprintf("finish Task %d failed", id)
	}
	mgr.CurrentTask.Status = Completed
	mgr.CurrentTask = nil
	return fmt.Sprintf("finish Task %d success", id)
}

func (mgr *TaskContextMgr) createTask(content string) string {
	if mgr.CurrentTask != nil {
		return "create new task failed because previous task not finished"
	}
	task := &Task{
		Content: content,
		Status:  Progress,
	}
	mgr.TaskList = append(mgr.TaskList, task)
	task.ID = uint(len(mgr.TaskList))
	mgr.CurrentTask = task
	return fmt.Sprintf("create new Task %d: %s", task.ID, task.Content)
}
func (mgr *TaskContextMgr) writeTaskList(buf *bytes.Buffer) {
	buf.WriteString("# Task List & Conclusion\n\n")
	buf.WriteString(fmt.Sprintf("User's overall goal:\n%s\n\n", mgr.UserTask))
	buf.WriteString("1.** Conclusions & Results **\n\n")
	if len(mgr.Records) == 0 {
		buf.WriteString("NO conclusions\n")
	} else {
		for _, record := range mgr.Records {
			buf.WriteString(record.toString())
			buf.WriteByte('\n')
		}
	}
	buf.WriteByte('\n')
	buf.WriteString("2.** Task List **\n\n")
	if len(mgr.TaskList) == 0 {
		buf.WriteString("NO tasks\n")
	} else {
		for _, task := range mgr.TaskList {
			buf.WriteString(fmt.Sprintf("%s\n", task.toString()))
		}
	}
	buf.WriteByte('\n')
	if mgr.CurrentTask != nil {
		buf.WriteString(fmt.Sprintf("Current Working Task:\nTask %d: %s\n", mgr.CurrentTask.ID, mgr.CurrentTask.Content))
	}
	buf.WriteString(`
You CAN do:
- finish task and create the next task.
- using tools to complete the current working task
- record conclusion and result

IMPORTANT: If there is not a working task, you MUST create a task first before you do other thing.
`)
}

func (mgr *TaskContextMgr) WriteContext(buf *bytes.Buffer) {
	buf.WriteString("### TASK MANAGEMENT ###\n")
	buf.WriteString(taskPrompt)
	mgr.writeTaskList(buf)
	buf.WriteString("### END OF TASK MANAGEMENT ###\n")
}

func (mgr *TaskContextMgr) GetToolDef() []model.ToolDef {
	recordHandler := func(argsStr string) (string, error) {
		args := struct {
			Type       string
			Statement  string
			References []string
		}{}
		err := json.Unmarshal([]byte(argsStr), &args)
		if err != nil {
			return "", err
		}
		mgr.Records = append(mgr.Records, Conclusion{
			Type:       args.Type,
			Statement:  args.Statement,
			References: args.References,
		})
		return fmt.Sprintf("Record conclusion: %s success", args.Statement), nil
	}
	createTaskHandler := func(argsStr string) (string, error) {
		args := struct {
			Content string
		}{}
		err := json.Unmarshal([]byte(argsStr), &args)
		if err != nil {
			return "", err
		}
		return mgr.createTask(args.Content), nil
	}
	finishTaskHandler := func(argsStr string) (string, error) {
		args := struct {
			Id uint
		}{}
		err := json.Unmarshal([]byte(argsStr), &args)
		if err != nil {
			return "", err
		}
		return mgr.finishTask(args.Id), nil
	}
	res := []model.ToolDef{
		{FunctionDefinition: createTask, Handler: createTaskHandler},
		{FunctionDefinition: finishTask, Handler: finishTaskHandler},
		{FunctionDefinition: record, Handler: recordHandler},
	}
	return res
}
