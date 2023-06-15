package azurekeyops

import (
	"github.com/broadinstitute/yale/internal/yale/keyops"
	"github.com/manicminer/hamilton/msgraph"
)

type azKeyOps struct {
	applicationsClient *msgraph.ApplicationsClient
}

func New(applicationsClient *msgraph.ApplicationsClient) keyops.KeyOps {
	return &azKeyOps{applicationsClient: applicationsClient}
}

func (a *azKeyOps) Create(appId string, keyName string) (keyops.Key, []byte, error) {
	panic("implement me")
}

func (a *azKeyOps) IsDisabled(key keyops.Key) (bool, error) {
	panic("implement me")
}

func (a *azKeyOps) EnsureDisabled(key keyops.Key) error {
	panic("implement me")
}

func (a *azKeyOps) DeleteIfDisabled(key keyops.Key) error {
	panic("implement me")
}
