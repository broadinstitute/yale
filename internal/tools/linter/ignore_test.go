package linter

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func Test_parseIgnore(t *testing.T) {
	var i ignoreCfg

	i = parseIgnoreAnnotations(map[string]string{})
	assert.False(t, i.all)
	assert.Empty(t, i.secrets)
	assert.False(t, i.ignoresSecret("foo"))

	i = parseIgnoreAnnotations(map[string]string{
		"yale.terra.bio/linter-ignore": "all",
	})
	assert.True(t, i.all)
	assert.Empty(t, i.secrets)
	assert.True(t, i.ignoresSecret("foo"))

	i = parseIgnoreAnnotations(map[string]string{
		"yale.terra.bio/linter-ignore": "baz, all ,bar",
	})
	assert.True(t, i.all)
	assert.Equal(t, 2, len(i.secrets))
	assert.True(t, i.ignoresSecret("foo"))

	i = parseIgnoreAnnotations(map[string]string{
		"yale.terra.bio/linter-ignore": "baz, foo ,bar",
	})
	assert.False(t, i.all)
	assert.Equal(t, 3, len(i.secrets))
	assert.True(t, i.ignoresSecret("foo"))
}
