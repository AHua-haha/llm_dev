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
	ExtNode() []Node
	AddExternalNode(n Node)
	Key() string
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

func (f *File) Key() string {
	return f.Path
}
func (f *File) ExtNode() []Node {
	return f.ExternalNode
}
func (f *File) AddExternalNode(node Node) {
	f.ExternalNode = append(f.ExternalNode, node)
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

func (d *Dir) Key() string {
	return d.Path
}
func (d *Dir) ExtNode() []Node {
	return d.ExternalNode
}
func (d *Dir) AddExternalNode(node Node) {
	d.ExternalNode = append(d.ExternalNode, node)
}
