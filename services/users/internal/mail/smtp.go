// Package mail šalje mejlove preko SMTP-a (za razvoj: MailHog).
package mail

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"mime"
	"net"
	"net/smtp"
	"strings"
	"time"
)

// Mailer šalje jedan tekstualni mejl.
type Mailer interface {
	Send(ctx context.Context, to, subject, body string) error
}

// SMTPMailer šalje mejlove SMTP serveru. User i Password su opcioni (MailHog
// ih ne traži); kad su zadati, koristi se STARTTLS ako ga server nudi.
type SMTPMailer struct {
	Host     string
	Port     string
	From     string
	User     string
	Password string // nikad se ne loguje
	Timeout  time.Duration
}

// Send šalje mejl. Ceo razgovor sa serverom je ograničen timeout-om.
func (m *SMTPMailer) Send(ctx context.Context, to, subject, body string) error {
	if strings.ContainsAny(to, "\r\n") || strings.ContainsAny(m.From, "\r\n") {
		return errors.New("mail: neispravna adresa")
	}

	timeout := m.Timeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	deadline := time.Now().Add(timeout)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}

	dialer := net.Dialer{Deadline: deadline}
	conn, err := dialer.DialContext(ctx, "tcp", net.JoinHostPort(m.Host, m.Port))
	if err != nil {
		return fmt.Errorf("mail: povezivanje: %w", err)
	}
	_ = conn.SetDeadline(deadline)

	c, err := smtp.NewClient(conn, m.Host)
	if err != nil {
		_ = conn.Close()
		return fmt.Errorf("mail: smtp klijent: %w", err)
	}
	defer c.Close()

	if m.User != "" {
		if ok, _ := c.Extension("STARTTLS"); ok {
			if err := c.StartTLS(&tls.Config{ServerName: m.Host, MinVersion: tls.VersionTLS12}); err != nil {
				return fmt.Errorf("mail: starttls: %w", err)
			}
		}
		if err := c.Auth(smtp.PlainAuth("", m.User, m.Password, m.Host)); err != nil {
			return fmt.Errorf("mail: autentifikacija: %w", err)
		}
	}
	if err := c.Mail(m.From); err != nil {
		return fmt.Errorf("mail: MAIL FROM: %w", err)
	}
	if err := c.Rcpt(to); err != nil {
		return fmt.Errorf("mail: RCPT TO: %w", err)
	}
	w, err := c.Data()
	if err != nil {
		return fmt.Errorf("mail: DATA: %w", err)
	}
	if _, err := w.Write(buildMessage(m.From, to, subject, body, time.Now())); err != nil {
		return fmt.Errorf("mail: slanje poruke: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("mail: završetak poruke: %w", err)
	}
	return c.Quit()
}

// buildMessage sklapa RFC 5322 poruku. Adrese su već proverene da nemaju
// prelome redova, a naslov se enkodira (dijakritici).
func buildMessage(from, to, subject, body string, now time.Time) []byte {
	subject = strings.NewReplacer("\r", " ", "\n", " ").Replace(subject)
	var b strings.Builder
	b.WriteString("From: " + from + "\r\n")
	b.WriteString("To: " + to + "\r\n")
	b.WriteString("Subject: " + mime.QEncoding.Encode("utf-8", subject) + "\r\n")
	b.WriteString("Date: " + now.Format(time.RFC1123Z) + "\r\n")
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString("Content-Type: text/plain; charset=UTF-8\r\n")
	b.WriteString("Content-Transfer-Encoding: 8bit\r\n\r\n")
	body = strings.ReplaceAll(body, "\r\n", "\n")
	b.WriteString(strings.ReplaceAll(body, "\n", "\r\n"))
	b.WriteString("\r\n")
	return []byte(b.String())
}
