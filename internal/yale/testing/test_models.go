package yale

import (
	"encoding/base64"
	"github.com/broadinstitute/yale/internal/yale/crd/api/v1beta1"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/iam/v1"
	"google.golang.org/api/policyanalyzer/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const FAKE_JSON_KEY = `{"private_key":"fake-sakey"}`
const OLD_KEY_NAME = "projects/my-fake-project/serviceAccounts/my-sa@blah.com/e0b1b971487ffff7f725b124d"

var FAKE_PEM = "fake-sakey"

const NEW_JSON_KEY = `{"private_key": "newPrivateKeyData"}`

var NEW_FAKE_PEM = "newPrivateKeyData"

var OLD_SECRET = corev1.Secret{
	TypeMeta: metav1.TypeMeta{},
	ObjectMeta: metav1.ObjectMeta{
		Name:      "my-fake-secret",
		Namespace: "my-fake-namespace",
		UID:       "FakeUId",
		Annotations: map[string]string{
			"validAfterDate":        "2022-04-08T14:21:44Z",
			"serviceAccountName":    "my-sa@blah.com",
			"serviceAccountKeyName": OLD_KEY_NAME,
		},
	},
	Data: map[string][]byte{
		"agora.pem":  []byte(FAKE_PEM),
		"agora.json": []byte(FAKE_JSON_KEY),
	},
}
var CRD = v1beta1.GCPSaKey{
	ObjectMeta: metav1.ObjectMeta{
		Name:      "my-gcp-sa-key",
		Namespace: "my-fake-namespace",
	},
	Spec: v1beta1.GCPSaKeySpec{
		GoogleServiceAccount: v1beta1.GoogleServiceAccount{
			Name:    "my-sa@blah.com",
			Project: "my-fake-project",
		},
		Secret: v1beta1.Secret{
			Name:        "my-fake-secret",
			PemKeyName:  "agora.pem",
			JsonKeyName: "agora.json",
		},
		KeyRotation: v1beta1.KeyRotation{
			DisableAfter: 14,
			DeleteAfter:  7,
		},
	},
}

var activityResponse = policyanalyzer.GoogleCloudPolicyanalyzerV1QueryActivityResponse{
	Activities: []*policyanalyzer.GoogleCloudPolicyanalyzerV1Activity{
		{
			Activity:     googleapi.RawMessage(activity),
			ActivityType: "serviceAccountKeyLastAuthentication",
		}},
}

var activity = `{"lastAuthenticatedTime":"2021-04-18T07:00:00Z","serviceAccountKey":{"serviceAccountId":"108004111716625043518","projectNumber":"635957978953","fullResourceName":"//iam.googleapis.com/projects/my-fake-project/serviceAccounts/my-sa@blah.com/keys/e0b1b971487ffff7f725b124d"}}`

const keyName = "my-sa@blah.com/keys/e0b1b971487ffff7f725b124d"

var saKey = iam.ServiceAccountKey{
	Disabled:       true,
	Name:           OLD_KEY_NAME,
	PrivateKeyData: base64.StdEncoding.EncodeToString([]byte(FAKE_JSON_KEY)),
	ValidAfterTime: "2014-04-08T14:21:44Z",
	ServerResponse: googleapi.ServerResponse{},
}

var newSecret = corev1.Secret{
	ObjectMeta: metav1.ObjectMeta{
		Name:      "my-fake-secret",
		Namespace: "my-fake-namespace",
		UID:       "FakeUId",
		Annotations: map[string]string{
			"serviceAccountKeyName":    keyName,
			"validAfterTime":           "2021-04-08T14:21:44Z",
			"oldServiceAccountKeyName": OLD_KEY_NAME,
		},
	},
	Data: map[string][]byte{
		"agora.pem":  []byte(NEW_FAKE_PEM),
		"agora.json": []byte(NEW_JSON_KEY),
	},
}
