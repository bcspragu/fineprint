# Fineprint

A tool for determining what has changed in a company's legal agreement (ToS, Privacy Policy, etc), built for [Postmark's Inbox Innovators challenge](https://postmarkapp.com/blog/announcing-the-postmark-challenge-inbox-innovators%20)

## Local Testing

### Dependencies

To run the service locally, you'll need all of the following:

- A [Go](https://go.dev/) compiler (likely 1.24.x)
- A [Node](https://nodejs.org/) environment (I'm using Node 22)
  - `mjml` needs to be installed in this env, for formatting emails
- Postmark credentials
- Internet Archive keys
- An Anthropic API key

### Running

`./scripts/run.sh` is a helper for running the service, but it assumes you have your credentials stored in `pass` under specific names.

```bash
curl -v \
  -u $(pass show postmark/webhook-username):$(pass show postmark/webhook-password) \
  --data @json-body.json \
  -H 'X-Forwarded-For: 127.0.0.1' \
  localhost:8080/webhook
```

## Limitations

- If the legal document has changed a lot, the diff may be very large and overflow our LLM context, so we trim it down to size.
  - This stops it from breaking, but means we might not be capturing all the changes.

## Usage with Docker

```bash
# Build the image
docker build -t fineprint .

# Run it, pulling secrets from `pass` in this example
docker run -it --rm \
  -p 8080:8080 fineprint \
  --reply-from-email=app@fineprint.help \
  --postmark-server-token=$(pass show postmark/server-token) \
  --postmark-webhook-username=$(pass show postmark/webhook-username) \
  --postmark-webhook-password=$(pass show postmark/webhook-password) \
  --anthropic-api-key=$(pass show llm/anthropic) \
  --archive-access-key=$(pass show internetarchive/access_key) \
  --archive-secret-key=$(pass show internetarchive/secret_key)
```

## TODO

- [ ] Follow redirects
- [ ] Add rate-limiting
  - Separately for sent emails and overall requests
    - Because not all requests will send emails, and emails aren't super expensive but LLMs can be
- [ ] Let the user know when we had to trim the diff down
  - Alternatively, chunk up the diff and reassemble it after
  - Though this could get arbitrarily expensive
- [ ] Add a landing page for the site
- [ ] Deploy it
- [ ] Add links to the previous and current policies
- [ ] Add a warning that summaries may be wrong and you should always dig in if it's important
- [x] Add LICENSE
- [x] Fill out README
- [x] Test end-to-end
- [x] Iterate on email format
- [x] Add text email response back
