package main

import (
	"archive/zip"
	"bytes"
	"encoding/csv"
	"encoding/xml"
	"errors"
	"fmt"
	"html/template"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"net/smtp"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"unicode"
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

type smtpProvider struct {
	Server string
	Port   int
}

var smtpProviders = map[string]smtpProvider{
	"gmail.com":      {Server: "smtp.gmail.com", Port: 587},
	"googlemail.com": {Server: "smtp.gmail.com", Port: 587},
	"outlook.com":    {Server: "smtp.office365.com", Port: 587},
	"hotmail.com":    {Server: "smtp.office365.com", Port: 587},
	"live.com":       {Server: "smtp.office365.com", Port: 587},
	"office365.com":  {Server: "smtp.office365.com", Port: 587},
	"qq.com":         {Server: "smtp.qq.com", Port: 587},
	"163.com":        {Server: "smtp.163.com", Port: 587},
	"126.com":        {Server: "smtp.126.com", Port: 587},
	"yeah.net":       {Server: "smtp.yeah.net", Port: 587},
	"sina.com":       {Server: "smtp.sina.com", Port: 587},
	"sina.cn":        {Server: "smtp.sina.com", Port: 587},
	"aliyun.com":     {Server: "smtp.aliyun.com", Port: 587},
	"aliyun.cn":      {Server: "smtp.aliyun.com", Port: 587},
	"yahoo.com":      {Server: "smtp.mail.yahoo.com", Port: 587},
	"yahoo.com.cn":   {Server: "smtp.mail.yahoo.com", Port: 587},
	"yahoo.com.tw":   {Server: "smtp.mail.yahoo.com", Port: 587},
	"yahoo.com.hk":   {Server: "smtp.mail.yahoo.com", Port: 587},
	"yahoo.co.jp":    {Server: "smtp.mail.yahoo.co.jp", Port: 587},
	"icloud.com":     {Server: "smtp.mail.me.com", Port: 587},
	"me.com":         {Server: "smtp.mail.me.com", Port: 587},
	"mac.com":        {Server: "smtp.mail.me.com", Port: 587},
	"foxmail.com":    {Server: "smtp.qq.com", Port: 587},
	"gmail.cn":       {Server: "smtp.gmail.com", Port: 587},
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
    <p>Upload spreadsheets describing your sender accounts and recipients, compose the message once, and deliver it in bulk.</p>
    {{if .Error}}
    <p class="error">{{.Error}}</p>
    {{end}}
    <form method="post" action="/" enctype="multipart/form-data">
        <label>Sender Accounts File
            <input type="file" name="senders_file" accept=".csv,.xlsx" required>
            <small>Include header columns for the sender email and its authorization code (for example: email,auth_code).</small>
        </label>
        <label>Recipients File
            <input type="file" name="recipients_file" accept=".csv,.xlsx" required>
            <small>Provide at least one column named email.</small>
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

		sendersFile, sendersHeader, err := r.FormFile("senders_file")
		if err != nil {
			data.Error = fmt.Sprintf("failed to read senders file: %v", err)
			renderTemplate(w, data)
			return
		}
		defer sendersFile.Close()

		senders, err := parseSendersUpload(sendersFile, sendersHeader.Filename)
		if err != nil {
			data.Error = fmt.Sprintf("invalid senders file: %v", err)
			renderTemplate(w, data)
			return
		}
		if len(senders) == 0 {
			data.Error = "Provide at least one sender account in the uploaded file."
			renderTemplate(w, data)
			return
		}

		recipientsFile, recipientsHeader, err := r.FormFile("recipients_file")
		if err != nil {
			data.Error = fmt.Sprintf("failed to read recipients file: %v", err)
			renderTemplate(w, data)
			return
		}
		defer recipientsFile.Close()

		recipients, err := parseRecipientsUpload(recipientsFile, recipientsHeader.Filename)
		if err != nil {
			data.Error = fmt.Sprintf("invalid recipients file: %v", err)
			renderTemplate(w, data)
			return
		}

		if len(recipients) == 0 {
			data.Error = "Provide at least one recipient email address in the uploaded file."
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

func parseSendersUpload(file multipart.File, filename string) ([]senderConfig, error) {
	data, err := io.ReadAll(file)
	if err != nil {
		return nil, fmt.Errorf("reading senders file: %w", err)
	}

	return parseSendersData(data, filename)
}

func parseSendersData(data []byte, filename string) ([]senderConfig, error) {
	ext := strings.ToLower(filepath.Ext(filename))
	switch ext {
	case ".xlsx":
		return parseSendersFromXLSX(data)
	case ".csv", "":
		return parseSendersFromCSV(data)
	default:
		return nil, fmt.Errorf("unsupported file extension %q", ext)
	}
}

func parseSendersFromCSV(data []byte) ([]senderConfig, error) {
	reader := csv.NewReader(bytes.NewReader(data))
	reader.TrimLeadingSpace = true

	rows, err := reader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("reading CSV: %w", err)
	}
	if len(rows) == 0 {
		return nil, errors.New("file does not contain a header row")
	}

	header := rows[0]
	body := [][]string{}
	if len(rows) > 1 {
		body = rows[1:]
	}

	return buildSendersFromTable(header, body)
}

func parseSendersFromXLSX(data []byte) ([]senderConfig, error) {
	rows, err := readXLSXTable(data)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, errors.New("sheet does not contain a header row")
	}

	header := rows[0]
	body := [][]string{}
	if len(rows) > 1 {
		body = rows[1:]
	}

	return buildSendersFromTable(header, body)
}

func buildSendersFromTable(header []string, rows [][]string) ([]senderConfig, error) {
	colIndex := buildColumnIndex(header)

	emailIdx, ok := findColumn(colIndex, "email", "senderemail", "account", "username", "from")
	if !ok {
		return nil, errors.New("missing required column for sender email")
	}

	passIdx, ok := findColumn(colIndex, "authcode", "authorizationcode", "authpassword", "password", "passcode", "apppassword", "authorization", "code")
	if !ok {
		return nil, errors.New("missing required column for sender authorization code")
	}

	senders := []senderConfig{}
	for _, row := range rows {
		email := getValue(row, emailIdx)
		authCode := getValue(row, passIdx)

		if email == "" && authCode == "" {
			continue
		}
		if email == "" || authCode == "" {
			return nil, fmt.Errorf("sender row must include both email and authorization code")
		}

		server, port, err := determineSMTPSettings(email)
		if err != nil {
			return nil, err
		}

		senders = append(senders, senderConfig{
			Server:   server,
			Port:     port,
			Username: email,
			Password: authCode,
			From:     email,
		})
	}

	return senders, nil
}

func parseRecipientsUpload(file multipart.File, filename string) ([]recipientEntry, error) {
	data, err := io.ReadAll(file)
	if err != nil {
		return nil, fmt.Errorf("reading recipients file: %w", err)
	}

	ext := strings.ToLower(filepath.Ext(filename))
	switch ext {
	case ".xlsx":
		return parseRecipientsFromXLSX(data)
	case ".csv", "":
		return parseRecipientsFromCSV(data)
	default:
		return nil, fmt.Errorf("unsupported file extension %q", ext)
	}
}

func parseRecipientsFromCSV(data []byte) ([]recipientEntry, error) {
	reader := csv.NewReader(bytes.NewReader(data))
	reader.TrimLeadingSpace = true

	rows, err := reader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("reading CSV: %w", err)
	}
	if len(rows) == 0 {
		return nil, errors.New("file does not contain a header row")
	}

	header := rows[0]
	body := [][]string{}
	if len(rows) > 1 {
		body = rows[1:]
	}

	return buildRecipientsFromTable(header, body)
}

