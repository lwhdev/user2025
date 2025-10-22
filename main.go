package main

import (
	"encoding/csv"
	"errors"
	"fmt"
	"html/template"
	"io"
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
	Subject string
	Body    string
}

type sendResult struct {
	Sender    string
	Recipient string
	Success   bool
	Message   string
}

type senderConfig struct {
	Server   string
	Port     int
	Username string
	Password string
	From     string
}

type recipientEntry struct {
	Email string
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
    <p>Upload CSV files describing your sender accounts and recipients, compose the message once, and deliver it in bulk.</p>
    {{if .Error}}
    <p class="error">{{.Error}}</p>
    {{end}}
    <form method="post" action="/" enctype="multipart/form-data">
        <label>Sender Accounts CSV
            <input type="file" name="senders_file" accept=".csv" required>
            <small>Header columns: smtp_server,smtp_port,smtp_user,smtp_pass,from (from is optional).</small>
        </label>
        <label>Recipients CSV
            <input type="file" name="recipients_file" accept=".csv" required>
            <small>Header columns: email.</small>
        </label>
        <label>Subject
            <input type="text" name="subject" value="{{.Form.Subject}}" required>
        </label>
        <label>Body
            <textarea name="body" required>{{.Form.Body}}</textarea>
        </label>
        <button type="submit">Send Emails</button>
    </form>

    {{if .Results}}
    <div class="results">
        <h2>Send Results</h2>
        <ul>
            {{range .Results}}
            <li class="{{if .Success}}result-success{{else}}result-failure{{end}}">{{.Sender}} → {{.Recipient}} — {{.Message}}</li>
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
	data := pageData{}

	if r.Method == http.MethodPost {
		if err := r.ParseMultipartForm(10 << 20); err != nil {
			data.Error = fmt.Sprintf("failed to parse form: %v", err)
			renderTemplate(w, data)
			return
		}

		data.Form.Subject = strings.TrimSpace(r.FormValue("subject"))
		data.Form.Body = r.FormValue("body")

		if data.Form.Subject == "" || strings.TrimSpace(data.Form.Body) == "" {
			data.Error = "Subject and body are required."
			renderTemplate(w, data)
			return
		}

		sendersFile, _, err := r.FormFile("senders_file")
		if err != nil {
			data.Error = fmt.Sprintf("failed to read senders file: %v", err)
			renderTemplate(w, data)
			return
		}
		defer sendersFile.Close()

		senders, err := parseSendersCSV(sendersFile)
		if err != nil {
			data.Error = fmt.Sprintf("invalid senders CSV: %v", err)
			renderTemplate(w, data)
			return
		}
		if len(senders) == 0 {
			data.Error = "Provide at least one sender account in the CSV."
			renderTemplate(w, data)
			return
		}

		recipientsFile, _, err := r.FormFile("recipients_file")
		if err != nil {
			data.Error = fmt.Sprintf("failed to read recipients file: %v", err)
			renderTemplate(w, data)
			return
		}
		defer recipientsFile.Close()

		recipients, err := parseRecipientsCSV(recipientsFile)
		if err != nil {
			data.Error = fmt.Sprintf("invalid recipients CSV: %v", err)
			renderTemplate(w, data)
			return
		}

		if len(recipients) == 0 {
			data.Error = "Provide at least one recipient email address in the CSV."
			renderTemplate(w, data)
			return
		}

		results := make([]sendResult, 0, len(recipients))
		for i, recipient := range recipients {
			sender := senders[i%len(senders)]
			msg := buildMessage(sender.From, recipient.Email, data.Form.Subject, data.Form.Body)
			err := sendEmail(sender.Server, sender.Port, sender.Username, sender.Password, sender.From, recipient.Email, []byte(msg))
			if err != nil {
				log.Printf("Failed to send email from %s to %s: %v", sender.Username, recipient.Email, err)
				results = append(results, sendResult{
					Sender:    sender.Username,
					Recipient: recipient.Email,
					Success:   false,
					Message:   err.Error(),
				})
				continue
			}

			results = append(results, sendResult{
				Sender:    sender.Username,
				Recipient: recipient.Email,
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

func parseSendersCSV(r io.Reader) ([]senderConfig, error) {
	reader := csv.NewReader(r)
	reader.TrimLeadingSpace = true

	header, err := reader.Read()
	if err != nil {
		return nil, fmt.Errorf("reading header: %w", err)
	}

	colIndex := map[string]int{}
	for idx, col := range header {
		normalized := strings.ToLower(strings.TrimSpace(col))
		colIndex[normalized] = idx
	}

	required := []string{"smtp_server", "smtp_port", "smtp_user", "smtp_pass"}
	for _, col := range required {
		if _, ok := colIndex[col]; !ok {
			return nil, fmt.Errorf("missing required column %q", col)
		}
	}

	fromIdx, hasFrom := colIndex["from"]
	senders := []senderConfig{}

	for {
		record, err := reader.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("reading record: %w", err)
		}

		server := strings.TrimSpace(record[colIndex["smtp_server"]])
		portStr := strings.TrimSpace(record[colIndex["smtp_port"]])
		user := strings.TrimSpace(record[colIndex["smtp_user"]])
		pass := strings.TrimSpace(record[colIndex["smtp_pass"]])

		if server == "" || portStr == "" || user == "" || pass == "" {
			return nil, errors.New("sender rows must include smtp_server, smtp_port, smtp_user, and smtp_pass values")
		}

		port, err := strconv.Atoi(portStr)
		if err != nil {
			return nil, fmt.Errorf("invalid port %q for sender %s", portStr, user)
		}

		from := user
		if hasFrom {
			value := strings.TrimSpace(record[fromIdx])
			if value != "" {
				from = value
			}
		}

		senders = append(senders, senderConfig{
			Server:   server,
			Port:     port,
			Username: user,
			Password: pass,
			From:     from,
		})
	}

	return senders, nil
}

func parseRecipientsCSV(r io.Reader) ([]recipientEntry, error) {
	reader := csv.NewReader(r)
	reader.TrimLeadingSpace = true

	header, err := reader.Read()
	if err != nil {
		return nil, fmt.Errorf("reading header: %w", err)
	}

	emailIdx := -1
	for idx, col := range header {
		if strings.EqualFold(strings.TrimSpace(col), "email") {
			emailIdx = idx
			break
		}
	}

	if emailIdx == -1 {
		return nil, errors.New("missing required column \"email\"")
	}

	recipients := []recipientEntry{}
	for {
		record, err := reader.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("reading record: %w", err)
		}

		email := strings.TrimSpace(record[emailIdx])
		if email == "" {
			continue
		}

		recipients = append(recipients, recipientEntry{Email: email})
	}

	return recipients, nil
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
