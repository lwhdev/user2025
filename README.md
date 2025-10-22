# Bulk Mailer

A lightweight Go web app for sending a plain-text email to multiple recipients. Run the server locally, open it in a browser,
upload sender and recipient CSVs, compose the content, and click **Send Emails**.

## Getting started

1. Install Go 1.24 or newer.
2. Start the server:

   ```bash
   go run ./...
   ```

   The app listens on <http://localhost:8080> by default.

3. Open the URL in your browser. Upload the sender and recipient CSV files, compose your message once, and submit the form.
   The page reports success or failure for each pairing.

## Form fields

| Field | Description |
| --- | --- |
| Sender Accounts CSV | Upload a CSV with the SMTP settings for each sender. |
| Recipients CSV | Upload a CSV listing every recipient email address. |
| Subject | Subject line for the outgoing email. |
| Body | Plain-text body shared by every recipient. |

## CSV formats

### Sender accounts

The sender CSV **must** include a header row with the following columns:

| Column | Required | Description |
| --- | --- | --- |
| `smtp_server` | Yes | Hostname of the SMTP service (e.g. `smtp.example.com`). |
| `smtp_port` | Yes | Port number (commonly `587` for STARTTLS or `465` for SMTPS). |
| `smtp_user` | Yes | Username or email used to authenticate with the SMTP server. |
| `smtp_pass` | Yes | Password or app-specific token for the SMTP user. |
| `from` | No | Override the visible From address. Defaults to `smtp_user` when omitted. |

Example (`senders.csv`):

```csv
smtp_server,smtp_port,smtp_user,smtp_pass,from
smtp.example.com,587,sender1@example.com,app-password-1,Marketing Team <marketing@example.com>
smtp.example.com,587,sender2@example.com,app-password-2,
```

### Recipients

The recipients CSV **must** include a header row with an `email` column. Any additional columns are ignored.

Example (`recipients.csv`):

```csv
email
user1@example.com
user2@example.com
```

## Notes

- Passwords are never persisted on the page; you must re-upload the sender CSV for each send.
- The server sends plain-text emails and authenticates using the SMTP `PLAIN` mechanism via `net/smtp`.
- When multiple senders are provided, the app distributes recipients in a round-robin pattern across the accounts.
- For bulk mail, ensure you comply with your provider's sending limits and anti-spam policies.
