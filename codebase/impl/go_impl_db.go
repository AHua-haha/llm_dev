package impl

import (
	"context"
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"io/fs"
	"llm_dev/codebase/common"
	"llm_dev/database"
	"os"
	"path/filepath"
	"strings"

	"github.com/rs/zerolog/log"
	ignore "github.com/sabhiram/go-gitignore"
	tree_sitter "github.com/tree-sitter/go-tree-sitter"
	golang "github.com/tree-sitter/tree-sitter-go/bindings/go"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"golang.org/x/tools/go/packages"
)

type ContentRange [2]uint

type TypeInfo struct {
	ID           primitive.ObjectID `bson:"_id,omitempty"` // Maps to MongoDB _id
	Identifier   string
	Keyword      []string
	DeclareFile  string
	UseFile      string
	IsDependency bool
}

func (info *TypeInfo) genIDFilter() bson.M {
	builder := database.NewFilter()
	builder.AddKV("_id", info.ID)
	return builder.Build()
}
func (info *TypeInfo) genFilterForDef() bson.M {
	var file *string
	var identifier *string
	if info.DeclareFile != "" {
		file = &info.DeclareFile
	}
	if info.Identifier != "" {
		identifier = &info.Identifier
	}
	return genDefFilter(file, identifier, info.Keyword)
}

func (info *TypeInfo) addKeyword(value string) {
	info.Keyword = append(info.Keyword, value)
}

type Definition struct {
	ID         primitive.ObjectID `bson:"_id,omitempty"` // Maps to MongoDB _id
	Identifier string
	Keyword    []string
	RelFile    string
	Summary    ContentRange
	Content    ContentRange
	MinPrefix  string
}

func genDefFilter(relfile *string, identifier *string, keyword []string) bson.M {
	builder := database.NewFilter()
	if relfile != nil {
		builder.AddKV("relfile", relfile)
	}
	if identifier != nil {
		builder.AddKV("identifier", identifier)
	}
	if len(keyword) != 0 {
		keywordFilter := database.NewFilterKV(database.All, keyword)
		builder.AddFilter("keyword", keywordFilter)
	}
	filter := builder.Build()
	return filter
}
func (def *Definition) getValue(key string) any {
	switch key {
	case "id":
		return def.ID
	case "identifier":
		return def.Identifier
	case "keyword":
		return def.Keyword
	case "relfile":
		return def.RelFile
	case "minprefix":
		return def.MinPrefix
	default:
		return nil
	}
}

func (def *Definition) genUpdate(keys ...string) bson.M {
	kv := bson.M{}
	for _, key := range keys {
		value := def.getValue(key)
		kv[key] = value
	}
	return bson.M{"$set": kv}
}

func (def *Definition) genIDFilter() bson.M {
	builder := database.NewFilter()
	builder.AddKV("_id", def.ID)
	return builder.Build()
}

func (def *Definition) AddKeyword(value string) {
	def.Keyword = append(def.Keyword, value)
}

type FileDirInfo struct {
	ID       primitive.ObjectID `bson:"_id,omitempty"` // Maps to MongoDB _id
	RelPath  string
	IsDir    bool
	UsedDefs []Definition
}

type BuildCodeBaseCtxOps struct {
	rootPath string
	db       *mongo.Database
}

