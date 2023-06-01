package main

import (
	"fmt"
	"github.com/broadinstitute/yale/internal/tools/linter"
	"github.com/spf13/cobra"
	"os"
)

func main() {
	cmd := &cobra.Command{
		Use:   "linter [path/to/manifests] [path/to/manifests] ...",
		Short: "A linter for K8s resources that use Yale",
		Long: `
The Yale linter searches directories of K8s manifests for Deployments 
and Statefulsets that depend on Yale-managed secrets.

The linter will verify that each Deployment or StatefulSet has one of the
following annotations:

    reloader.stakater.com/auto: "true"
    reloader.stakater.com/search: "true"
    secret.reloader.stakater.com/reload: "<name-of-the-Yale-secret>"

This is important because if a Deployment or StatefulSet is not restarted
after a key rotation, Yale could end up disabling a key that is still in use,
causing an outage.

Note: If running the linter on a bulk Thelma render with manifests for multiple environments
(produced by, say, thelma render -e ALL -r ALL), be sure to pass in each environment's
directory as a separate CLI argument. For example:

    ./linter ${THELMA_HOME}/output/dev ${THELMA_HOME}/output/alpha ${THELMA_HOME}/output/staging ...

This is easy to do with globbing:

    ./linter ${THELMA_HOME}/output/*

Otherwise, the linter will confuse which resources belong to which environment.
`,
	}

	cmd.ArgAliases = []string{"path/to/manifests"}

	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		return linter.Run(args)
	}

	if err := cmd.Execute(); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
}
