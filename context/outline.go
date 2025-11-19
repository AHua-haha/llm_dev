package context

import (
	"bytes"
	"encoding/json"
	"fmt"
	"llm_dev/codebase/impl"
	"llm_dev/model"
	"llm_dev/utils"
	"os"
	"path/filepath"

	"github.com/rs/zerolog/log"
	"github.com/sashabaranov/go-openai"
	"github.com/sashabaranov/go-openai/jsonschema"
)

var dirOverview = openai.FunctionDefinition{
	Name:   "get_directory_overview",
	Strict: true,
	Description: `
This tool is used for load the definition overview for a file or directory.
The definition overview shows the definition which are declared in the directory and used by code out of the directory.
The definition overview shows how certain file or directory is used by other code.
<example>
directory A has the following structure.
# A
- File test.go
- dir test
- dir utils
- dir codebase

function call: get_directory_overview path = "A/test.go", load the definition overview for file A/test.go.
function call: get_directory_overview path = "A/codebase", load the definition overview for directory A/codebase
</example>
	`,
	Parameters: jsonschema.Definition{
		Type:                 jsonschema.Object,
		AdditionalProperties: false,
		Properties: map[string]jsonschema.Definition{
			"path": {
				Type:        jsonschema.String,
				Description: "the file path to load, e.g. src/codebase",
			},
		},
		Required: []string{"path"},
	},
}

type OutlineContextMgr struct {
	rootPath   string
	buildCtxOp *impl.BuildCodeBaseCtxOps
}

func NewOutlineCtxMgr(root string, buildOp *impl.BuildCodeBaseCtxOps) OutlineContextMgr {
	return OutlineContextMgr{
		rootPath:   root,
		buildCtxOp: buildOp,
	}
}

func (mgr *OutlineContextMgr) writeLeafNode(buf *bytes.Buffer, path string, isDir bool) {
	usedDefs := mgr.buildCtxOp.FindUsedDefOutline(path)
	defByFile := make(map[string]*utils.FileContent)
	for _, def := range usedDefs {
		fc, exist := defByFile[def.RelFile]
		if !exist {
			defByFile[def.RelFile] = &utils.FileContent{}
			fc = defByFile[def.RelFile]
		}
		fc.AddChunk(def.Summary)
	}
	if len(defByFile) == 0 {
		buf.WriteString(fmt.Sprintf("# %s\n\n", path))
		buf.WriteString("NO Definition Used by Outer code\n\n")
		return
	}
	if isDir {
		buf.WriteString(fmt.Sprintf("# %s\n\n", path))
		for path, fc := range defByFile {
			file := filepath.Join(mgr.rootPath, path)
			buf.WriteString(fmt.Sprintf("- %s\n", path))
			err := fc.WriteContent(buf, file)
			if err != nil {
				log.Error().Err(err).Msg("write file content fail")
			}
			buf.WriteByte('\n')
		}
	} else {
		if len(defByFile) > 1 {
			log.Fatal().Any("file", path).Msg("def by file len is more than 1 for file")
		}
		fc := defByFile[path]
		buf.WriteString(fmt.Sprintf("# %s\n\n", path))
		fc.WriteContent(buf, filepath.Join(mgr.rootPath, path))
		buf.WriteByte('\n')
	}
}

func (mgr *OutlineContextMgr) WriteContext(buf *bytes.Buffer) {
}

func (mgr *OutlineContextMgr) writeOverview(buf *bytes.Buffer, path string) {
	info, err := os.Stat(filepath.Join(mgr.rootPath, path))
	if err != nil {
		buf.WriteString(fmt.Sprintf("load definition overview for %s failed\n", path))
		return
	}
	dir := path
	if !info.IsDir() {
		dir = filepath.Dir(path)
	}
	entries, err := os.ReadDir(filepath.Join(mgr.rootPath, dir))
	if err != nil {
		buf.WriteString(fmt.Sprintf("load definition overview for %s failed\n", path))
		return
	}
	buf.WriteString(fmt.Sprintf("load definition overview for %s success\n\n", path))
	buf.WriteString("Directory Structure\n")
	buf.WriteString(fmt.Sprintf("# %s\n", dir))
	for _, entry := range entries {
		if entry.IsDir() {
			buf.WriteString(fmt.Sprintf("- dir %s\n", entry.Name()))
		} else {
			buf.WriteString(fmt.Sprintf("- file %s\n", entry.Name()))
		}
	}
	buf.WriteByte('\n')
	buf.WriteString("Directory Definition Overview:\n")
	mgr.writeLeafNode(buf, path, info.IsDir())
	buf.WriteString("IMPORTANT: This only shows the definition declared within one directory or file and used by code out of this directory or file. Definitions not being used is omitted. You can use 'load_file_definition' and 'load_definition_detail' tools to check the detailed context.\n")
}

func (mgr *OutlineContextMgr) GetToolDef() []model.ToolDef {
	dirOverviewHandler := func(argsStr string) (string, error) {
		args := struct {
			Path string
		}{}
		err := json.Unmarshal([]byte(argsStr), &args)
		if err != nil {
			return "", err
		}
		var buf bytes.Buffer
		mgr.writeOverview(&buf, args.Path)
		return buf.String(), nil
	}
	res := []model.ToolDef{
		{FunctionDefinition: dirOverview, Handler: dirOverviewHandler},
	}
	return res
}
