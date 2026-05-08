package metamodel

import (
	"strings"
	"time"

	"github.com/FINTLabs/fint-model/common/types"
)

func Build(classes []*types.Class, fintVersion, sourceCommit string) *Document {
	prefix := detectPackagePrefix(classes)
	namespacePrefix := detectNamespacePrefix(classes)

	byQualified := make(map[string]*types.Class, len(classes))
	for _, c := range classes {
		byQualified[c.Package+"."+c.Name] = c
	}

	byComponent := make(map[string][]*types.Class)
	componentOrder := make([]string, 0)
	componentPath := make(map[string][]string)
	for _, c := range classes {
		name := componentNameFromPackage(c.Package, prefix)
		path := componentPathFromNamespace(c.Namespace, namespacePrefix)
		if _, seen := byComponent[name]; !seen {
			componentOrder = append(componentOrder, name)
			componentPath[name] = path
		}
		byComponent[name] = append(byComponent[name], c)
	}

	components := make([]Component, 0, len(componentOrder))
	for _, name := range componentOrder {
		components = append(components, Component{
			Name:  name,
			Path:  componentPath[name],
			Types: convertTypes(name, byComponent[name], byQualified, prefix),
		})
	}

	return &Document{
		SchemaVersion: SchemaVersion,
		FintVersion:   fintVersion,
		GeneratedAt:   time.Now().UTC().Format(time.RFC3339),
		SourceCommit:  sourceCommit,
		Components:    components,
	}
}

func detectPackagePrefix(classes []*types.Class) string {
	if len(classes) == 0 {
		return ""
	}
	return commonPrefix(classes, func(c *types.Class) string { return c.Package })
}

func detectNamespacePrefix(classes []*types.Class) string {
	if len(classes) == 0 {
		return ""
	}
	return commonPrefix(classes, func(c *types.Class) string { return c.Namespace })
}

func commonPrefix(classes []*types.Class, get func(*types.Class) string) string {
	prefix := strings.Split(get(classes[0]), ".")
	for _, c := range classes[1:] {
		parts := strings.Split(get(c), ".")
		n := len(prefix)
		if len(parts) < n {
			n = len(parts)
		}
		i := 0
		for ; i < n; i++ {
			if prefix[i] != parts[i] {
				break
			}
		}
		prefix = prefix[:i]
	}
	return strings.Join(prefix, ".")
}

func componentNameFromPackage(pkg, prefix string) string {
	rest := strings.TrimPrefix(pkg, prefix+".")
	return strings.ReplaceAll(rest, ".", "-")
}

func componentPathFromNamespace(ns, prefix string) []string {
	rest := strings.TrimPrefix(ns, prefix+".")
	return strings.Split(rest, ".")
}

func componentRefForQualified(qualified, prefix string) string {
	rest := strings.TrimPrefix(qualified, prefix+".")
	lastDot := strings.LastIndex(rest, ".")
	if lastDot < 0 {
		return rest
	}
	pkgRel := rest[:lastDot]
	name := rest[lastDot+1:]
	return strings.ReplaceAll(pkgRel, ".", "-") + ":" + name
}

func convertTypes(componentName string, classes []*types.Class, byQualified map[string]*types.Class, prefix string) []Type {
	out := make([]Type, 0, len(classes))
	for _, c := range classes {
		out = append(out, convertType(componentName, c, byQualified, prefix))
	}
	return out
}

func convertType(componentName string, c *types.Class, byQualified map[string]*types.Class, prefix string) Type {
	t := Type{
		Name:                c.Name,
		Stereotype:          c.Stereotype,
		Abstract:            c.Abstract,
		Deprecated:          c.Deprecated,
		Parent:              resolveParent(c, byQualified, prefix),
		Identifiable:        c.Identifiable,
		ExtendsIdentifiable: c.ExtendsIdentifiable,
		ExtendsResource:     c.ExtendsResource,
		ExtendsRelations:    c.ExtendsRelations,
		Writable:            c.Writable,
		Documentation:       c.Documentation,
		Attributes:          convertAttributes(c, byQualified, prefix),
		Relations:           convertRelations(c, prefix),
	}
	if c.Stereotype == StereotypeMain {
		path := restPathFor(componentName, c.Name)
		t.Path = &path
	}
	if c.Identifiable {
		t.IdFields = collectIdFields(c, byQualified)
	}
	return t
}

func restPathFor(componentName, typeName string) string {
	return strings.ReplaceAll(componentName, "-", "/") + "/" + strings.ToLower(typeName)
}

func collectIdFields(c *types.Class, byQualified map[string]*types.Class) []string {
	seen := map[string]struct{}{}
	visited := map[*types.Class]struct{}{}
	out := make([]string, 0, 1)

	var visit func(cur *types.Class)
	visit = func(cur *types.Class) {
		if cur == nil {
			return
		}
		if _, dup := visited[cur]; dup {
			return
		}
		visited[cur] = struct{}{}

		if cur.Extends != "" {
			if qualified, ok := resolveShortName(cur.Extends, cur, byQualified); ok {
				visit(byQualified[qualified])
			}
		}

		for _, a := range cur.Attributes {
			if !strings.EqualFold(a.Type, "identifikator") {
				continue
			}
			if _, dup := seen[a.Name]; dup {
				continue
			}
			seen[a.Name] = struct{}{}
			out = append(out, a.Name)
		}
	}
	visit(c)
	return out
}

func resolveParent(c *types.Class, byQualified map[string]*types.Class, prefix string) *string {
	if c.Extends == "" {
		return nil
	}
	qualified, ok := resolveShortName(c.Extends, c, byQualified)
	if !ok {
		return nil
	}
	ref := componentRefForQualified(qualified, prefix)
	return &ref
}

func resolveShortName(name string, c *types.Class, byQualified map[string]*types.Class) (string, bool) {
	if candidate := c.Package + "." + name; byQualified[candidate] != nil {
		return candidate, true
	}
	for _, imp := range c.Imports {
		if strings.HasSuffix(imp, "."+name) {
			if byQualified[imp] != nil {
				return imp, true
			}
		}
	}
	return "", false
}

func convertAttributes(c *types.Class, byQualified map[string]*types.Class, prefix string) []Attribute {
	out := make([]Attribute, 0, len(c.Attributes))
	for _, a := range c.Attributes {
		out = append(out, Attribute{
			Name:       a.Name,
			Type:       resolveTypeRef(a.Type, c, byQualified, prefix),
			List:       a.List,
			Optional:   a.Optional,
			Deprecated: a.Deprecated,
			Writable:   a.Writable,
		})
	}
	return out
}

func resolveTypeRef(typeName string, c *types.Class, byQualified map[string]*types.Class, prefix string) string {
	lower := strings.ToLower(typeName)
	if IsPrimitive(lower) {
		return lower
	}
	if qualified, ok := resolveShortName(typeName, c, byQualified); ok {
		return componentRefForQualified(qualified, prefix)
	}
	return typeName
}

func convertRelations(c *types.Class, prefix string) []Relation {
	out := make([]Relation, 0, len(c.Relations))
	for _, r := range c.Relations {
		var bidi *Bidirectional
		if r.InverseName != "" {
			bidi = &Bidirectional{
				IsSource:    r.IsSource,
				InverseName: r.InverseName,
			}
		}
		target := componentRefForQualified(r.Package+"."+r.Target, prefix)
		out = append(out, Relation{
			Name:          r.Name,
			Target:        target,
			Multiplicity:  r.Multiplicity,
			Bidirectional: bidi,
			Deprecated:    r.Deprecated,
		})
	}
	return out
}
