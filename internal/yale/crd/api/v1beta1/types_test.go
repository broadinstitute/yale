package v1beta1

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func Test_ReplicationFormatSerialization(t *testing.T) {
	testCases := []struct {
		str string
		fmt ReplicationFormat
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
		{
			str: "plaintext",
			fmt: PlainText,
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
			var f ReplicationFormat

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

func Test_GoogleSecretManagerReplicationSerialization(t *testing.T) {
	v := GoogleSecretManagerReplication{
		Secret:  "foo",
		Project: "my-project",
		Format:  PEM,
		Key:     "bar",
	}

	var err error
	var actual GoogleSecretManagerReplication

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

func Test_GitHubReplicationSerialization(t *testing.T) {
	v := GitHubReplication{
		Secret: "MY_SECRET",
		Repo:   "my-org/my-repo",
		Format: JSON,
	}

	var err error
	var actual GitHubReplication

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