func (op *BuildCodeBaseCtxOps) ExtractDefs() {
	op.genAllDefs()
	// op.insertDefs(defArray)
	op.genAllUseInfo()
	// op.insertUsedTypeInfo(usedTypeArray)
	// op.setMinPrefix(usedTypeArray)
	// op.genFileMap()
}
func (op *BuildCodeBaseCtxOps) genFileMap() {
	fileChan := op.walkProjectFileTree()
	fileMap := make(map[string]*FileDirInfo)
	for fileInfo := range fileChan {
		relpath, _ := filepath.Rel(op.rootPath, fileInfo.path)
		info := &FileDirInfo{
			RelPath: relpath,
		}
		if fileInfo.d.IsDir() {
			info.IsDir = true
		} else {
			info.IsDir = false
			ext := filepath.Ext(fileInfo.d.Name())
			if ext != ".go" {
				continue
			}
		}
		fileMap[relpath] = info
	}
	filter := bson.M{
		database.Expr: bson.M{
			database.Ne: []string{"$minprefix", "$relfile"},
		},
	}
	result := op.findDefs(filter)
	for _, def := range result {
		// fmt.Printf("%v %v\n", def.Keyword, def.MinPrefix)
		p := def.RelFile
		root := def.MinPrefix
		if def.MinPrefix == "" {
			root = "."
		}
		for {
			if strings.HasPrefix(root, p) {
				break
			}
			fileMap[p].UsedDefs = append(fileMap[p].UsedDefs, def)
			p = filepath.Dir(p)
		}
	}
	for k, v := range fileMap {
		fmt.Printf("f/d %s\n", k)
		for _, def := range v.UsedDefs {
			fmt.Printf("%v\n", def.Keyword)
		}
	}
}

func (op *BuildCodeBaseCtxOps) genAllUseInfo() []TypeInfo {
	moduleName, err := common.GetModulePath(filepath.Join(op.rootPath, "go.mod"))
	if err != nil {
		return nil
	}
	ctxChan := op.walkProjectTypeAst()
	res := []TypeInfo{}

	for ctx := range ctxChan {
		p := ctx.pos
		obj := ctx.obj
		relPath, _ := filepath.Rel(op.rootPath, ctx.path)
		var typeInfo TypeInfo
		typeInfo.UseFile = relPath
		typeInfo.Identifier = obj.Name()
		pkgPath := obj.Pkg().Path()
		if strings.HasPrefix(pkgPath, moduleName) {
			declare_file, _ := filepath.Rel(op.rootPath, p.Filename)
			typeInfo.DeclareFile = declare_file
			typeInfo.IsDependency = false
		} else {
			typeInfo.addKeyword(pkgPath)
			typeInfo.IsDependency = true
		}
		switch obj := obj.(type) {
		case *types.Var:
			if obj.IsField() {
				continue
			}
			typeName := obj.Type().String()
			typeInfo.addKeyword("var")
			typeInfo.addKeyword(obj.Name())
			idx := strings.LastIndex(typeName, ".")
			shortName := typeName[idx+1:]
			typeInfo.addKeyword(shortName)
		case *types.PkgName:
			typeInfo.addKeyword("package")
			typeInfo.addKeyword(obj.Name())
		case *types.TypeName:
			typeInfo.addKeyword("type")
			typeInfo.addKeyword(obj.Name())
		case *types.Func:
			rece := obj.Signature().Recv()
			if rece != nil {
				typeInfo.addKeyword("method")
				typeName := rece.Type().String()
				idx := strings.LastIndex(typeName, ".")
				shortName := typeName[idx+1:]
				typeInfo.addKeyword(shortName)
			} else {
				typeInfo.addKeyword("function")
			}
			typeInfo.addKeyword(obj.Name())
		default:
			continue
		}
		res = append(res, typeInfo)
	}
	return res
}

