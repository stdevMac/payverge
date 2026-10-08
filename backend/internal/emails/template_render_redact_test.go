package emails

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
)

const templateRenderEmailProbe = "guest.render.redact+test@example.com"

type templateRenderProbeProvider struct{}

func (templateRenderProbeProvider) Send(context.Context, EmailMessage) error {
	return nil
}

func TestTemplateRenderFailureLogDoesNotIncludeRawRecipient(t *testing.T) {
	server, err := NewEmailServer(
		templateRenderProbeProvider{},
		"noreply@example.com",
		"updates@example.com",
		resolveTemplatesRoot(t),
	)
	require.NoError(t, err)

	var buf bytes.Buffer
	prev := logrus.StandardLogger().Out
	logrus.SetOutput(&buf)
	t.Cleanup(func() { logrus.SetOutput(prev) })

	renderErr := server.sendEmailWithAttachmentsIdempotent(
		[]string{templateRenderEmailProbe},
		"noreply@example.com",
		"guest-render-probe",
		"does_not_exist_template_for_redact_probe",
		"en",
		map[string]interface{}{},
		MessageTypeTransactional,
		nil,
		"",
	)
	require.Error(t, renderErr)

	logs := buf.String()
	if !strings.Contains(logs, "Failed to render email template") {
		t.Fatalf("expected template-render failure log, got %q", logs)
	}
	if strings.Contains(logs, templateRenderEmailProbe) {
		t.Fatalf("template-render log leaked raw guest address %q:\n%s", templateRenderEmailProbe, logs)
	}
	if !strings.Contains(logs, "recipient_count") {
		t.Fatalf("template-render log must record recipient_count instead of the raw to slice; got %q", logs)
	}
}
