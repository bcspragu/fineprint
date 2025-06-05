# Fineprint

A tool for determining what has changed in a company's legal agreement (ToS, Privacy Policy, etc), built for [Postmark's Inbox Innovators challenge](https://postmarkapp.com/blog/announcing-the-postmark-challenge-inbox-innovators%20)

## Local Testing

```
curl -v \
  -u (pass show postmark/webhook-username):(pass show postmark/webhook-password) \
  --data @json-body.json \
  -H 'X-Forwarded-For: 127.0.0.1' \
  localhost:8080/webhook
```

## TODO

- [ ] Add links to the previous and current policies
- [x] Add LICENSE
- [x] Fill out README
- [x] Test end-to-end
- [x] Iterate on email format
- [x] Add text email response back
