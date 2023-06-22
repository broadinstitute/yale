package msgraphmock

import (
	"context"

	"github.com/hashicorp/go-azure-sdk/sdk/odata"
	"github.com/manicminer/hamilton/msgraph"
)

type Expect interface {
	AddPassword(ctx context.Context, applicationId string, passwordCredential msgraph.PasswordCredential) (*msgraph.PasswordCredential, int, error)
	Get(ctx context.Context, applicationId string, query odata.Query) (*msgraph.Application, int, error)
	RemovePassword(ctx context.Context, applicationId, keyId string) (int, error)
}

func newExpect() *expect {
	return &expect{}
}

type expect struct {
	requests []Request
}
