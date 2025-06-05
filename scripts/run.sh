#!/bin/bash


go run . \
  --reply-from-email=app@fineprint.help \
  --postmark-server-token=$(pass show postmark/server-token) \
  --postmark-webhook-username=$(pass show postmark/webhook-username) \
  --postmark-webhook-password=$(pass show postmark/webhook-password) \
  --anthropic-api-key=$(pass show llm/anthropic) \
  --archive-access-key=$(pass show internetarchive/access_key) \
  --archive-secret-key=$(pass show internetarchive/secret_key)
  
