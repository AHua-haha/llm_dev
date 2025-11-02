package common

import (
	"net/http"
	"os"
	"testing"

	tree_sitter "github.com/tree-sitter/go-tree-sitter"
	golang "github.com/tree-sitter/tree-sitter-go/bindings/go"
)

func Test_lspClient_requestDefinition(t *testing.T) {
	t.Run("test lsp client", func(t *testing.T) {
		// TODO: construct the receiver type.
		args := RequestDefinitionArgs{
			File: "",
		}
		lsp := lspClient{
			httpClient: &http.Client{},
		}
		lsp.RequestDefinition(args)
	})
}

func TestDefinitionApi(t *testing.T) {

	t.Run("test definition api", func(t *testing.T) {
		InitLsp()
		defer CloseLsp()
		data, _ := os.ReadFile("/root/workspace/llm_dev/codebase/impl/go_impl_db.go")
		parser := tree_sitter.NewParser()
		parser.SetLanguage(tree_sitter.NewLanguage(golang.Language()))
		tree := parser.Parse(data, nil)
		args := RequestDefinitionArgs{
			File: "codebase/impl/go_impl_db.go",
		}
		WalkAst(tree.RootNode(), func(root *tree_sitter.Node) bool {
			Kind := root.Kind()
			switch Kind {
			case "source_file":
				return true
			case "function_declaration":
				namePos := root.ChildByFieldName("name").StartPosition()
				p := Point{
					Line:   namePos.Row,
					Column: namePos.Column,
				}
				args.Loc = append(args.Loc, p)
				return false
			default:
				return false
			}
		})
		G_lspClient.requestReference(args)
	})
}
