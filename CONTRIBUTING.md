# CONTRIBUTING

> **_NOTE:_**
> For compliance reasons, all pull requests must be submitted with a Jira ID as a part of the pull
> request.
>
> You should include the Jira ID near the beginning of the title for better readability.
>
> For example:
> `[XX-1234]: add statement to CONTRIBUTING.md about including Jira IDs in PR titles`
>
> If there is more than one relevant ticket, include all associated Jira IDs.
>
> For example:
> `[DDO-1997] [DDO-2002] [DDO-2005]: fix for many bugs with the same root cause`
>

This section describes the steps necessary to finish setting up and contributing to this repo. If
you would like more information about the design and reasons "why" this repo is structured like it
is, please continue reading in the [DESIGN.md](./DESIGN.md)-document located in this directory.

This document assumes you've completed the steps located
in [Environment setup](./README.md#environment-setup) for how to `setup` and `run` the service.
If you haven't completed those steps yet, please go back and make sure you can successfully run the
service from your device.

If you have questions or run into issues, please see
the [Frequently Asked Questions](#frequently-asked-questions-faq) section in this document. If your
question is not answered there, additional help resources can be found at the bottom of this
document.

## Developer Convenience Scripts

To help accelerate interacting with this repo,
there are a series of scripts available in the `./scripts` directory.

Each script begins with a self documenting `usage()` function, run `./scripts/name -h` to print usage

- `./scripts/setup` will help you get your local environment setup for working on this codebase
- `./scripts/build` will build artifacts for you, either a compiled binary or docker image
- `./scripts/run` can be used to either execute the full test suite or run the application on your local environment

see the [./scripts/README.md](./scripts/README.md).

### Runtime Flags

These flags are only useful when running in local mode
```
Usage of yale:
  -kubeconfig string
    	(optional) absolute path to kubectl config (default "~/.kube/config")
  -local
    	use this flag when running locally (outside of cluster to use local kube config
```

### Environment variables

`YALE_DEBUG_ENABLED`: set to `true` to enable debug logging

### Running Yale in Local Mode

When Developing Yale, the easiest way to test it out locally is to run it with the `-local` runtime flag set.
The `run script` will do this for you automatically. The `setup` script will spin up a `kind` "KubenetesInDocker" cluster
for you which Yale will run against. In this execution mode yale will run as a process on your local machine but use your kubectl context
to perform it's logic against the kind cluster created by the setup script.

You can manually add example yale crds from `/examples` to your local kind cluster with `kubectl apply -f <example.yaml>`

When running in local mode yale will use your local google, azure, and vault credentials to initialize it's clients.

## Developing

[GoLand](https://www.jetbrains.com/go/) Is the recommended IDE for working on this project.
Just opening this repo within goland should be sufficient to get you setup.

An alternative option is to use Vs Code with the [go extension](https://code.visualstudio.com/docs/languages/go)

## Testing

Yale includes a comprehensive unit and integration test suite. `./scripts/run tests` will run both in parallel.
Generally there is not much need to separate them as the entire suite completes in a matter of seconds.
The integration tests are in `internal/yale/yale_test.go` The unit tests are spread throughout the code base in

`*_test.go` files as per common convention when working with go.

## Mocking
The test suites leverage auto generated mocks from the `mockery` tool to mock requests to external dependencies.

If you update one of the packages containing a mocked interface run `mockery` within that directory to regenerate the mocks.

## Frequently Asked Questions (FAQ)

If you have feature suggestions or are interested in contributing to Yale and are not a member of the DSP DevOps team.
Then reach out to us about it on slack in #dsp-devops-discussions.

(build out this section based on questions asked of the team)
