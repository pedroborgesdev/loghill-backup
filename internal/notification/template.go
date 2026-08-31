package notification

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"html/template"
	"net/url"
	"regexp"
	"strings"
	texttemplate "text/template"
	"time"

	"logtheater/internal/domain"
)

type Template struct {
	publicURL string
}

type templateData struct {
	AlertName, SenderName, SenderID, SenderStatus                 string
	Severity, Message, Timestamp, SentAt, Metadata, Link, LogoURL string
	Kicker, Headline, Description, ButtonLabel                    string
	SeverityColor, SeverityBackground, SeverityBorder             string
	Test, ProviderTest, HasMetadata                               bool
}

type severityTheme struct {
	Headline, Subject, Color, Background, Border string
}

func NewTemplate(publicURL string) *Template {
	return &Template{publicURL: strings.TrimRight(publicURL, "/")}
}

func (t *Template) Render(value domain.Notification) (domain.EmailMessage, error) {
	if value.SourceType == domain.NotificationSourceEvent || value.Event.ID != "" {
		return t.renderEvent(value)
	}
	metadata, err := json.MarshalIndent(value.Entry.Metadata, "", "  ")
	if err != nil {
		return domain.EmailMessage{}, err
	}
	theme := themeFor(value.Entry.Severity)
	link := t.publicURL + "/senders/" + url.PathEscape(value.Sender.ID) + "?severity=" + url.QueryEscape(string(value.Entry.Severity))
	description := fmt.Sprintf("Rule “%s” found a new record that requires your attention.", value.Alert.Name)
	kicker := "LOG ALERT"
	if value.Test {
		kicker = "ALERT TEST"
		description = fmt.Sprintf("This is a test delivery for rule “%s”. No real occurrence was recorded.", value.Alert.Name)
	}
	data := templateData{
		AlertName: value.Alert.Name, SenderName: value.Sender.Name, SenderID: value.Sender.ID,
		SenderStatus: statusLabel(value.Sender.Status), Severity: string(value.Entry.Severity),
		Message: value.Entry.Message, Timestamp: formatEmailTime(value.Entry.Timestamp), SentAt: formatEmailTime(time.Now()),
		Metadata: string(metadata), Link: link, LogoURL: t.publicURL + "/loghill.png",
		Kicker: kicker, Headline: theme.Headline, Description: description, ButtonLabel: "Open sender logs",
		SeverityColor: theme.Color, SeverityBackground: theme.Background, SeverityBorder: theme.Border,
		Test: value.Test, HasMetadata: len(value.Entry.Metadata) > 0,
	}
	subjectPrefix := "LogHill"
	if value.Test {
		subjectPrefix = "LogHill test"
	}
	subject := cleanSubject(fmt.Sprintf("%s — %s em %s: %s", subjectPrefix, theme.Subject, data.SenderName, data.Message))
	return renderMessage(value.Alert.Recipients, subject, data)
}

var eventPlaceholderPattern = regexp.MustCompile(`{{\s*([^{}]+?)\s*}}`)

func (t *Template) renderEvent(value domain.Notification) (domain.EmailMessage, error) {
	metadata, err := json.MarshalIndent(value.Entry.Metadata, "", "  ")
	if err != nil {
		return domain.EmailMessage{}, err
	}
	messageText := renderEventValue(value.Event.MessageTemplate, value, t.publicURL)
	subject := cleanSubject(renderEventValue(value.Event.SubjectTemplate, value, t.publicURL))
	link := t.publicURL + "/senders/" + url.PathEscape(value.Sender.ID) + "?event_key=" + url.QueryEscape(value.Event.Key)
	theme := themeFor(value.Entry.Severity)
	description := fmt.Sprintf("Event “%s” was explicitly reported by the sender.", value.Event.Name)
	kicker := "LOG EVENT"
	if value.Test {
		kicker = "EVENT TEST"
		description = fmt.Sprintf("This is a test for event “%s”. No real occurrence was recorded.", value.Event.Name)
	}
	data := templateData{
		AlertName: value.Event.Name, SenderName: value.Sender.Name, SenderID: value.Sender.ID,
		SenderStatus: statusLabel(value.Sender.Status), Severity: string(value.Entry.Severity),
		Message: messageText, Timestamp: formatEmailTime(value.Entry.Timestamp), SentAt: formatEmailTime(time.Now()),
		Metadata: string(metadata), Link: link, LogoURL: t.publicURL + "/loghill.png",
		Kicker: kicker, Headline: value.Event.Name, Description: description, ButtonLabel: "Open sender logs",
		SeverityColor: theme.Color, SeverityBackground: theme.Background, SeverityBorder: theme.Border,
		Test: value.Test, HasMetadata: len(value.Entry.Metadata) > 0,
	}
	recipients := value.Event.Recipients
	if len(value.Recipients) > 0 {
		recipients = value.Recipients
	}
	return renderMessage(recipients, subject, data)
}

