package common

import (
	"fmt"
	"io/fs"
	"testing"

	tree_sitter "github.com/tree-sitter/go-tree-sitter"
	golang "github.com/tree-sitter/tree-sitter-go/bindings/go"
)

func Test_walk(t *testing.T) {
	src_code := `package file_map

import (
	"bytes"
	"fmt"
	"strings"

	"golang.org/x/net/html"
)

func mapMarkup(content []byte) []Definition {
	reader := bytes.NewReader(content)
	doc, err := html.Parse(reader)
	if err != nil {
		return nil
	}

	var walk func(*html.Node) []Definition
	walk = func(n *html.Node) []Definition {
		var defs []Definition

		if n.Type == html.ElementNode {
			// Only track semantically significant elements
			if isSignificantTag(n.Data) {
				def := Definition{
					Type:      "tag",
					Signature: n.Data,
				}

				// Only include semantic classes/ids
				for _, attr := range n.Attr {
					if attr.Key == "id" {
						def.TagAttrs = append(def.TagAttrs, fmt.Sprintf("#%s", attr.Val))
					} else if attr.Key == "class" {
						classes := strings.Fields(attr.Val)
						if len(classes) > 3 {
							classes = classes[:3]
						}
						def.TagAttrs = append(def.TagAttrs, fmt.Sprintf(".%s", strings.Join(classes, ".")))
					}
				}

				// Get children of this element
				def.Children = []Definition{}
				for c := n.FirstChild; c != nil; c = c.NextSibling {
					def.Children = append(def.Children, walk(c)...)
				}

				defs = append(defs, def)
			}
		}

		// Only process siblings for non-significant elements
		if !isSignificantTag(n.Data) {
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				defs = append(defs, walk(c)...)
			}
		}

		return defs
	}

	defs := walk(doc)
	defs = consolidateRepeatedTags(defs)
	return defs
}

// Helper function to check if two definitions are equivalent
func areMarkupDefinitionsEqual(a, b Definition) bool {
	if a.Type != b.Type || a.Signature != b.Signature || len(a.TagAttrs) != len(b.TagAttrs) {
		return false
	}

	// Compare attributes
	for i, attr := range a.TagAttrs {
		if attr != b.TagAttrs[i] {
			return false
		}
	}

	return true
}

// Helper function to consolidate repeated tags
func consolidateRepeatedTags(defs []Definition) []Definition {
	var result []Definition

	firstDef := defs[0]
	count := 1
	allEqual := true

	// fmt.Printf("consolidateRepeatedTags: checking %d definitions for equality\n", len(defs))
	// spew.Dump(defs)

	if len(defs) > 1 {
		for i, def := range defs {
			if len(def.Children) > 0 {
				// fmt.Printf("consolidateRepeatedTags: definition %d has children, cannot consolidate\n", i)
				allEqual = false
				break
			}

			if i == 0 {
				continue
			}

			if !areMarkupDefinitionsEqual(firstDef, def) {
				// fmt.Printf("consolidateRepeatedTags: definition %d is not equal to first definition\n", i)
				allEqual = false
				break
			}
			count++
		}
	}

	if allEqual && count > 1 {
		// fmt.Printf("consolidateRepeatedTags: consolidated %d equal definitions\n", count)
		firstDef.TagReps = count
		result = []Definition{firstDef}
	} else {
		// fmt.Printf("consolidateRepeatedTags: definitions not equal, keeping original %d definitions\n", len(defs))
		result = defs
	}

	for i := range result {
		def := &result[i]
		if len(def.Children) > 0 {
			// fmt.Printf("consolidateRepeatedTags: recursively consolidating children of definition %d\n", i)
			def.Children = consolidateRepeatedTags(def.Children)
		}
	}

	return result
}

var significantHtmlTags = map[string]bool{
	"html":     true,
	"head":     true,
	"body":     true,
	"main":     true,
	"nav":      true,
	"header":   true,
	"footer":   true,
	"article":  true,
	"section":  true,
	"form":     true,
	"dialog":   true,
	"template": true,
	"table":    true,
	"div":      true,
	"ul":       true,
	"aside":    true,
}

func isSignificantTag(tag string) bool {
	return significantHtmlTags[tag]
}
	`
	parser := tree_sitter.NewParser()
	parser.SetLanguage(tree_sitter.NewLanguage(golang.Language()))

	tree := parser.Parse([]byte(src_code), nil)
	print_op := func(root *tree_sitter.Node) bool {
		Kind := root.Kind()
		fmt.Printf("Kind: %v\n", Kind)
		return true
	}
	tests := []struct {
		name string // description of this test case
		// Named input parameters for target function.
		root *tree_sitter.Node
		op   AstNodeOps
	}{
		// TODO: Add test cases.
		{
			name: "print operation",
			root: tree.RootNode(),
			op:   print_op,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			WalkAst(tt.root, tt.op)
		})
	}
}

type simpleNode struct {
	name      string
	childList []Node
}

func (n *simpleNode) Child() []Node {
	return n.childList
}

func (n *simpleNode) AddChild(node Node) {
	n.childList = append(n.childList, node)
}
func (n *simpleNode) Identifier() string {
	return n.name
}

func TestWalkDirGenTreeNode(t *testing.T) {

	ops := func(path string, d fs.DirEntry) (Node, bool) {
		fmt.Printf("info.Name(): %v\n", d.Name())
		if d.IsDir() {
			n := &simpleNode{
				name: d.Name(),
			}
			return n, true
		} else {
			n := &simpleNode{
				name: d.Name(),
			}
			return n, false
		}
	}

	print_ops := func(node Node) bool {
		simple_node, _ := node.(*simpleNode)
		fmt.Printf("simple_node.name: %v\n", simple_node.name)
		return true
	}

	tests := []struct {
		name string // description of this test case
		// Named input parameters for target function.
		root    string
		file_op FileOps
		want    Node
	}{
		// TODO: Add test cases.
		{
			name:    "test simple node",
			root:    "/home/ahua/workspace/llm/llm_dev",
			file_op: ops,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			op := GenIgnoreOps(tt.root, tt.file_op)
			got := WalkDirGenNode(tt.root, op)
			WalkNode(got, print_ops)
		})
	}
}
