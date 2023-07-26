package mocks

//go:generate mockery --with-expecter --dir=.. --name=YaleCRDInterface --output=. --outpkg=mocks --filename=crd.go
//go:generate mockery --with-expecter --dir=.. --name=GcpSaKeyInterface --output=. --outpkg=mocks --filename=gcpsakey.go
//go:generate mockery --with-expecter --dir=.. --name=AzureClientSecretInterface --output=. --outpkg=mocks --filename=azureClientSecret.go