func parseRecipientsFromXLSX(data []byte) ([]recipientEntry, error) {
	rows, err := readXLSXTable(data)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, errors.New("sheet does not contain a header row")
	}

	header := rows[0]
	body := [][]string{}
	if len(rows) > 1 {
		body = rows[1:]
	}

	return buildRecipientsFromTable(header, body)
}

func buildRecipientsFromTable(header []string, rows [][]string) ([]recipientEntry, error) {
	colIndex := buildColumnIndex(header)

	emailIdx, ok := findColumn(colIndex, "email", "recipient", "recipientemail")
	if !ok {
		return nil, errors.New("missing required column for recipient email")
	}

	recipients := []recipientEntry{}
	for _, row := range rows {
		email := getValue(row, emailIdx)
		if email == "" {
			continue
		}
		recipients = append(recipients, recipientEntry{Email: email})
	}

	return recipients, nil
}

func readXLSXTable(data []byte) ([][]string, error) {
	reader := bytes.NewReader(data)
	zipReader, err := zip.NewReader(reader, int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("opening XLSX: %w", err)
	}

	sheetPath, err := firstWorksheetPath(zipReader)
	if err != nil {
		return nil, err
	}

	sheetFile := findFile(zipReader, sheetPath)
	if sheetFile == nil {
		return nil, fmt.Errorf("worksheet %q not found", sheetPath)
	}

	sharedStrings, err := loadSharedStrings(zipReader)
	if err != nil {
		return nil, err
	}

	return readWorksheet(sheetFile, sharedStrings)
}

