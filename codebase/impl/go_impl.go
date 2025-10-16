package impl

import (
	"fmt"
	"io/fs"
	"llm_dev/codebase/common"
	"os"
	"path/filepath"

	_ "llm_dev/utils"

	"github.com/rs/zerolog/log"

	tree_sitter "github.com/tree-sitter/go-tree-sitter"
	golang "github.com/tree-sitter/tree-sitter-go/bindings/go"
)

type CodeBase struct {
	rootPath         string
	rootNode         common.Node
	nodeByIdentifier map[string]common.Node
	nodeCount        int
}

func (cb *CodeBase) buildMapOp(r common.Node) bool {
	switch node := r.(type) {
	case *common.Dir:
		relPath, err := filepath.Rel(cb.rootPath, node.Path)
		if err != nil {
			log.Error().Msg("get relative path failed")
			return true
		}
		cb.tryInsertNode(relPath, node)
		return true
	case *common.File:
		relPath, err := filepath.Rel(cb.rootPath, node.Path)
		if err != nil {
			log.Error().Msg("get relative path failed")
			return true
		}
		cb.tryInsertNode(relPath, node)
		for _, symbol := range node.ChildNode {
			key := fmt.Sprintf("%s-%s", relPath, symbol.Name())
			cb.tryInsertNode(key, symbol)
		}
		return false
	default:
		return false
	}
}
func (cb *CodeBase) constructNode() {
	ignore_git_ops := common.GenIgnoreOps(cb.rootPath, cb.createNodeOp)
	cb.rootNode = common.WalkDirGenNode(cb.rootPath, ignore_git_ops)
}
func (cb *CodeBase) constructNodeMap() {
	common.WalkNode(cb.rootNode, cb.buildMapOp)
}
func (cb *CodeBase) tryInsertNode(key string, node common.Node) {
	_, exist := cb.nodeByIdentifier[key]
	if exist {
		log.Error().Msgf("key %s already exist", key)
		return
	}
	cb.nodeByIdentifier[key] = node
	log.Info().Msgf("insert key %s", key)
}

func (cb *CodeBase) createNodeOp(path string, d fs.DirEntry) (common.Node, bool) {
	if d.IsDir() {
		dir := common.Dir{
			Path: path,
		}
		cb.nodeCount++
		log.Info().
			Str("dir", path).
			Msg("create dir node")
		return &dir, true
	} else {
		name := d.Name()
		ext := filepath.Ext(name)
		if ext == ".go" {
			goFileNode := parseGoFile(path, d)
			if goFileNode != nil {
				cb.nodeCount += len(goFileNode.ChildNode) + 1
			}
			log.Info().
				Str("file", path).
				Msg("create go file node")
			return goFileNode, false
		}
		log.Info().
			Str("ext", ext).
			Msg("skip file")
		return nil, false
	}
}
func BuildCodeBase(root string) *CodeBase {
	codebase := &CodeBase{
		rootPath:         root,
		nodeByIdentifier: make(map[string]common.Node),
	}
	codebase.constructNode()
	codebase.constructNodeMap()
	return codebase
}

func parseGoFile(path string, d fs.DirEntry) *common.File {
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
	return file_node
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

func (s *symbolGo) Name() string {
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
		fileops.symbols = append(fileops.symbols, &symbol)
		return false
	default:
		return false
	}
}

func (fileops *fileSymExtractOps) extractSymbol() {
	data, err := os.ReadFile(fileops.path)
	if err != nil {
		log.Error().Msgf("read file error %s", err)
		return
	}
	fileops.data = data

	parser := tree_sitter.NewParser()
	parser.SetLanguage(tree_sitter.NewLanguage(golang.Language()))
	tree := parser.Parse(data, nil)
	common.WalkAst(tree.RootNode(), fileops.nodeOps)
}
