# Bulk Mailer

A small Go command-line tool for sending templated emails to many recipients using an SMTP server.

## Features

- Reads recipient data from a CSV file (first row used as column headers).
- Renders the message body with Go's `text/template`, allowing per-recipient personalization.
- Supports a `--dry-run` mode to preview outgoing messages without sending them.
- Provides a helper `upper` template function for simple transformations.

## Installation

```bash
go build
```

This command produces a `bulkmailer` binary in the working directory.

## Usage

```bash
./bulkmailer \
  --smtp-server smtp.example.com \
  --smtp-port 587 \
  --smtp-username no-reply@example.com \
  --smtp-password "app-specific-password" \
  --from no-reply@example.com \
  --subject "Hello {{.name}}" \
  --body-template body.tmpl \
  --recipients recipients.csv
```

- `--subject` accepts literal text; you can embed template expressions by rendering them in the body template instead.
- `--body-template` points to a file using Go template syntax. Every column from the CSV becomes available in the template.
- `--recipients` should contain an `email` column plus any other data you want to reference in the template.
- `--dry-run` logs the generated messages without contacting the SMTP server.

### Example files

`recipients.csv`

```csv
email,name,favorite_color
alex@example.com,Alex,blue
casey@example.com,Casey,green
```

`body.tmpl`

```
Hi {{.name}},

Your favorite color is {{upper .favorite_color}}.

Best regards,
Bulk Mailer Bot
```

Run in dry-run mode while testing:

```bash
./bulkmailer --dry-run --smtp-server smtp.example.com --smtp-port 587 \
  --smtp-username no-reply@example.com --smtp-password ignored \
  --from no-reply@example.com --subject "Greetings" \
  --body-template body.tmpl --recipients recipients.csv
```

## Notes

- The tool currently supports plain-text messages and SMTP authentication via the `PLAIN` mechanism. Ensure your SMTP server supports this.
- When using `--dry-run`, the password flag can be left empty.
