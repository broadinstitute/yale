package linter

import (
	"bufio"
	"bytes"
	"fmt"
	"github.com/broadinstitute/yale/internal/yale/crd/api/v1beta1"
	"github.com/broadinstitute/yale/internal/yale/logs"
	appsv1 "k8s.io/api/apps/v1"
	"regexp"
)

type resources struct {
	deployments  []resource[appsv1.Deployment]
	statefulSets []resource[appsv1.StatefulSet]
	gsks         []resource[v1beta1.GcpSaKey]
}

type resource[T any] struct {
	typed       T
	document    document
	kind        string
	name        string
	annotations map[string]string
}

type reference struct {
	filename string
	lineno   int
	kind     string
	name     string
	secret   string
}

func (r reference) summarize() string {
	return fmt.Sprintf("%s:%d -- %s %s references Yale secret %s", r.filename, r.lineno, r.kind, r.name, r.secret)
}

type secret struct {
	name   string
	regexp *regexp.Regexp
}

type document struct {
	content  []byte
	offset   int
	filename string
}

func Run(dir string) error {
	matches, err := scanDir(dir)
	if err != nil {
		return err
	}

	count := len(matches)
	msg := fmt.Sprintf("Found %d resources with missing annotations", count)
	if count <= 0 {
		logs.Info.Println(msg)
		return nil
	}

	msg = msg + ":\n"
	for _, m := range matches {
		msg = msg + "    " + m.summarize() + "\n"
	}
	return fmt.Errorf(msg)
}

func scanDir(dir string) ([]reference, error) {
	parser, err := newParser()
	if err != nil {
		return nil, err
	}

	resources, err := parser.parseFilesInDirectory(dir)
	if err != nil {
		return nil, err
	}

	var secrets []secret
	for _, gsk := range resources.gsks {
		secretName := gsk.typed.Spec.Secret.Name
		secrets = append(secrets, secret{
			name:   secretName,
			regexp: buildRegexpToMatchSecretName(secretName),
		})
	}

	var matches []reference
	matches = append(matches, scanAllOfType(resources.deployments, secrets)...)
	matches = append(matches, scanAllOfType(resources.statefulSets, secrets)...)
	return matches, nil
}

func scanAllOfType[T any](rs []resource[T], secrets []secret) []reference {
	var matches []reference
	for _, r := range rs {
		matches = append(matches, scan(r, secrets)...)
	}
	return matches
}

// here we walk through the document line by line and search for references to Yale secrets
func scan[T any](r resource[T], secrets []secret) []reference {
	var matches []reference

	reloader := parseReloaderAnnotations(r.annotations)

	scanner := bufio.NewScanner(bytes.NewReader(r.document.content))
	lineoffset := 0
	for scanner.Scan() {
		for _, s := range secrets {
			line := scanner.Bytes()
			line = bytes.SplitN(line, []byte("#"), 2)[0] // strip comments

			if !s.regexp.Match(line) {
				continue
			}

			ref := reference{
				filename: r.document.filename,
				lineno:   r.document.offset + lineoffset,
				kind:     r.kind,
				name:     r.name,
				secret:   s.name,
			}

			reason, reloads := reloader.reloadsOnSecret(s.name)
			if reloads {
				logs.Info.Printf("%s: will reload (%s)", ref.summarize(), reason)
				continue
			}

			logs.Warn.Printf("%s: WILL NOT reload on changes", ref.summarize())

			matches = append(matches, ref)
		}
		lineoffset++
	}

	return matches
}

func buildRegexpToMatchSecretName(secretName string) *regexp.Regexp {
	// match lines that include the secret name, bordered by non-alphanumeric-plus-slash characters or start-of-line/end-of-line
	return regexp.MustCompile("(^|[^a-z0-9-])" + regexp.QuoteMeta(secretName) + "([^a-z0-9-]|$)")
}
