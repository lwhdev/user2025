# Bulk Mailer

A lightweight Go web app for sending a plain-text email to multiple recipients through your SMTP server. Run the server locally, open it in a browser, fill out the form, and click **Send Emails**.

## Getting started

1. Install Go 1.24 or newer.
2. Start the server:

   ```bash
   go run ./...
   ```

   The app listens on <http://localhost:8080> by default.

3. Open the URL in your browser. Provide the SMTP settings, compose your message, list recipients (one email address per line), and submit the form. The page reports success or failure for each recipient.

## Form fields

| Field | Description |
| --- | --- |
| SMTP Server | Hostname of your SMTP service (e.g. `smtp.example.com`). |
| SMTP Port | Port number (commonly `587` for STARTTLS or `465` for SMTPS). |
| SMTP Username | Username or email used to authenticate with the SMTP server. |
| SMTP Password | Password or app-specific token for the SMTP user. |
| From Address | Optional. If left empty, the SMTP username is used as the sender. |
| Subject | Subject line for the outgoing email. |
| Body | Plain-text body shared by every recipient. |
| Recipients | Provide one email address per line. Blank lines are ignored. |

## Notes

- Passwords are never persisted on the page; you must re-enter the value for each send.
- The server only sends plain-text emails and authenticates using the SMTP `PLAIN` mechanism via `net/smtp`.
- For bulk mail, ensure you comply with your provider's sending limits and anti-spam policies.
