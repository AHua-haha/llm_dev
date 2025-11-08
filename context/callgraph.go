package context

import (
	"bytes"
	"encoding/json"
	"fmt"
	"llm_dev/codebase/impl"
	"llm_dev/model"
	"llm_dev/utils"
	"path/filepath"

	"github.com/rs/zerolog/log"
	"github.com/sashabaranov/go-openai"
	"github.com/sashabaranov/go-openai/jsonschema"
	"go.mongodb.org/mongo-driver/bson"
)

var findReference = openai.FunctionDefinition{
	Name: "find_reference",
	Description: `
This tool is used for finding where some definition is used or referenced. You can use this tool to find where some function or type is used.
This tool help you understand the codebase call graph.

<example>
Given some code in utils.go

- codebase/common/utils.go

277| type ContextHandler struct {
278|    OutputChan chan map[string]any
279|    ctxValue   map[string]any
280|    handler    HandlerFunc
281| }
...
309| func GetAs[T any](ctx *ContextHandler, key string) T {

function call: find_reference file = codebase/common/utils.go, name = GetAs, line = 309. find definition which calls function GetAs.
function call: find_reference file = codebase/common/utils.go, name = ContextHandler, line = 277. find all the definition which uses the struct ContextHandler.
</example>
	`,
	Parameters: jsonschema.Definition{
		Type: jsonschema.Object,
		Properties: map[string]jsonschema.Definition{
			"file": {
				Type:        jsonschema.String,
				Description: "the file path of the code e.g. src/codebase",
			},
			"name": {
				Type:        jsonschema.String,
				Description: "the name of the function or type",
			},
			"line": {
				Type:        jsonschema.Number,
				Description: "the line num where the function name or type name is declared",
			},
		},
		Required: []string{"file", "name", "line"},
	},
}

var findDefUsed = openai.FunctionDefinition{
	Name: "find_used_definition",
	Description: `
This tool is used for finding all the definition used within some function or type struct.
Use this tool when you do not konw what some symbols actually refer to, where is the function or type is declared.
IMPORTANT: DO NOT guess the definition based on obly the symbol name, it may leader to wrong definition. You should always use this tool to find the accurate definition used.

<example>
Given some code in utils.go

- codebase/common/utils.go

277| type ContextHandler struct {
278|    OutputChan chan map[string]any
279|    ctxValue   map[string]any
280|    handler    HandlerFunc
281| }
...
309| func GetAs[T any](ctx *ContextHandler, key string) T {

function call: find_used_definition file = codebase/common/utils.go, name = GetAs, line = 309. find all the definition used within the function GetAs.
function call: find_used_definition file = codebase/common/utils.go, name = ContextHandler, line = 277. find all the definition used within the struct ContextHandler.
</example>
	`,
	Parameters: jsonschema.Definition{
		Type: jsonschema.Object,
		Properties: map[string]jsonschema.Definition{
			"file": {
				Type:        jsonschema.String,
				Description: "the file path of the code e.g. src/codebase",
			},
			"name": {
				Type:        jsonschema.String,
				Description: "the name of the function or type",
			},
			"line": {
				Type:        jsonschema.Number,
				Description: "the line num where the function name or type name is declared",
			},
		},
		Required: []string{"file", "name", "line"},
	},
}

type CallGraphContextMgr struct {
	rootPath    string
	buildCtxOps *impl.BuildCodeBaseCtxOps
}

func (mgr *CallGraphContextMgr) genReferenceOutput(usedDefs []impl.UsedDef) string {
	var buf bytes.Buffer
	defMap := make(map[string][]impl.UsedDef)
	for _, useddef := range usedDefs {
		if useddef.Isdependency {
			continue
		}
		defMap[useddef.File] = append(defMap[useddef.File], useddef)
	}
	buf.WriteString("# The following code use the definition\n\n")
	for file, defs := range defMap {
		fc := utils.FileContent{}
		for _, usedef := range defs {
			def := impl.Definition{
				RelFile:    usedef.File,
				Identifier: usedef.Identifier,
				Keyword:    usedef.Keyword,
			}
			findDef, err := mgr.buildCtxOps.FindOneDef(def)
			if err != nil {
				log.Error().Err(err).Msg("find exact one def fail")
				continue
			}
			fc.AddChunk(findDef.Summary)
		}
		buf.WriteString(fmt.Sprintf("- %s\n\n", file))
		fc.WriteContent(&buf, filepath.Join(mgr.rootPath, file))
	}
	buf.WriteByte('\n')
	return buf.String()
}

