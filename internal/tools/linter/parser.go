package linter

import (
	"bytes"
	"fmt"
	"github.com/broadinstitute/yale/internal/yale/crd/api/v1beta1"
	"github.com/broadinstitute/yale/internal/yale/logs"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"os"
	"path/filepath"
	"strings"
)

var yamlExts = []string{".yaml", ".yml"}

// parser scans a directory of YAML files and parses them into resource objects, classified
// by type.
type parser struct {
	decoder runtime.Decoder
}

func newParser() (*parser, error) {
	var err error

	sch := runtime.NewScheme()
	if err = clientgoscheme.AddToScheme(sch); err != nil {
		return nil, fmt.Errorf("error adding built-in K8s resources to new scheme: %v", err)
	}
	if err = v1beta1.AddToScheme(sch); err != nil {
		return nil, fmt.Errorf("error adding Yale CRD to scheme: %v", err)
	}

	decoder := serializer.NewCodecFactory(sch).UniversalDeserializer()

	return &parser{
		decoder: decoder,
	}, nil
}

func (p *parser) parseFilesInDirectory(dir string) (*resources, error) {
	resources := new(resources)

	files, err := listYamlFiles(dir)
	if err != nil {
		return nil, err
	}

	for _, file := range files {
		if err = p.parseFile(resources, file); err != nil {
			return nil, err
		}
	}

	return resources, nil
}

func (p *parser) parseFile(resources *resources, file string) error {
	docs, err := splitFileIntoDocuments(file)
	if err != nil {
		return err
	}

	for _, doc := range docs {
		var obj runtime.Object
		obj, _, err = p.decoder.Decode(doc.content, nil, nil)
		if err != nil {
			if strings.Contains(err.Error(), "is registered for version") {
				// handle errors for CRDs we haven't added to the schema, eg.
				//   no kind "BackendConfig" is registered for version "cloud.google.com/v1" in scheme "pkg/runtime/scheme.go:100"
				logs.Warn.Printf("ignoring CRD at line %d in %s: %v", doc.offset, doc.filename, err)
			} else {
				return fmt.Errorf("error parsing CRD at line %d in %s: %v", doc.offset, doc.filename, err)
			}
		}

		if gsk, ok := obj.(*v1beta1.GcpSaKey); ok {
			resources.gsks = append(resources.gsks, resource[v1beta1.GcpSaKey]{*gsk, doc, gsk.Kind, gsk.Name, gsk.Annotations})
		} else if dep, ok := obj.(*appsv1.Deployment); ok {
			resources.deployments = append(resources.deployments, resource[appsv1.Deployment]{*dep, doc, dep.Kind, dep.Name, dep.Annotations})
		} else if sts, ok := obj.(*appsv1.StatefulSet); ok {
			resources.statefulSets = append(resources.statefulSets, resource[appsv1.StatefulSet]{*sts, doc, sts.Kind, sts.Name, sts.Annotations})
		}
	}

	return nil
}

func splitFileIntoDocuments(file string) ([]document, error) {
	content, err := os.ReadFile(file)
	if err != nil {
		return nil, fmt.Errorf("error reading file %s: %v", file, err)
	}

	var documents []document
	offset := 1

	fragments := bytes.Split(content, []byte("---\n"))
	for _, fragment := range fragments {
		nlines := bytes.Count(fragment, []byte("\n"))

		// only add the fragment if it has non-comment content, empty docs seem to make
		// the k8s parser unhappy
		if hasContent(fragment) {
			documents = append(documents, document{
				content:  fragment,
				offset:   offset,
				filename: file,
			})
		}

		offset += nlines + 1 // add 1 to account for the "---\n" separator
	}

	return documents, nil
}

// returns true if the yaml doc has any content other than whitespace and comments
func hasContent(yamlDoc []byte) bool {
	for _, line := range bytes.Split(yamlDoc, []byte("\n")) {
		trimmed := bytes.TrimSpace(line)
		if len(trimmed) > 0 && !bytes.HasPrefix(trimmed, []byte("#")) {
			return true
		}
	}
	return false
}

// returns a list of yaml files in a directory
func listYamlFiles(dir string) ([]string, error) {
	var files []string
	err := filepath.Walk(dir,
		func(filepath string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() {
				return nil
			}
			if !isYamlFile(filepath) {
				return nil
			}
			files = append(files, filepath)
			return nil
		})
	if err != nil {
		return nil, fmt.Errorf("error traversing directory %s: %v", dir, err)
	}
	return files, nil
}

func isYamlFile(filepath string) bool {
	for _, ext := range yamlExts {
		if strings.HasSuffix(filepath, ext) {
			return true
		}
	}
	return false
}