func firstWorksheetPath(zr *zip.Reader) (string, error) {
	workbookFile := findFile(zr, "xl/workbook.xml")
	if workbookFile == nil {
		if defaultSheet := findFile(zr, "xl/worksheets/sheet1.xml"); defaultSheet != nil {
			return defaultSheet.Name, nil
		}
		return "", errors.New("workbook metadata missing")
	}

	data, err := readZipFile(workbookFile)
	if err != nil {
		return "", fmt.Errorf("reading workbook metadata: %w", err)
	}

	var doc workbookDocument
	if err := xml.Unmarshal(data, &doc); err != nil {
		return "", fmt.Errorf("parsing workbook metadata: %w", err)
	}
	if len(doc.Sheets) == 0 {
		return "", errors.New("workbook does not contain any sheets")
	}

	rels, err := loadWorkbookRelationships(zr)
	if err != nil {
		return "", err
	}

	first := doc.Sheets[0]
	if target, ok := rels[first.RelID]; ok {
		cleaned := path.Clean(path.Join("xl", target))
		return cleaned, nil
	}

	// Fall back to the conventional location when relationships are missing.
	candidate := path.Clean(path.Join("xl", fmt.Sprintf("worksheets/%s.xml", strings.ToLower(first.Name))))
	if file := findFile(zr, candidate); file != nil {
		return file.Name, nil
	}

	return "", fmt.Errorf("unable to resolve worksheet for id %q", first.RelID)
}

func loadWorkbookRelationships(zr *zip.Reader) (map[string]string, error) {
	relFile := findFile(zr, "xl/_rels/workbook.xml.rels")
	if relFile == nil {
		return map[string]string{}, nil
	}

	data, err := readZipFile(relFile)
	if err != nil {
		return nil, fmt.Errorf("reading workbook relationships: %w", err)
	}

	var doc workbookRelationships
	if err := xml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("parsing workbook relationships: %w", err)
	}

	rels := make(map[string]string, len(doc.Relationships))
	for _, rel := range doc.Relationships {
		rels[rel.ID] = rel.Target
	}
	return rels, nil
}

func loadSharedStrings(zr *zip.Reader) ([]string, error) {
	sharedFile := findFile(zr, "xl/sharedStrings.xml")
	if sharedFile == nil {
		return nil, nil
	}

	rc, err := sharedFile.Open()
	if err != nil {
		return nil, fmt.Errorf("opening shared strings: %w", err)
	}
	defer rc.Close()

	decoder := xml.NewDecoder(rc)
	var (
		results  []string
		builder  strings.Builder
		inText   bool
		inShared bool
	)

	for {
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("parsing shared strings: %w", err)
		}

		switch t := token.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "si":
				builder.Reset()
				inShared = true
			case "t":
				if inShared {
					inText = true
				}
			}
		case xml.CharData:
			if inText {
				builder.Write([]byte(t))
			}
		case xml.EndElement:
			switch t.Name.Local {
			case "t":
				inText = false
			case "si":
				results = append(results, builder.String())
				builder.Reset()
				inShared = false
			}
		}
	}

	return results, nil
}

func readWorksheet(file *zip.File, shared []string) ([][]string, error) {
	rc, err := file.Open()
	if err != nil {
		return nil, fmt.Errorf("opening worksheet: %w", err)
	}
	defer rc.Close()

	decoder := xml.NewDecoder(rc)
	var (
		rows    [][]string
		current []string
		inRow   bool
		lastCol int
	)

	for {
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("parsing worksheet: %w", err)
		}

		switch t := token.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "row":
				inRow = true
				current = []string{}
				lastCol = -1
			case "c":
				if !inRow {
					if err := decoder.Skip(); err != nil {
						return nil, fmt.Errorf("skipping cell: %w", err)
					}
					continue
				}

				var cell xlsxCell
				if err := decoder.DecodeElement(&cell, &t); err != nil {
					return nil, fmt.Errorf("decoding cell: %w", err)
				}

				colIdx := cell.columnIndex(lastCol + 1)
				if colIdx > lastCol {
					lastCol = colIdx
				}

				for len(current) <= colIdx {
					current = append(current, "")
				}
				current[colIdx] = cell.value(shared)
			}
		case xml.EndElement:
			if t.Name.Local == "row" {
				rows = append(rows, trimTrailingEmpty(current))
				inRow = false
			}
		}
	}

	return rows, nil
}

func trimTrailingEmpty(values []string) []string {
	last := len(values)
	for last > 0 && values[last-1] == "" {
		last--
	}
	return values[:last]
}