func (mgr *CallGraphContextMgr) genUseOutput(usedDefs []impl.UsedDef) string {
	var buf bytes.Buffer
	defMap := make(map[string][]impl.UsedDef)
	dependencyDefMap := make(map[string][]impl.UsedDef)
	for _, useddef := range usedDefs {
		if useddef.Isdependency {
			dependencyDefMap[useddef.PkgPath] = append(dependencyDefMap[useddef.PkgPath], useddef)
		} else {
			defMap[useddef.DefFile] = append(defMap[useddef.DefFile], useddef)
		}
	}
	buf.WriteString("# Use Definition In the codebase\n\n")
	for file, defs := range defMap {
		fc := utils.FileContent{}
		for _, usedef := range defs {
			def := impl.Definition{
				RelFile:    usedef.DefFile,
				Identifier: usedef.DefIdentifier,
				Keyword:    usedef.DefKeyword,
			}
			findDef, err := mgr.buildCtxOps.FindOneDef(def)
			if err != nil {
				log.Error().Err(err).Msg("find exact one def fail")
				continue
			}
			fc.AddChunk(findDef.Summary)
		}
		buf.WriteString(fmt.Sprintf("- %s\n\n", file))
		fc.WriteContent(&buf, filepath.Join(mgr.rootPath, file))
	}
	buf.WriteByte('\n')
	buf.WriteString("# Use Definition from Dependency\n\n")
	for pkg, useddefs := range dependencyDefMap {
		buf.WriteString(fmt.Sprintf("- Use pkg %s\n", pkg))
		size := len(useddefs)
		for i, usedef := range useddefs {
			kind := usedef.DefKeyword[0]
			if kind == "type" {
				buf.WriteString(fmt.Sprintf("%s %s", usedef.DefKeyword[0], usedef.DefKeyword[1]))
			}
			if kind == "var" {
				buf.WriteString(fmt.Sprintf("%s %s %s", usedef.DefKeyword[0], usedef.DefKeyword[1], usedef.DefKeyword[2]))
			}
			if kind == "function" {
				buf.WriteString(fmt.Sprintf("%s %s", usedef.DefKeyword[0], usedef.DefKeyword[1]))
			}
			if kind == "method" {
				buf.WriteString(fmt.Sprintf("%s %s.%s", usedef.DefKeyword[0], usedef.DefKeyword[2], usedef.DefKeyword[1]))
			}
			if i != size-1 {
				buf.WriteString(", ")
			}
		}
		buf.WriteString("\n\n")
	}
	buf.WriteByte('\n')
	return buf.String()
}

func (mgr *CallGraphContextMgr) findReference(file string, identifier string, line uint) ([]impl.UsedDef, error) {
	filter := bson.M{
		"relfile":           file,
		"identifier":        identifier,
		"content.startline": line,
	}
	res := mgr.buildCtxOps.FindDefs(filter)
	if len(res) != 1 {
		return nil, fmt.Errorf("could not identify the exact definition using %s %s %d", file, identifier, line)
	}
	def := res[0]
	useDefFilter := bson.M{
		"deffile":       def.RelFile,
		"defidentifier": def.Identifier,
		"defkeyword": bson.M{
			"$all": def.Keyword,
		},
	}
	useDefRes := mgr.buildCtxOps.FindUsedDefs(useDefFilter)
	return useDefRes, nil
}
func (mgr *CallGraphContextMgr) findUsedDefs(file string, identifier string, line uint) ([]impl.UsedDef, error) {
	filter := bson.M{
		"relfile":           file,
		"identifier":        identifier,
		"content.startline": line,
	}
	res := mgr.buildCtxOps.FindDefs(filter)
	if len(res) != 1 {
		return nil, fmt.Errorf("could not identify the exact definition using %s %s %d", file, identifier, line)
	}
	def := res[0]
	useDefFilter := bson.M{
		"file":       def.RelFile,
		"identifier": def.Identifier,
		"keyword": bson.M{
			"$all": def.Keyword,
		},
	}
	useDefRes := mgr.buildCtxOps.FindUsedDefs(useDefFilter)
	return useDefRes, nil
}
func (mgr *CallGraphContextMgr) WriteContext(buf *bytes.Buffer) {
}

func (mgr *CallGraphContextMgr) GetToolDef() []model.ToolDef {
	findDefHandler := func(argsStr string) (string, error) {
		args := struct {
			File string
			Name string
			Line uint
		}{}
		err := json.Unmarshal([]byte(argsStr), &args)
		if err != nil {
			return "", err
		}
		usedef, err := mgr.findUsedDefs(args.File, args.Name, args.Line)
		if err != nil {
			return "", err
		}
		res := mgr.genUseOutput(usedef)
		return res, nil
	}
	findRefHandler := func(argsStr string) (string, error) {
		args := struct {
			File string
			Name string
			Line uint
		}{}
		err := json.Unmarshal([]byte(argsStr), &args)
		if err != nil {
			return "", err
		}
		usedef, err := mgr.findReference(args.File, args.Name, args.Line)
		if err != nil {
			return "", err
		}
		res := mgr.genReferenceOutput(usedef)
		return res, nil
	}
	res := []model.ToolDef{
		{FunctionDefinition: findDefUsed, Handler: findDefHandler},
		{FunctionDefinition: findReference, Handler: findRefHandler},
	}
	return res
}
