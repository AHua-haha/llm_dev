package impl

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"io/fs"
	"llm_dev/codebase/common"
	"os"
	"path/filepath"

	"github.com/rs/zerolog/log"
	ignore "github.com/sabhiram/go-gitignore"
	tree_sitter "github.com/tree-sitter/go-tree-sitter"
	golang "github.com/tree-sitter/tree-sitter-go/bindings/go"
	"go.mongodb.org/mongo-driver/mongo"
	"golang.org/x/tools/go/packages"
)

type ContentRange [2]uint

type TypeInfo struct {
	keyword []string
	relfile string
}

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

func (op *BuildCodeBaseCtxOps) genAllUseInfo(outputChan chan TypeInfo) {
	cfg := &packages.Config{
		Mode: packages.NeedSyntax | packages.NeedTypes | packages.NeedTypesInfo | packages.NeedFiles,
		Fset: token.NewFileSet(),
		Dir:  op.rootPath,
	}

	pkgs, err := packages.Load(cfg, "./...")
	if err != nil {
		log.Error().Err(err).Msg("type check project fail")
		return
	}

	for _, pkg := range pkgs {
		for i, file := range pkg.Syntax {

			typeMap := make(map[types.Object]struct{})
			fileName := pkg.GoFiles[i]
			relPath, _ := filepath.Rel(op.rootPath, fileName)
			fmt.Printf("relPath: %v\n", relPath)
			ast.Inspect(file, func(n ast.Node) bool {
				ident, ok := n.(*ast.Ident)
				if !ok {
					return true
				}
				obj, ok := pkg.TypesInfo.Uses[ident]
				if obj == nil {
					return true
				}
				pos := obj.Pos()
				p := cfg.Fset.Position(pos)
				if p.Filename == fileName || p.Filename == "" {
					return true
				}

				typeMap[obj] = struct{}{}
				return true
			})
			for obj, _ := range typeMap {
				pos := obj.Pos()
				p := cfg.Fset.Position(pos)
				fmt.Printf("p.String(): %v\n", p.String())
				switch obj := obj.(type) {
				case *types.Var:
					fmt.Printf("obj.Name(): %v\n", obj.Name())
				case *types.PkgName:
					fmt.Printf("obj.Name(): %v\n", obj.Name())
				case *types.TypeName:
					fmt.Printf("obj.Name(): %v\n", obj.Name())
				case *types.Func:
					fmt.Printf("obj.Name(): %v\n", obj.Name())
				}
			}
		}
	}
	return
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
	op.genAllUseInfo(nil)
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
	relPath, err := filepath.Rel(op.rootPath, file)
	if err != nil {
		return
	}
	astNodeOp := func(root *tree_sitter.Node) bool {
		return op.astNodeOp(root, relPath, data, outputChan)
	}
	parser := tree_sitter.NewParser()
	parser.SetLanguage(tree_sitter.NewLanguage(golang.Language()))
	tree := parser.Parse(data, nil)
	common.WalkAst(tree.RootNode(), astNodeOp)
}
func (op *BuildCodeBaseCtxOps) astNodeOp(root *tree_sitter.Node, relPath string, fileData []byte, outputChan chan Definition) bool {
	getRange := func(n *tree_sitter.Node) ContentRange {
		s, e := n.ByteRange()
		return [2]uint{s, e}
	}
	getString := func(n *tree_sitter.Node) string {
		pos := getRange(n)
		return string(fileData[pos[0]:pos[1]])
	}
	var def Definition
	def.relFile = relPath
	Kind := root.Kind()
	switch Kind {
	case "source_file":
		return true
	case "package_clause":
		identifier := root.Child(1)
		def.content = getRange(root)
		def.summary = getRange(root)
		def.addKeyword("package")
		def.addKeyword(getString(identifier))
		outputChan <- def
		return false
	case "import_declaration":
		def.content = getRange(root)
		def.summary = getRange(root)
		def.addKeyword("import")
		outputChan <- def
		return false
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
