package vault

import (
	"github.com/stretchr/testify/assert"
	"strings"
	"testing"
)

type Expect interface {
	HasSecret(path string, data map[string]interface{})
	assert(t *testing.T, serverState *state)
}

func newExpect() Expect {
	return &expect{
		secrets: make(map[string]map[string]interface{}),
	}
}

type expect struct {
	secrets map[string]map[string]interface{}
}

func (e *expect) HasSecret(path string, data map[string]interface{}) {
	e.secrets[path] = data
}

func (e *expect) assert(t *testing.T, serverState *state) {
	for path, expected := range e.secrets {
		path = strings.TrimPrefix(path, secretPrefix)
		actual, exists := serverState.secrets[path]
		if !assert.True(t, exists, "expected secret at path does not exist: %s", path) {
			return
		}
		assert.Equal(t, expected, actual, "secret at path does not have expected value: %s", path)
	}
}
