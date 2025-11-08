package context

import (
	"bytes"
	"fmt"
	"llm_dev/database"
	"path/filepath"
	"strings"
	"testing"
)

func TestOutlineContextMgr_writeLeafNode(t *testing.T) {
	t.Run("test write outline for lead", func(t *testing.T) {
		// TODO: construct the receiver type.
		database.InitDB()
		defer database.CloseDB()
		// mgr := OutlineContextMgr{
		// 	rootPath: "/root/workspace/llm_dev",
		// 	buildCtxOp: impl.BuildCodeBaseCtxOps{
		// 		RootPath: "/root/workspace/llm_dev",
		// 		Db:       database.GetDBClient().Database("llm_dev"),
		// 	},
		// }
		// var buf bytes.Buffer
		// mgr.writeLeafNode(&buf, "codebase/common")
		// fmt.Printf("%s\n", buf.String())
	})
}

func TestSetLeaf(t *testing.T) {
	t.Run("test write outline for lead", func(t *testing.T) {
		// TODO: construct the receiver type.
		// mgr := OutlineContextMgr{
		// 	rootPath: "/root/workspace/llm_dev",
		// 	buildCtxOp: impl.BuildCodeBaseCtxOps{
		// 		RootPath: "/root/workspace/llm_dev",
		// 	},
		// }
		p := filepath.Clean("./codebase/impl/")
		fmt.Printf("p: %v\n", p)
		parts := strings.Split(p, "/")
		fmt.Printf("parts: %v\n", parts)
	})
}

func TestFileTree(t *testing.T) {
	t.Run("test file tree node", func(t *testing.T) {
		database.InitDB()
		defer database.CloseDB()
		handler := func(node *FileTreeNode) {
			fmt.Printf("node.relpath: %v\n", node.relpath)
		}
		mgr := NewOutlineCtxMgr("/root/workspace/llm_dev", nil)
		mgr.walkNode(mgr.fileTree, handler)
		mgr.OpenDir("codebase/impl")
		fmt.Printf("mgr.fileTree.children: %v\n", mgr.fileTree.children)
		mgr.walkNode(mgr.fileTree, handler)
	})
}

func TestWrite(t *testing.T) {
	t.Run("test write outline", func(t *testing.T) {
		database.InitDB()
		defer database.CloseDB()
		mgr := NewOutlineCtxMgr("/root/workspace/llm_dev", nil)
		mgr.OpenDir(".")
		var buf bytes.Buffer
		mgr.writeOutline(&buf)
		fmt.Printf("%v\n", buf.String())
	})
}
