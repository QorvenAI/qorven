## What this changes

One or two sentences — the *why*, not the *what*. The diff shows what.

## Related issue

Fixes #NNN / Closes #NNN / Addresses #NNN — or "new work, no issue".

## How it was tested

- [ ] `cd backend && go build ./...` passes.
- [ ] `cd backend && go test -short ./...` passes.
- [ ] `cd web && npx tsc --noEmit` passes (for frontend changes).
- [ ] Manual verification notes (what I clicked / what I saw):

## Anything reviewers should pay extra attention to?

Tricky parts, assumptions, things I wasn't sure about.

## Checklist

- [ ] Commit messages follow Conventional Commits (`feat(scope): …`).
- [ ] Public APIs have godoc comments.
- [ ] No hardcoded secrets, paths, or credentials.
- [ ] New config options are documented in `config.toml.example`.
