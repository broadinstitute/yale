package v1beta1

import (
	"encoding/json"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
	"testing"
)

func Test_VaultReplicationFormatSerialization(t *testing.T) {
	testCases := []struct {
		str string
		fmt VaultReplicationFormat
		err bool
	}{
		{
			str: "map",
			fmt: Map,
		},
		{
			str: "json",
			fmt: JSON,
		},
		{
			str: "base64",
			fmt: Base64,
		},
		{
			str: "pem",
			fmt: PEM,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.str, func(t *testing.T) {
			yamlFormattedBytes, err := yaml.Marshal(tc.str)
			require.NoError(t, err)
			yamlFormatted := string(yamlFormattedBytes)
			jsonFormattedBytes, err := json.Marshal(tc.str)
			require.NoError(t, err)
			jsonFormatted := string(jsonFormattedBytes)

			// test serialization to yaml and json
			var serialized []byte

			serialized, err = yaml.Marshal(tc.fmt)
			require.NoError(t, err)
			assert.Equal(t, yamlFormatted, string(serialized))

			serialized, err = json.Marshal(tc.fmt)
			require.NoError(t, err)
			assert.Equal(t, jsonFormatted, string(serialized))

			// test deserialization from yaml and json
			var f VaultReplicationFormat

			err = yaml.Unmarshal([]byte(yamlFormatted), &f)
			require.NoError(t, err)
			assert.Equal(t, tc.fmt, f)

			err = json.Unmarshal([]byte(jsonFormatted), &f)
			require.NoError(t, err)
			assert.Equal(t, tc.fmt, f)
		})
	}
}

func Test_VaultReplicationSerialization(t *testing.T) {
	v := VaultReplication{
		Format: PEM,
		Key:    "bar",
		Path:   "/secret/foo",
	}

	var err error
	var actual VaultReplication

	// test round-trip yaml serialization and deserialization
	asYaml, err := yaml.Marshal(v)
	require.NoError(t, err)
	err = yaml.Unmarshal(asYaml, &actual)
	require.NoError(t, err)
	assert.Equal(t, v, actual)

	// test round-trip json serialization and deserialization
	asJson, err := json.Marshal(v)
	require.NoError(t, err)
	err = json.Unmarshal(asJson, &actual)
	require.NoError(t, err)
	assert.Equal(t, v, actual)
}
