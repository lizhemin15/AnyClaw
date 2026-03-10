package mail

import (
	"crypto/tls"
	"fmt"
	"net"
	"net/smtp"
	"os"
	"strconv"
	"strings"
	"time"
)

// SMTPParams holds SMTP connection params
type SMTPParams struct {
	Host string
	Port int
	User string
	Pass string
	From string
}

func (p *SMTPParams) Ok() bool {
	return p != nil && strings.TrimSpace(p.Host) != ""
}

// ConfigFromEnv from env: SMTP_HOST, SMTP_PORT, SMTP_USER, SMTP_PASS, SMTP_FROM
func ConfigFromEnv() (host string, port int, user, pass, from string, ok bool) {
	host = os.Getenv("SMTP_HOST")
	if host == "" {
		return "", 0, "", "", "", false
	}
	port = 587
	if p := os.Getenv("SMTP_PORT"); p != "" {
		if n, err := strconv.Atoi(p); err == nil {
			port = n
		}
	}
	user = os.Getenv("SMTP_USER")
	pass = os.Getenv("SMTP_PASS")
	from = os.Getenv("SMTP_FROM")
	if from == "" {
		from = user
	}
	return host, port, user, pass, from, true
}

// TestSMTP tests SMTP connectivity.
func TestSMTP(host string, port int, user, pass, from string) error {
	if port <= 0 {
		port = 587
	}
	addr := fmt.Sprintf("%s:%d", host, port)
	conn, err := net.DialTimeout("tcp", addr, 10*time.Second)
	if err != nil {
		return fmt.Errorf("连接失败: %w", err)
	}
	conn.Close()
	if user == "" || pass == "" {
		return nil
	}
	auth := smtp.PlainAuth("", user, pass, host)
	if port == 465 {
		tlsConfig := &tls.Config{ServerName: host}
		conn, err = tls.Dial("tcp", addr, tlsConfig)
	} else {
		conn, err = net.DialTimeout("tcp", addr, 10*time.Second)
	}
	if err != nil {
		return fmt.Errorf("连接失败: %w", err)
	}
	defer conn.Close()
	client, err := smtp.NewClient(conn, host)
	if err != nil {
		return fmt.Errorf("连接失败: %w", err)
	}
	defer client.Close()
	if port != 465 {
		if err = client.StartTLS(&tls.Config{ServerName: host}); err != nil {
			return fmt.Errorf("STARTTLS 失败: %w", err)
		}
	}
	if err = client.Auth(auth); err != nil {
		return fmt.Errorf("认证失败: %w", err)
	}
	return nil
}

// SendVerificationCode sends a verification code email. Uses params if provided, else env.
func SendVerificationCode(to, code string, params *SMTPParams) error {
	var host string
	var port int
	var user, pass, from string
	if params != nil && params.Ok() {
		host = params.Host
		port = params.Port
		if port <= 0 {
			port = 587
		}
		user = params.User
		pass = params.Pass
		from = params.From
		if from == "" {
			from = user
		}
	} else {
		var ok bool
		host, port, user, pass, from, ok = ConfigFromEnv()
		if !ok {
			return fmt.Errorf("SMTP 未配置")
		}
	}
	subject := "AnyClaw 注册验证码"
	body := fmt.Sprintf(`您好，

您的 AnyClaw 注册验证码是：%s

验证码 5 分钟内有效，请勿泄露给他人。

如非本人操作，请忽略此邮件。

—— AnyClaw`, code)
	return send(host, port, user, pass, from, to, subject, body)
}

func send(host string, port int, user, pass, from, to, subject, body string) error {
	addr := fmt.Sprintf("%s:%d", host, port)
	msg := "From: " + from + "\r\n" +
		"To: " + to + "\r\n" +
		"Subject: " + subject + "\r\n" +
		"Content-Type: text/plain; charset=UTF-8\r\n" +
		"\r\n" + body

	var auth smtp.Auth
	if user != "" && pass != "" {
		auth = smtp.PlainAuth("", user, pass, host)
	}
	if port == 465 {
		tlsConfig := &tls.Config{ServerName: host}
		conn, err := tls.Dial("tcp", addr, tlsConfig)
		if err != nil {
			return err
		}
		defer conn.Close()
		client, err := smtp.NewClient(conn, host)
		if err != nil {
			return err
		}
		defer client.Close()
		if auth != nil {
			if err := client.Auth(auth); err != nil {
				return err
			}
		}
		if err := client.Mail(from); err != nil {
			return err
		}
		if err := client.Rcpt(to); err != nil {
			return err
		}
		w, err := client.Data()
		if err != nil {
			return err
		}
		_, err = w.Write([]byte(msg))
		if err != nil {
			return err
		}
		return w.Close()
	}
	return smtp.SendMail(addr, auth, from, []string{to}, []byte(msg))
}

// IsValidEmail basic format check
func IsValidEmail(s string) bool {
	s = strings.TrimSpace(strings.ToLower(s))
	if len(s) < 5 {
		return false
	}
	at := strings.Index(s, "@")
	if at <= 0 || at >= len(s)-1 {
		return false
	}
	dot := strings.LastIndex(s[at+1:], ".")
	return dot > 0 && at+1+dot < len(s)-1
}