func renderEventValue(templateValue string, value domain.Notification, publicURL string) string {
	return eventPlaceholderPattern.ReplaceAllStringFunc(templateValue, func(token string) string {
		match := eventPlaceholderPattern.FindStringSubmatch(token)
		if len(match) != 2 {
			return ""
		}
		name := strings.TrimSpace(match[1])
		switch name {
		case "rule.name":
			return value.Event.Name
		case "event.key":
			return value.Event.Key
		case "event.name":
			return value.Event.Name
		case "sender.id":
			return value.Sender.ID
		case "sender.name":
			return value.Sender.Name
		case "sender.status":
			return statusLabel(value.Sender.Status)
		case "log.message":
			return value.Entry.Message
		case "log.severity":
			return string(value.Entry.Severity)
		case "log.timestamp":
			return value.Entry.Timestamp.Format(time.RFC3339)
		case "app.public_url":
			return publicURL
		}
		if strings.HasPrefix(name, "metadata.") {
			key := strings.TrimPrefix(name, "metadata.")
			if metadataValue, exists := value.Entry.Metadata[key]; exists && metadataValue != nil {
				return fmt.Sprint(metadataValue)
			}
		}
		return ""
	})
}

func (t *Template) RenderProviderTest(recipient string, provider domain.EmailProviderType) (domain.EmailMessage, error) {
	now := formatEmailTime(time.Now())
	providerName := "Microsoft 365 / Outlook"
	integrationName := "Microsoft Graph"
	message := "This message confirms that LogHill successfully authenticated and sent email through Microsoft 365."
	description := "The LogHill integration with Outlook is working and ready to deliver your alerts."
	if provider == domain.EmailProviderGmail {
		providerName = "Gmail"
		integrationName = "SMTP with STARTTLS"
		message = "This message confirms that LogHill successfully authenticated and sent email through Gmail via SMTP."
		description = "The LogHill integration with Gmail is working and ready to deliver your alerts."
	}
	data := templateData{
		AlertName: "Configuration de e-mail", SenderName: providerName, SenderID: integrationName,
		SenderStatus: "Configuration validada", Severity: "SUCCESS",
		Message:   message,
		Timestamp: now, SentAt: now, Link: t.publicURL, LogoURL: t.publicURL + "/loghill.png",
		Kicker: "EMAIL TEST", Headline: "Your email is ready",
		Description: description,
		ButtonLabel: "Open LogHill", SeverityColor: "#34d399", SeverityBackground: "#052e2b", SeverityBorder: "#065f46",
		Test: true, ProviderTest: true,
	}
	return renderMessage([]string{recipient}, "LogHill email test — configuration complete", data)
}

func renderMessage(recipients []string, subject string, data templateData) (domain.EmailMessage, error) {
	var htmlBody bytes.Buffer
	if err := template.Must(template.New("email").Parse(htmlTemplateSource)).Execute(&htmlBody, data); err != nil {
		return domain.EmailMessage{}, err
	}
	var textBody bytes.Buffer
	if err := texttemplate.Must(texttemplate.New("email").Parse(textTemplateSource)).Execute(&textBody, data); err != nil {
		return domain.EmailMessage{}, err
	}
	return domain.EmailMessage{To: append([]string(nil), recipients...), Subject: subject, Text: textBody.String(), HTML: htmlBody.String()}, nil
}

func cleanSubject(value string) string {
	value = strings.Join(strings.Fields(strings.ReplaceAll(strings.ReplaceAll(value, "\r", " "), "\n", " ")), " ")
	runes := []rune(value)
	if len(runes) > 200 {
		value = string(runes[:197]) + "..."
	}
	return value
}

func formatEmailTime(value time.Time) string {
	return value.Format("01/02/2006 03:04:05 PM MST")
}

func statusLabel(status domain.SenderStatus) string {
	switch status {
	case domain.StatusOnline:
		return "Online"
	case domain.StatusInactive:
		return "Inactive"
	case domain.StatusArchived:
		return "Arquivado"
	case domain.StatusExpired:
		return "Expired"
	default:
		return string(status)
	}
}

func themeFor(severity domain.LogSeverity) severityTheme {
	switch severity {
	case domain.Undefined:
		return severityTheme{Headline: "New system log recorded", Subject: "System log recorded", Color: "#a1a1aa", Background: "#18181b", Border: "#3f3f46"}
	case domain.Trace:
		return severityTheme{Headline: "New trace recorded", Subject: "Rastreamento registrado", Color: "#d4d4d8", Background: "#27272a", Border: "#52525b"}
	case domain.Debug:
		return severityTheme{Headline: "New diagnostics available", Subject: "Diagnostics recorded", Color: "#c4b5fd", Background: "#2e1065", Border: "#5b21b6"}
	case domain.Info:
		return severityTheme{Headline: "New information recorded", Subject: "Information recorded", Color: "#7dd3fc", Background: "#082f49", Border: "#075985"}
	case domain.Warn:
		return severityTheme{Headline: "An event requires attention", Subject: "Attention required", Color: "#fbbf24", Background: "#451a03", Border: "#92400e"}
	case domain.Fatal:
		return severityTheme{Headline: "A critical failure was detected", Subject: "Critical failure detected", Color: "#fda4af", Background: "#4c0519", Border: "#9f1239"}
	default:
		return severityTheme{Headline: "An error was detected", Subject: "Erro detectado", Color: "#fca5a5", Background: "#450a0a", Border: "#991b1b"}
	}
}

//go:embed templates/email.html
var htmlTemplateSource string

//go:embed templates/email.txt
var textTemplateSource string