func (op *BuildCodeBaseCtxOps) genAllDefs() []Definition {
	getRange := func(n *tree_sitter.Node) ContentRange {
		s, e := n.ByteRange()
		return [2]uint{s, e}
	}
	ctxChan := op.walkPojectStaticAst()
	defs := []Definition{}
	for ctx := range ctxChan {
		getString := func(n *tree_sitter.Node) string {
			pos := getRange(n)
			return string(ctx.data[pos[0]:pos[1]])
		}
		relPath, _ := filepath.Rel(op.rootPath, ctx.path)
		var def Definition
		def.RelFile = relPath
		node := ctx.astNode
		Kind := node.Kind()
		switch Kind {
		case "var_declaration":
			var_spec := node.Child(1)
			name := var_spec.ChildByFieldName("name")
			typeName := var_spec.ChildByFieldName("type")
			def.Identifier = getString(name)
			def.Content = getRange(node)
			def.Summary = getRange(node)
			def.AddKeyword("var")
			def.AddKeyword(getString(name))
			def.AddKeyword(getString(typeName))
		case "short_var_declaration":
			exp_list := node.ChildByFieldName("left")
			count := exp_list.ChildCount()
			def.Content = getRange(node)
			def.Summary = getRange(node)
			def.AddKeyword("var")
			for i := range count {
				if i%2 == 1 {
					continue
				}
				def.AddKeyword(getString(exp_list.Child(i)))
			}
		case "package_clause":
			identifier := node.Child(1)
			def.Identifier = getString(identifier)
			def.Content = getRange(node)
			def.Summary = getRange(node)
			def.AddKeyword("package")
			def.AddKeyword(getString(identifier))
		case "import_declaration":
			def.Content = getRange(node)
			def.Summary = getRange(node)
			def.AddKeyword("import")
		case "type_declaration":
			identifier := node.Child(1).ChildByFieldName("name")
			def.Identifier = getString(identifier)
			def.Content = getRange(node)
			def.Summary = getRange(node)
			def.AddKeyword("type")
			def.AddKeyword(getString(identifier))
		case "function_declaration":
			name := node.ChildByFieldName("name")
			def.Identifier = getString(name)
			def.Content = getRange(node)
			def.Summary = [2]uint{node.StartByte(), node.ChildByFieldName("body").StartByte()}
			def.AddKeyword("function")
			def.AddKeyword(getString(name))
		case "method_declaration":
			receiver := node.ChildByFieldName("receiver").Child(1).ChildByFieldName("type").Child(1)
			name := node.ChildByFieldName("name")
			def.Identifier = getString(name)
			def.Content = getRange(node)
			def.Summary = [2]uint{node.StartByte(), node.ChildByFieldName("body").StartByte()}
			def.AddKeyword("method")
			def.AddKeyword(getString(receiver))
			def.AddKeyword(getString(name))
		default:
			continue
		}
		defs = append(defs, def)
	}
	return defs
}
func (op *BuildCodeBaseCtxOps) setMinPrefix(usedTypeInfos []TypeInfo) {
	for _, useInfo := range usedTypeInfos {
		if useInfo.IsDependency {
			continue
		}
		var identifier *string = nil
		if useInfo.Identifier != "" {
			identifier = &useInfo.Identifier
		}
		filter := genDefFilter(&useInfo.DeclareFile, identifier, useInfo.Keyword)
		res := op.findDefs(filter)
		size := len(res)
		if size == 0 {
			log.Info().Any("useinfo keyword", useInfo.Keyword).Msg("definition not found")
			continue
		} else if size > 1 {
			log.Error().Int("size", size).Any("useinfo keyword", useInfo.Keyword).Msg("definition found more than one")
		}
		def := res[0]
		minPrefix := common.CommonRootDir(def.MinPrefix, useInfo.UseFile)
		if minPrefix == def.MinPrefix {
			continue
		}
		def.MinPrefix = minPrefix
		update := def.genUpdate("minprefix")
		collection := op.db.Collection("Defs")
		_, err := collection.UpdateByID(context.TODO(), def.ID, update)
		if err != nil {
			log.Error().Err(err).Any("def", def).Msg("update definition failed")
		} else {
			log.Info().Any("def keyword", def.Keyword).Msg("update def minprefix")
		}
	}
}
func ToAnySlice[T any](input []T) []any {
	result := make([]any, len(input))
	for i, v := range input {
		result[i] = v
	}
	return result
}
func (op *BuildCodeBaseCtxOps) insertUsedTypeInfo(array []TypeInfo) {
	anySlice := ToAnySlice(array)
	op.db.Collection("Uses").InsertMany(context.TODO(), anySlice)
}

func (op *BuildCodeBaseCtxOps) insertDefs(array []Definition) {
	anySlice := ToAnySlice(array)
	op.db.Collection("Defs").InsertMany(context.TODO(), anySlice)
}

