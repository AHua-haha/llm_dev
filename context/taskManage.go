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
You have access to 'create_task', 'finish_task' tools for task management, 'record_conclusion' tool for record conclusion and result, and other tools for doing tasks.
- create task: decompose the user's overall goal to tasks, create task and then use tools to complete task, You MUST create task first before you do any other things.
- finish task: mark the task completed IMMEDIATELY after you finish the task, you MUST specify the task result success or fail.
- record conclusion: record all conclusinos and results that is crucial to complete the task while doing the task. Once you get a conclusion or result, record it IMMEDIATELY.

IMPORTANT: for the final response to the user's prompt, you MUST output with 'FINAL RESPONSE' at the beginning.
<example>
FINAL RESPONSE:
you final response
</example>

IMPORTANT: You MUST review the history first to get the relevant context.


`
var createTask = openai.FunctionDefinition{
	Name:   "create_task",
	Strict: true,
	Description: `
Create a new task
# Task States and Management

1. **Task States**: Use these states to track progress:
   - in_progress: Currently working on (limit to ONE task at a time)
   - decompose: This task is decomposed to smaller subtasks, and now working on subtask to advance this task.
   - completed: Task finished successfully
   - failed: Task can not be completed, task failed.

2. **Task Management**:
   - Mark tasks complete or failed IMMEDIATELY after finishing or the task failed because errors.
   - Only have ONE task in_progress at any time
   - Complete current tasks before starting new ones
   - Breakdown complex task to smaller subtasks.

3. **Task Breakdown**:
   - Create specific, actionable items
   - Break complex tasks into smaller, manageable subtasks.

# Usage

- After the previous task is finished, you can create a next task.
- The in progress task is complex, you can decompose this task to smaller subtask, and create the subtask, and work on this subtask.
- specify the parent task id for this new task, and use empty string if the parent task is the user's overall task.
 IMPORTANT: you MUST create task first before you do any other things.

<example>
(in_progress) Task 1: Understand the core part of this project by analyzing the codebase structure and identifying key components

function call: create_task content = "identify the main entry of this project", parenttask = 1

(decompose  ) Task 1: Understand the core part of this project by analyzing the codebase structure and identifying key components
(in_progress) Task 1.1: identify the main entry of this project
</example>
`,
	Parameters: jsonschema.Definition{
		Type:                 jsonschema.Object,
		AdditionalProperties: false,
		Properties: map[string]jsonschema.Definition{
			"content": {
				Type:        jsonschema.String,
				Description: "the content of the task, what this task do",
			},
			"parenttask": {
				Type:        jsonschema.String,
				Description: "the id of the parent task, use empty string id if the parent task is the user's overall goal, e.g. 1.1, 2.3, 2, 3",
			},
		},
		Required: []string{"content", "parenttask"},
	},
}
var finishTask = openai.FunctionDefinition{
	Name:   "finish_task",
	Strict: true,
	Description: `
Mark the current working task finish status.
# Task States and Management
1. **Task States**: Use these states to track progress:
   - in_progress: Currently working on (limit to ONE task at a time)
   - decompose: This task is decomposed to smaller subtasks, and now working on subtask to advance this task.
   - completed: Task finished successfully
   - failed: Task can not be completed, task failed.

2. **Task Management**:
   - Mark tasks complete or failed IMMEDIATELY after finishing or the task failed because errors.
   - Only have ONE task in_progress at any time
   - Complete current tasks before starting new ones
   - Breakdown complex task to smaller subtasks.

3. **Task Completion Requirements**:
   - ONLY mark a task as completed when you have FULLY accomplished it
   - If you encounter errors, blockers, or cannot finish, mark the task failed.
   - Never mark a task as completed if:
     - Tests are failing
     - Implementation is partial
     - You encountered unresolved errors
     - You couldn't find necessary files or dependencies

# Usage
- You can only mark the task finish status completed or failed.
- Mark task success IMMEDIATELY after finish task successfully
- Mark task failed if the task failed and can not be accomplished.

<example>
(in_progress) Task 1: Understand the core part of this project by analyzing the codebase structure and identifying key components
function call: finish_task id = 1, status = completed
</example>

`,
	Parameters: jsonschema.Definition{
		Type:                 jsonschema.Object,
		AdditionalProperties: false,
		Properties: map[string]jsonschema.Definition{
			"id": {
				Type:        jsonschema.String,
				Description: "the id of the task, e.g. 1.1, 2.3, 3, 4",
			},
			"status": {
				Type:        jsonschema.String,
				Description: "the status of the finished task",
				Enum:        []string{"completed", "failed"},
			},
		},
		Required: []string{"id", "status"},
	},
}
var record = openai.FunctionDefinition{
	Name:   "record_conclusion",
	Strict: true,
	Description: `
