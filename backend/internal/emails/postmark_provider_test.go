package emails

import (
	"context"
	"strings"
	"testing"
)

func TestPostmarkRecipients(t *testing.T) {
	tests := []struct {
		name    string
		to      []string
		want    string
		wantErr bool
	}{
		{name: "comma list", to: []string{"a@x.com, b@y.com"}, wantErr: true},
		{name: "crlf injection", to: []string{"a@x.com\r\nBcc: c@z.com"}, wantErr: true},
		{name: "semicolon list", to: []string{"a@x.com; b@y.com"}, wantErr: true},
		{name: "display name", to: []string{"Name <a@x.com>"}, want: "a@x.com"},
		{name: "two entries", to: []string{"a@x.com", "b@y.com"}, want: "a@x.com,b@y.com"},
		{name: "empty", to: nil, wantErr: true},
		{name: "empty slice", to: []string{}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := postmarkRecipients(tt.to)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				if !strings.Contains(err.Error(), "postmark: invalid recipient:") {
					t.Fatalf("error = %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("postmarkRecipients: %v", err)
			}
			if got != tt.want {
				t.Fatalf("postmarkRecipients = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestPostmarkSendWithReceiptRejectsCommaListWithoutNetwork(t *testing.T) {
	p := NewPostmarkProvider("tok")
	_, err := p.SendWithReceipt(context.Background(), EmailMessage{
		From:    "noreply@payverge.io",
		To:      []string{"a@x.com, b@y.com"},
		Subject: "hi",
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "postmark: invalid recipient:") {
		t.Fatalf("error = %v", err)
	}
}
