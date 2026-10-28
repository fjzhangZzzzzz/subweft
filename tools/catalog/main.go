// catalog extracts field/type facts from a fixed source archive. It neither
// links nor copies upstream implementations into the application.
package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"os"
	"path"
	"reflect"
	"strconv"
	"strings"
)

type Field struct {
	Kind         string            `json:"kind"`
	Enum         []string          `json:"enum,omitempty"`
	Alternatives []*Field          `json:"alternatives,omitempty"`
	Required     bool              `json:"required,omitempty"`
	Properties   map[string]*Field `json:"properties,omitempty"`
	Items        *Field            `json:"items,omitempty"`
	Values       *Field            `json:"values,omitempty"`
	Min          *float64          `json:"min,omitempty"`
	Max          *float64          `json:"max,omitempty"`
	Secret       bool              `json:"secret,omitempty"`
}
type definition struct {
	expr    ast.Expr
	dir     string
	imports map[string]string
}

var definitions = map[string]definition{}
var unresolved = map[string]bool{}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run() error {
	source := flag.String("source", "output/mihomo-source.tar.gz", "reviewed source archive")
	out := flag.String("output", "output/catalog-raw.json", "output field facts")
	flag.Parse()
	raw, err := os.ReadFile(*source)
	if err != nil {
		return err
	}
	sum := sha256.Sum256(raw)
	if hex.EncodeToString(sum[:]) != "ecda5f8c8a8a1d60bc088c2640c85d92c8f002c0686234a26bdf671735afe216" {
		return fmt.Errorf("source_archive_digest_mismatch")
	}
	gzipReader, err := gzip.NewReader(bytes.NewReader(raw))
	if err != nil {
		return err
	}
	defer gzipReader.Close()
	reader := tar.NewReader(gzipReader)
	for {
		header, err := reader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		if !strings.HasSuffix(header.Name, ".go") || strings.HasSuffix(header.Name, "_test.go") || header.Size > 4<<20 {
			continue
		}
		name := strings.SplitN(header.Name, "/", 2)
		if len(name) != 2 {
			continue
		}
		body, err := io.ReadAll(io.LimitReader(reader, 4<<20))
		if err != nil {
			return err
		}
		file, err := parser.ParseFile(token.NewFileSet(), name[1], body, 0)
		if err != nil {
			return err
		}
		imports := map[string]string{}
		for _, imp := range file.Imports {
			value, _ := strconv.Unquote(imp.Path.Value)
			alias := path.Base(value)
			if imp.Name != nil {
				alias = imp.Name.Name
			}
			imports[alias] = strings.TrimPrefix(value, "github.com/metacubex/mihomo/")
		}
		dir := path.Dir(name[1])
		for _, decl := range file.Decls {
			group, ok := decl.(*ast.GenDecl)
			if !ok {
				continue
			}
			for _, spec := range group.Specs {
				typ, ok := spec.(*ast.TypeSpec)
				if ok {
					definitions[dir+"/"+typ.Name.Name] = definition{typ.Type, dir, imports}
				}
			}
		}
	}
	roots := map[string]string{"ss": "ShadowSocksOption", "ssr": "ShadowSocksROption", "vmess": "VmessOption", "vless": "VlessOption", "trojan": "TrojanOption", "http": "HttpOption", "socks5": "Socks5Option", "hysteria": "HysteriaOption", "hysteria2": "Hysteria2Option", "tuic": "TuicOption", "wireguard": "WireGuardOption", "snell": "SnellOption", "anytls": "AnyTLSOption"}
	protocols := map[string]*Field{}
	for name, typ := range roots {
		protocols[name] = resolve("adapter/outbound/"+typ, "proxy", nil)
	}
	dns := resolve("config/RawDNS", "yaml", nil)
	config := resolve("config/RawConfig", "yaml", nil)
	extras := map[string]*Field{}
	for _, name := range []string{"simpleObfsOption", "v2rayObfsOption", "gostObfsOption", "shadowTLSOption", "restlsOption", "jlsOption", "kcpTunOption"} {
		extras[name] = resolve("adapter/outbound/"+name, "obfs", nil)
	}
	groups := map[string]*Field{}
	for name, typ := range map[string]string{"select": "SelectorOption", "url-test": "URLTestOption", "fallback": "FallbackOption", "load-balance": "LoadBalanceOption"} {
		f := resolve("adapter/outboundgroup/GroupCommonOption", "group", nil)
		specific := resolve("adapter/outboundgroup/"+typ, "group", nil)
		for k, v := range specific.Properties {
			f.Properties[k] = v
		}
		groups[name] = f
	}
	payload := map[string]any{"version": "v1.19.33", "commit": "3f27ef39f8c96bbfbc8b0b9bb10f5c9ef60149d8", "source_archive_sha256": hex.EncodeToString(sum[:]), "protocols": protocols, "dns": dns, "config": config, "groups": groups, "pluginTypes": extras, "unresolved": unresolved}
	review(payload)
	encoded, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(*out, append(encoded, '\n'), 0644)
}
func resolve(name, tag string, stack map[string]bool) *Field {
	if stack == nil {
		stack = map[string]bool{}
	}
	if stack[name] {
		unresolved[name] = true
		return &Field{Kind: "any"}
	}
	d, ok := definitions[name]
	if !ok {
		unresolved[name] = true
		return &Field{Kind: "any"}
	}
	next := map[string]bool{}
	for k, v := range stack {
		next[k] = v
	}
	next[name] = true
	return shape(d.expr, d, tag, next)
}
func shape(expr ast.Expr, d definition, tag string, stack map[string]bool) *Field {
	switch x := expr.(type) {
	case *ast.Ident:
		switch x.Name {
		case "string":
			return &Field{Kind: "string"}
		case "bool":
			return &Field{Kind: "boolean"}
		case "any":
			return &Field{Kind: "any"}
		case "float32", "float64":
			return &Field{Kind: "number"}
		case "int", "int8", "int16", "int32", "int64", "uint", "uint8", "uint16", "uint32", "uint64", "byte":
			f := &Field{Kind: "integer"}
			bounds := map[string]float64{"int8": 127, "int16": 32767, "int32": 2147483647, "uint8": 255, "byte": 255, "uint16": 65535, "uint32": 4294967295}
			if strings.HasPrefix(x.Name, "uint") || x.Name == "byte" {
				zero := float64(0)
				f.Min = &zero
			}
			if max, ok := bounds[x.Name]; ok {
				f.Max = &max
			}
			return f
		}
		return resolve(d.dir+"/"+x.Name, tag, stack)
	case *ast.StarExpr:
		return shape(x.X, d, tag, stack)
	case *ast.SelectorExpr:
		if pkg, ok := x.X.(*ast.Ident); ok {
			if d.imports[pkg.Name] == "net/netip" {
				return &Field{Kind: "string"}
			}
			return resolve(d.imports[pkg.Name]+"/"+x.Sel.Name, tag, stack)
		}
	case *ast.ArrayType:
		return &Field{Kind: "array", Items: shape(x.Elt, d, tag, stack)}
	case *ast.MapType:
		return &Field{Kind: "map", Values: shape(x.Value, d, tag, stack)}
	case *ast.InterfaceType:
		return &Field{Kind: "any"}
	case *ast.IndexExpr:
		return &Field{Kind: "map", Values: shape(x.Index, d, tag, stack)}
	case *ast.IndexListExpr:
		if len(x.Indices) > 0 {
			return &Field{Kind: "map", Values: shape(x.Indices[len(x.Indices)-1], d, tag, stack)}
		}
	case *ast.StructType:
		fields := map[string]*Field{}
		for _, member := range x.Fields.List {
			if len(member.Names) == 0 {
				embedded := shape(member.Type, d, tag, stack)
				for key, value := range embedded.Properties {
					fields[key] = value
				}
				continue
			}
			if member.Tag == nil {
				continue
			}
			tags, _ := strconv.Unquote(member.Tag.Value)
			value, ok := reflect.StructTag(tags).Lookup(tag)
			if !ok && tag == "obfs" {
				value, ok = reflect.StructTag(tags).Lookup("proxy")
			}
			if !ok || value == "-" {
				continue
			}
			parts := strings.Split(value, ",")
			if parts[0] == "" {
				continue
			}
			field := shape(member.Type, d, tag, stack)
			field.Required = (tag == "proxy" || tag == "obfs") && !strings.Contains(value, ",omitempty")
			key := parts[0]
			lower := strings.ToLower(key)
			field.Secret = strings.Contains(lower, "password") || strings.Contains(lower, "private-key") || strings.Contains(lower, "auth") || lower == "uuid" || lower == "username" || lower == "psk" || lower == "token" || lower == "short-id" || lower == "headers" || lower == "path" || lower == "grpc-service-name"
			fields[key] = field
		}
		return &Field{Kind: "object", Properties: fields}
	}
	unresolved[fmt.Sprintf("%T", expr)] = true
	return &Field{Kind: "any"}
}