Record conclusinos and results that is crucial to complete the task.
You can execute multiple function calls to record multiple conclusion in one go.

You MUST record conclusion and result in this fotmat:
- Type: the type of the conclusion, e.g. analyze, build, plain text.
- Statement: the concise and straightforward statement of the conclusion.
- References: the location of referenced code in the codebase, e.g. src/test.go:22, src/common/utils.go:56-189

There are three different types of conclusion and result, Analyze, Build, Plain Text.
1. Analyze: the conclusion of analyze and understand the codebase that is crucial to colve the task.
2. Build: the result of editing the codebase that is crucial to colve the task.
3. Plain Text: other conclusion that is crucial to solve the task.

When to record plain text conclusion:
- you search and get some plain text, and the text refers to some code in the project, record the conclusion.

IMPORTANT: You should record all conclusinos and results that is crucial to complete the task while solving the task.
IMPORTANT: Once you get a conclusion or result, you should record it IMMEDIATELY use tools.
IMPORTANT: Make each conclusion minimal, each conclusion should be about exact one point.
IMPORTANT: Make conclusion concise and short and straightforward.
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
	Decompose TaskStatus = "decompose"
	Progress  TaskStatus = "in_progress"
	Completed TaskStatus = "completed"
	Failed    TaskStatus = "failed"
)

type Task struct {
	ID      string
	Content string
	Status  TaskStatus

	SubTaskCount int
	ParentTask   *Task
}

func (t *Task) toString() string {
	return fmt.Sprintf("(%-11s) Task %s: %s", t.Status, t.ID, t.Content)
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
	UserTask    Task
	TaskList    []*Task
	CurrentTask *Task
	Records     []Conclusion
}

func NewTaskCtxMgr(userprompt string) TaskContextMgr {
	mgr := TaskContextMgr{
		UserTask: Task{
			ID:      "",
			Content: userprompt,
			Status:  Progress,
		},
	}
	mgr.CurrentTask = &mgr.UserTask
	return mgr
}

func (mgr *TaskContextMgr) finishTask(id string, status TaskStatus) string {
	if mgr.CurrentTask == nil || mgr.CurrentTask.ID != id {
		return fmt.Sprintf("finish Task %s failed", id)
	}
	mgr.CurrentTask.Status = status
	mgr.CurrentTask = mgr.CurrentTask.ParentTask
	return fmt.Sprintf("finish Task %s success", id)
}

func (mgr *TaskContextMgr) createTask(content string, parentTask string) string {
	if mgr.CurrentTask == nil || mgr.CurrentTask.ID != parentTask {
		return fmt.Sprintf("create new task %s under parent task %s failed", content, parentTask)
	}
	mgr.CurrentTask.SubTaskCount++
	var id string
	if mgr.CurrentTask.ID == "" {
		id = fmt.Sprintf("%d", mgr.CurrentTask.SubTaskCount)
	} else {
		id = fmt.Sprintf("%s.%d", mgr.CurrentTask.ID, mgr.CurrentTask.SubTaskCount)
	}
	task := &Task{
		ID:         id,
		Content:    content,
		ParentTask: mgr.CurrentTask,
		Status:     Progress,
	}
	mgr.CurrentTask.Status = Decompose
	mgr.CurrentTask = task
	mgr.TaskList = append(mgr.TaskList, task)
	return fmt.Sprintf("create new Task %s: %s success", task.ID, task.Content)
}
func (mgr *TaskContextMgr) writeTaskList(buf *bytes.Buffer) {
	buf.WriteString("# Task List & Conclusion\n\n")
	buf.WriteString(fmt.Sprintf("User's overall goal:\n%s\n\n", mgr.UserTask.Content))
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
		buf.WriteString(fmt.Sprintf("Current Working on: Task %s: %s\n", mgr.CurrentTask.ID, mgr.CurrentTask.Content))
	}
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
			Content    string
			Parenttask string
		}{}
		err := json.Unmarshal([]byte(argsStr), &args)
		if err != nil {
			return "", err
		}
		return mgr.createTask(args.Content, args.Parenttask), nil
	}
	finishTaskHandler := func(argsStr string) (string, error) {
		args := struct {
			Id     string
			Status string
		}{}
		err := json.Unmarshal([]byte(argsStr), &args)
		if err != nil {
			return "", err
		}
		status := TaskStatus(args.Status)
		if status != Failed && status != Completed {
			return fmt.Sprintf("unrecognized status %s, you can only set the task completed or failed", status), nil
		}
		return mgr.finishTask(args.Id, status), nil
	}
	res := []model.ToolDef{
		{FunctionDefinition: createTask, Handler: createTaskHandler},
		{FunctionDefinition: finishTask, Handler: finishTaskHandler},
		{FunctionDefinition: record, Handler: recordHandler},
	}
	return res
}
