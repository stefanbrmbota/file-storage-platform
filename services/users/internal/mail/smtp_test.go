package mail

import (
	"bufio"
	"context"
	"net"
	"strings"
	"testing"
	"time"
)

// fakeSMTP je minimalan SMTP server koji hvata jednu poruku.
type fakeSMTP struct {
	ln   net.Listener
	done chan string // sirov sadržaj sesije posle DATA
	cmds chan []string
}

func startFakeSMTP(t *testing.T) *fakeSMTP {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	s := &fakeSMTP{ln: ln, done: make(chan string, 1), cmds: make(chan []string, 1)}
	t.Cleanup(func() { _ = ln.Close() })

	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		r := bufio.NewReader(conn)
		w := func(line string) { _, _ = conn.Write([]byte(line + "\r\n")) }
		var cmds []string
		w("220 fake ESMTP")
		for {
			line, err := r.ReadString('\n')
			if err != nil {
				return
			}
			line = strings.TrimRight(line, "\r\n")
			cmds = append(cmds, line)
			switch up := strings.ToUpper(line); {
			case strings.HasPrefix(up, "EHLO"), strings.HasPrefix(up, "HELO"):
				w("250 fake")
			case strings.HasPrefix(up, "MAIL"), strings.HasPrefix(up, "RCPT"):
				w("250 OK")
			case up == "DATA":
				w("354 go")
				var data strings.Builder
				for {
					l, err := r.ReadString('\n')
					if err != nil {
						return
					}
					if l == ".\r\n" {
						break
					}
					data.WriteString(l)
				}
				s.done <- data.String()
				w("250 queued")
			case up == "QUIT":
				w("221 bye")
				s.cmds <- cmds
				return
			default:
				w("250 OK")
			}
		}
	}()
	return s
}

func (s *fakeSMTP) hostPort(t *testing.T) (string, string) {
	t.Helper()
	h, p, err := net.SplitHostPort(s.ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	return h, p
}

func TestSMTPMailer_Send(t *testing.T) {
	srv := startFakeSMTP(t)
	host, port := srv.hostPort(t)
	m := &SMTPMailer{Host: host, Port: port, From: "no-reply@example.com", Timeout: 3 * time.Second}

	err := m.Send(context.Background(), "pera@example.com", "Potvrda registracije", "Zdravo Pero,\nlink: http://x/confirm#token=abc\n.tačka na početku\n")
	if err != nil {
		t.Fatal(err)
	}

	msg := <-srv.done
	for _, want := range []string{
		"From: no-reply@example.com\r\n",
		"To: pera@example.com\r\n",
		"Content-Type: text/plain; charset=UTF-8\r\n",
		"link: http://x/confirm#token=abc\r\n",
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("poruka ne sadrži %q:\n%s", want, msg)
		}
	}
	cmds := strings.Join(<-srv.cmds, "|")
	if !strings.Contains(cmds, "MAIL FROM:<no-reply@example.com>") || !strings.Contains(cmds, "RCPT TO:<pera@example.com>") {
		t.Errorf("neočekivane SMTP komande: %s", cmds)
	}
}

func TestSMTPMailer_RejectsHeaderInjection(t *testing.T) {
	m := &SMTPMailer{Host: "127.0.0.1", Port: "1", From: "a@b.c"}
	err := m.Send(context.Background(), "x@y.z\r\nBcc: tudji@example.com", "s", "b")
	if err == nil || !strings.Contains(err.Error(), "neispravna adresa") {
		t.Fatalf("err = %v, očekivano odbijanje adrese sa prelomom reda", err)
	}
}

func TestSMTPMailer_SubjectCannotInjectHeaders(t *testing.T) {
	msg := string(buildMessage("a@b.c", "x@y.z", "Naslov\r\nBcc: tudji@example.com", "telo", time.Now()))
	head, _, _ := strings.Cut(msg, "\r\n\r\n")
	for _, line := range strings.Split(head, "\r\n") {
		if strings.HasPrefix(line, "Bcc:") {
			t.Fatalf("ubačeno zaglavlje:\n%s", head)
		}
	}
}

func TestSMTPMailer_UnreachableServer(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	host, port, _ := net.SplitHostPort(ln.Addr().String())
	_ = ln.Close() // niko ne sluša

	m := &SMTPMailer{Host: host, Port: port, From: "a@b.c", Timeout: time.Second}
	if err := m.Send(context.Background(), "x@y.z", "s", "b"); err == nil {
		t.Fatal("očekivana greška za nedostupan server")
	}
}
