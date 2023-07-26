package linter

import "strings"

const ignoreAnnotation = "yale.terra.bio/linter-ignore"

const ignoreAll = "all"

type ignoreCfg struct {
	all     bool
	secrets map[string]struct{}
}

func parseIgnoreAnnotations(annotations map[string]string) ignoreCfg {
	cfg := ignoreCfg{
		all:     false,
		secrets: make(map[string]struct{}),
	}

	value, exists := annotations[ignoreAnnotation]
	if !exists {
		return cfg
	}

	for _, token := range strings.Split(value, ",") {
		token = strings.TrimSpace(token)
		if token == ignoreAll {
			cfg.all = true
		} else {
			cfg.secrets[token] = struct{}{}
		}
	}

	return cfg
}

func (i ignoreCfg) ignoresSecret(name string) bool {
	if i.all {
		return true
	}
	_, exists := i.secrets[name]
	return exists
}
