The mocks in this directory were generated using [mockery 2](https://github.com/mockery/mockery), run from the parent (`v1` package):

```
mockery --dir . --name GcpSaKeyInterface --filename gcpsakey.go
mockery --dir . --name YaleCrdV1Interface --filename crd.go
```