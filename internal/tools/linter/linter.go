package linter

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"github.com/broadinstitute/yale/internal/yale/crd/api/v1beta1"
	"github.com/broadinstitute/yale/internal/yale/logs"
	appsv1 "k8s.io/api/apps/v1"
	"os"
	"path/filepath"
	"regexp"
	"strings"
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

type secret struct {
	name   string
	regexp *regexp.Regexp
}

type document struct {
	content  []byte
	offset   int
	filename string
}

func Run(globs ...string) ([]reference, error) {
	parser, err := newParser()
	if err != nil {
		return nil, err
	}

	dirs, err := expandGlobsToDirs(globs)
	if err != nil {
		return nil, err
	}

	var matches []reference
	for _, dir := range dirs {
		dirMatches, err := scanDir(parser, dir)
		if err != nil {
			return nil, fmt.Errorf("error scanning dir %s: %v", dir, err)
		}
		matches = append(dirMatches, matches...)
	}

	count := len(matches)
	msg := fmt.Sprintf("Found %d resources with missing annotations", count)
	if count <= 0 {
		logs.Info.Println(msg)
		return nil, nil
	}

	msg = msg + ":\n"
	for _, m := range matches {
		msg = msg + "    " + m.summarize() + "\n"
	}
	return matches, errors.New(msg)
}

func scanDir(parser *parser, dir string) ([]reference, error) {
	logs.Info.Printf("Scanning %s...", dir)
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

	ignore := parseIgnoreAnnotations(r.annotations)

	scanner := bufio.NewScanner(bytes.NewReader(r.document.content))
	var lineoffset int
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
				logs.Debug.Printf("%s: will reload (%s)", ref.summarize(), reason)
				continue
			}
			logs.Info.Printf("%s: WILL NOT reload on changes", ref.summarize())
			if ignore.ignoresSecret(s.name) {
				logs.Info.Printf("%s: ignoring missing reloader annotation", ref.summarize())
				continue
			}

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

func expandGlobsToDirs(globs []string) ([]string, error) {
	var dirs []string

	for _, glob := range globs {
		matches, err := expandOneGlob(glob)
		if err != nil {
			return nil, err
		}

		for _, match := range matches {
			fileInfo, err := os.Stat(match)
			if err != nil {
				return nil, fmt.Errorf("error checking if %s is a directory: %v", match, err)
			}
			if !fileInfo.IsDir() {
				logs.Warn.Printf("Ignoring non-directory %s\n", match)
			}
			dirs = append(dirs, match)
		}
	}
	return dirs, nil
}

func expandOneGlob(glob string) ([]string, error) {
	if !strings.Contains(glob, "*") {
		return []string{glob}, nil
	}

	matches, err := filepath.Glob(glob)
	if err != nil {
		return nil, fmt.Errorf("error expanding glob %s: %v", glob, err)
	}
	if len(matches) == 0 {
		logs.Warn.Printf("%s matched 0 directories on filesystem", glob)
	}
	return matches, nil
}
