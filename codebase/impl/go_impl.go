package impl

import (
	"fmt"
	"io/fs"
	"llm_dev/codebase/common"
	"os"

	tree_sitter "github.com/tree-sitter/go-tree-sitter"
	golang "github.com/tree-sitter/tree-sitter-go/bindings/go"
)

func ParseGoProject(root string) common.Node {
	ops := func(path string, d fs.DirEntry) (common.Node, bool) {
		if d.IsDir() {
			dir := common.Dir{
				Path: path,
			}
			return &dir, true
		} else {
			return parseGoFile(path, d)
		}
	}
	ignore_git_ops := common.GenIgnoreOps(root, ops)
	node := common.WalkDirGenNode(root, ignore_git_ops)
	return node
}

func parseGoFile(path string, d fs.DirEntry) (*common.File, bool) {
	parse_ops := fileSymExtractOps{
		path: path,
	}
	parse_ops.extractSymbol()
	nodes := make([]common.Node, len(parse_ops.symbols))
	for i, v := range parse_ops.symbols {
		nodes[i] = v
	}

	file_node := &common.File{
		Path:      path,
		Ext:       "go",
		ChildNode: nodes,
	}
	return file_node, false
}

type symbolGo struct {
	name    string
	body    pos
	summary pos
}

func (s *symbolGo) Child() []common.Node {
	var empty_child []common.Node
	return empty_child
}

func (s *symbolGo) AddChild(node common.Node) {
	panic("not implemented") // TODO: Implement
}

func (s *symbolGo) Identifier() string {
	return s.name
}

type pos struct {
	start uint
	end   uint
}

func NewPos(root *tree_sitter.Node) pos {
	s, e := root.ByteRange()
	return pos{
		start: s,
		end:   e,
	}
}

type fileSymExtractOps struct {
	path    string
	data    []byte
	symbols []*symbolGo
}

func (fileops *fileSymExtractOps) getString(p pos) string {
	s := p.start
	e := p.end
	return string(fileops.data[s:e])
}

func (fileops *fileSymExtractOps) nodeOps(root *tree_sitter.Node) bool {
	var symbol symbolGo

	Kind := root.Kind()
	switch Kind {
	case "source_file":
		return true
	case "type_declaration":
		identifier := root.Child(1).ChildByFieldName("name")
		symbol.name = fileops.getString(NewPos(identifier))
		symbol.body = NewPos(root)
		symbol.summary = NewPos(root)
		fmt.Printf("symbol.name: %v\n", symbol.name)
		fileops.symbols = append(fileops.symbols, &symbol)
		return false
	case "function_declaration":
		name := root.ChildByFieldName("name")
		symbol.name = fileops.getString(NewPos(name))
		symbol.body = NewPos(root)
		symbol.summary = pos{
			start: root.StartByte(),
			end:   root.ChildByFieldName("body").StartByte(),
		}
		fmt.Printf("symbol.name: %v\n", symbol.name)
		fileops.symbols = append(fileops.symbols, &symbol)
		return false
	case "method_declaration":
		receiver := root.ChildByFieldName("receiver").Child(1).ChildByFieldName("type").Child(1)
		name := root.ChildByFieldName("name")
		receiver_str := fileops.getString(NewPos(receiver))
		name_str := fileops.getString(NewPos(name))

		symbol.name = fmt.Sprintf("%s::%s", receiver_str, name_str)
		symbol.body = NewPos(root)
		symbol.summary = pos{
			start: root.StartByte(),
			end:   root.ChildByFieldName("body").StartByte(),
		}
		fmt.Printf("symbol.name: %v\n", symbol.name)
		fileops.symbols = append(fileops.symbols, &symbol)
		return false
	default:
		return false
	}
}

func (fileops *fileSymExtractOps) extractSymbol() {
	data, err := os.ReadFile(fileops.path)
	fileops.data = data
	if err != nil {
		fmt.Printf("err: %v\n", err)
		return
	}

	parser := tree_sitter.NewParser()
	parser.SetLanguage(tree_sitter.NewLanguage(golang.Language()))
	tree := parser.Parse(data, nil)
	common.WalkAst(tree.RootNode(), fileops.nodeOps)
}
