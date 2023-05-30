package linter

import "strings"

// if "true", reloader will reload on all CMs/Secrets used by the Deployment/StatefulSet
const autoAnnotation = "reloader.stakater.com/auto"

// if "true", reloader will reload on CMs/Secrets used by the Deployment/StatefulSet that have a "match" annotation.
// (Yale adds this annotation to secrets it creates)
const searchAnnotation = "reloader.stakater.com/search"

// reloader will reload on a configured list of Secrets used by the Deployment/StatefulSet
const secretListAnnotation = "secret.reloader.stakater.com/reload"

type reloaderCfg struct {
	auto   bool
	search bool
	list   map[string]struct{}
}

func (r reloaderCfg) reloadsOnSecret(secretName string) (string, bool) {
	if r.auto {
		return "has annotation " + autoAnnotation, true
	}
	if r.search {
		return "has annotation " + searchAnnotation, true
	}
	_, exists := r.list[secretName]
	if exists {
		return "annotation " + secretListAnnotation + " includes " + secretName, true
	}

	return "", false
}

func parseReloaderAnnotations(annotations map[string]string) reloaderCfg {
	parsed := reloaderCfg{
		auto:   false,
		search: false,
		list:   make(map[string]struct{}),
	}

	auto, exists := annotations[autoAnnotation]
	if exists && auto == "true" {
		parsed.auto = true
	}

	search, exists := annotations[searchAnnotation]
	if exists && search == "true" {
		parsed.search = true
	}

	list, exists := annotations[secretListAnnotation]
	if exists {
		secrets := strings.Split(list, ",")
		for _, secret := range secrets {
			secret = strings.TrimSpace(secret)
			parsed.list[secret] = struct{}{}
		}
	}

	return parsed
}
