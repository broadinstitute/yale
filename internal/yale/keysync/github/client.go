package github

import (
	"context"
	"fmt"
	"github.com/broadinstitute/yale/internal/yale/logs"
	"github.com/google/go-github/v62/github"
)

func NewClient(c *github.Client) Client {
	return &client{
		github: c,
	}
}

type Client interface {
	WriteSecret(owner string, repo string, secretName string, content []byte) error
}

type client struct {
	github *github.Client
}

func (c *client) WriteSecret(owner string, repo string, secretName string, content []byte) error {
	pubkey, _, err := c.github.Actions.GetRepoPublicKey(context.Background(), owner, repo)
	if err != nil {
		return fmt.Errorf("error retrieving public key for %s/%s: %v", owner, repo, err)
	}

	encryptedSecret, err := Encrypt(*pubkey.Key, string(content))
	if err != nil {
		return fmt.Errorf("error encrypting secret for %s/%s: %v", owner, repo, err)
	}

	logs.Info.Printf("Writing to GitHub secret %s in repo %s/%s", secretName, owner, repo)

	_, err = c.github.Actions.CreateOrUpdateRepoSecret(context.Background(), owner, repo, &github.EncryptedSecret{
		Name:           secretName,
		KeyID:          *pubkey.KeyID,
		EncryptedValue: encryptedSecret,
	})
	if err != nil {
		return fmt.Errorf("error pushing encrypted GitHub secret %s to %s/%s: %v", secretName, owner, repo, err)
	}
	return nil
}
