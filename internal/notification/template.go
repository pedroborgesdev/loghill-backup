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
	description := fmt.Sprintf("A regra “%s” encontrou um novo registro que merece sua atenção.", value.Alert.Name)
	kicker := "ALERTA DE LOG"
	if value.Test {
		kicker = "TESTE DE ALERTA"
		description = fmt.Sprintf("Este é um envio de teste da regra “%s”. Nenhuma ocorrência real foi registrada.", value.Alert.Name)
	}
	data := templateData{
		AlertName: value.Alert.Name, SenderName: value.Sender.Name, SenderID: value.Sender.ID,
		SenderStatus: statusLabel(value.Sender.Status), Severity: string(value.Entry.Severity),
		Message: value.Entry.Message, Timestamp: formatEmailTime(value.Entry.Timestamp), SentAt: formatEmailTime(time.Now()),
		Metadata: string(metadata), Link: link, LogoURL: t.publicURL + "/loghill.png",
		Kicker: kicker, Headline: theme.Headline, Description: description, ButtonLabel: "Abrir logs do sender",
		SeverityColor: theme.Color, SeverityBackground: theme.Background, SeverityBorder: theme.Border,
		Test: value.Test, HasMetadata: len(value.Entry.Metadata) > 0,
	}
	subjectPrefix := "LogHill"
	if value.Test {
		subjectPrefix = "Teste do LogHill"
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
	description := fmt.Sprintf("O evento “%s” foi informado explicitamente pelo sender.", value.Event.Name)
	kicker := "EVENTO DO LOG"
	if value.Test {
		kicker = "TESTE DE EVENTO"
		description = fmt.Sprintf("Este é um teste do evento “%s”. Nenhuma ocorrência real foi registrada.", value.Event.Name)
	}
	data := templateData{
		AlertName: value.Event.Name, SenderName: value.Sender.Name, SenderID: value.Sender.ID,
		SenderStatus: statusLabel(value.Sender.Status), Severity: string(value.Entry.Severity),
		Message: messageText, Timestamp: formatEmailTime(value.Entry.Timestamp), SentAt: formatEmailTime(time.Now()),
		Metadata: string(metadata), Link: link, LogoURL: t.publicURL + "/loghill.png",
		Kicker: kicker, Headline: value.Event.Name, Description: description, ButtonLabel: "Abrir logs do sender",
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
	message := "Esta mensagem confirma que o LogHill conseguiu autenticar e enviar e-mails pelo Microsoft 365."
	description := "A integração do LogHill com o Outlook está funcionando e pronta para entregar seus alertas."
	if provider == domain.EmailProviderGmail {
		providerName = "Gmail"
		integrationName = "SMTP com STARTTLS"
		message = "Esta mensagem confirma que o LogHill conseguiu autenticar e enviar e-mails pelo Gmail via SMTP."
		description = "A integração do LogHill com o Gmail está funcionando e pronta para entregar seus alertas."
	}
	data := templateData{
		AlertName: "Configuração de e-mail", SenderName: providerName, SenderID: integrationName,
		SenderStatus: "Configuração validada", Severity: "SUCESSO",
		Message:   message,
		Timestamp: now, SentAt: now, Link: t.publicURL, LogoURL: t.publicURL + "/loghill.png",
		Kicker: "TESTE DE E-MAIL", Headline: "Tudo certo com o seu e-mail",
		Description: description,
		ButtonLabel: "Abrir o LogHill", SeverityColor: "#34d399", SeverityBackground: "#052e2b", SeverityBorder: "#065f46",
		Test: true, ProviderTest: true,
	}
	return renderMessage([]string{recipient}, "Teste de e-mail do LogHill — configuração concluída", data)
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
	return value.Format("02/01/2006 às 15:04:05 MST")
}

func statusLabel(status domain.SenderStatus) string {
	switch status {
	case domain.StatusOnline:
		return "Online"
	case domain.StatusInactive:
		return "Inativo"
	case domain.StatusArchived:
		return "Arquivado"
	case domain.StatusExpired:
		return "Expirado"
	default:
		return string(status)
	}
}

func themeFor(severity domain.LogSeverity) severityTheme {
	switch severity {
	case domain.Undefined:
		return severityTheme{Headline: "Novo log do sistema registrado", Subject: "Log do sistema registrado", Color: "#a1a1aa", Background: "#18181b", Border: "#3f3f46"}
	case domain.Trace:
		return severityTheme{Headline: "Novo rastreamento registrado", Subject: "Rastreamento registrado", Color: "#d4d4d8", Background: "#27272a", Border: "#52525b"}
	case domain.Debug:
		return severityTheme{Headline: "Novo diagnóstico disponível", Subject: "Diagnóstico registrado", Color: "#c4b5fd", Background: "#2e1065", Border: "#5b21b6"}
	case domain.Info:
		return severityTheme{Headline: "Nova informação registrada", Subject: "Informação registrada", Color: "#7dd3fc", Background: "#082f49", Border: "#075985"}
	case domain.Warn:
		return severityTheme{Headline: "Um evento precisa de atenção", Subject: "Atenção necessária", Color: "#fbbf24", Background: "#451a03", Border: "#92400e"}
	case domain.Fatal:
		return severityTheme{Headline: "Uma falha crítica foi detectada", Subject: "Falha crítica detectada", Color: "#fda4af", Background: "#4c0519", Border: "#9f1239"}
	default:
		return severityTheme{Headline: "Um erro foi detectado", Subject: "Erro detectado", Color: "#fca5a5", Background: "#450a0a", Border: "#991b1b"}
	}
}

//go:embed templates/email.html
var htmlTemplateSource string

//go:embed templates/email.txt
var textTemplateSource string
