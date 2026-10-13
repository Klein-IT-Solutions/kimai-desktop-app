# Security policy

## Reporting a vulnerability

Please do **not** open a public issue for security problems.

Report them privately via GitHub's
[private vulnerability reporting](https://github.com/klein-it-solutions/kimai-desktop-app/security/advisories/new)
or by e-mail to the contact listed on [klein-it.solutions](https://klein-it.solutions).
Include the affected version (*Settings → Info*), your platform and steps to reproduce.

We aim to acknowledge reports within a few working days and will keep you informed
until a fix is released.

## Supported versions

Security fixes are made for the latest release only.

## Scope notes

- The Kimai API token and the speech API key are stored in the system keyring (or, if you
  explicitly choose so, in `~/.config/kimai-tracker/secrets.json` with mode 0600). They are
  never sent to the user interface or to any server other than the one you configured.
- Downloads (whisper models, llama.cpp, language models) come from the official upstream
  locations (Hugging Face, the llama.cpp GitHub releases) unless you set a mirror via the
  documented environment variables.
