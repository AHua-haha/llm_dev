package common

import (
	"io/fs"
	"os"
	"path/filepath"

	ignore "github.com/sabhiram/go-gitignore"
	tree_sitter "github.com/tree-sitter/go-tree-sitter"
)

func WalkAst(root *tree_sitter.Node, op AstNodeOps) {
	walk_child := op(root)
	if walk_child {
		for i := uint(0); i < root.ChildCount(); i++ {
			child := root.Child(i)
			WalkAst(child, op)
		}
	}
}

func GenIgnoreOps(root string, op FileOps) FileOps {
	ig, err := ignore.CompileIgnoreFile(filepath.Join(root, ".gitignore"))
	if err != nil {
		return op
	}
	ignore_ops := func(path string, d fs.DirEntry) (Node, bool) {
		relPath, err := filepath.Rel(root, path)
		if err != nil {
			return nil, false
		}
		if d.Type()&os.ModeSymlink != 0 {
			return nil, false
		}

		if ig.MatchesPath(relPath) {
			return nil, false
		}
		return op(path, d)
	}
	return ignore_ops
}
func WalkDirGenNode(root string, file_op FileOps) Node {
	info, err := os.Stat(root)
	if err != nil {
		return nil
	}
	return walkDir(root, fs.FileInfoToDirEntry(info), file_op)
}

func walkDir(root string, d fs.DirEntry, file_op FileOps) Node {
	node, walk_child := file_op(root, d)
	if node == nil {
		return nil
	}
	if !walk_child {
		return node
	}
	if !d.IsDir() {
		return node
	}

	entries, err := os.ReadDir(root)
	if err != nil {
		return node
	}

	for _, entry := range entries {
		child := walkDir(filepath.Join(root, entry.Name()), entry, file_op)
		if child != nil {
			node.addChild(child)
		}
	}
	return node
}

func WalkNode(root Node, node_ops NodeOps) {
	walk_child := node_ops(root)
	if !walk_child {
		return
	}
	for _, child := range root.child() {
		WalkNode(child, node_ops)
	}
}
