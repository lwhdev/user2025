package main

import (
	"bytes"
	"encoding/csv"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"text/template"

	"net/smtp"
)

type Recipient map[string]string

func main() {
	var (
		smtpServer   = flag.String("smtp-server", "", "SMTP server hostname")
		smtpPort     = flag.Int("smtp-port", 587, "SMTP server port")
		smtpUser     = flag.String("smtp-username", "", "SMTP username")
		smtpPassword = flag.String("smtp-password", "", "SMTP password")
		fromAddress  = flag.String("from", "", "From email address (defaults to smtp-username)")
		subject      = flag.String("subject", "", "Email subject line")
		bodyPath     = flag.String("body-template", "", "Path to the body template file")
		recipients   = flag.String("recipients", "", "Path to CSV file containing recipients")
		dryRun       = flag.Bool("dry-run", false, "Log emails without sending")
	)

	flag.Parse()

	if err := run(*smtpServer, *smtpPort, *smtpUser, *smtpPassword, *fromAddress, *subject, *bodyPath, *recipients, *dryRun); err != nil {
		log.Fatalf("%v", err)
	}
}

func run(server string, port int, username, password, from, subject, bodyTemplatePath, recipientsPath string, dryRun bool) error {
	if server == "" {
		return errors.New("smtp-server is required")
	}
	if username == "" {
		return errors.New("smtp-username is required")
	}
	if password == "" && !dryRun {
		return errors.New("smtp-password is required unless dry-run is enabled")
	}
	if subject == "" {
		return errors.New("subject is required")
	}
	if bodyTemplatePath == "" {
		return errors.New("body-template is required")
	}
	if recipientsPath == "" {
		return errors.New("recipients is required")
	}

	if from == "" {
		from = username
	}

	tmpl, err := parseTemplate(bodyTemplatePath)
	if err != nil {
		return fmt.Errorf("failed to parse template: %w", err)
	}

	recipientsList, err := loadRecipients(recipientsPath)
	if err != nil {
		return fmt.Errorf("failed to load recipients: %w", err)
	}

	log.Printf("Loaded %d recipients", len(recipientsList))

	for _, recipient := range recipientsList {
		email, ok := recipient["email"]
		if !ok || strings.TrimSpace(email) == "" {
			log.Printf("Skipping recipient missing 'email' field: %#v", recipient)
			continue
		}

		body, err := renderBody(tmpl, recipient)
		if err != nil {
			log.Printf("Failed to render template for %s: %v", email, err)
			continue
		}

		msg, err := buildMessage(from, email, subject, body)
		if err != nil {
			log.Printf("Failed to build message for %s: %v", email, err)
			continue
		}

		if dryRun {
			log.Printf("[DRY RUN] Would send email to %s\n%s", email, msg)
			continue
		}

		if err := sendEmail(server, port, username, password, from, email, []byte(msg)); err != nil {
			log.Printf("Failed to send email to %s: %v", email, err)
			continue
		}

		log.Printf("Sent email to %s", email)
	}

	return nil
}

func parseTemplate(path string) (*template.Template, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return template.New("body").Funcs(template.FuncMap{"upper": strings.ToUpper}).Parse(string(content))
}

func loadRecipients(path string) ([]Recipient, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	reader := csv.NewReader(f)
	reader.TrimLeadingSpace = true

	headers, err := reader.Read()
	if err != nil {
		return nil, err
	}

	for i, h := range headers {
		headers[i] = strings.ToLower(strings.TrimSpace(h))
	}

	var recipients []Recipient
	for {
		record, err := reader.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}

		if len(record) != len(headers) {
			return nil, fmt.Errorf("recipient record has %d fields, expected %d", len(record), len(headers))
		}

		r := make(Recipient, len(headers))
		for i, field := range record {
			r[headers[i]] = field
		}
		recipients = append(recipients, r)
	}

	return recipients, nil
}

func renderBody(tmpl *template.Template, recipient Recipient) (string, error) {
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, recipient); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func buildMessage(from, to, subject, body string) (string, error) {
	if strings.TrimSpace(from) == "" {
		return "", errors.New("from address cannot be empty")
	}
	if strings.TrimSpace(to) == "" {
		return "", errors.New("to address cannot be empty")
	}

	var msg bytes.Buffer
	msg.WriteString(fmt.Sprintf("From: %s\r\n", from))
	msg.WriteString(fmt.Sprintf("To: %s\r\n", to))
	msg.WriteString(fmt.Sprintf("Subject: %s\r\n", subject))
	msg.WriteString("MIME-Version: 1.0\r\n")
	msg.WriteString("Content-Type: text/plain; charset=\"UTF-8\"\r\n")
	msg.WriteString("Content-Transfer-Encoding: 8bit\r\n\r\n")
	msg.WriteString(body)

	return msg.String(), nil
}

func sendEmail(server string, port int, username, password, from, to string, msg []byte) error {
	auth := smtp.PlainAuth("", username, password, server)
	addr := fmt.Sprintf("%s:%d", server, port)
	return smtp.SendMail(addr, auth, from, []string{to}, msg)
}