func (op *BuildCodeBaseCtxOps) findDefs(filter bson.M) []Definition {
	collection := op.db.Collection("Defs")
	cursor, err := collection.Find(context.TODO(), filter)
	if err != nil {
		log.Error().Err(err).Any("filter", filter).Msgf("run fild failed")
		return nil
	}
	defer cursor.Close(context.TODO())
	result := []Definition{}
	err = cursor.All(context.TODO(), &result)
	if err != nil {
		log.Error().Err(err).Msg("parse result to []Definition failed")
		return nil
	}
	return result
}

type typeAstCtx struct {
	path string
	obj  types.Object
	pos  token.Position
}

func (op *BuildCodeBaseCtxOps) walkProjectTypeAst() <-chan typeAstCtx {
	outputChan := make(chan typeAstCtx, 10)
	go func() {
		defer close(outputChan)
		cfg := &packages.Config{
			Mode:  packages.NeedSyntax | packages.NeedTypes | packages.NeedTypesInfo | packages.NeedFiles,
			Fset:  token.NewFileSet(),
			Dir:   op.rootPath,
			Tests: true,
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
				if !strings.HasPrefix(fileName, op.rootPath) {
					continue
				}
				ast.Inspect(file, func(n ast.Node) bool {
					ident, ok := n.(*ast.Ident)
					if !ok {
						return true
					}
					obj := pkg.TypesInfo.Uses[ident]
					if obj == nil {
						return true
					}
					pos := cfg.Fset.Position(obj.Pos())
					if pos.Filename == fileName || pos.Filename == "" {
						return true
					}
					typeMap[obj] = struct{}{}
					return true
				})
				for k, _ := range typeMap {
					outputChan <- typeAstCtx{
						path: fileName,
						obj:  k,
						pos:  cfg.Fset.Position(k.Pos()),
					}
				}
			}
		}
	}()
	return outputChan
}

type StaticAstCtx struct {
	path    string
	data    []byte
	astNode *tree_sitter.Node
}

func (op *BuildCodeBaseCtxOps) walkPojectStaticAst() <-chan StaticAstCtx {
	outputChan := make(chan StaticAstCtx, 10)
	go func() {
		fileChan := op.walkProjectFileTree()
		for fileInfo := range fileChan {
			if fileInfo.d.IsDir() || filepath.Ext(fileInfo.d.Name()) != ".go" {
				continue
			}
			data, err := os.ReadFile(fileInfo.path)
			if err != nil {
				log.Error().Msgf("read file error %s", err)
				continue
			}
			parser := tree_sitter.NewParser()
			parser.SetLanguage(tree_sitter.NewLanguage(golang.Language()))
			tree := parser.Parse(data, nil)
			common.WalkAst(tree.RootNode(), func(root *tree_sitter.Node) bool {
				output := StaticAstCtx{
					path:    fileInfo.path,
					data:    data,
					astNode: root,
				}
				outputChan <- output
				Kind := root.Kind()
				switch Kind {
				case "source_file":
					return true
				default:
					return false
				}
			})
		}
		close(outputChan)
	}()
	return outputChan
}

type fileTreeCtx struct {
	path string
	d    fs.DirEntry
}

func (op *BuildCodeBaseCtxOps) walkProjectFileTree() <-chan fileTreeCtx {
	outputChan := make(chan fileTreeCtx, 10)
	go func() {
		ig, err := ignore.CompileIgnoreFile(filepath.Join(op.rootPath, ".gitignore"))
		if err != nil {
			log.Error().Msgf("compile ignore failed")
			return
		}
		walkDirFunc := func(path string, d fs.DirEntry, err error) error {
			keep := common.NewFilter(path, d).
				FilterSymlink().
				FilterGitIgnore(op.rootPath, ig).Keep()
			if !keep {
				return filepath.SkipDir
			}
			outputChan <- fileTreeCtx{
				path: path,
				d:    d,
			}
			return nil
		}
		filepath.WalkDir(op.rootPath, walkDirFunc)
		close(outputChan)
	}()
	return outputChan
}
