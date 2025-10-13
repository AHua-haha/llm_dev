package common

import (
	tree_sitter "github.com/tree-sitter/go-tree-sitter"
)

func Walk(root *tree_sitter.Node, op NodeOperation) {
	walk_child := op(root)
	if walk_child {
		for i := uint(0); i < root.ChildCount(); i++ {
			child := root.Child(i)
			Walk(child, op)
		}
	}
}
