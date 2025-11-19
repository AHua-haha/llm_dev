package context

import (
	"bytes"
	"encoding/json"
	"fmt"
	"llm_dev/codebase/impl"
	"llm_dev/model"
	"llm_dev/utils"
	"path/filepath"
	"sort"

	"github.com/sashabaranov/go-openai"
	"github.com/sashabaranov/go-openai/jsonschema"
)

var loadFileTool = openai.FunctionDefinition{
	Name:   "load_file_definition",
	Strict: true,
	Description: `
Load all the definition of a given source code file.
IMPORTANT: this ONLY load the declaration of the definition, the implementation content is omitted.
IMPORTANT: use 'load_definition_detail' to check the whole content of the definition.

Usage:
- use this to see what definition are declared in a file, understand the source code file from a high level.
- identify which defintion is relevant, load the detail content then.
- 
	`,
	Parameters: jsonschema.Definition{
		Type:                 jsonschema.Object,
		AdditionalProperties: false,
		Properties: map[string]jsonschema.Definition{
			"file": {
				Type: jsonschema.Array,
				Items: &jsonschema.Definition{
					Type: jsonschema.String,
				},
				Description: `
the file path array to load, e.g. ["src/foo.go", "src/test/bar.go"]
				`,
			},
		},
		Required: []string{"file"},
	},
}

var loadFileDefsTool = openai.FunctionDefinition{
	Name:   "load_definition_detail",
	Strict: true,
	Description: `
Load all the content of some definition in a given file.

<example>
# src/foo.go
var baseUrl string
type File struct {
	a int
	b string
}

func GetFileContent(file string)

func testrun()

function call load_definition_detail file= src/foo.go identifier={"GetFileContent", "testrun"}
</example>

Usage:
- use the definition identifier to get the full content of the implementation.

`,
	Parameters: jsonschema.Definition{
		Type:                 jsonschema.Object,
		AdditionalProperties: false,
		Properties: map[string]jsonschema.Definition{
			"file": {
				Type:        jsonschema.String,
				Description: "the file path to load, e.g. src/foo.go",
			},
			"identifiers": {
				Type: jsonschema.Array,
				Items: &jsonschema.Definition{
					Type: jsonschema.String,
				},
				Description: `an array of the definition identifier you want to load, struct name, function name, variable name, e.g. ["baseUrl", "File", "GetFileContent"]`,
			},
		},
		Required: []string{"file", "identifiers"},
	},
}

type FileContentCtxMgr struct {
	rootPath           string
	BuildCodeBaseCtxop *impl.BuildCodeBaseCtxOps

	autoLoadCtx map[string]*CodeFile
}

func NewFileCtxMgr(root string, buildOp *impl.BuildCodeBaseCtxOps) FileContentCtxMgr {
	mgr := FileContentCtxMgr{
		rootPath:           root,
		BuildCodeBaseCtxop: buildOp,
		autoLoadCtx:        make(map[string]*CodeFile),
	}
	return mgr
}

func (mgr *FileContentCtxMgr) writeAutoLoadCtx(buf *bytes.Buffer) {
	description := `
This section shows all the loaded definitions of source code.
You have access to 'load_file_definition' and 'load_definition_detail' tools to load the definition. 'find_reference' and 'find_used_definition' tools to search for relevant definitions, 'get_directory_overview' tool to get the definition overview.

Usage:
- from top down, use 'get_directory_overview' tool to get the used definition of a directory. Get a overall understanding of the directory and how the directory is used and what in the directory is used.
- Based on the used definition in directory, search for relevant context from the used definition.
- Use 'load_file_definition' tool to load all the definitions in a file, identify which definition is relevant.
- Then use 'load_definition_detail' tool to load the complete implementation of the definition.
- Analyze the functionality of definitions, use 'find_reference' tool to examine where the definition is used and how the definition is used, analyze what the definition is used for.
- Analyze definition implementation details, use 'find_used_definition' tool to examine the exact definition used within one function.

When to use the definition oriented tools
- if you know the exact defintion infomation, use thers tools to get the infomation you want.
- if you want to search for the exact definition type infomation, not just with the same name text. use these definition tools.
- if you want to search for the definition relation, like used and referenced, use thses definition tools

When to use the 'grep' tools
- if you only have plain text infomation, use grep to search.

`
	buf.WriteString("## CODEBASE DEFINITION ##\n\n")
	buf.WriteString(description)
	buf.WriteString("```\n")
	for path, codefile := range mgr.autoLoadCtx {
		fc := codefile.getContent()
		buf.WriteString(fmt.Sprintf("# %s\n\n", path))
		fc.WriteContent(buf, filepath.Join(mgr.rootPath, path))
	}
	buf.WriteString("```\n")
	buf.WriteString("## END OF CODEBASE DEFINITION ##\n\n")
}

