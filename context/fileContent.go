package context

import (
	"bytes"
	"fmt"
	"llm_dev/codebase/impl"
	"llm_dev/model"
	"path/filepath"
	"sort"

	"github.com/sashabaranov/go-openai"
	"github.com/sashabaranov/go-openai/jsonschema"
)

var loadFileTool = openai.FunctionDefinition{
	Name: "",
	Description: `
Load the context of a given file.
For source code file, it will load all the definition in the source code.
For example 'load_context_file src/foo.go' will load all definition in source code src/foo.go.
Use this tool when you want to examine the content in certain file
	`,
	Parameters: jsonschema.Definition{
		Type: jsonschema.Object,
		Properties: map[string]jsonschema.Definition{
			"file": {
				Type: jsonschema.Array,
				Description: `
the file path array to load, e.g. ["src/foo.go", "src/test/bar.go"]
				`,
			},
		},
		Required: []string{"file"},
	},
}

var loadFileDefsTool = openai.FunctionDefinition{
	Name: "",
	Description: `
Load the context of some definition in a given file.
For example, given code block in file src/foo.go
`+"```" + `
# src/foo.go
var baseUrl string
type File struct {
	a int
	b string
}

func GetFileContent(file string)
` + "```" + `
the code block just show the definition in the code file, if you want to get the detailed content of some definiton,
use this tool to load context of definiton, you should specify two parameters:
- the file path, e.g. src/foo.go
- an array of the definition names you want to load, struct name, function name, variable name, e.g. ["baseUrl", "File", "GetFileContent"]
	`,
	Parameters: jsonschema.Definition{
		Type: jsonschema.Object,
		Properties: map[string]jsonschema.Definition{
			"file": {
				Type:        jsonschema.String,
				Description: "the file path to load, e.g. src/foo.go",
			},
			"defsName" : {
				Type:        jsonschema.Array,
				Description: `an array of the definition names you want to load, struct name, function name, variable name, e.g. ["baseUrl", "File", "GetFileContent"]`,
			}
		},
		Required: []string{"file", "defsName"},
	},
}

type FileContentCtxMgr struct {
	rootPath           string
	fileMap  			map[string]*impl.FileDirInfo
	autoLoadCtx      map[string]*CodeFile
	buildCodeBaseCtxop *impl.BuildCodeBaseCtxOps
}

func (mgr *FileContentCtxMgr) writeExternalDefs(buf *bytes.Buffer) {
	fileChan := mgr.buildCodeBaseCtxop.WalkProjectFileTree()
	for file := range fileChan {
		fdInfo := mgr.fileMap[file]
		if fdInfo == nil {
			continue
		}
	if fd.IsDir {
		buf.WriteString(headLine(fd.RelPath))
		for file, _ := range contentRange {
			buf.WriteString(headLine(file))
		}
	} else {
		if len(contentRange) != 1 {
			return
		}
		buf.WriteString("# " + fd.RelPath)
	}
	}
}

func (mgr *FileContentCtxMgr) GetToolDef() model.ToolDef {
	handler := func(args string) {
		mgr.loadFile("")
	}
	def := model.ToolDef{
		FunctionDefinition: loadFileTool,
		Handler:            handler,
	}
	return def
}

func (mgr *FileContentCtxMgr) loadFile(relPath string) error {
	if mgr.autoLoadCtx[relPath] == nil {
		codeFile := NewCodeFile(relPath)
		mgr.autoLoadCtx[relPath] = &codeFile
	}
	codeFile := mgr.autoLoadCtx[relPath]
	return codeFile.loadAllDefs(mgr.buildCodeBaseCtxop)
}
func (mgr *FileContentCtxMgr) loadDefs(relPath string, identifier string) error {
	if mgr.autoLoadCtx[relPath] == nil {
		codeFile := NewCodeFile(relPath)
		mgr.autoLoadCtx[relPath] = &codeFile
	}
	codeFile := mgr.autoLoadCtx[relPath]
	return codeFile.loadDefs(identifier, mgr.buildCodeBaseCtxop)
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

func (file *CodeFile) loadAllDefs(op *impl.BuildCodeBaseCtxOps) error {
	if file.defs != nil {
		return nil
	}
	filter := impl.GenDefFilter(&file.path, nil, nil)
	res := op.FindDefs(filter)
	if len(res) == 0 {
		return fmt.Errorf("load file %s all definition fail", file.path)
	}
	file.defs = res
	return nil
}
func (file *CodeFile) loadDefs(identifier string, op *impl.BuildCodeBaseCtxOps) error {
	filter := impl.GenDefFilter(&file.path, &identifier, nil)
	res := op.FindDefs(filter)
	if len(res) == 0 {
		return fmt.Errorf("load file %s %s definition fail", file.path, identifier)
	}
	file.loadedDefs = addDefs(file.loadedDefs, res)
	return nil
}
func addDefs(defs []impl.Definition, new []impl.Definition) []impl.Definition {
	res := append(defs, new...)
	sort.Slice(res, func(i, j int) bool {
		return res[i].Content[0] < res[j].Content[0]
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
