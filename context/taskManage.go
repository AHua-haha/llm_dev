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
You are working on a Thought -> Action -> Summary workflow to help user solve task.
Each phase serve different purpose:

- Thought: examine the user's task and previous finished task, think about what to do next to solve the task, and declare new task.
- Action: Use different tools to solve the current task.
- Summary: After the task is finished, summary what this task has done.

# Task List
You can create a two level task list:
- Main Task: Decompose user's tasks to main tasks by task purpose from a high level perspective.
- Subtask: each subtask should aim at one concrete thing to solve the main task.

Task list Example:
<example>

Task 1: Identify the relevant context in the codebase. (Status: completed)
Task 1 Summary: the summary of the conclusion and result of the task.

Task 2: Add the log info message. (Status: in progress)
SubTasks:
- Task 2.1: add log info message for function A (status: completed)
  Task 2.1 Summary:
- Task 2.2: add log info message for function B (status: in progress)
  Task 2.2 Summary:

</example>

# Best Practice

Here are some best practice for task management.

## Decompose and create task

Good Example to Decompose and Create Task:

- Create main task from a high level perspective, Main Task has a overall goal, 
  For example: analyze and identify relevant context for log function, 
  implement the log info function, check error for implementaion, 
  run test for the implementation, fix bug for run test.

- Create subtask to solve one concrete part of the main task, 
  For example: identify relevant context for the <symbol>, 
  find out how <symbol> is implemented, find out the main workflow for <symbol>, 
  add log info message for function <name>.



## Action and Tool Usage

Good workflow examples to Identify the relevant context:
- from top down, use 'get_directory_overview' tool to get the used definition of a directory. Get a overall understanding of the directory and how the directory is used and what in the directory is used.
- Based on the used definition in directory, search for relevant context from the used definition.
- Use 'load_file_context' tool to load all the definitions in a file, identify which definition is relevant.
- Then use 'load_definition_context' tool to load the complete implementation of the definition.
- Analyze the functionality of definitions, use 'find_reference' tool to examine where the definition is used and how the definition is used, analyze what the definition is used for.
- Analyze definition implementation details, use 'find_used_definition' tool to examine the exact definition used within one function.

## Summary task result and conclusion

Good Example to symmary task:
- Summary the main task based on all the subtask of the main task.
- Summary the result and conclusion of the tasks to short, concise, straightforward statement. 
  For example: Implement a new log function <identifier> in <file>, 
  function <identifier> in <file> is used for ..., 
  the main workflow is implemented in function <identifier> in <file>

- Add reference to the context in the codebase for each statement. Reference format: identifier file, For example: function NewBuildOps in src/agent.go

`
var createTask = openai.FunctionDefinition{
	Name:   "create_task",
	Strict: true,
	Description: `
Create a new task, you can create a main task or a subtask, you must specify the content of the task.
You CAN NOT create a new task if the previous task is not completed.
You MUST finish that task first.
`,
	Parameters: jsonschema.Definition{
		Type:                 jsonschema.Object,
		AdditionalProperties: false,
		Properties: map[string]jsonschema.Definition{
			"content": {
				Type:        jsonschema.String,
				Description: "the content of the task, what this task do",
			},
			"type": {
				Type:        jsonschema.String,
				Description: "whether the task is a main task or subtask",
				Enum:        []string{"main task", "subtask"},
			},
		},
		Required: []string{"content", "type"},
	},
}
var finishTask = openai.FunctionDefinition{
	Name:   "finish_task",
	Strict: true,
	Description: `