func (mgr *FileContentCtxMgr) WriteContext(buf *bytes.Buffer) {
	mgr.writeAutoLoadCtx(buf)
}

func (mgr *FileContentCtxMgr) GetToolDef() []model.ToolDef {
	loadFileHandler := func(argsStr string) (string, error) {
		args := struct {
			File []string
		}{}
		err := json.Unmarshal([]byte(argsStr), &args)
		if err != nil {
			return "", err
		}
		res := "IMPORTANT: This only load the definitions in file, the implementations of definition is omitted. You can use 'load_definition_context' tool to load it.\n"
		for _, v := range args.File {
			err := mgr.loadFile(v)
			if err != nil {
				res += fmt.Sprintf("load file context for %s failed, error: %v\n", v, err)
			} else {
				res += fmt.Sprintf("load file context for %s success\n", v)
			}
		}
		return res, nil
	}
	loadDefsHandler := func(argsStr string) (string, error) {
		args := struct {
			File        string
			Identifiers []string
		}{}
		err := json.Unmarshal([]byte(argsStr), &args)
		if err != nil {
			return "", err
		}
		res := ""
		for _, name := range args.Identifiers {
			err := mgr.loadDefs(args.File, name)
			if err != nil {
				res += fmt.Sprintf("load file %s %s definition failed, error: %v\n", args.File, name, err)
			} else {
				res += fmt.Sprintf("load file %s %s definition success\n", args.File, name)
			}
		}
		return res, nil
	}
	res := []model.ToolDef{
		{FunctionDefinition: loadFileTool, Handler: loadFileHandler},
		{FunctionDefinition: loadFileDefsTool, Handler: loadDefsHandler},
	}
	return res
}

func (mgr *FileContentCtxMgr) loadFile(relPath string) error {
	if mgr.autoLoadCtx[relPath] == nil {
		codeFile := NewCodeFile(relPath)
		mgr.autoLoadCtx[relPath] = &codeFile
	}
	codeFile := mgr.autoLoadCtx[relPath]
	return codeFile.loadAllDefs(mgr.BuildCodeBaseCtxop)
}
func (mgr *FileContentCtxMgr) loadDefs(relPath string, identifier string) error {
	if mgr.autoLoadCtx[relPath] == nil {
		codeFile := NewCodeFile(relPath)
		mgr.autoLoadCtx[relPath] = &codeFile
	}
	codeFile := mgr.autoLoadCtx[relPath]
	return codeFile.loadDefs(identifier, mgr.BuildCodeBaseCtxop)
}

type CodeFile struct {
	path       string
	ext        string
	defs       []impl.Definition
	loadedDefs []impl.Definition
	usedType   []impl.TypeInfo
}

func NewCodeFile(path string) CodeFile {
	return CodeFile{
		path: path,
		ext:  filepath.Ext(path),
	}
}
func (file *CodeFile) getContent() utils.FileContent {
	fc := utils.FileContent{}
	for _, def := range file.defs {
		fc.AddChunk(def.Summary)
	}
	for _, def := range file.loadedDefs {
		fc.AddChunk(def.Content)
	}
	return fc
}

func (file *CodeFile) loadAllDefs(op *impl.BuildCodeBaseCtxOps) error {
	if file.defs != nil {
		return nil
	}
	filter := impl.GenDefFilter(&file.path, nil, nil)
	res := op.FindDefs(filter)
	if len(res) == 0 {
		return fmt.Errorf("file %s definition empty", file.path)
	}
	file.defs = res
	return nil
}
func (file *CodeFile) loadDefs(identifier string, op *impl.BuildCodeBaseCtxOps) error {
	filter := impl.GenDefFilter(&file.path, &identifier, nil)
	res := op.FindDefs(filter)
	if len(res) == 0 {
		return fmt.Errorf("file %s %s definition not found", file.path, identifier)
	}
	file.loadedDefs = addDefs(file.loadedDefs, res)
	return nil
}
func addDefs(defs []impl.Definition, new []impl.Definition) []impl.Definition {
	res := append(defs, new...)
	sort.Slice(res, func(i, j int) bool {
		return res[i].Content.StartLine < res[j].Content.StartLine
	})
	unique := []impl.Definition{}
	resLen := len(res)
	if resLen != 0 {
		unique = append(unique, res[0])
	}
	for i := 1; i < resLen; i++ {
		if res[i].Content != res[i-1].Content {
			unique = append(unique, res[i])
		}
	}

	return unique
}
