package main

import (
	"fmt"
	"github.com/broadinstitute/yale/internal/tools/linter"
	"github.com/spf13/cobra"
	"os"
)

func main() {
	cmd := &cobra.Command{
		Use:   "linter [path/to/manifests]",
		Short: "A linter for K8s resources that use Yale",
		Long: `
The Yale linter searches a directory of K8s manifests for Deployments 
and Statefulsets that depend on Yale-managed secrets.

The linter will verify that each Deployment or StatefulSet has one of the
following annotations:

    reloader.stakater.com/auto: "true"
    reloader.stakater.com/search: "true"
    secret.reloader.stakater.com/reload: "<name-of-the-Yale-secret>"

This is important because if a Deployment or StatefulSet is not restarted
after a key rotation, Yale could end up disabling a key that is still in use,
causing an outage.
`,
	}

	cmd.Args = cobra.ExactArgs(1)
	cmd.ArgAliases = []string{"path/to/manifests"}

	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		dir := args[0]
		return linter.Run(dir)
	}

	if err := cmd.Execute(); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
}
