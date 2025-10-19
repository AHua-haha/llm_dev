package impl

import (
	"fmt"
	"go/ast"
	"go/token"
	"io/fs"
	"llm_dev/codebase/common"
	"os"
	"path/filepath"
	"strings"

	_ "llm_dev/utils"

	"github.com/rs/zerolog/log"
	ignore "github.com/sabhiram/go-gitignore"
	"go.mongodb.org/mongo-driver/mongo"
	"golang.org/x/tools/go/packages"

	tree_sitter "github.com/tree-sitter/go-tree-sitter"
	golang "github.com/tree-sitter/tree-sitter-go/bindings/go"
)

type CodeBase struct {
	rootPath         string
	rootNode         common.Node
	nodeByIdentifier map[string]common.Node
	nodeCount        int
}

func (cb *CodeBase) debugExternalNode() {
	print_op := func(root common.Node) bool {
		switch node := root.(type) {
		case *common.Dir:
			fmt.Printf("node.Path: %v\n", node.Path)
			for _, extNode := range node.ExternalNode {
				fmt.Printf("extNode.Name(): %v\n", extNode.Key())
			}
			return true
		case *common.File:
			fmt.Printf("node.Path: %v\n", node.Path)
			for _, extNode := range node.ExternalNode {
				fmt.Printf("extNode.Name(): %v\n", extNode.Key())
			}
			return false
		default:
			return false
		}
	}
	common.WalkNode(cb.rootNode, print_op)
}

func (cb *CodeBase) markExternal() bool {
	for key, value := range cb.nodeByIdentifier {
		if n, ok := value.(*symbolGo); ok {
			n.minPrefix = key
		}
	}
	cfg := &packages.Config{
		Mode: packages.NeedSyntax | packages.NeedTypes | packages.NeedTypesInfo | packages.NeedFiles,
		Fset: token.NewFileSet(),
		Dir:  cb.rootPath, // Change this
	}

	pkgs, err := packages.Load(cfg, "./...")
	if err != nil {
		log.Error().Err(err).Msg("type check project fail")
		return false
	}

	for _, pkg := range pkgs {
		for i, file := range pkg.Syntax {

			fileName := pkg.GoFiles[i]
			relPath, _ := filepath.Rel(cb.rootPath, fileName)
			ast.Inspect(file, func(n ast.Node) bool {
				ident, ok := n.(*ast.Ident)
				if !ok {
					return true
				}
				obj, ok := pkg.TypesInfo.Uses[ident]
				if !ok {
					return true
				}
				pos := obj.Pos()
				p := cfg.Fset.Position(pos)
				if p.Filename == fileName {
					return true
				}
				declare_file_path, err := filepath.Rel(cb.rootPath, p.Filename)
				if err != nil {
					return true
				}
				key := fmt.Sprintf("%s:%d:%d", declare_file_path, p.Line, p.Column)
				fmt.Printf("key: %v\n", key)
				node := cb.nodeByIdentifier[key]
				if node == nil {
					return true
				}
				symbol, ok := node.(*symbolGo)
				if !ok {
					return true
				}
				symbol.minPrefix = common.CommonPrefix(symbol.minPrefix, relPath)
				return true
			})
		}
	}
	for key, value := range cb.nodeByIdentifier {
		if n, ok := value.(*symbolGo); ok {
			cb.addExternalNode(key, n)
		}
	}
	return false
}
func (cb *CodeBase) addExternalNode(key string, symbol_node *symbolGo) {
	idx := strings.IndexRune(key, ':')
	if idx == -1 {
		return
	}
	file := key[:idx]

	prefix := symbol_node.minPrefix
	p := file
	for {
		if strings.HasPrefix(prefix, p) {
			break
		}
		node := cb.nodeByIdentifier[p]
		if node != nil {
			node.AddExternalNode(symbol_node)
			log.Info().Str("node", p).Msgf("add external node %s", key)
		}
		p = filepath.Dir(p)
	}
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
			key := fmt.Sprintf("%s:%s", relPath, symbol.Key())
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
	cb.nodeByIdentifier = make(map[string]common.Node, cb.nodeCount)
	log.Info().Int("node count", cb.nodeCount).Msg("init node map")
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
		rootPath: root,
	}
	codebase.constructNode()
	codebase.constructNodeMap()
	codebase.markExternal()
	// codebase.debugExternalNode()
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
	name      string
	identPos  tree_sitter.Point
	body      pos
	summary   pos
	minPrefix string
}

func (s *symbolGo) Child() []common.Node {
	var empty_child []common.Node
	return empty_child
}

func (s *symbolGo) AddChild(node common.Node) {
	panic("not implemented") // TODO: Implement
}

