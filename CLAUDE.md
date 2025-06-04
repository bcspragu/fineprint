We're building a tool for let users know how a company's Terms of Service (ToS) or Privacy Policy (or similar legal document) has changed.

# Tools used

- **Postmark** - Used for Inbound Email processing to receive webhooks of emails from users, and the API is used to send reply emails.
  - The JSON-formatted request payload is available here: https://postmarkapp.com/developer/user-guide/inbound/parse-an-email
  - Lives in `postmark/` directory
- **Claude API** - Used for confirming the email is a policy change email, and extracting metadata. Also used for creating summaries of the diff.
  - Lives in `claude/` directory
- **ToS;DR** - Used for getting the grade of user agreements, as well as summaries. Also used to help find links to agreement documents.
  - Lives in `tosdr/` directory
- **MJML** - Used for making our email replies nicely formatted + responsive.
  - We shell out to it from our main Go service
  - Our `Dockerfile` includes the relevant Node environment for running in production
  - The template + shelling out code lives in `templates/` the script itself lives in `mjml/`


The workflow is:

1. User receives some "We've updated our Terms" email
2. They forward it to our bot
3. We receive the email as JSON in our webhook handler
4. We confirm it's a "legal agreement update" email and extract metadata using LLMs
5. We see if there's a ToS;DR entry for the site
6. We attempt to load a previous version of the legal agreement
7. We diff the two versions
8. We use LLMs to write up a summary of the diff

## Features

- [DONE] Make sure the email is something we can handle (e.g. a policy update)
- [DONE] Extract information about the policy update
- [DONE] Use the  ToS;DR API to see if this ToS has been summarized
- [DONE] Load the changed entity (ToS, PP, etc)
- [DONE] Load an old version (e.g. from archive.org)
- [DONE] Diff them
- [DONE] Have an LLM summarize the delta
