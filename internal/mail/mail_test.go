package mail

import (
	"bufio"
	"context"
	"net"
	"net/mail"
	"strings"
	"testing"
	"time"
)

func TestMensaje(t *testing.T) {
	from := &mail.Address{Name: "Gremio", Address: "gremio@ejemplo.org"}
	to := &mail.Address{Address: "alice@ejemplo.org"}
	msg, err := message(from, to, "Tu enlace para entrar", "Ábrelo aquí:\nhttps://gremio.ejemplo.org/login/abc", time.Date(2026, 11, 3, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	m, err := mail.ReadMessage(strings.NewReader(string(msg)))
	if err != nil {
		t.Fatal(err)
	}
	if got := m.Header.Get("To"); got != "<alice@ejemplo.org>" {
		t.Errorf("To = %q", got)
	}
	if !strings.HasSuffix(m.Header.Get("Message-ID"), "@ejemplo.org>") || m.Header.Get("Content-Transfer-Encoding") != "quoted-printable" {
		t.Errorf("cabeceras: %v", m.Header)
	}
	if !strings.Contains(string(msg), "=C3=81brelo") {
		t.Error("el cuerpo va en quoted-printable")
	}
}

func TestDestinatarioInvalido(t *testing.T) {
	m := SMTP{Host: "127.0.0.1", Port: 587, From: "gremio@ejemplo.org"}
	if err := m.Send(context.Background(), "a@b.org\r\nBcc: c@d.org", "x", "y"); err == nil {
		t.Error("una dirección con saltos de línea debe rechazarse")
	}
}

// Un servidor sin STARTTLS no recibe nunca la clave.
func TestSinTLSNoSeAutentica(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	got := make(chan string, 1)
	go func() {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		defer c.Close()
		c.SetDeadline(time.Now().Add(5 * time.Second))
		r := bufio.NewReader(c)
		c.Write([]byte("220 falso ESMTP\r\n"))
		var seen strings.Builder
		for {
			line, err := r.ReadString('\n')
			seen.WriteString(line)
			if err != nil || strings.HasPrefix(line, "QUIT") {
				break
			}
			if strings.HasPrefix(line, "EHLO") {
				c.Write([]byte("250-falso\r\n250 AUTH PLAIN\r\n"))
			} else {
				c.Write([]byte("250 ok\r\n"))
			}
		}
		got <- seen.String()
	}()
	port := ln.Addr().(*net.TCPAddr).Port
	m := SMTP{Host: "127.0.0.1", Port: port, Username: "u", Password: "secreta", From: "gremio@ejemplo.org"}
	err = m.Send(context.Background(), "alice@ejemplo.org", "x", "y")
	if err == nil || !strings.Contains(err.Error(), "STARTTLS") {
		t.Fatalf("debe negarse sin STARTTLS: %v", err)
	}
	if s := <-got; strings.Contains(s, "AUTH") || strings.Contains(s, "MAIL") {
		t.Errorf("el cliente siguió sin TLS:\n%s", s)
	}
}
