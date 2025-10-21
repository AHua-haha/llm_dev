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

func (op *BuildCodeBaseCtxOps) markExternal() {
	usedTypeInfoChan := make(chan TypeInfo, 10)
	go func() {
		op.genAllUseInfo(usedTypeInfoChan)
		close(usedTypeInfoChan)
	}()
}

func (op *BuildCodeBaseCtxOps) genAllUseInfo(outputChan chan TypeInfo) {
	cfg := &packages.Config{
		Mode:  packages.NeedSyntax | packages.NeedTypes | packages.NeedTypesInfo | packages.NeedFiles,
		Fset:  token.NewFileSet(),
		Dir:   op.rootPath,
		Tests: true,
	}
	moduleName, err := common.GetModulePath(filepath.Join(op.rootPath, "go.mod"))
	if err != nil {
		return
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
			relPath, _ := filepath.Rel(op.rootPath, fileName)
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
				var typeInfo TypeInfo
				pos := obj.Pos()
				p := cfg.Fset.Position(pos)
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
				outputChan <- typeInfo
			}
		}
	}
}

func (op *BuildCodeBaseCtxOps) ExtractDefs() {
	defChan := make(chan Definition, 10)
	defArray := []Definition{}
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
		def.MinPrefix = def.RelFile
		defArray = append(defArray, def)
	}
	// op.insertDefs(defArray)
	useTypeInfoChan := make(chan TypeInfo, 10)
	useTypeInfoArray := []TypeInfo{}
	go func() {
		op.genAllUseInfo(useTypeInfoChan)
		close(useTypeInfoChan)
	}()
	for info := range useTypeInfoChan {
		useTypeInfoArray = append(useTypeInfoArray, info)
	}
	// op.insertUsedTypeInfo(useTypeInfoArray)
	op.setMinPrefix(useTypeInfoArray)
	op.genFileMap()
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
	keep := common.NewFilter(path, d).
		FilterSymlink().
		FilterGitIgnore(op.rootPath, ig).Keep()
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
	def.RelFile = relPath
	Kind := root.Kind()
	switch Kind {
	case "source_file":
		return true
	case "var_declaration":
		var_spec := root.Child(1)
		name := var_spec.ChildByFieldName("name")
		typeName := var_spec.ChildByFieldName("type")
		def.Identifier = getString(name)
		def.Content = getRange(root)
		def.Summary = getRange(root)
		def.AddKeyword("var")
		def.AddKeyword(getString(name))
		def.AddKeyword(getString(typeName))
		outputChan <- def
		return false
	case "short_var_declaration":
		exp_list := root.ChildByFieldName("left")
		count := exp_list.ChildCount()
		def.Content = getRange(root)
		def.Summary = getRange(root)
		def.AddKeyword("var")
		for i := range count {
			if i%2 == 1 {
				continue
			}
			def.AddKeyword(getString(exp_list.Child(i)))
		}
		outputChan <- def
		return false
	case "package_clause":
		identifier := root.Child(1)
		def.Identifier = getString(identifier)
		def.Content = getRange(root)
		def.Summary = getRange(root)
		def.AddKeyword("package")
		def.AddKeyword(getString(identifier))
		outputChan <- def
		return false
	case "import_declaration":
		def.Content = getRange(root)
		def.Summary = getRange(root)
		def.AddKeyword("import")
		outputChan <- def
		return false
	case "type_declaration":
		identifier := root.Child(1).ChildByFieldName("name")
		def.Identifier = getString(identifier)
		def.Content = getRange(root)
		def.Summary = getRange(root)
		def.AddKeyword("type")
		def.AddKeyword(getString(identifier))
		outputChan <- def
		return false
	case "function_declaration":
		name := root.ChildByFieldName("name")
		def.Identifier = getString(name)
		def.Content = getRange(root)
		def.Summary = [2]uint{root.StartByte(), root.ChildByFieldName("body").StartByte()}
		def.AddKeyword("function")
		def.AddKeyword(getString(name))
		outputChan <- def
		return false
	case "method_declaration":
		receiver := root.ChildByFieldName("receiver").Child(1).ChildByFieldName("type").Child(1)
		name := root.ChildByFieldName("name")
		def.Identifier = getString(name)
		def.Content = getRange(root)
		def.Summary = [2]uint{root.StartByte(), root.ChildByFieldName("body").StartByte()}
		def.AddKeyword("method")
		def.AddKeyword(getString(receiver))
		def.AddKeyword(getString(name))
		outputChan <- def
		return false
	default:
		return false
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

func (op *BuildCodeBaseCtxOps) genFileMap() {
	ig, err := ignore.CompileIgnoreFile(filepath.Join(op.rootPath, ".gitignore"))
	if err != nil {
		log.Error().Msgf("compile ignore failed")
		return
	}
	fileMap := make(map[string]*FileDirInfo)
	walkDirFunc := func(path string, d fs.DirEntry, err error) error {
		relpath, _ := filepath.Rel(op.rootPath, path)
		keep := common.NewFilter(path, d).
			FilterSymlink().
			FilterGitIgnore(op.rootPath, ig).Keep()
		if !keep {
			return filepath.SkipDir
		}
		info := &FileDirInfo{
			RelPath: relpath,
		}
		if d.IsDir() {
			info.IsDir = true
		} else {
			info.IsDir = false
			ext := filepath.Ext(d.Name())
			if ext != ".go" {
				return nil
			}
		}
		fileMap[relpath] = info
		return nil
	}
	filepath.WalkDir(op.rootPath, walkDirFunc)
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