func (s *symbolGo) Key() string {
	return fmt.Sprintf("%d:%d", s.identPos.Row+1, s.identPos.Column+1)
}
func (s *symbolGo) ExtNode() []common.Node {
	panic("not implemented") // TODO: Implement
}
func (s *symbolGo) AddExternalNode(node common.Node) {
	panic("not implemented") // TODO: Implement
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
		symbol.identPos = identifier.StartPosition()
		symbol.name = fileops.getString(NewPos(identifier))
		symbol.body = NewPos(root)
		symbol.summary = NewPos(root)
		fileops.symbols = append(fileops.symbols, &symbol)
		return false
	case "function_declaration":
		name := root.ChildByFieldName("name")
		symbol.identPos = name.StartPosition()
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

		symbol.identPos = name.StartPosition()
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

type ContentRange [2]uint

type Definition struct {
	keyword string
	relFile string
	summary ContentRange
	content ContentRange
}

func (def *Definition) addKeyword(value string) {
	if def.keyword == "" {
		def.keyword += value
	} else {
		def.keyword += " " + value
	}
}

type BuildCodeBaseCtxOps struct {
	rootPath string
	db       *mongo.Database
}

func (op *BuildCodeBaseCtxOps) ExtractDefs() {
	defChan := make(chan Definition, 10)
	fileChan := make(chan string, 10)
	go func() {
		op.genAllFiles(fileChan)
		close(fileChan)
	}()
	go func() {
		for file := range fileChan {
			op.genAllDefs(file, defChan)
		}
		close(defChan)
	}()
	for def := range defChan {
		fmt.Printf("def.keyword: %v\n", def.keyword)
	}
}
func (op *BuildCodeBaseCtxOps) genAllFiles(outputChan chan string) {
	ig, err := ignore.CompileIgnoreFile(filepath.Join(op.rootPath, ".gitignore"))
	if err != nil {
		log.Error().Msgf("compile ignore failed")
		return
	}
	walkDirFunc := func(path string, d fs.DirEntry, err error) error {
		return op.walkDirOp(path, d, err, ig, outputChan)
	}
	filepath.WalkDir(op.rootPath, walkDirFunc)
}
func (op *BuildCodeBaseCtxOps) walkDirOp(path string, d fs.DirEntry, err error, ig *ignore.GitIgnore, outputChan chan string) error {
	keep := common.NewFillter(path, d).
		FillterSymlink().
		FillterGitIgnore(op.rootPath, ig).Keep()
	if !keep {
		return filepath.SkipDir
	}
	if d.IsDir() {
		return nil
	}
	ext := filepath.Ext(d.Name())
	if ext == ".go" {
		outputChan <- path
	}
	return nil
}

func (op *BuildCodeBaseCtxOps) genAllDefs(file string, outputChan chan Definition) {
	data, err := os.ReadFile(file)
	if err != nil {
		log.Error().Msgf("read file error %s", err)
		return
	}
	astNodeOp := func(root *tree_sitter.Node) bool {
		return op.astNodeOp(root, data, outputChan)
	}
	parser := tree_sitter.NewParser()
	parser.SetLanguage(tree_sitter.NewLanguage(golang.Language()))
	tree := parser.Parse(data, nil)
	common.WalkAst(tree.RootNode(), astNodeOp)
}
func (op *BuildCodeBaseCtxOps) astNodeOp(root *tree_sitter.Node, fileData []byte, outputChan chan Definition) bool {
	getRange := func(n *tree_sitter.Node) ContentRange {
		s, e := n.ByteRange()
		return [2]uint{s, e}
	}
	getString := func(n *tree_sitter.Node) string {
		pos := getRange(n)
		return string(fileData[pos[0]:pos[1]])
	}
	var def Definition
	Kind := root.Kind()
	switch Kind {
	case "source_file":
		return true
	case "type_declaration":
		identifier := root.Child(1).ChildByFieldName("name")
		def.content = getRange(root)
		def.summary = getRange(root)
		def.addKeyword("type")
		def.addKeyword(getString(identifier))
		outputChan <- def
		return false
	case "function_declaration":
		name := root.ChildByFieldName("name")
		def.content = getRange(root)
		def.summary = [2]uint{root.StartByte(), root.ChildByFieldName("body").StartByte()}
		def.addKeyword("function")
		def.addKeyword(getString(name))
		outputChan <- def
		return false
	case "method_declaration":
		receiver := root.ChildByFieldName("receiver").Child(1).ChildByFieldName("type").Child(1)
		name := root.ChildByFieldName("name")
		def.content = getRange(root)
		def.summary = [2]uint{root.StartByte(), root.ChildByFieldName("body").StartByte()}
		def.addKeyword("method")
		def.addKeyword(getString(receiver))
		def.addKeyword(getString(name))
		outputChan <- def
		return false
	default:
		return false
	}
}
