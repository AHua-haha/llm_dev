package context

import (
	"bytes"
	"fmt"
	"llm_dev/codebase/impl"
	"llm_dev/database"
	"llm_dev/utils"
	"os"
	"path/filepath"
	"strings"

	"github.com/rs/zerolog/log"
	ignore "github.com/sabhiram/go-gitignore"
)

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
	buildCtxOp impl.BuildCodeBaseCtxOps

	fileTree *FileTreeNode
}

func NewOutlineCtxMgr(root string) OutlineContextMgr {
	return OutlineContextMgr{
		rootPath: root,
		buildCtxOp: impl.BuildCodeBaseCtxOps{
			RootPath: root,
			Db:       database.GetDBClient().Database("llm_dev"),
		},
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

func (mgr *OutlineContextMgr) openDir(relpath string) (*FileTreeNode, error) {
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
	mgr.walkNode(mgr.fileTree, func(ftn *FileTreeNode) {
		if ftn.isOpen {
			return
		}
		res = append(res, ftn)
	})
	return res
}
func (mgr *OutlineContextMgr) WriteOutline(buf *bytes.Buffer) {
	ig, err := ignore.CompileIgnoreFile(filepath.Join(mgr.rootPath, ".gitignore"))
	if err != nil {
		log.Error().Msgf("compile ignore failed")
	}
	leafNodes := mgr.leafNode()
	for _, node := range leafNodes {
		if ig != nil && ig.MatchesPath(node.relpath) {
			continue
		}
		if !node.isDir && filepath.Ext(node.relpath) != ".go" {
			continue
		}
		mgr.writeLeafNode(buf, node.relpath, node.isDir)
	}
}
