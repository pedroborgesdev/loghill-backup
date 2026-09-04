package emailprovider

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/smtp"
	"strconv"
	"strings"

	"logmate/internal/domain"
	"logmate/internal/emailconfig"
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
		return &Error{Code: "GMAIL_INVALID_MESSAGE", Message: "The email message contains invalid headers."}
	}
	client, err := smtpClient(ctx, runtime)
	if err != nil {
		return err
	}
	defer client.Close()
	if err = client.Mail(runtime.SenderEmail); err != nil {
		return gmailError("GMAIL_SENDER_REJECTED", "Gmail rejected the configured sender.", err)
	}
	for _, recipient := range message.To {
		if err = client.Rcpt(recipient); err != nil {
			return gmailError("GMAIL_RECIPIENT_REJECTED", "Gmail rejected one of the recipients.", err)
		}
	}
	writer, err := client.Data()
	if err != nil {
		return gmailError("GMAIL_SEND_FAILED", "Gmail rejected the start of delivery.", err)
	}
	if _, err = writer.Write(body); err == nil {
		err = writer.Close()
	}
	if err != nil {
		return gmailError("GMAIL_SEND_FAILED", "Unable to send the message through Gmail.", err)
	}
	return client.Quit()
}

func smtpClient(ctx context.Context, runtime emailconfig.Runtime) (*smtp.Client, error) {
	address := net.JoinHostPort(runtime.SMTPHost, strconv.Itoa(runtime.SMTPPort))
	connection, err := (&net.Dialer{}).DialContext(ctx, "tcp", address)
	if err != nil {
		return nil, gmailError("GMAIL_CONNECTION_FAILED", "Unable to connect to the Gmail SMTP server.", err)
	}
	if deadline, ok := ctx.Deadline(); ok {
		_ = connection.SetDeadline(deadline)
	}
	client, err := smtp.NewClient(connection, runtime.SMTPHost)
	if err != nil {
		_ = connection.Close()
		return nil, gmailError("GMAIL_CONNECTION_FAILED", "The Gmail SMTP server returned an invalid response.", err)
	}
	if ok, _ := client.Extension("STARTTLS"); !ok {
		_ = client.Close()
		return nil, &Error{Code: "GMAIL_TLS_REQUIRED", Message: "The SMTP server does not offer STARTTLS."}
	}
	if err = client.StartTLS(&tls.Config{ServerName: runtime.SMTPHost, MinVersion: tls.VersionTLS12}); err != nil {
		_ = client.Close()
		return nil, gmailError("GMAIL_TLS_FAILED", "Unable to establish a secure connection to Gmail.", err)
	}
	auth := smtp.PlainAuth("", runtime.SMTPUsername, runtime.SMTPPassword, runtime.SMTPHost)
	if err = client.Auth(auth); err != nil {
		_ = client.Close()
		return nil, gmailError("GMAIL_AUTH_FAILED", "Gmail rejected the credentials. Use a valid app password.", err)
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
		return &Error{Code: "GMAIL_TIMEOUT", Message: "The Gmail SMTP server did not respond within the expected time."}
	}
	return &Error{Code: code, Message: fmt.Sprintf("%s (%v)", message, err)}
}
