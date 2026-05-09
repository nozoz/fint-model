package metamodel

import (
	"encoding/json"
	"os"
	"strings"

	"github.com/FINTLabs/fint-model/common/config"
	"github.com/FINTLabs/fint-model/common/types"
)

const dateImport = "java.util.Date"

func Load(path string) (*Document, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var doc Document
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, err
	}
	return &doc, nil
}

// ToClasses reverses Build: converts a Document into the flat []*types.Class
// shape the existing emitters consume, plus the Java-package and C#-namespace
// classMaps used by action-enum generation. All derived fields (Resource,
// Resources, Imports, Using) are recomputed; Identifiable / Extends* /
// Writable are taken straight from the JSON since they're already baked there.
func ToClasses(doc *Document) ([]*types.Class, map[string][]*types.Class, map[string][]*types.Class) {
	classes := make([]*types.Class, 0)
	byRef := make(map[string]*types.Class)
	typeByClass := make(map[*types.Class]*Type)
	parentRefByClass := make(map[*types.Class]string)

	for ci := range doc.Components {
		comp := &doc.Components[ci]
		javaPkg := javaPackageFor(comp.Name)
		csNs := csNamespaceFor(comp.Name)
		for ti := range comp.Types {
			t := &comp.Types[ti]
			c := &types.Class{
				Name:          t.Name,
				Abstract:      t.Abstract,
				Deprecated:    t.Deprecated,
				Documentation: t.Documentation,
				Stereotype:    t.Stereotype,
				Package:       javaPkg,
				Namespace:     csNs,
				Identifiable:  t.Identifiable,
				GitTag:        doc.FintVersion,
			}
			if t.Parent != nil {
				_, parentName := splitRef(*t.Parent)
				c.Extends = parentName
				parentRefByClass[c] = *t.Parent
			}
			classes = append(classes, c)
			byRef[comp.Name+":"+t.Name] = c
			typeByClass[c] = t
		}
	}

	for _, c := range classes {
		t := typeByClass[c]
		c.Attributes = attributesFromJSON(t.Attributes)
		c.Relations = relationsFromJSON(t.Relations)
	}

	var resourceOf func(c *types.Class) bool
	resourceOf = func(c *types.Class) bool {
		if len(c.Relations) > 0 {
			return true
		}
		if pref, ok := parentRefByClass[c]; ok {
			if p, ok := byRef[pref]; ok {
				return resourceOf(p)
			}
		}
		return false
	}
	for _, c := range classes {
		c.Resource = resourceOf(c)
	}

	for _, c := range classes {
		t := typeByClass[c]
		ownIdx := 0
		for _, a := range t.Attributes {
			if a.Inherited {
				continue
			}
			if strings.Contains(a.Type, ":") {
				if target, ok := byRef[a.Type]; ok && target.Resource {
					c.Resources = append(c.Resources, c.Attributes[ownIdx])
				}
			}
			ownIdx++
		}
	}

	for _, c := range classes {
		ref, hasParent := parentRefByClass[c]
		if !hasParent {
			continue
		}
		parent, ok := byRef[ref]
		if !ok {
			continue
		}
		c.ExtendsIdentifiable = parent.Identifiable
		c.ExtendsRelations = len(parent.Relations) > 0
		c.ExtendsResource = parent.Resource || len(parent.Resources) > 0
	}

	for _, c := range classes {
		t := typeByClass[c]
		c.Imports = computeJavaImports(c, t, byRef)
		c.Using = computeCSUsing(c, t, byRef)
	}

	javaPCM := make(map[string][]*types.Class)
	csPCM := make(map[string][]*types.Class)
	for _, c := range classes {
		javaPCM[c.Package] = append(javaPCM[c.Package], c)
		csPCM[c.Namespace] = append(csPCM[c.Namespace], c)
	}
	return classes, javaPCM, csPCM
}

func javaPackageFor(componentName string) string {
	return config.JAVA_PACKAGE_BASE + "." + strings.ReplaceAll(componentName, "-", ".")
}

func csNamespaceFor(componentName string) string {
	parts := strings.Split(componentName, "-")
	for i, p := range parts {
		if p != "" {
			parts[i] = strings.ToUpper(p[:1]) + p[1:]
		}
	}
	return config.NET_NAMESPACE_BASE + "." + strings.Join(parts, ".")
}

func javaPackageForComponent(componentName string) string {
	return config.JAVA_PACKAGE_BASE + "." + strings.ReplaceAll(componentName, "-", ".")
}

func splitRef(ref string) (component, name string) {
	idx := strings.Index(ref, ":")
	if idx < 0 {
		return "", ref
	}
	return ref[:idx], ref[idx+1:]
}

func attributesFromJSON(in []Attribute) []types.Attribute {
	out := make([]types.Attribute, 0, len(in))
	for _, a := range in {
		if a.Inherited {
			continue
		}
		typeStr := a.Type
		if strings.Contains(a.Type, ":") {
			_, typeStr = splitRef(a.Type)
		}
		out = append(out, types.Attribute{
			Name:       a.Name,
			Type:       typeStr,
			List:       a.List,
			Optional:   a.Optional,
			Deprecated: a.Deprecated,
		})
	}
	return out
}

func relationsFromJSON(in []Relation) []types.Association {
	out := make([]types.Association, 0, len(in))
	for _, r := range in {
		if r.Inherited {
			continue
		}
		targetComp, targetName := splitRef(r.Target)
		assoc := types.Association{
			Name:         r.Name,
			Target:       targetName,
			Multiplicity: r.Multiplicity,
			Package:      javaPackageForComponent(targetComp),
			Deprecated:   r.Deprecated,
		}
		if r.Bidirectional != nil {
			assoc.IsSource = r.Bidirectional.IsSource
			assoc.InverseName = r.Bidirectional.InverseName
		}
		out = append(out, assoc)
	}
	return out
}

func computeJavaImports(c *types.Class, t *Type, byRef map[string]*types.Class) []string {
	self := c.Package + "." + c.Name
	seen := map[string]struct{}{}
	imps := make([]string, 0)
	add := func(qualified string) {
		if qualified == "" || qualified == self {
			return
		}
		if _, dup := seen[qualified]; dup {
			return
		}
		seen[qualified] = struct{}{}
		imps = append(imps, qualified)
	}
	for _, a := range t.Attributes {
		if a.Inherited {
			continue
		}
		if strings.Contains(a.Type, ":") {
			if target, ok := byRef[a.Type]; ok {
				add(target.Package + "." + target.Name)
			}
			continue
		}
		if a.Type == "date" || a.Type == "datetime" {
			add(dateImport)
		}
	}
	if t.Parent != nil {
		if target, ok := byRef[*t.Parent]; ok {
			add(target.Package + "." + target.Name)
		}
	}
	return imps
}

func computeCSUsing(c *types.Class, t *Type, byRef map[string]*types.Class) []string {
	seen := map[string]struct{}{}
	usings := make([]string, 0)
	add := func(ns string) {
		if ns == "" || ns == c.Namespace {
			return
		}
		if _, dup := seen[ns]; dup {
			return
		}
		seen[ns] = struct{}{}
		usings = append(usings, ns)
	}
	for _, a := range t.Attributes {
		if a.Inherited {
			continue
		}
		if !strings.Contains(a.Type, ":") {
			continue
		}
		if target, ok := byRef[a.Type]; ok {
			add(target.Namespace)
		}
	}
	if t.Parent != nil {
		if target, ok := byRef[*t.Parent]; ok {
			add(target.Namespace)
		}
	}
	return usings
}
