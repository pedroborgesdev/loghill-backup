package emailprovider

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/smtp"
	"strconv"
	"strings"

	"logtheater/internal/domain"
	"logtheater/internal/emailconfig"
)

type Gmail struct{ settings *emailconfig.Store }

func NewGmail(settings *emailconfig.Store) *Gmail   { return &Gmail{settings: settings} }
func (g *Gmail) Provider() domain.EmailProviderType { return domain.EmailProviderGmail }

func (g *Gmail) TestConnection(ctx context.Context) error {
	runtime, err := g.settings.Runtime()
	if err != nil || runtime.Provider != domain.EmailProviderGmail || !runtimeConfiguredSMTP(runtime) {
		return ErrNotConfigured
	}
	client, err := smtpClient(ctx, runtime)
	if err != nil {
		return err
	}
	return client.Quit()
}

func (g *Gmail) Send(ctx context.Context, message domain.EmailMessage) error {
	runtime, err := g.settings.Runtime()
	if err != nil || runtime.Provider != domain.EmailProviderGmail || !runtime.Enabled || !runtimeConfiguredSMTP(runtime) {
		return ErrNotConfigured
	}
	body, err := buildMIME(runtime, message)
	if err != nil {
		return &Error{Code: "GMAIL_INVALID_MESSAGE", Message: "A mensagem de e-mail contém cabeçalhos inválidos."}
	}
	client, err := smtpClient(ctx, runtime)
	if err != nil {
		return err
	}
	defer client.Close()
	if err = client.Mail(runtime.SenderEmail); err != nil {
		return gmailError("GMAIL_SENDER_REJECTED", "O Gmail recusou o remetente configurado.", err)
	}
	for _, recipient := range message.To {
		if err = client.Rcpt(recipient); err != nil {
			return gmailError("GMAIL_RECIPIENT_REJECTED", "O Gmail recusou um dos destinatários.", err)
		}
	}
	writer, err := client.Data()
	if err != nil {
		return gmailError("GMAIL_SEND_FAILED", "O Gmail recusou o início do envio.", err)
	}
	if _, err = writer.Write(body); err == nil {
		err = writer.Close()
	}
	if err != nil {
		return gmailError("GMAIL_SEND_FAILED", "Não foi possível enviar a mensagem pelo Gmail.", err)
	}
	return client.Quit()
}

func smtpClient(ctx context.Context, runtime emailconfig.Runtime) (*smtp.Client, error) {
	address := net.JoinHostPort(runtime.SMTPHost, strconv.Itoa(runtime.SMTPPort))
	connection, err := (&net.Dialer{}).DialContext(ctx, "tcp", address)
	if err != nil {
		return nil, gmailError("GMAIL_CONNECTION_FAILED", "Não foi possível conectar ao servidor SMTP do Gmail.", err)
	}
	if deadline, ok := ctx.Deadline(); ok {
		_ = connection.SetDeadline(deadline)
	}
	client, err := smtp.NewClient(connection, runtime.SMTPHost)
	if err != nil {
		_ = connection.Close()
		return nil, gmailError("GMAIL_CONNECTION_FAILED", "O servidor SMTP do Gmail retornou uma resposta inválida.", err)
	}
	if ok, _ := client.Extension("STARTTLS"); !ok {
		_ = client.Close()
		return nil, &Error{Code: "GMAIL_TLS_REQUIRED", Message: "O servidor SMTP não oferece STARTTLS."}
	}
	if err = client.StartTLS(&tls.Config{ServerName: runtime.SMTPHost, MinVersion: tls.VersionTLS12}); err != nil {
		_ = client.Close()
		return nil, gmailError("GMAIL_TLS_FAILED", "Não foi possível estabelecer uma conexão segura com o Gmail.", err)
	}
	auth := smtp.PlainAuth("", runtime.SMTPUsername, runtime.SMTPPassword, runtime.SMTPHost)
	if err = client.Auth(auth); err != nil {
		_ = client.Close()
		return nil, gmailError("GMAIL_AUTH_FAILED", "O Gmail recusou as credenciais. Use uma senha de aplicativo válida.", err)
	}
	return client, nil
}

func runtimeConfiguredSMTP(value emailconfig.Runtime) bool {
	return value.SMTPHost != "" && value.SMTPPort > 0 && value.SMTPUsername != "" && value.SMTPPassword != "" && value.SenderEmail != ""
}

func gmailError(code, message string, err error) error {
	if err == nil {
		return &Error{Code: code, Message: message}
	}
	if strings.Contains(strings.ToLower(err.Error()), "timeout") {
		return &Error{Code: "GMAIL_TIMEOUT", Message: "O servidor SMTP do Gmail não respondeu dentro do tempo esperado."}
	}
	return &Error{Code: code, Message: fmt.Sprintf("%s (%v)", message, err)}
}
