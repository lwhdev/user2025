package main

import (
	"fmt"
	"html/template"
	"log"
	"net/http"
	"net/smtp"
	"strconv"
	"strings"
)

type pageData struct {
	Form    formData
	Results []sendResult
	Error   string
}

type formData struct {
	SMTPServer string
	SMTPPort   string
	SMTPUser   string
	From       string
	Subject    string
	Body       string
	Recipients string
}

type sendResult struct {
	Recipient string
	Success   bool
	Message   string
}

var tmpl = template.Must(template.New("page").Parse(`<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <title>Bulk Mailer</title>
    <style>
        body { font-family: sans-serif; margin: 2rem; }
        form { max-width: 720px; display: grid; gap: 1rem; }
        label { display: flex; flex-direction: column; font-weight: 600; }
        input[type="text"], input[type="password"], textarea { padding: 0.5rem; font-size: 1rem; }
        textarea { min-height: 10rem; }
        .error { color: #b00020; font-weight: 600; }
        .results { margin-top: 2rem; }
        .result-success { color: #006400; }
        .result-failure { color: #8b0000; }
        button { padding: 0.75rem 1.5rem; font-size: 1rem; }
    </style>
</head>
<body>
    <h1>Bulk Mailer</h1>
    <p>Send a plain-text email to multiple recipients by filling in the SMTP and message details below.</p>
    {{if .Error}}
    <p class="error">{{.Error}}</p>
    {{end}}
    <form method="post" action="/">
        <label>SMTP Server
            <input type="text" name="smtp_server" value="{{.Form.SMTPServer}}" required>
        </label>
        <label>SMTP Port
            <input type="text" name="smtp_port" value="{{.Form.SMTPPort}}" required>
        </label>
        <label>SMTP Username
            <input type="text" name="smtp_user" value="{{.Form.SMTPUser}}" required>
        </label>
        <label>SMTP Password
            <input type="password" name="smtp_pass" value="" placeholder="Not stored" required>
        </label>
        <label>From Address
            <input type="text" name="from" value="{{.Form.From}}" placeholder="Defaults to SMTP username">
        </label>
        <label>Subject
            <input type="text" name="subject" value="{{.Form.Subject}}" required>
        </label>
        <label>Body
            <textarea name="body" required>{{.Form.Body}}</textarea>
        </label>
        <label>Recipients (one email address per line)
            <textarea name="recipients" required>{{.Form.Recipients}}</textarea>
        </label>
        <button type="submit">Send Emails</button>
    </form>

    {{if .Results}}
    <div class="results">
        <h2>Send Results</h2>
        <ul>
            {{range .Results}}
            <li class="{{if .Success}}result-success{{else}}result-failure{{end}}">{{.Recipient}} — {{.Message}}</li>
            {{end}}
        </ul>
    </div>
    {{end}}
</body>
</html>`))

func main() {
	http.HandleFunc("/", formHandler)

	addr := ":8080"
	log.Printf("Listening on %s", addr)
	if err := http.ListenAndServe(addr, nil); err != nil {
		log.Fatalf("failed to start server: %v", err)
	}
}

func formHandler(w http.ResponseWriter, r *http.Request) {
	data := pageData{
		Form: formData{
			SMTPPort: "587",
		},
	}

	if r.Method == http.MethodPost {
		if err := r.ParseForm(); err != nil {
			data.Error = fmt.Sprintf("failed to parse form: %v", err)
			renderTemplate(w, data)
			return
		}

		data.Form.SMTPServer = strings.TrimSpace(r.FormValue("smtp_server"))
		data.Form.SMTPPort = strings.TrimSpace(r.FormValue("smtp_port"))
		data.Form.SMTPUser = strings.TrimSpace(r.FormValue("smtp_user"))
		password := strings.TrimSpace(r.FormValue("smtp_pass"))
		data.Form.From = strings.TrimSpace(r.FormValue("from"))
		data.Form.Subject = strings.TrimSpace(r.FormValue("subject"))
		data.Form.Body = r.FormValue("body")
		data.Form.Recipients = strings.TrimSpace(r.FormValue("recipients"))

		if data.Form.SMTPServer == "" || data.Form.SMTPPort == "" || data.Form.SMTPUser == "" || data.Form.Subject == "" || data.Form.Body == "" || data.Form.Recipients == "" {
			data.Error = "All fields except From are required."
			renderTemplate(w, data)
			return
		}

		port, err := strconv.Atoi(data.Form.SMTPPort)
		if err != nil {
			data.Error = "SMTP Port must be a number."
			renderTemplate(w, data)
			return
		}

		recipients := parseRecipients(data.Form.Recipients)
		if len(recipients) == 0 {
			data.Error = "Provide at least one recipient email address."
			renderTemplate(w, data)
			return
		}

		from := data.Form.From
		if strings.TrimSpace(from) == "" {
			from = data.Form.SMTPUser
		}

		if password == "" {
			data.Error = "SMTP Password is required to send emails."
			renderTemplate(w, data)
			return
		}

		results := make([]sendResult, 0, len(recipients))
		for _, recipient := range recipients {
			msg := buildMessage(from, recipient, data.Form.Subject, data.Form.Body)
			err := sendEmail(data.Form.SMTPServer, port, data.Form.SMTPUser, password, from, recipient, []byte(msg))
			if err != nil {
				log.Printf("Failed to send email to %s: %v", recipient, err)
				results = append(results, sendResult{
					Recipient: recipient,
					Success:   false,
					Message:   err.Error(),
				})
				continue
			}

			results = append(results, sendResult{
				Recipient: recipient,
				Success:   true,
				Message:   "sent successfully",
			})
		}

		data.Results = results
	}

	renderTemplate(w, data)
}

func renderTemplate(w http.ResponseWriter, data pageData) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.Execute(w, data); err != nil {
		log.Printf("failed to render template: %v", err)
	}
}

func parseRecipients(input string) []string {
	lines := strings.Split(input, "\n")
	recipients := make([]string, 0, len(lines))
	for _, line := range lines {
		email := strings.TrimSpace(line)
		if email == "" {
			continue
		}
		recipients = append(recipients, email)
	}
	return recipients
}

func buildMessage(from, to, subject, body string) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("From: %s\r\n", from))
	sb.WriteString(fmt.Sprintf("To: %s\r\n", to))
	sb.WriteString(fmt.Sprintf("Subject: %s\r\n", subject))
	sb.WriteString("MIME-Version: 1.0\r\n")
	sb.WriteString("Content-Type: text/plain; charset=\"UTF-8\"\r\n")
	sb.WriteString("Content-Transfer-Encoding: 8bit\r\n\r\n")
	sb.WriteString(body)
	return sb.String()
}

func sendEmail(server string, port int, username, password, from, to string, msg []byte) error {
	auth := smtp.PlainAuth("", username, password, server)
	addr := fmt.Sprintf("%s:%d", server, port)
	return smtp.SendMail(addr, auth, from, []string{to}, msg)
}
