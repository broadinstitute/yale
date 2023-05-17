package slack

import "github.com/slack-go/slack"

// slackClient is an interface for sending messages via slack webhooks
// it exists to allow for mocking in tests
type slackClient interface {
	PostWebhook(message *slack.WebhookMessage) error
}

func newSlackClient(webhookUrl string) slackClient {
	return realClient{webhookUrl: webhookUrl}
}

type realClient struct {
	webhookUrl string
}

func (r realClient) PostWebhook(message *slack.WebhookMessage) error {
	return slack.PostWebhook(r.webhookUrl, message)
}
