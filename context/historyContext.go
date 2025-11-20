package context

import (
	"bytes"
	"encoding/json"
	"fmt"
	"llm_dev/codebase/impl"
	"llm_dev/model"
	"strconv"
	"strings"

	"github.com/sashabaranov/go-openai"
	"github.com/sashabaranov/go-openai/jsonschema"
)

var reviewHistory = openai.FunctionDefinition{
	Name:   "review_history",
	Strict: true,
	Description: `
Review the chat history of the agent.
You can check the user prompt and agent final response and all the important conclusions obtained.
You can then review the detailed conclusion using 'review_conclusion_detail' tools.

IMPORTANT: you MUST review the history at the beginning of a chat to get the relevant context.

`,
	Parameters: jsonschema.Definition{
		Type:                 jsonschema.Object,
		AdditionalProperties: false,
		Properties: map[string]jsonschema.Definition{
			"historylen": {
				Type:        jsonschema.Number,
				Description: "the length of the chat history",
			},
		},
	},
}

var reviewConclusion = openai.FunctionDefinition{
	Name:   "review_conclusion_detail",
	Strict: true,
	Description: `
Review the detail content of conclusions.
Each conclusion has a statement and the references to the file content.
This tool will show the exact reference content of the conclusion.
`,
	Parameters: jsonschema.Definition{
		Type:                 jsonschema.Object,
		AdditionalProperties: false,
		Properties: map[string]jsonschema.Definition{
			"ids": {
				Type:        jsonschema.Array,
				Description: "array of id of the conclusion, e.g. 1.2, 2.2, 4.5",
				Items: &jsonschema.Definition{
					Type:        jsonschema.String,
					Description: "the conclusion id",
				},
			},
		},
	},
}

type UserTask struct {
	userprompt string
	response   string

	conclusions []Conclusion
}

func (task *UserTask) writeTask(buf *bytes.Buffer, id int) {
	buf.WriteString(fmt.Sprintf("User Prompt: %s\n", task.userprompt))
	if len(task.conclusions) != 0 {
		buf.WriteString("<conclusions>\n")
		for i, c := range task.conclusions {
			buf.WriteString(fmt.Sprintf("%d.%d %s\n", id, i, c.Statement))
		}
		buf.WriteString("</conclusions>\n")
	}
	buf.WriteString("<final response>\n")
	if len(task.response) > 150 {
		buf.WriteString(task.response[:150])
		buf.WriteString("...\n")
	} else {
		buf.WriteString(task.response)
	}
	buf.WriteString("</final response>\n\n")
}

type HistoryContextMgr struct {
	userTasks []UserTask
	BuildOps  *impl.BuildCodeBaseCtxOps
}

func (mgr *HistoryContextMgr) reviewConclusions(ids []string) string {
	var buf bytes.Buffer
	for _, id := range ids {
		fields := strings.Split(id, ".")
		size := len(mgr.userTasks)
		usertaskID, _ := strconv.Atoi(fields[0])
		conclusionID, _ := strconv.Atoi(fields[1])
		conclusion := mgr.userTasks[size-usertaskID].conclusions[conclusionID]
		buf.WriteString(fmt.Sprintf("Type: %s, Statement: %s\n", conclusion.Type, conclusion.Statement))
		if len(conclusion.References) != 0 {
			buf.WriteString("References:\n")
			for _, ref := range conclusion.References {
				buf.WriteString(fmt.Sprintf("- %s\n", ref))
			}
		}
		buf.WriteByte('\n')
	}
	return buf.String()
}

func (mgr *HistoryContextMgr) reviewHistory() string {
	var buf bytes.Buffer
	buf.WriteString("# Chat History\n\n")
	size := len(mgr.userTasks)
	if size == 0 {
		buf.WriteString("NO chat history\n")
	} else {
		for i := size - 1; i >= 0; i-- {
			mgr.userTasks[i].writeTask(&buf, i+1)
		}
	}
	buf.WriteString(`
You can use 'review_conclusion_detail' tool to review the detail content of the conclusion that is helpful to solve the task.
`)
	return buf.String()
}

func (mgr *HistoryContextMgr) revealRefs(refs []*string) {
	var defs []impl.Definition

	files := make(map[string]bool)
	for _, ref := range refs {
		fields := strings.FieldsFunc(*ref, func(r rune) bool {
			return r == ':' || r == '-'
		})
		files[fields[0]] = true
	}
	for f, _ := range files {
		filter := impl.GenDefFilter(&f, nil, nil)
		res := mgr.BuildOps.FindDefs(filter)
		defs = append(defs, res...)
	}
	for _, ref := range refs {
		fields := strings.FieldsFunc(*ref, func(r rune) bool {
			return r == ':' || r == '-'
		})
		s, _ := strconv.Atoi(fields[1])
		e := s
		if len(fields) == 3 {
			e, _ = strconv.Atoi(fields[2])
		}
		var match *impl.Definition
		for i, def := range defs {
			if def.RelFile == fields[0] && def.Content.StartLine <= uint(s) && def.Content.EndLine >= uint(e) {
				match = &defs[i]
				break
			}
		}
		if match == nil {
			continue
		}
		*ref = fmt.Sprintf("%s in definition '%s'", *ref, match.Identifier)
	}
}

func (mgr *HistoryContextMgr) RecordUserTask(userprompt string, response string, conclusions []Conclusion) {
	refsPtr := []*string{}
	for i := range conclusions {
		for j := range conclusions[i].References {
			refsPtr = append(refsPtr, &conclusions[i].References[j])
		}
	}
	mgr.revealRefs(refsPtr)
	mgr.userTasks = append(mgr.userTasks, UserTask{
		userprompt:  userprompt,
		response:    response,
		conclusions: conclusions,
	})
}
func (mgr *HistoryContextMgr) WriteContext(buf *bytes.Buffer) {
}
func (mgr *HistoryContextMgr) GetToolDef() []model.ToolDef {
	reviewHistoryHandler := func(argsStr string) (string, error) {
		return mgr.reviewHistory(), nil
	}
	reviewConclusionHandler := func(argsStr string) (string, error) {
		args := struct {
			Ids []string
		}{}
		err := json.Unmarshal([]byte(argsStr), &args)
		if err != nil {
			return "", err
		}
		return mgr.reviewConclusions(args.Ids), nil
	}
	res := []model.ToolDef{
		{FunctionDefinition: reviewHistory, Handler: reviewHistoryHandler},
		{FunctionDefinition: reviewConclusion, Handler: reviewConclusionHandler},
	}
	return res
}
