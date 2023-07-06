package msgraphmock

import (
	"context"
	"fmt"
	"net/http"

	"github.com/hashicorp/go-azure-sdk/sdk/odata"
	"github.com/manicminer/hamilton/msgraph"
)

const msGraphUrl = "https://graph.microsoft.com/beta"

type Expect interface {
	AddPassword(ctx context.Context, applicationId string, passwordCredential msgraph.PasswordCredential) AddPasswordRequest
	Get(ctx context.Context, applicationId string, query odata.Query) GetApplicationRequest
	RemovePassword(ctx context.Context, applicationId, keyId string) RemovePasswordRequest
}

func newExpect() *expect {
	return &expect{}
}

type expect struct {
	requests []Request
}

func (e *expect) addNewRequest(r Request) {
	e.requests = append(e.requests, r)
}

func (e *expect) AddPassword(ctx context.Context, applicationId string, passwordCredential msgraph.PasswordCredential) AddPasswordRequest {
	url := fmt.Sprintf("%s/applications/%s/addPassword", msGraphUrl, applicationId)
	r := newAddPasswordRequest(http.MethodPost, url)
	r.With(passwordCredential)
	e.addNewRequest(r)
	return r
}

func (e *expect) Get(ctx context.Context, applicationId string, query odata.Query) GetApplicationRequest {
	url := fmt.Sprintf("%s/applications/%s", msGraphUrl, applicationId)
	r := newGetApplicationRequest(http.MethodGet, url)
	e.addNewRequest(r)
	return r
}

func (e *expect) RemovePassword(ctx context.Context, applicationId, keyId string) RemovePasswordRequest {
	url := fmt.Sprintf("%s/applications/%s/removePassword", msGraphUrl, applicationId)
	r := newRemovePasswordRequest(http.MethodPost, url)
	e.addNewRequest(r)
	return r
}