type workbookDocument struct {
	Sheets []workbookSheet `xml:"sheets>sheet"`
}

type workbookSheet struct {
	Name  string `xml:"name,attr"`
	RelID string `xml:"http://schemas.openxmlformats.org/officeDocument/2006/relationships id,attr"`
}

type workbookRelationships struct {
	Relationships []workbookRelationship `xml:"Relationship"`
}

type workbookRelationship struct {
	ID     string `xml:"Id,attr"`
	Target string `xml:"Target,attr"`
}

type xlsxCell struct {
	Ref    string       `xml:"r,attr"`
	Type   string       `xml:"t,attr"`
	Value  string       `xml:"v"`
	Inline inlineString `xml:"is"`
}

type inlineString struct {
	Raw string `xml:",innerxml"`
}

func (is inlineString) String() string {
	if is.Raw == "" {
		return ""
	}

	decoder := xml.NewDecoder(strings.NewReader(is.Raw))
	var (
		builder strings.Builder
		inText  bool
	)

	for {
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return builder.String()
		}

		switch t := token.(type) {
		case xml.StartElement:
			if t.Name.Local == "t" {
				inText = true
			}
		case xml.CharData:
			if inText {
				builder.Write([]byte(t))
			}
		case xml.EndElement:
			if t.Name.Local == "t" {
				inText = false
			}
		}
	}

	return builder.String()
}

func (c xlsxCell) columnIndex(defaultIndex int) int {
	if c.Ref == "" {
		return defaultIndex
	}

	lettersEnd := 0
	for lettersEnd < len(c.Ref) {
		r := rune(c.Ref[lettersEnd])
		if unicode.IsLetter(r) {
			lettersEnd++
			continue
		}
		break
	}

	if lettersEnd == 0 {
		return defaultIndex
	}

	column := 0
	for _, r := range strings.ToUpper(c.Ref[:lettersEnd]) {
		if r < 'A' || r > 'Z' {
			return defaultIndex
		}
		column = column*26 + int(r-'A'+1)
	}

	return column - 1
}

func (c xlsxCell) value(shared []string) string {
	switch c.Type {
	case "s":
		idx, err := strconv.Atoi(strings.TrimSpace(c.Value))
		if err != nil || idx < 0 || idx >= len(shared) {
			return ""
		}
		return shared[idx]
	case "b":
		if strings.TrimSpace(c.Value) == "1" {
			return "TRUE"
		}
		return "FALSE"
	case "inlineStr":
		return c.Inline.String()
	case "str":
		return strings.TrimSpace(c.Value)
	default:
		if c.Inline.Raw != "" {
			return c.Inline.String()
		}
		return strings.TrimSpace(c.Value)
	}
}

func findFile(zr *zip.Reader, name string) *zip.File {
	lowerName := strings.ToLower(name)
	for _, file := range zr.File {
		if strings.ToLower(file.Name) == lowerName {
			return file
		}
	}
	return nil
}

func readZipFile(file *zip.File) ([]byte, error) {
	rc, err := file.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()

	return io.ReadAll(rc)
}

func buildColumnIndex(header []string) map[string]int {
	index := make(map[string]int)
	for idx, col := range header {
		key := normalizeHeader(col)
		if key == "" {
			continue
		}
		if _, exists := index[key]; !exists {
			index[key] = idx
		}
	}
	return index
}

func normalizeHeader(value string) string {
	normalized := strings.ToLower(strings.TrimSpace(value))
	normalized = strings.ReplaceAll(normalized, " ", "")
	normalized = strings.ReplaceAll(normalized, "-", "")
	normalized = strings.ReplaceAll(normalized, "_", "")
	return normalized
}

func findColumn(index map[string]int, keys ...string) (int, bool) {
	for _, key := range keys {
		if idx, ok := index[normalizeHeader(key)]; ok {
			return idx, true
		}
	}
	return 0, false
}

func getValue(row []string, idx int) string {
	if idx < 0 || idx >= len(row) {
		return ""
	}
	return strings.TrimSpace(row[idx])
}

func determineSMTPSettings(email string) (string, int, error) {
	parts := strings.Split(email, "@")
	if len(parts) != 2 {
		return "", 0, fmt.Errorf("invalid email address %q", email)
	}

	domain := strings.ToLower(strings.TrimSpace(parts[1]))
	if domain == "" {
		return "", 0, fmt.Errorf("invalid email address %q", email)
	}

	if provider, ok := smtpProviders[domain]; ok {
		return provider.Server, provider.Port, nil
	}

	return fmt.Sprintf("smtp.%s", domain), 587, nil
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
