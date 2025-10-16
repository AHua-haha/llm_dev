package common

import (
	"io/fs"

	tree_sitter "github.com/tree-sitter/go-tree-sitter"
)

type AstNodeOps func(root *tree_sitter.Node) bool

type FileOps func(path string, d fs.DirEntry) (Node, bool)

type NodeOps func(node Node) bool

type Node interface {
	Child() []Node
	AddChild(node Node)
	Identifier() string
}

type File struct {
	Path         string
	Ext          string
	ChildNode    []Node
	ExternalNode []Node
}

func (f *File) Child() []Node {
	return f.ChildNode
}

func (f *File) AddChild(node Node) {
	f.ChildNode = append(f.ChildNode, node)
}

func (f *File) Identifier() string {
	return f.Path
}

type Dir struct {
	Path         string
	ChildNode    []Node
	ExternalNode []Node
}

func (d *Dir) Child() []Node {
	return d.ChildNode
}

func (d *Dir) AddChild(node Node) {
	d.ChildNode = append(d.ChildNode, node)
}

func (d *Dir) Identifier() string {
	return d.Path
}
