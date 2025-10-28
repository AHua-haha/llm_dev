package context

import (
	"bytes"
	"fmt"
	"llm_dev/codebase/impl"
	"llm_dev/database"
	"llm_dev/model"
	"os"
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
				Type:        jsonschema.Array,
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
	}
	return mgr
}

func (mgr *FileContentCtxMgr) writeFileContent(buf *bytes.Buffer, relPath string, ranges []impl.ContentRange) {
	data, err := os.ReadFile(filepath.Join(mgr.rootPath, relPath))
	if err != nil {
		return
	}
	for _, r := range ranges {
		s := r[0]
		e := r[1]
		buf.Write(data[s:e])
		buf.WriteByte('\n')
	}
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
		for file, ranges := range contentRange {
			buf.WriteString(fmt.Sprintf("- %s\n", file))
			mgr.writeFileContent(buf, file, ranges)
			buf.WriteByte('\n')
		}
	} else {
		if len(contentRange) != 1 {
			return
		}
		ranges := contentRange[fd.RelPath]
		buf.WriteString(fmt.Sprintf("# %s\n\n", fd.RelPath))
		mgr.writeFileContent(buf, fd.RelPath, ranges)
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
	buf.WriteString("## END OF CODEBASE USED DEFINITION ##\n")
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
	buf.WriteString("## END OF CODEBASE FILE TREE ##\n")
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
