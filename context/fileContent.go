package context

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"llm_dev/codebase/impl"
	"llm_dev/database"
	"llm_dev/model"
	"llm_dev/utils"
	"os"
	"path/filepath"
	"sort"

	"github.com/sashabaranov/go-openai"
	"github.com/sashabaranov/go-openai/jsonschema"
)

var loadFileTool = openai.FunctionDefinition{
	Name: "load_file_context",
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
	Name: "load_definition_context",
	Description: `
Load the context of some definition in a given file.
For example, given code block in file src/foo.go
` + "```" + `
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
			"defsName": {
				Type: jsonschema.Array,
				Items: &jsonschema.Definition{
					Type: jsonschema.String,
				},
				Description: `an array of the definition names you want to load, struct name, function name, variable name, e.g. ["baseUrl", "File", "GetFileContent"]`,
			},
		},
		Required: []string{"file", "defsName"},
	},
}

type FileContentCtxMgr struct {
	rootPath           string
	fileMap            map[string]*impl.FileDirInfo
	autoLoadCtx        map[string]*CodeFile
	buildCodeBaseCtxop *impl.BuildCodeBaseCtxOps
}

func NewFileCtxMgr(root string) *FileContentCtxMgr {
	mgr := &FileContentCtxMgr{
		rootPath: root,
		buildCodeBaseCtxop: &impl.BuildCodeBaseCtxOps{
			RootPath: root,
			Db:       database.GetDBClient().Database("llm_dev"),
		},
		autoLoadCtx: make(map[string]*CodeFile),
	}
	return mgr
}

func (mgr *FileContentCtxMgr) writeFdinfo(buf *bytes.Buffer, fd *impl.FileDirInfo) {
	contentRange := fd.GetSummary()
	if len(contentRange) == 0 {
		buf.WriteString(fmt.Sprintf("# %s\n\n", fd.RelPath))
		buf.WriteString("NO Definition Used by Outer code\n\n")
		return
	}
	if fd.IsDir {
		buf.WriteString(fmt.Sprintf("# %s\n\n", fd.RelPath))
		for file, filecontent := range contentRange {
			buf.WriteString(fmt.Sprintf("- %s\n", file))
			filecontent.WriteContent(buf, filepath.Join(mgr.rootPath, file))
			buf.WriteByte('\n')
		}
	} else {
		if len(contentRange) != 1 {
			return
		}
		ranges := contentRange[fd.RelPath]
		buf.WriteString(fmt.Sprintf("# %s\n\n", fd.RelPath))
		ranges.WriteContent(buf, filepath.Join(mgr.rootPath, fd.RelPath))
		buf.WriteByte('\n')
	}
}

func (mgr *FileContentCtxMgr) WriteUsedDefs(buf *bytes.Buffer) {
	description := `
This section shows the definition under certain file or firectory that is being used by some code that is not under the same file or directory.
So fot certain file or directory, the definiton that is only used within the same file or directory is omitted。
This helps you better understand the functionality of a file or directory from the perspective of the whole codebase.

`
	buf.WriteString("## CODEBASE USED DEFINITION ##\n\n")
	buf.WriteString(description)
	buf.WriteString("```\n")
	mgr.fileMap = mgr.buildCodeBaseCtxop.GenFileMap()
	fileChan := mgr.buildCodeBaseCtxop.WalkProjectFileTree()
	for file := range fileChan {
		relPath, _ := filepath.Rel(mgr.rootPath, file.Path)
		fdInfo := mgr.fileMap[relPath]
		if fdInfo == nil {
			continue
		}
		mgr.writeFdinfo(buf, fdInfo)
	}
	buf.WriteString("```\n")
	buf.WriteString("## END OF CODEBASE USED DEFINITION ##\n\n")
}
func (mgr *FileContentCtxMgr) WriteFileTree(buf *bytes.Buffer) {
	buf.WriteString("## CODEBASE FILE TREE ##\n\n")
	buf.WriteString("This section shows the file tree structure of the codebase.\n")
	buf.WriteString("```\n")
	fileChan := mgr.buildCodeBaseCtxop.WalkProjectFileTree()
	for file := range fileChan {
		if !file.D.IsDir() {
			continue
		}
		relPath, _ := filepath.Rel(mgr.rootPath, file.Path)
		entries, err := os.ReadDir(file.Path)
		if err != nil {
			continue
		}
		buf.WriteString(fmt.Sprintf("# %s\n", relPath))
		for _, entry := range entries {
			if entry.IsDir() {
				buf.WriteString(fmt.Sprintf("- dir %s\n", entry.Name()))
			} else {
				buf.WriteString(fmt.Sprintf("- file %s\n", entry.Name()))
			}
		}
		buf.WriteByte('\n')
	}
	buf.WriteString("```\n")
	buf.WriteString("## END OF CODEBASE FILE TREE ##\n\n")
}
func (mgr *FileContentCtxMgr) WriteAutoLoadCtx(buf *bytes.Buffer) {
	description := `
This section shows all the dynamic loaded context in this codebase.
You should:
- Examine the user's request and available codebase context information
- Determine what context is truly relevant for the task.
- If you need certain context, load the relevant context using the tools provided.
- If NO additional context is needed, Continue with your response conversationally

`
	buf.WriteString("## CODEBASE LOADED FILE CONTEXT ##\n\n")
	buf.WriteString(description)
	buf.WriteString("```\n")
	for path, codefile := range mgr.autoLoadCtx {
		fc := codefile.getContent()
		buf.WriteString(fmt.Sprintf("# %s\n\n", path))
		fc.WriteContent(buf, filepath.Join(mgr.rootPath, path))
	}
	buf.WriteString("```\n")
	buf.WriteString("## END OF CODEBASE LOADED FILE CONTEXT ##\n\n")
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
		res := ""
		var e error
		for _, v := range args.File {
			err := mgr.loadFile(v)
			if err != nil {
				res += fmt.Sprintf("load file context for %s failed, error: %v\n", v, err)
				e = errors.Join(e, err)
			} else {
				res += fmt.Sprintf("load file context for %s successn", v)
			}
		}
		return res, e
	}
	loadDefsHandler := func(argsStr string) (string, error) {
		args := struct {
			File     string
			DefsName []string
		}{}
		err := json.Unmarshal([]byte(argsStr), &args)
		if err != nil {
			return "", err
		}
		res := ""
		var e error
		for _, name := range args.DefsName {
			err := mgr.loadDefs(args.File, name)
			if err != nil {
				res += fmt.Sprintf("load file %s %s definition failed, error: %v\n", args.File, name, err)
				e = errors.Join(e, err)
			} else {
				res += fmt.Sprintf("load file %s %s definition success\n", args.File, name)
			}
		}
		return res, e
	}
	var res []model.ToolDef
	res = append(res, model.ToolDef{
		FunctionDefinition: loadFileTool,
		Handler:            loadFileHandler,
	})
	res = append(res, model.ToolDef{
		FunctionDefinition: loadFileDefsTool,
		Handler:            loadDefsHandler,
	})
	return res
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
