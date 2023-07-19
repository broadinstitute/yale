package slack

import (
	"fmt"

	"github.com/broadinstitute/yale/internal/yale/cache"
	"github.com/slack-go/slack"
)

const okColor = "#32a852"
const errorColor = "#a32f2f"

type event int64

const (
	keyIssuedEvent event = iota
	keyDisabledEvent
	keyDeletedEvent
	errorEvent
)

type SlackNotifier interface {
	// Error reports an error message via Slack webhook
	Error(entry *cache.Entry, message string) error
	// KeyIssued reports a key issued event via Slack webhook
	KeyIssued(entry *cache.Entry, id string) error
	// KeyDisabled reports a key issued event via Slack webhook
	KeyDisabled(entry *cache.Entry, id string) error
	// KeyDeleted reports a key deleted event via Slack webhook
	KeyDeleted(entry *cache.Entry, id string) error
}

func New(webhookUrl string) SlackNotifier {
	return &slackNotifier{
		client: newSlackClient(webhookUrl),
	}
}

type slackNotifier struct {
	client slackClient
}

func (s *slackNotifier) KeyIssued(entry *cache.Entry, id string) error {
	return s.buildAndSendMessage(keyIssuedEvent, entry, keyIdField(id))
}

func (s *slackNotifier) KeyDisabled(entry *cache.Entry, id string) error {
	return s.buildAndSendMessage(keyDisabledEvent, entry, keyIdField(id))
}

func (s *slackNotifier) KeyDeleted(entry *cache.Entry, id string) error {
	return s.buildAndSendMessage(keyDeletedEvent, entry, keyIdField(id))
}

func (s *slackNotifier) Error(entry *cache.Entry, message string) error {
	return s.buildAndSendMessage(errorEvent, entry, errorField(message))
}

// build a slack message to report an event
func (s *slackNotifier) buildAndSendMessage(evt event, entry *cache.Entry, fields map[string]string) error {
	attachment := slack.Attachment{}
	if evt == errorEvent {
		attachment.Color = errorColor
	} else {
		attachment.Color = okColor
	}

	linker := serviceAccountLinker{entry: entry}
	attachment.TitleLink = linker.url()

	switch evt {
	case keyIssuedEvent:
		attachment.Title = fmt.Sprintf("%s Issued", entry.Type)
		attachment.Text = fmt.Sprintf("A new %s was issued in `%s`", linker.hyperlink(), entry.Scope())
	case keyDisabledEvent:
		attachment.Title = fmt.Sprintf("%s Disabled", entry.Type)
		attachment.Text = fmt.Sprintf("A %s was disabled in `%s`", linker.hyperlink(), entry.Scope())
	case keyDeletedEvent:
		attachment.Title = fmt.Sprintf("%s Deleted", entry.Type)
		attachment.Text = fmt.Sprintf("A %s was deleted in `%s`", linker.hyperlink(), entry.Scope())
	case errorEvent:
		attachment.Title = "Error"
		attachment.Text = fmt.Sprintf("Error processing %s in `%s`", linker.hyperlink(), entry.Scope())
	}

	attachment.Fields = append(attachment.Fields, slack.AttachmentField{
		Title: "Email",
		Value: entry.Identify(),
		Short: false,
	})

	for name, value := range fields {
		attachment.Fields = append(attachment.Fields, slack.AttachmentField{
			Title: name,
			Value: value,
			Short: false,
		})
	}

	msg := slack.WebhookMessage{
		Attachments: []slack.Attachment{attachment},
	}

	err := s.client.PostWebhook(&msg)
	if err != nil {
		return fmt.Errorf("error sending slack notification: %v", err)
	}
	return nil
}

func keyIdField(id string) map[string]string {
	return map[string]string{
		"Key ID": "`" + id + "`",
	}
}

func errorField(message string) map[string]string {
	return map[string]string{
		"Error": message,
	}
}

type serviceAccountLinker struct {
	entry *cache.Entry
}

func (h serviceAccountLinker) url() string {
	return fmt.Sprintf("https://console.cloud.google.com/iam-admin/serviceaccounts/details/%s?project=%s", h.entry.Identify(), h.entry.Scope())
}

func (h serviceAccountLinker) hyperlink() string {
	return fmt.Sprintf("<%s|%s>", h.url(), h.entry.Type)
}
