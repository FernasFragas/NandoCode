package semantic

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"path/filepath"
	"sort"
	"strings"
)

func extractGoSymbolRecords(relPath string, body []byte, contentHash string) ([]Record, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, relPath, body, parser.ParseComments|parser.AllErrors)
	if err != nil && file == nil {
		return nil, err
	}
	if file == nil {
		return nil, nil
	}

	text := string(body)
	out := make([]Record, 0, len(file.Decls))

	for _, decl := range file.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			start, end := lineRangeForNode(fset, d)
			name := d.Name.Name
			parent := ""
			if d.Recv != nil && len(d.Recv.List) > 0 {
				recv := renderExpr(fset, d.Recv.List[0].Type)
				parent = recv
				name = fmt.Sprintf("(%s).%s", recv, d.Name.Name)
			}
			snippet := snippetForRange(text, start, end)
			embed := fmt.Sprintf("go symbol %s in %s\n%s", name, normalizeRelPath(relPath), snippet)
			out = append(out, makeRecord(
				RecordKindSymbol,
				relPath,
				"go",
				name,
				parent,
				start,
				end,
				contentHash,
				embed,
			))
		case *ast.GenDecl:
			for _, spec := range d.Specs {
				switch s := spec.(type) {
				case *ast.TypeSpec:
					start, end := lineRangeForNode(fset, s)
					name := s.Name.Name
					snippet := snippetForRange(text, start, end)
					embed := fmt.Sprintf("go type %s in %s\n%s", name, normalizeRelPath(relPath), snippet)
					out = append(out, makeRecord(
						RecordKindSymbol,
						relPath,
						"go",
						name,
						"type",
						start,
						end,
						contentHash,
						embed,
					))
				case *ast.ValueSpec:
					start, end := lineRangeForNode(fset, s)
					kind := strings.ToLower(strings.TrimSpace(d.Tok.String()))
					for _, ident := range s.Names {
						if ident == nil {
							continue
						}
						name := ident.Name
						snippet := snippetForRange(text, start, end)
						embed := fmt.Sprintf("go %s %s in %s\n%s", kind, name, normalizeRelPath(relPath), snippet)
						out = append(out, makeRecord(
							RecordKindSymbol,
							relPath,
							"go",
							name,
							kind,
							start,
							end,
							contentHash,
							embed,
						))
					}
				}
			}
		}
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].StartLine != out[j].StartLine {
			return out[i].StartLine < out[j].StartLine
		}
		if out[i].EndLine != out[j].EndLine {
			return out[i].EndLine < out[j].EndLine
		}
		return out[i].Name < out[j].Name
	})
	return out, err
}

func lineRangeForNode(fset *token.FileSet, n ast.Node) (int, int) {
	if n == nil {
		return 0, 0
	}
	start := fset.PositionFor(n.Pos(), true).Line
	end := fset.PositionFor(n.End(), true).Line
	if start <= 0 {
		start = 1
	}
	if end < start {
		end = start
	}
	return start, end
}

func renderExpr(fset *token.FileSet, expr ast.Expr) string {
	var buf bytes.Buffer
	if err := format.Node(&buf, fset, expr); err != nil {
		return strings.TrimSpace(filepath.ToSlash(fmt.Sprintf("%T", expr)))
	}
	return strings.TrimSpace(buf.String())
}
