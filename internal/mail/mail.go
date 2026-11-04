// Package mail envía correos por SMTP genérico, siempre con TLS (ADR-009 §3).
package mail

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/hex"
	"errors"
	"fmt"
	"mime"
	"mime/quotedprintable"
	"net"
	"net/mail"
	"net/smtp"
	"strconv"
	"strings"
	"time"
)

// SMTP es la cuenta de envío. Con el puerto 465 usa TLS implícito; con cualquier otro exige STARTTLS.
type SMTP struct {
	Host     string
	Port     int
	Username string
	Password string // solo del entorno (GREMIO_SMTP_PASSWORD), nunca de un flag
	From     string
}

func (m SMTP) Configured() bool { return m.Host != "" && m.From != "" }

// Send manda un correo de texto plano a una sola dirección.
func (m SMTP) Send(ctx context.Context, to, subject, body string) error {
	if !m.Configured() {
		return errors.New("SMTP sin configurar: faltan GREMIO_SMTP_HOST y GREMIO_SMTP_FROM")
	}
	from, err := mail.ParseAddress(m.From)
	if err != nil {
		return fmt.Errorf("remitente %q: %w", m.From, err)
	}
	rcpt, err := mail.ParseAddress(to)
	if err != nil {
		return fmt.Errorf("destinatario: %w", err)
	}
	msg, err := message(from, rcpt, subject, body, time.Now())
	if err != nil {
		return err
	}

	addr := net.JoinHostPort(m.Host, strconv.Itoa(m.Port))
	cfg := &tls.Config{ServerName: m.Host, MinVersion: tls.VersionTLS12}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	var conn net.Conn
	if m.Port == 465 {
		conn, err = (&tls.Dialer{Config: cfg}).DialContext(ctx, "tcp", addr)
	} else {
		conn, err = (&net.Dialer{}).DialContext(ctx, "tcp", addr)
	}
	if err != nil {
		return err
	}
	if dl, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(dl)
	}
	c, err := smtp.NewClient(conn, m.Host)
	if err != nil {
		conn.Close()
		return err
	}
	defer c.Close()
	if m.Port != 465 {
		if ok, _ := c.Extension("STARTTLS"); !ok {
			return fmt.Errorf("%s no ofrece STARTTLS: Gremio no envía sin TLS", addr)
		}
		if err := c.StartTLS(cfg); err != nil {
			return err
		}
	}
	if m.Username != "" {
		if err := c.Auth(smtp.PlainAuth("", m.Username, m.Password, m.Host)); err != nil {
			return err
		}
	}
	if err := c.Mail(from.Address); err != nil {
		return err
	}
	if err := c.Rcpt(rcpt.Address); err != nil {
		return err
	}
	w, err := c.Data()
	if err != nil {
		return err
	}
	if _, err := w.Write(msg); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	return c.Quit()
}

func message(from, to *mail.Address, subject, body string, date time.Time) ([]byte, error) {
	id := make([]byte, 12)
	if _, err := rand.Read(id); err != nil {
		return nil, err
	}
	domain := from.Address[strings.LastIndex(from.Address, "@")+1:]
	var b bytes.Buffer
	fmt.Fprintf(&b, "From: %s\r\nTo: %s\r\nSubject: %s\r\nDate: %s\r\nMessage-ID: <%s@%s>\r\n",
		from, to, mime.QEncoding.Encode("utf-8", subject), date.Format(time.RFC1123Z), hex.EncodeToString(id), domain)
	b.WriteString("MIME-Version: 1.0\r\nContent-Type: text/plain; charset=utf-8\r\nContent-Transfer-Encoding: quoted-printable\r\n\r\n")
	qp := quotedprintable.NewWriter(&b)
	if _, err := qp.Write([]byte(body)); err != nil {
		return nil, err
	}
	if err := qp.Close(); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}
