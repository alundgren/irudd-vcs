# irudd-vcs

## Project goal

This is a fork of Gitea that we are slimming down to a backend-only VCS server:
no web UI, no Actions, no workflows, no schedulers or other background automation.

The goal is a locally running VCS that works as a Gitea provider for
[t3code](https://github.com/pingdotgg/t3code) and similar software that
supports Gitea as a backend.

- Keep the APIs, so we can build a UI on top later
- Remove anything that a Gitea API client like t3code does not need

## Plan

Work through these phases in order. Do not jump ahead to a later phase.

1. **Audit**: catalog all functionality in the repo, then decide what to keep and what to remove
2. **Strip**: remove everything we do not need
3. **Test locally**: check that the slimmed server works as a Gitea backend
4. **Optimize**: improve performance, footprint and startup time

## Current rules

- We are in the local testing phase (3): running and fixing tests is allowed; do not start optimizing yet
- Work directly on `main`. Do not open pull requests for now

## Conventions

- Use `make help` to find available development targets
- Run `make fmt` to format `.go` files, and run `make lint-go` to lint them
- Run `make tidy` after any `go.mod` changes
- Add the current year into the copyright header of new `.go` files
- Ensure no trailing whitespace in edited files
- Use Conventional Commits for commit messages, e.g. `type(scope): subject`; `!` before the colon if breaking
- Never force-push, amend, or squash unless asked. Use new commits and a normal push
- Preserve existing code comments, do not remove or rewrite comments that are still relevant
- Keep comments short, prefer same-line, explain why, never narrate code
- Include authorship attribution in issue and pull request comments
- Always add `Assisted-By` trailers to commit messages in format `Assisted-by: AGENT_NAME:MODEL_VERSION`
- Never add `Co-Authored-By` `Signed-off-by` trailer to commit messages. Sign off must be done by a human.
