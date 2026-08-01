package notification

import (
	"bytes"
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

func (t *Template) RenderProviderTest(recipient string) (domain.EmailMessage, error) {
	now := formatEmailTime(time.Now())
	data := templateData{
		AlertName: "Configuração de e-mail", SenderName: "Microsoft 365 / Outlook", SenderID: "Microsoft Graph",
		SenderStatus: "Configuração validada", Severity: "SUCESSO",
		Message:   "Esta mensagem confirma que o LogHill conseguiu autenticar e enviar e-mails pelo Microsoft 365.",
		Timestamp: now, SentAt: now, Link: t.publicURL, LogoURL: t.publicURL + "/loghill.png",
		Kicker: "TESTE DE E-MAIL", Headline: "Tudo certo com o seu e-mail",
		Description: "A integração do LogHill com o Outlook está funcionando e pronta para entregar seus alertas.",
		ButtonLabel: "Abrir o LogHill", SeverityColor: "#34d399", SeverityBackground: "#052e2b", SeverityBorder: "#065f46",
		Test: true, ProviderTest: true,
	}
	return renderMessage([]string{recipient}, "Teste de e-mail do LogHill — configuração concluída", data)
}

func renderMessage(recipients []string, subject string, data templateData) (domain.EmailMessage, error) {
	var htmlBody bytes.Buffer
	if err := template.Must(template.New("email").Parse(htmlTemplate)).Execute(&htmlBody, data); err != nil {
		return domain.EmailMessage{}, err
	}
	var textBody bytes.Buffer
	if err := texttemplate.Must(texttemplate.New("email").Parse(textTemplate)).Execute(&textBody, data); err != nil {
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

const htmlTemplate = `
<!doctype html>
<html
  lang="pt-BR"
  xmlns:v="urn:schemas-microsoft-com:vml"
  xmlns:o="urn:schemas-microsoft-com:office:office"
>
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">

  <meta name="color-scheme" content="light">
  <meta name="supported-color-schemes" content="light">

  <title>{{.Headline}}</title>

  <!--[if mso]>
  <noscript>
    <xml>
      <o:OfficeDocumentSettings>
        <o:PixelsPerInch>96</o:PixelsPerInch>
      </o:OfficeDocumentSettings>
    </xml>
  </noscript>
  <![endif]-->

  <style>
    body,
    table,
    td,
    a {
      -webkit-text-size-adjust: 100%;
      -ms-text-size-adjust: 100%;
    }

    table,
    td {
      mso-table-lspace: 0;
      mso-table-rspace: 0;
    }

    table {
      border-collapse: collapse;
      border-spacing: 0;
    }

    img {
      display: block;
      border: 0;
      outline: none;
      text-decoration: none;
      -ms-interpolation-mode: bicubic;
    }

    a {
      text-decoration: none;
    }

    @media only screen and (max-width: 620px) {
      .email-shell {
        width: 100% !important;
      }

      .email-padding {
        padding: 0 !important;
      }

      .header-padding {
        padding: 17px 18px !important;
      }

      .email-card-padding {
        padding: 28px 20px !important;
      }

      .mobile-block {
        display: block !important;
        width: 100% !important;
      }

      .mobile-top {
        padding-top: 14px !important;
        text-align: left !important;
      }

      .mobile-top table {
        margin-left: 0 !important;
      }

      .info-label {
        width: 38% !important;
      }

      .button-wrap {
        width: 100% !important;
      }

      .button-link {
        display: block !important;
        width: auto !important;
        padding-left: 28px !important;
        padding-right: 28px !important;
      }

      .footer-column {
        display: block !important;
        width: 100% !important;
        text-align: left !important;
      }

      .footer-right {
        padding-top: 6px !important;
      }
    }
  </style>
</head>

<body
  style="
    margin:0;
    padding:0;
    width:100%;
    background-color:#ffffff;
    color:#27272a;
    font-family:Arial,'Segoe UI',Helvetica,sans-serif;
  "
>
  <!-- Pré-visualização -->
  <div
    style="
      display:none;
      max-height:0;
      overflow:hidden;
      opacity:0;
      color:transparent;
      mso-hide:all;
    "
  >
    {{.Headline}} — {{.SenderName}}
  </div>

  <!-- Fundo global -->
  <table
    role="presentation"
    width="100%"
    cellspacing="0"
    cellpadding="0"
    border="0"
    bgcolor="#ffffff"
    style="
      width:100%;
      margin:0;
      padding:0;
      border-collapse:collapse;
      border-spacing:0;
      background-color:#ffffff;
    "
  >
    <tr>
      <td
        align="center"
        valign="top"
        class="email-padding"
        bgcolor="#ffffff"
        style="
          margin:0;
          padding:14px;
          background-color:#ffffff;
        "
      >
        <!--[if mso]>
        <table role="presentation" width="680" cellspacing="0" cellpadding="0" border="0">
          <tr>
            <td>
        <![endif]-->

        <!-- Estrutura principal -->
        <table
          role="presentation"
          width="680"
          cellspacing="0"
          cellpadding="0"
          border="0"
          bgcolor="#111113"
          class="email-shell"
          style="
            width:100%;
            max-width:680px;
            margin:0;
            padding:0;
            border-collapse:collapse;
            border-spacing:0;
            background-color:#111113;
          "
        >
          <!-- Header -->
          <tr bgcolor="#111113" style="background-color:#111113;">
            <td
              bgcolor="#111113"
              class="header-padding"
              style="
                margin:0;
                padding:18px 20px;
                background-color:#111113;
                border-left:1px solid #27272a;
                border-right:1px solid #27272a;
                border-top:1px solid #27272a;
                border-bottom:3px solid {{.SeverityColor}};
              "
            >
              <table
                role="presentation"
                width="100%"
                cellspacing="0"
                cellpadding="0"
                border="0"
                bgcolor="#111113"
                style="
                  width:100%;
                  margin:0;
                  padding:0;
                  border-collapse:collapse;
                  background-color:#111113;
                "
              >
                <tr bgcolor="#111113" style="background-color:#111113;">
                  <td
                    valign="middle"
                    bgcolor="#111113"
                    style="background-color:#111113;"
                  >
                    <table
                      role="presentation"
                      cellspacing="0"
                      cellpadding="0"
                      border="0"
                      bgcolor="#111113"
                      style="
                        margin:0;
                        padding:0;
                        border-collapse:collapse;
                        background-color:#111113;
                      "
                    >
                      <tr bgcolor="#111113" style="background-color:#111113;">
                        <!-- Logo sem fundo -->
                        <td
                          width="46"
                          valign="middle"
                          bgcolor="#111113"
                          style="
                            width:46px;
                            padding:0;
                            background-color:#111113;
                          "
                        >
                          <img
                            src="{{.LogoURL}}"
                            width="40"
                            alt="LogHill"
                            style="
                              display:block;
                              width:40px;
                              height:auto;
                              margin:0;
                              padding:0;
                            "
                          >
                        </td>

                        <!-- Nome -->
                        <td
                          valign="middle"
                          bgcolor="#111113"
                          style="
                            padding:0 0 0 13px;
                            background-color:#111113;
                          "
                        >
                          <p
                            style="
                              margin:0;
                              padding:0;
                              font-family:Arial,'Segoe UI',Helvetica,sans-serif;
                              font-size:29px;
                              line-height:33px;
                              font-weight:bold;
                              color:#fafafa;
                            "
                          >
                            Log<span style="color:#f59e0b;">Hill</span>
                          </p>
                          <p
                            style="
                              margin:2px 0 0 0;
                              padding:0;
                              font-family:Arial,'Segoe UI',Helvetica,sans-serif;
                              font-size:11px;
                              line-height:15px;
                              color:#71717a;
                            "
                          >
                            Central de observabilidade
                          </p>
                        </td>
                      </tr>
                    </table>
                  </td>

                  <!-- Severidade -->
                  <td
                    align="right"
                    valign="middle"
                    bgcolor="#111113"
                    class="mobile-block mobile-top"
                    style="background-color:#111113;"
                  >
                    <table
                      role="presentation"
                      cellspacing="0"
                      cellpadding="0"
                      border="0"
                      align="right"
                      bgcolor="{{.SeverityBackground}}"
                      style="
                        margin:0;
                        padding:0;
                        border-collapse:collapse;
                        background-color:{{.SeverityBackground}};
                      "
                    >
                      <tr bgcolor="{{.SeverityBackground}}">
                        <td
                          align="center"
                          bgcolor="{{.SeverityBackground}}"
                          style="
                            padding:9px 14px;
                            background-color:{{.SeverityBackground}};
                            border:1px solid {{.SeverityBorder}};
                          "
                        >
                          <span
                            style="
                              font-family:Arial,'Segoe UI',Helvetica,sans-serif;
                              font-size:13px;
                              line-height:17px;
                              font-weight:bold;
                              letter-spacing:0.6px;
                              color:{{.SeverityColor}};
                            "
                          >
                            {{.Severity}}
                          </span>
                        </td>
                      </tr>
                    </table>
                  </td>
                </tr>
              </table>
            </td>
          </tr>

          <!-- Card principal -->
          <tr bgcolor="#111113" style="background-color:#111113;">
            <td
              bgcolor="#111113"
              style="
                margin:0;
                padding:0;
                background-color:#111113;
                border-left:1px solid #27272a;
                border-right:1px solid #27272a;
              "
            >
              <table
                role="presentation"
                width="100%"
                cellspacing="0"
                cellpadding="0"
                border="0"
                bgcolor="#111113"
                style="
                  width:100%;
                  margin:0;
                  padding:0;
                  border-collapse:collapse;
                  border-spacing:0;
                  background-color:#111113;
                "
              >
                <tr bgcolor="#111113" style="background-color:#111113;">
                  <td
                    bgcolor="#111113"
                    class="email-card-padding"
                    style="
                      padding:36px 34px 28px 34px;
                      background-color:#111113;
                    "
                  >
                    <!-- Categoria -->
                    <p
                      style="
                        margin:0;
                        padding:0;
                        font-family:Arial,'Segoe UI',Helvetica,sans-serif;
                        font-size:13px;
                        line-height:18px;
                        font-weight:bold;
                        letter-spacing:1.3px;
                        color:{{.SeverityColor}};
                      "
                    >
                      {{.Kicker}}
                    </p>

                    <!-- Título -->
                    <h1
                      style="
                        margin:11px 0 0 0;
                        padding:0;
                        font-family:Arial,'Segoe UI',Helvetica,sans-serif;
                        font-size:23px;
                        line-height:31px;
                        font-weight:bold;
                        color:#fafafa;
                      "
                    >
                      {{.Headline}}
                    </h1>

                    <!-- Descrição -->
                    <p
                      style="
                        margin:13px 0 0 0;
                        padding:0;
                        font-family:Arial,'Segoe UI',Helvetica,sans-serif;
                        font-size:15px;
                        line-height:23px;
                        color:#a1a1aa;
                      "
                    >
                      {{.Description}}
                    </p>

                    <!-- Mensagem -->
                    <table
                      role="presentation"
                      width="100%"
                      cellspacing="0"
                      cellpadding="0"
                      border="0"
                      bgcolor="#18181b"
                      style="
                        width:100%;
                        margin:22px 0 0 0;
                        padding:0;
                        border-collapse:collapse;
                        background-color:#18181b;
                      "
                    >
                      <tr bgcolor="#18181b" style="background-color:#18181b;">
                        <td
                          bgcolor="#18181b"
                          style="
                            padding:16px 17px;
                            background-color:#18181b;
                            border:1px solid #2f2f33;
                            border-left:3px solid {{.SeverityColor}};
                          "
                        >
                          <p
                            style="
                              margin:0;
                              padding:0;
                              font-family:Arial,'Segoe UI',Helvetica,sans-serif;
                              font-size:12px;
                              line-height:16px;
                              font-weight:bold;
                              letter-spacing:1px;
                              color:#8b8b94;
                            "
                          >
                            MENSAGEM
                          </p>

                          <div
                            style="
                              padding-top:9px;
                              font-family:Consolas,'Courier New',monospace;
                              font-size:13px;
                              line-height:20px;
                              color:#e4e4e7;
                              white-space:pre-wrap;
                              word-break:break-word;
                            "
                          >{{.Message}}</div>
                        </td>
                      </tr>
                    </table>

                    <!-- Informações -->
                    <table
                      role="presentation"
                      width="100%"
                      cellspacing="0"
                      cellpadding="0"
                      border="0"
                      bgcolor="#0c0c0e"
                      style="
                        width:100%;
                        margin:0;
                        padding:0;
                        border-collapse:collapse;
                        background-color:#0c0c0e;
                        border-left:1px solid #27272a;
                        border-right:1px solid #27272a;
                        border-bottom:1px solid #27272a;
                      "
                    >
                      <tr bgcolor="#0c0c0e" style="background-color:#0c0c0e;">
                        <td
                          width="34%"
                          valign="top"
                          bgcolor="#0c0c0e"
                          class="info-label"
                          style="
                            padding:15px 15px;
                            background-color:#0c0c0e;
                            border-bottom:1px solid #27272a;
                            font-family:Arial,'Segoe UI',Helvetica,sans-serif;
                            font-size:13px;
                            line-height:19px;
                            color:#8b8b94;
                          "
                        >
                          {{if .ProviderTest}}Provedor{{else}}Sender{{end}}
                        </td>

                        <td
                          valign="top"
                          bgcolor="#0c0c0e"
                          style="
                            padding:15px 15px;
                            background-color:#0c0c0e;
                            border-bottom:1px solid #27272a;
                            font-family:Arial,'Segoe UI',Helvetica,sans-serif;
                            font-size:14px;
                            line-height:20px;
                            font-weight:bold;
                            color:#e4e4e7;
                            word-break:break-word;
                          "
                        >
                          {{.SenderName}}
                        </td>
                      </tr>

                      <tr bgcolor="#0c0c0e" style="background-color:#0c0c0e;">
                        <td
                          width="34%"
                          valign="top"
                          bgcolor="#0c0c0e"
                          class="info-label"
                          style="
                            padding:15px 15px;
                            background-color:#0c0c0e;
                            border-bottom:1px solid #27272a;
                            font-family:Arial,'Segoe UI',Helvetica,sans-serif;
                            font-size:13px;
                            line-height:19px;
                            color:#8b8b94;
                          "
                        >
                          {{if .ProviderTest}}Integração{{else}}Identificador{{end}}
                        </td>

                        <td
                          valign="top"
                          bgcolor="#0c0c0e"
                          style="
                            padding:15px 15px;
                            background-color:#0c0c0e;
                            border-bottom:1px solid #27272a;
                            font-family:Consolas,'Courier New',monospace;
                            font-size:12px;
                            line-height:20px;
                            color:#d4d4d8;
                            word-break:break-all;
                          "
                        >
                          {{.SenderID}}
                        </td>
                      </tr>

                      <tr bgcolor="#0c0c0e" style="background-color:#0c0c0e;">
                        <td
                          width="34%"
                          valign="top"
                          bgcolor="#0c0c0e"
                          class="info-label"
                          style="
                            padding:15px 15px;
                            background-color:#0c0c0e;
                            border-bottom:1px solid #27272a;
                            font-family:Arial,'Segoe UI',Helvetica,sans-serif;
                            font-size:13px;
                            line-height:19px;
                            color:#8b8b94;
                          "
                        >
                          Status
                        </td>

                        <td
                          valign="top"
                          bgcolor="#0c0c0e"
                          style="
                            padding:15px 15px;
                            background-color:#0c0c0e;
                            border-bottom:1px solid #27272a;
                            font-family:Arial,'Segoe UI',Helvetica,sans-serif;
                            font-size:14px;
                            line-height:20px;
                            color:#d4d4d8;
                          "
                        >
                          {{.SenderStatus}}
                        </td>
                      </tr>

                      {{if not .ProviderTest}}
                      <tr bgcolor="#0c0c0e" style="background-color:#0c0c0e;">
                        <td
                          width="34%"
                          valign="top"
                          bgcolor="#0c0c0e"
                          class="info-label"
                          style="
                            padding:15px 15px;
                            background-color:#0c0c0e;
                            border-bottom:1px solid #27272a;
                            font-family:Arial,'Segoe UI',Helvetica,sans-serif;
                            font-size:13px;
                            line-height:19px;
                            color:#8b8b94;
                          "
                        >
                          Horário do log
                        </td>

                        <td
                          valign="top"
                          bgcolor="#0c0c0e"
                          style="
                            padding:15px 15px;
                            background-color:#0c0c0e;
                            border-bottom:1px solid #27272a;
                            font-family:Arial,'Segoe UI',Helvetica,sans-serif;
                            font-size:14px;
                            line-height:20px;
                            color:#d4d4d8;
                          "
                        >
                          {{.Timestamp}}
                        </td>
                      </tr>
                      {{end}}

                      <tr bgcolor="#0c0c0e" style="background-color:#0c0c0e;">
                        <td
                          width="34%"
                          valign="top"
                          bgcolor="#0c0c0e"
                          class="info-label"
                          style="
                            padding:15px 15px;
                            background-color:#0c0c0e;
                            font-family:Arial,'Segoe UI',Helvetica,sans-serif;
                            font-size:13px;
                            line-height:19px;
                            color:#8b8b94;
                          "
                        >
                          Enviado em
                        </td>

                        <td
                          valign="top"
                          bgcolor="#0c0c0e"
                          style="
                            padding:15px 15px;
                            background-color:#0c0c0e;
                            font-family:Arial,'Segoe UI',Helvetica,sans-serif;
                            font-size:14px;
                            line-height:20px;
                            color:#d4d4d8;
                          "
                        >
                          {{.SentAt}}
                        </td>
                      </tr>
                    </table>

                    <!-- Metadados -->
                    {{if .HasMetadata}}
                    <p
                      style="
                        margin:20px 0 0 0;
                        padding:0;
                        font-family:Arial,'Segoe UI',Helvetica,sans-serif;
                        font-size:12px;
                        line-height:16px;
                        font-weight:bold;
                        letter-spacing:1px;
                        color:#8b8b94;
                      "
                    >
                      METADADOS
                    </p>

                    <table
                      role="presentation"
                      width="100%"
                      cellspacing="0"
                      cellpadding="0"
                      border="0"
                      bgcolor="#0c0c0e"
                      style="
                        width:100%;
                        margin:8px 0 0 0;
                        padding:0;
                        border-collapse:collapse;
                        background-color:#0c0c0e;
                        border:1px solid #27272a;
                      "
                    >
                      <tr bgcolor="#0c0c0e" style="background-color:#0c0c0e;">
                        <td
                          bgcolor="#0c0c0e"
                          style="
                            padding:16px 16px;
                            background-color:#0c0c0e;
                            font-family:Consolas,'Courier New',monospace;
                            font-size:12px;
                            line-height:19px;
                            color:#b4b4bc;
                            white-space:pre-wrap;
                            word-break:break-word;
                          "
                        >{{.Metadata}}</td>
                      </tr>
                    </table>
                    {{end}}

                    <!-- Botão -->
                    <table
                      role="presentation"
                      width="100%"
                      cellspacing="0"
                      cellpadding="0"
                      border="0"
                      bgcolor="#111113"
                      style="
                        width:100%;
                        margin:24px 0 0 0;
                        padding:0;
                        border-collapse:collapse;
                        background-color:#111113;
                      "
                    >
                      <tr bgcolor="#111113" style="background-color:#111113;">
                        <td
                          align="center"
                          bgcolor="#111113"
                          style="
                            margin:0;
                            padding:0;
                            background-color:#111113;
                          "
                        >
                          <!--[if mso]>
                          <v:rect
                            xmlns:v="urn:schemas-microsoft-com:vml"
                            href="{{.Link}}"
                            style="height:50px;v-text-anchor:middle;width:360px;"
                            fillcolor="#f59e0b"
                            strokecolor="#f59e0b"
                            arcsize="0%"
                          >
                            <w:anchorlock/>
                            <center
                              style="
                                color:#18181b;
                                font-family:Arial,sans-serif;
                                font-size:14px;
                                font-weight:bold;
                              "
                            >
                              {{.ButtonLabel}} &nbsp;&rarr;
                            </center>
                          </v:rect>
                          <![endif]-->

                          <!--[if !mso]><!-->
                          <table
                            role="presentation"
                            width="360"
                            cellspacing="0"
                            cellpadding="0"
                            border="0"
                            bgcolor="#f59e0b"
                            class="button-wrap"
                            style="
                              width:360px;
                              margin:0 auto;
                              padding:0;
                              border-collapse:collapse;
                              background-color:#f59e0b;
                            "
                          >
                            <tr bgcolor="#f59e0b" style="background-color:#f59e0b;">
                              <td
                                align="center"
                                bgcolor="#f59e0b"
                                style="
                                  margin:0;
                                  padding:0;
                                  background-color:#f59e0b;
                                "
                              >
                                <a
                                  href="{{.Link}}"
                                  target="_blank"
                                  class="button-link"
                                  style="
                                    display:block;
                                    margin:0;
                                    padding:16px 58px;
                                    background-color:#f59e0b;
                                    color:#18181b;
                                    font-family:Arial,'Segoe UI',Helvetica,sans-serif;
                                    font-size:14px;
                                    line-height:18px;
                                    font-weight:bold;
                                    text-align:center;
                                    text-decoration:none;
                                  "
                                >
                                  {{.ButtonLabel}} &nbsp;&rarr;
                                </a>
                              </td>
                            </tr>
                          </table>
                          <!--<![endif]-->
                        </td>
                      </tr>
                    </table>
                  </td>
                </tr>

                <!-- Rodapé interno -->
                <tr bgcolor="#0d0d0f" style="background-color:#0d0d0f;">
                  <td
                    bgcolor="#0d0d0f"
                    style="
                      margin:0;
                      padding:17px 34px;
                      background-color:#0d0d0f;
                      border-top:1px solid #27272a;
                      border-bottom:1px solid #27272a;
                    "
                  >
                    <table
                      role="presentation"
                      width="100%"
                      cellspacing="0"
                      cellpadding="0"
                      border="0"
                      bgcolor="#0d0d0f"
                      style="
                        width:100%;
                        margin:0;
                        padding:0;
                        border-collapse:collapse;
                        background-color:#0d0d0f;
                      "
                    >
                      <tr bgcolor="#0d0d0f" style="background-color:#0d0d0f;">
                        <td
                          bgcolor="#0d0d0f"
                          class="footer-column"
                          style="
                            background-color:#0d0d0f;
                            font-family:Arial,'Segoe UI',Helvetica,sans-serif;
                            font-size:10px;
                            line-height:16px;
                            color:#71717a;
                          "
                        >
                          Enviado automaticamente pelo LogHill
                        </td>

                        <td
                          align="right"
                          bgcolor="#0d0d0f"
                          class="footer-column footer-right"
                          style="
                            background-color:#0d0d0f;
                            font-family:Arial,'Segoe UI',Helvetica,sans-serif;
                            font-size:10px;
                            line-height:16px;
                            color:#71717a;
                          "
                        >
                          {{.AlertName}}
                        </td>
                      </tr>
                    </table>
                  </td>
                </tr>
              </table>
            </td>
          </tr>
        </table>

        <!-- Rodapé externo -->
        <table
          role="presentation"
          width="680"
          cellspacing="0"
          cellpadding="0"
          border="0"
          bgcolor="#ffffff"
          class="email-shell"
          style="
            width:100%;
            max-width:680px;
            margin:0;
            padding:0;
            border-collapse:collapse;
            background-color:#ffffff;
          "
        >
          <tr>
            <td
              align="center"
              bgcolor="#ffffff"
              style="
                padding:12px 16px 0 16px;
                background-color:#ffffff;
                font-family:Arial,'Segoe UI',Helvetica,sans-serif;
                font-size:10px;
                line-height:16px;
                color:#71717a;
              "
            >
              Você recebeu este e-mail porque uma notificação foi configurada no LogHill.
            </td>
          </tr>
        </table>

        <!--[if mso]>
            </td>
          </tr>
        </table>
        <![endif]-->
      </td>
    </tr>
  </table>
</body>
</html>
`

const textTemplate = `LOGHILL — {{.Kicker}}

{{.Headline}}
{{.Description}}

MENSAGEM
{{.Message}}

{{if .ProviderTest}}Provedor{{else}}Sender{{end}}: {{.SenderName}}
{{if .ProviderTest}}Integração{{else}}Identificador{{end}}: {{.SenderID}}
Status: {{.SenderStatus}}
{{if not .ProviderTest}}Severidade: {{.Severity}}
Horário do log: {{.Timestamp}}
{{end}}Enviado em: {{.SentAt}}

{{if .HasMetadata}}METADADOS
{{.Metadata}}

{{end}}{{.ButtonLabel}}: {{.Link}}

Enviado automaticamente pelo LogHill.
`
