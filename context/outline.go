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
	"strings"

	"github.com/rs/zerolog/log"
	ignore "github.com/sabhiram/go-gitignore"
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

type FileTreeNode struct {
	isOpen   bool
	isDir    bool
	relpath  string
	children map[string]*FileTreeNode
}

func (node *FileTreeNode) close() {
	node.children = nil
	node.isOpen = false
}

func (node *FileTreeNode) open(rootpath string) {
	if !node.isDir {
		return
	}
	if node.isOpen {
		return
	}
	path := filepath.Join(rootpath, node.relpath)
	entries, err := os.ReadDir(path)
	if err != nil {
		log.Error().Err(err).Msg("open dir fail")
		return
	}
	node.children = make(map[string]*FileTreeNode, len(entries))
	for _, entry := range entries {
		node.children[entry.Name()] = &FileTreeNode{
			isOpen:  false,
			isDir:   entry.IsDir(),
			relpath: filepath.Join(node.relpath, entry.Name()),
		}
	}
	node.isOpen = true
}

type OutlineContextMgr struct {
	rootPath   string
	buildCtxOp *impl.BuildCodeBaseCtxOps

	fileTree *FileTreeNode
}

func NewOutlineCtxMgr(root string, buildOp *impl.BuildCodeBaseCtxOps) OutlineContextMgr {
	return OutlineContextMgr{
		rootPath:   root,
		buildCtxOp: buildOp,
		fileTree: &FileTreeNode{
			relpath: ".",
			isDir:   true,
			isOpen:  false,
		},
	}
}
func (mgr *OutlineContextMgr) walkNode(node *FileTreeNode, handler func(*FileTreeNode)) {
	if node == nil {
		return
	}
	handler(node)
	for _, child := range node.children {
		mgr.walkNode(child, handler)
	}
}

func (mgr *OutlineContextMgr) OpenDir(relpath string) (*FileTreeNode, error) {
	node, err := mgr.findFileTreeNode(relpath)
	if err != nil {
		return nil, err
	}
	node.open(mgr.rootPath)
	return node, nil
}

func (mgr *OutlineContextMgr) findFileTreeNode(relpath string) (*FileTreeNode, error) {
	p := filepath.Clean(relpath)
	if p == "." {
		return mgr.fileTree, nil
	}
	parts := strings.Split(p, "/")
	node := mgr.fileTree
	for _, part := range parts {
		if !node.isOpen {
			node.open(mgr.rootPath)
		}
		child, exist := node.children[part]
		if !exist {
			return nil, fmt.Errorf("path not found %s", relpath)
		}
		node = child
	}
	return node, nil
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

func (mgr *OutlineContextMgr) leafNode() []*FileTreeNode {
	res := []*FileTreeNode{}
	ig, err := ignore.CompileIgnoreFile(filepath.Join(mgr.rootPath, ".gitignore"))
	if err != nil {
		log.Error().Msgf("compile ignore failed")
	}
	mgr.walkNode(mgr.fileTree, func(ftn *FileTreeNode) {
		if ftn.isOpen {
			return
		}
		if ig != nil && ig.MatchesPath(ftn.relpath) {
			return
		}
		if !ftn.isDir && filepath.Ext(ftn.relpath) != ".go" {
			return
		}
		res = append(res, ftn)
	})
	return res
}

func (mgr *OutlineContextMgr) writeFileTree(buf *bytes.Buffer) {
	buf.WriteString("## CODEBASE FILE TREE ##\n\n")
	buf.WriteString("This section shows the file tree structure of the codebase.\n")
	buf.WriteString("```\n")
	fileChan := mgr.buildCtxOp.WalkProjectFileTree()
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
func (mgr *OutlineContextMgr) writeOutline(buf *bytes.Buffer) {
	description := `
This section shows the overview for a directory of file. For example, A is a file or directory, it shows
the definition defined in A and used by other code out of A. The format is as following:
<example>
# A
- A/foo
definitions in A/foo
- A/bar
definitions in A/bar
</example>
To help user with task, you should use this section to:
- understand the overall purpose of different module, figure out what each module is used for and what functionality each module provides.
- examine the user's prompt and determine which part in the codebase is relevant with the task and use tools to load the relevant context.

`
	buf.WriteString("## CODEBASE OVERVIEW ##\n\n")
	buf.WriteString(description)
	buf.WriteString("```\n")
	leafNodes := mgr.leafNode()
	for _, node := range leafNodes {
		mgr.writeLeafNode(buf, node.relpath, node.isDir)
	}
	buf.WriteString("```\n")
	buf.WriteString("## END OF CODEBASE OVERVIEW ##\n\n")
}

func (mgr *OutlineContextMgr) WriteContext(buf *bytes.Buffer) {
	mgr.writeFileTree(buf)
	mgr.writeOutline(buf)
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
		var res string
		p := filepath.Dir(args.Path)
		_, err = mgr.OpenDir(p)
		if err != nil {
			res = fmt.Sprintf("load definition overview for %s failed", args.Path)
		} else {
			res = fmt.Sprintf("load definition overview for %s success", args.Path)
		}
		return res, nil
	}
	res := []model.ToolDef{
		{FunctionDefinition: dirOverview, Handler: dirOverviewHandler},
	}
	return res
}
