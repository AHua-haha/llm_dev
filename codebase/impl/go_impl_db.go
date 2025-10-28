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
	"sort"
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

const (
	typeQueryStr string = `
(type_identifier) @type
	`
	nameQueryStr string = `
(identifier) @type
`
	varSpecQueryStr string = `
(var_spec) @var
`
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
	return GenDefFilter(file, identifier, info.Keyword)
}

func (info *TypeInfo) addKeyword(value string) {
	info.Keyword = append(info.Keyword, value)
}

type Definition struct {
	ID         primitive.ObjectID `bson:"_id,omitempty"` // Maps to MongoDB _id
	Identifier string
	Keyword    []string
	Summary    ContentRange
	Content    ContentRange
	MinPrefix  string
	RelFile    string
}

func GenDefFilter(relfile *string, identifier *string, keyword []string) bson.M {
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

func (fd *FileDirInfo) GetSummary() map[string][]ContentRange {
	defsByFile := fd.getDefByFile()
	res := make(map[string][]ContentRange, len(defsByFile))
	for file, ranges := range defsByFile {
		uniqueRange := []ContentRange{ranges[0].Summary}
		for i := 1; i < len(ranges); i++ {
			if ranges[i].Summary != ranges[i-1].Summary {
				uniqueRange = append(uniqueRange, ranges[i].Summary)
			}
		}
		res[file] = uniqueRange
	}
	return res
}

func (fd *FileDirInfo) getDefByFile() map[string][]Definition {
	res := make(map[string][]Definition)
	for _, usedDef := range fd.UsedDefs {
		res[usedDef.RelFile] = append(res[usedDef.RelFile], usedDef)
	}
	for _, v := range res {
		sort.Slice(v, func(i, j int) bool {
			return v[i].Summary[0] < v[j].Summary[0]
		})
	}
	return res
}

type BuildCodeBaseCtxOps struct {
	RootPath string
	Db       *mongo.Database
}

func (op *BuildCodeBaseCtxOps) ExtractDefs() {
	// op.genAllDefs()
	defArray := op.genAllDefs()
	fmt.Printf("len(defArray): %v\n", len(defArray))
	op.insertDefs(defArray)
	fmt.Printf("done\n")
	usedTypeInfoArray := op.genAllUseInfo()
	fmt.Printf("len(usedTypeInfoArray): %v\n", len(usedTypeInfoArray))
	fmt.Printf("done\n")
	op.setMinPrefix(usedTypeInfoArray)
}
func (op *BuildCodeBaseCtxOps) GenFileMap() map[string]*FileDirInfo {
	fileChan := op.WalkProjectFileTree()
	fileMap := make(map[string]*FileDirInfo)
	for fileInfo := range fileChan {
		relpath, _ := filepath.Rel(op.RootPath, fileInfo.Path)
		info := &FileDirInfo{
			RelPath: relpath,
		}
		if fileInfo.D.IsDir() {
			info.IsDir = true
		} else {
			info.IsDir = false
			ext := filepath.Ext(fileInfo.D.Name())
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
	result := op.FindDefs(filter)
	for _, def := range result {
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
	return fileMap
}

func (op *BuildCodeBaseCtxOps) genAllUseInfo() []TypeInfo {
	moduleName, err := common.GetModulePath(filepath.Join(op.RootPath, "go.mod"))
	if err != nil {
		return nil
	}
	ctxChan := op.walkProjectTypeAst()
	res := []TypeInfo{}

	for ctx := range ctxChan {
		p := ctx.pos
		obj := ctx.obj
		relPath, _ := filepath.Rel(op.RootPath, ctx.path)
		var typeInfo TypeInfo
		typeInfo.UseFile = relPath
		typeInfo.Identifier = obj.Name()
		pkgPath := obj.Pkg().Path()
		if strings.HasPrefix(pkgPath, moduleName) {
			declare_file, _ := filepath.Rel(op.RootPath, p.Filename)
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
	lang := tree_sitter.NewLanguage(golang.Language())
	typeQuery, err := common.NewTSQuery(typeQueryStr, lang)
	if err != nil {
		log.Error().Err(err).Msg("init query failed")
		return nil
	}
	defer typeQuery.Close()
	nameQuery, err := common.NewTSQuery(nameQueryStr, lang)
	if err != nil {
		log.Error().Err(err).Msg("init query failed")
		return nil
	}
	defer nameQuery.Close()
	varQuery, err := common.NewTSQuery(varSpecQueryStr, lang)
	if err != nil {
		log.Error().Err(err).Msg("init query failed")
		return nil
	}
	defer varQuery.Close()

	ctxChan := op.walkPojectStaticAst()
	defs := []Definition{}
	for ctx := range ctxChan {
		getString := func(n *tree_sitter.Node) string {
			return n.Utf8Text(ctx.data)
		}
		relPath, _ := filepath.Rel(op.RootPath, ctx.path)
		var def Definition
		def.RelFile = relPath
		def.MinPrefix = relPath
		node := ctx.astNode
		Kind := node.Kind()
		switch Kind {
		case "method_declaration", "function_declaration":
			def.Content = [2]uint{node.StartByte(), node.EndByte()}
			def.Summary = [2]uint{node.StartByte(), node.ChildByFieldName("body").StartByte()}
		default:
			def.Content = [2]uint{node.StartByte(), node.EndByte()}
			def.Summary = def.Content
		}
		switch Kind {
		case "var_declaration":
			def.AddKeyword("var")
			res := varQuery.Query(node, ctx.data)
			for _, value := range res {
				name := value.Node.ChildByFieldName("name")
				def.Identifier = getString(name)
				def.AddKeyword(def.Identifier)
				typeNode := value.Node.ChildByFieldName("type")
				if typeNode != nil {
					types := typeQuery.Query(typeNode, ctx.data)
					for _, value := range types {
						def.AddKeyword(getString(value.Node))
					}
				}
				defs = append(defs, def)
			}
		case "short_var_declaration":
			def.AddKeyword("var")
			res := nameQuery.Query(node, ctx.data)
			for _, value := range res {
				name := getString(value.Node)
				def.Identifier = name
				def.AddKeyword(name)
				defs = append(defs, def)
			}
		case "package_clause":
			identifier := node.Child(1)
			def.Identifier = getString(identifier)
			def.AddKeyword("package")
			def.AddKeyword(getString(identifier))
			defs = append(defs, def)
		case "import_declaration":
			def.AddKeyword("import")
			defs = append(defs, def)
		case "type_declaration":
			identifier := node.Child(1).ChildByFieldName("name")
			def.Identifier = getString(identifier)
			def.AddKeyword("type")
			def.AddKeyword(getString(identifier))
			defs = append(defs, def)
		case "function_declaration":
			name := node.ChildByFieldName("name")
			def.Identifier = getString(name)
			def.AddKeyword("function")
			def.AddKeyword(getString(name))
			defs = append(defs, def)
		case "method_declaration":
			receiver := node.ChildByFieldName("receiver")
			name := node.ChildByFieldName("name")
			def.Identifier = getString(name)
			def.AddKeyword("method")
			res := typeQuery.Query(receiver, ctx.data)
			for _, value := range res {
				def.AddKeyword(getString(value.Node))
			}
			def.AddKeyword(getString(name))
			defs = append(defs, def)

		default:
			continue
		}
	}
	return defs
}
func (op *BuildCodeBaseCtxOps) setMinPrefix(usedTypeInfos []TypeInfo) {
	findExcatDef := func(useInfo TypeInfo) *Definition {
		var identifier *string = nil
		if useInfo.Identifier != "" {
			identifier = &useInfo.Identifier
		}
		var def Definition
		foundExcat := false
		size := len(useInfo.Keyword)
		for i := range size {
			keyword := useInfo.Keyword[:i]
			filter := GenDefFilter(&useInfo.DeclareFile, identifier, keyword)
			res := op.FindDefs(filter)
			resLen := len(res)
			if resLen == 0 {
				break
			}
			if resLen == 1 {
				foundExcat = true
				def = res[0]
				break
			}
		}
		if foundExcat {
			return &def
		}
		return nil
	}
	for _, useInfo := range usedTypeInfos {
		if useInfo.IsDependency {
			continue
		}
		def := findExcatDef(useInfo)
		if def == nil {
			log.Error().Any("keyword", useInfo.Keyword).Msg("do not fund excat def")
			continue
		}
		minPrefix := common.CommonRootDir(def.MinPrefix, useInfo.UseFile)
		if minPrefix == def.MinPrefix {
			continue
		}
		def.MinPrefix = minPrefix
		update := def.genUpdate("minprefix")
		collection := op.Db.Collection("Defs")
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
	op.Db.Collection("Uses").InsertMany(context.TODO(), anySlice)
}

func (op *BuildCodeBaseCtxOps) insertDefs(array []Definition) {
	anySlice := ToAnySlice(array)
	op.Db.Collection("Defs").InsertMany(context.TODO(), anySlice)
}

func (op *BuildCodeBaseCtxOps) FindDefs(filter bson.M) []Definition {
	collection := op.Db.Collection("Defs")
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
			Dir:   op.RootPath,
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
				if !strings.HasPrefix(fileName, op.RootPath) {
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
		fileChan := op.WalkProjectFileTree()
		for fileInfo := range fileChan {
			if fileInfo.D.IsDir() || filepath.Ext(fileInfo.D.Name()) != ".go" {
				continue
			}
			data, err := os.ReadFile(fileInfo.Path)
			if err != nil {
				log.Error().Msgf("read file error %s", err)
				continue
			}
			parser := tree_sitter.NewParser()
			defer parser.Close()
			parser.SetLanguage(tree_sitter.NewLanguage(golang.Language()))
			tree := parser.Parse(data, nil)
			defer tree.Clone()
			common.WalkAst(tree.RootNode(), func(root *tree_sitter.Node) bool {
				output := StaticAstCtx{
					path:    fileInfo.Path,
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

type FileTreeCtx struct {
	Path string
	D    fs.DirEntry
}

func (op *BuildCodeBaseCtxOps) WalkProjectFileTree() <-chan FileTreeCtx {
	outputChan := make(chan FileTreeCtx, 10)
	go func() {
		ig, err := ignore.CompileIgnoreFile(filepath.Join(op.RootPath, ".gitignore"))
		if err != nil {
			log.Error().Msgf("compile ignore failed")
			return
		}
		walkDirFunc := func(path string, d fs.DirEntry, err error) error {
			keep := common.NewFilter(path, d).
				FilterSymlink().
				FilterGitIgnore(op.RootPath, ig).Keep()
			if !keep {
				return filepath.SkipDir
			}
			outputChan <- FileTreeCtx{
				Path: path,
				D:    d,
			}
			return nil
		}
		filepath.WalkDir(op.RootPath, walkDirFunc)
		close(outputChan)
	}()
	return outputChan
}