Finish the current task, you must specify the summary of the task.
You MUST finish the subtask first and then finish the main task.
`,
	Parameters: jsonschema.Definition{
		Type:                 jsonschema.Object,
		AdditionalProperties: false,
		Properties: map[string]jsonschema.Definition{
			"summary": {
				Type:        jsonschema.String,
				Description: "the summary of this task",
			},
			"type": {
				Type:        jsonschema.String,
				Description: "whether the task is a main task or subtask",
				Enum:        []string{"main task", "subtask"},
			},
		},
		Required: []string{"summary", "type"},
	},
}

type TaskStatus string

const (
	Progress  TaskStatus = "In Progress"
	Completed TaskStatus = "Completed"
)

type Task struct {
	TaskID   string
	Content  string
	Summary  string
	Status   TaskStatus
	SubTasks []*Task
}

type TaskContextMgr struct {
	TaskList        []*Task
	CurrentMainTask *Task
	CurrentSubTask  *Task
}

func (mgr *TaskContextMgr) finishTask(summary string, subTask bool) string {
	var task *Task
	if subTask {
		if mgr.CurrentSubTask == nil {
			return ""
		}
		task = mgr.CurrentSubTask
		mgr.CurrentSubTask.Status = Completed
		mgr.CurrentSubTask.Summary = summary
		mgr.CurrentSubTask = nil
	} else {
		if mgr.CurrentMainTask == nil || mgr.CurrentSubTask != nil {
			return ""
		}
		task = mgr.CurrentMainTask
		mgr.CurrentMainTask.Status = Completed
		mgr.CurrentMainTask.Summary = summary
		mgr.CurrentMainTask = nil
	}
	return fmt.Sprintf("Complete task [Task %s: %s]", task.TaskID, task.Content)
}

func (mgr *TaskContextMgr) createTask(content string, subTask bool) string {
	task := Task{
		Content: content,
		Status:  Progress,
	}
	if subTask {
		if mgr.CurrentSubTask != nil {
			return fmt.Sprintf("Previous subtask [Task %s: %s] is not finished, can not create new subtask", mgr.CurrentSubTask.TaskID, mgr.CurrentSubTask.Content)
		}
		mgr.CurrentMainTask.SubTasks = append(mgr.CurrentMainTask.SubTasks, &task)
		mgr.CurrentSubTask = &task
		task.TaskID = fmt.Sprintf("%d.%d", len(mgr.TaskList), len(mgr.CurrentMainTask.SubTasks))
	} else {
		if mgr.CurrentMainTask != nil {
			return fmt.Sprintf("Previous main task [Task %s: %s] is not finished, can not create new main task", mgr.CurrentMainTask.TaskID, mgr.CurrentMainTask.Content)
		}
		mgr.TaskList = append(mgr.TaskList, &task)
		mgr.CurrentMainTask = &task
		task.TaskID = fmt.Sprintf("%d", len(mgr.TaskList))
	}
	return fmt.Sprintf("Create new task [Task %s: %s]", task.TaskID, task.Content)
}
func (mgr *TaskContextMgr) writeTaskList(buf *bytes.Buffer) {
	buf.WriteString("# Current Task List\n\n")
	for _, mainTask := range mgr.TaskList {
		buf.WriteString(fmt.Sprintf("Task %s: %s (Status: %s)\n", mainTask.TaskID, mainTask.Content, mainTask.Status))
		if mainTask.Status == Completed {
			buf.WriteString(fmt.Sprintf("Task %s Summary: %s\n", mainTask.TaskID, mainTask.Summary))
		}
		if mainTask.Status == Progress && len(mainTask.SubTasks) != 0 {
			buf.WriteString("SubTasks:\n")
			for _, subTask := range mainTask.SubTasks {
				buf.WriteString(fmt.Sprintf("- Task %s: %s (Status: %s)\n", subTask.TaskID, subTask.Content, subTask.Status))
				if subTask.Status == Completed {
					buf.WriteString(fmt.Sprintf("  Task %s Summary: %s\n", subTask.TaskID, subTask.Summary))
				}
			}
		}
		buf.WriteByte('\n')
	}
	if mgr.CurrentMainTask == nil {
		buf.WriteString("No in progress task\n")
	} else {
		mainTask := mgr.CurrentMainTask
		buf.WriteString("You are now working on:\n")
		buf.WriteString(fmt.Sprintf("Task %s: %s (Status: %s)\n", mainTask.TaskID, mainTask.Content, mainTask.Status))
		if mgr.CurrentSubTask != nil {
			buf.WriteString("SubTasks:\n")
			buf.WriteString(fmt.Sprintf("- Task %s: %s (Status: %s)\n", mgr.CurrentSubTask.TaskID, mgr.CurrentSubTask.Content, mgr.CurrentSubTask.Status))
		}
	}
	buf.WriteString(`
You can do:
- Keep running the current task, use tools to solve the task.
- If task is finished, finish the task and set summary for task.
- Create a new task to further solve the user's task.

IMPORTANT: If there is NO in progress task, you MUST declare a task first before you execute any actions.
`)
}

func (mgr *TaskContextMgr) WriteContext(buf *bytes.Buffer) {
	buf.WriteString("### TASK MANAGEMENT ###\n")
	buf.WriteString(taskPrompt)
	mgr.writeTaskList(buf)
	buf.WriteString("### END OF TASK MANAGEMENT ###\n")
}

func (mgr *TaskContextMgr) GetToolDef() []model.ToolDef {
	createTaskHandler := func(argsStr string) (string, error) {
		args := struct {
			Content string
			Type    string
		}{}
		err := json.Unmarshal([]byte(argsStr), &args)
		if err != nil {
			return "", err
		}
		var subtask bool
		if args.Type == "subtask" {
			subtask = true
		} else if args.Type == "main task" {
			subtask = false
		}
		return mgr.createTask(args.Content, subtask), nil
	}
	finishTaskHandler := func(argsStr string) (string, error) {
		args := struct {
			Summary string
			Type    string
		}{}
		err := json.Unmarshal([]byte(argsStr), &args)
		if err != nil {
			return "", err
		}
		var subtask bool
		if args.Type == "subtask" {
			subtask = true
		} else if args.Type == "main task" {
			subtask = false
		}
		return mgr.finishTask(args.Summary, subtask), nil
	}
	res := []model.ToolDef{
		{FunctionDefinition: createTask, Handler: createTaskHandler},
		{FunctionDefinition: finishTask, Handler: finishTaskHandler},
	}
	return res
}
