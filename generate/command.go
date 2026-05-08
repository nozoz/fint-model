package generate

import (
	"fmt"
	"io/ioutil"
	"os"
	"strings"

	"github.com/FINTLabs/fint-model/common/config"
	"github.com/FINTLabs/fint-model/common/metamodel"
	"github.com/FINTLabs/fint-model/common/types"
	"github.com/urfave/cli"
)

func CmdGenerate(c *cli.Context) {
	fromJSON := c.String("from-json")
	if fromJSON == "" {
		fmt.Fprintln(os.Stderr, "ERROR: generate requires --from-json <path>")
		fmt.Fprintln(os.Stderr, "Produce a metamodel.json first with `fint-model -t <release> metamodel -o metamodel.json`.")
		os.Exit(2)
	}

	doc, err := metamodel.Load(fromJSON)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: load %s: %v\n", fromJSON, err)
		os.Exit(1)
	}
	classes, javaPCM, csPCM := metamodel.ToClasses(doc)
	tag := doc.FintVersion
	resource := c.Bool("resource")
	lang := c.String("lang")

	if lang == "JAVA" || lang == "ALL" {
		generateJavaCode(classes, javaPCM, tag, resource)
	}
	if lang == "CS" || lang == "ALL" {
		generateCSCode(classes, csPCM, tag, resource)
	}
}

func writeFile(path, filename string, content []byte) error {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		if err = os.MkdirAll(path, 0777); err != nil {
			return err
		}
	}
	return ioutil.WriteFile(path+"/"+filename, content, 0777)
}

func writeJavaClass(pkg, class string, content []byte) error {
	path := fmt.Sprintf("%s/%s", config.JAVA_BASE_PATH, strings.Replace(pkg, ".", "/", -1))
	return writeFile(removeJavaPackagePathFromFilePath(path), class+".java", content)
}

func generateJavaCode(classes []*types.Class, packageClassMap map[string][]*types.Class, tag string, resource bool) {
	fmt.Println("Generating Java code:")
	setupJavaDirStructure()
	for _, c := range classes {
		if resource && (c.Resource || len(c.Resources) > 0 || c.Identifiable) {
			fmt.Printf("  > Creating resource class: %sResource.java\n", c.Name)
			pkg := strings.Replace(c.Package, ".model.", ".model.resource.", -1)
			if err := writeJavaClass(pkg, c.Name+"Resource", []byte(GetJavaResourceClass(c))); err != nil {
				fmt.Printf("Unable to write file: %s", err)
			}
			fmt.Printf("  > Creating resources class: %sResources.java\n", c.Name)
			if err := writeJavaClass(pkg, c.Name+"Resources", []byte(GetJavaResourcesClass(c))); err != nil {
				fmt.Printf("Unable to write file: %s", err)
			}
		}

		fmt.Printf("  > Creating class: %s.java\n", c.Name)
		if err := writeJavaClass(c.Package, c.Name, []byte(GetJavaClass(c))); err != nil {
			fmt.Printf("Unable to write file: %s", err)
		}
	}

	for p, cl := range packageClassMap {
		action := getAction(p, cl, tag)
		fmt.Printf("  > Creating action: %s.java\n", action.Name)
		path := fmt.Sprintf("%s/%s", config.JAVA_BASE_PATH, strings.Replace(p, ".", "/", -1))
		if err := writeFile(removeJavaPackagePathFromFilePath(path), action.Name+".java", []byte(GetJavaActionEnum(action))); err != nil {
			fmt.Printf("Unable to write file: %s", err)
		}
	}

	fmt.Println("Finish generating Java code!")
}

func removeJavaPackagePathFromFilePath(path string) string {
	return strings.Replace(path, "no/novari/fint/model/", "", -1)
}

func getAction(p string, cl []*types.Class, tag string) types.Action {
	var action types.Action

	parts := strings.Split(p, ".")
	action.Name = fmt.Sprintf("%sActions", strings.Title(parts[len(parts)-1]))
	action.Package = p
	action.Namespace = p
	action.GitTag = tag

	for _, c := range cl {
		if c.Identifiable && !c.Abstract {
			action.Classes = append(action.Classes, strings.ToUpper(c.Name))
		}
	}
	return action
}

func writeCSClass(namespace, class string, content []byte, resource bool) error {
	return writeFile(getCSPath(namespace, resource), class+".cs", content)
}

func generateCSCode(classes []*types.Class, packageClassMap map[string][]*types.Class, tag string, resource bool) {
	fmt.Println("Generating CSharp code:")
	setupCSDirStructure()
	for _, c := range classes {
		if resource && (c.Resource || len(c.Resources) > 0 || c.Identifiable) {
			fmt.Printf("  > Creating resource class: %sResource.cs\n", c.Name)
			if err := writeCSClass(c.Namespace, c.Name+"Resource", []byte(GetCSResourceClass(c)), true); err != nil {
				fmt.Printf("Unable to write file: %s", err)
			}
			fmt.Printf("  > Creating resources class: %sResources.cs\n", c.Name)
			if err := writeCSClass(c.Namespace, c.Name+"Resources", []byte(GetCSResourcesClass(c)), true); err != nil {
				fmt.Printf("Unable to write file: %s", err)
			}
		}

		fmt.Printf("  > Creating class: %s.cs\n", c.Name)
		if err := writeCSClass(c.Namespace, c.Name, []byte(GetCSClass(c)), false); err != nil {
			fmt.Printf("Unable to write file: %s", err)
		}
	}

	for p, cl := range packageClassMap {
		action := getAction(p, cl, tag)
		fmt.Printf("  > Creating action: %s.cs\n", action.Name)
		if err := writeCSClass(p, action.Name, []byte(GetCSActionEnum(action)), false); err != nil {
			fmt.Printf("Unable to write file: %s", err)
		}
	}

	fmt.Println("Finish generating CSharp code!")
}

func setupCSDirStructure() {
	fmt.Println("  > Setup directory structure.")
	os.RemoveAll(config.CS_BASE_PATH)
	if err := os.MkdirAll(config.CS_BASE_PATH, 0777); err != nil {
		fmt.Println("Unable to create base structure:", err)
	}
}

func getCSPath(ns string, resource bool) string {
	base := getCSBase(resource)
	nsList := strings.Split(ns, ".")
	projectDir := fmt.Sprintf("%s.%s.%s", nsList[0], nsList[1], nsList[2])
	subDirs := strings.Replace(strings.Replace(ns, projectDir, "", -1), ".", "/", -1)
	return fmt.Sprintf("%s/%s/%s", base, projectDir, subDirs)
}

func getCSBase(resource bool) string {
	if resource {
		return config.CS_BASE_PATH + "/resource"
	}
	return config.CS_BASE_PATH
}

func setupJavaDirStructure() {
	fmt.Println("  > Setup directory structure.")
	os.RemoveAll(config.JAVA_BASE_PATH)
	if err := os.MkdirAll(config.JAVA_BASE_PATH, 0777); err != nil {
		fmt.Println("Unable to create base structure:", err)
	}
}
