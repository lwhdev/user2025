# Bulk Mailer

A lightweight Go web app for sending a plain-text email to multiple recipients. Run the server locally, open it in a browser,
upload sender and recipient spreadsheets (CSV or XLSX), compose the content, and click **Send Emails**.

## Getting started

1. Install Go 1.24 or newer.
2. Start the server:

   ```bash
   go run ./...
   ```

   The app listens on <http://localhost:8080> by default.

3. Open the URL in your browser. Upload the sender and recipient files, compose your message once, and submit the form.
   The page reports success or failure for each pairing.

## Form fields

| Field | Description |
| --- | --- |
| Sender Accounts File | Upload a CSV or XLSX file that lists each sender email and its authorization code/app password. |
| Recipients File | Upload a CSV or XLSX file listing every recipient email address. |
| Subject | Subject line for the outgoing email. |
| Body | Plain-text body shared by every recipient. |
## File formats

### Sender accounts

The sender file **must** include a header row with the following columns (column names are matched case-insensitively and can
contain spaces or underscores):

| Column | Required | Description |
| --- | --- | --- |
| `email` | Yes | The email address that will send mail. |
| `auth_code` | Yes | The provider-specific authorization/app password for the email address. Aliases such as `password` or `auth code` are also accepted. |

Example (`senders.xlsx`):

| email | auth_code |
| --- | --- |
| sender1@gmail.com | abcd efgh ijkl mnop |
| sender2@qq.com | app-password-2 |

The app automatically determines the SMTP server host and port based on each sender's email domain. Gmail, QQ, Outlook,
Yahoo, Aliyun, and other popular providers are recognised out of the box. Domains that are not in the built-in list default to
`smtp.<domain>` on port `587`.

### Recipients

The recipient file **must** include a header row with an `email` column. Any additional columns are ignored.

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
