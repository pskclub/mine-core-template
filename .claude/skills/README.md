# Skills

An agent loads a skill by its `description` when a task matches it — nothing
here has to be invoked by hand, though in Claude Code `/mine-core-review` and
friends work too.

Each one is the *decisions this repository already made* about one subject, with
pointers to the file that demonstrates them. They are the short form; the long
form is [README.md](../../README.md) and the framework's own docs
(`mine-core-docs` explains how to reach those).

**This directory is the only copy.** A `SKILL.md` is agent-neutral — markdown
behind a `name` and a `description` — but each agent looks for skills somewhere
else, so `make skills` mirrors these to `.codex/skills/` (Codex's convention) as
stubs that point back here, and writes the index in
[AGENTS.md](../../AGENTS.md) for agents that have no skill mechanism at all.
Write the skill here; run `make skills` when its `name` or `description`
changed, and commit what changes. CI runs `make skills-check`.

| Skill | For |
|---|---|
| `mine-core-module` | a new feature, module, or endpoint; wiring; the import rules |
| `mine-core-http` | routes, handlers, binding, pagination, responses, postman |
| `mine-core-validation` | request rules, custom codes, nested and array payloads |
| `mine-core-errors` | `emsgs`, `IError`, statuses, mapping an upstream's failure |
| `mine-core-database` | prisma → model → testkit, stores, `Repo[M]`, the GORM traps |
| `mine-core-auth` | protecting routes, the token scheme, ownership, roles |
| `mine-core-jobs` | cron and background work, the worker role |
| `mine-core-testing` | testkit, the three tiers, what a module's tests must cover |
| `mine-core-requester` | calling another service, and surviving it |
| `mine-core-infra` | cache, storage, MQ, pub/sub, mail, push, health |
| `mine-core-config` | env keys, the `APP_` prefix traps, opening a backend |
| `mine-core-logging` | levels, boundaries, what never to log |
| `mine-core-workflow` | make targets, docker, migrations, lint, CI |
| `mine-core-review` | the pre-push checklist |
| `mine-core-docs` | finding the real mine-core API, docs and source |

## Keeping them true

A skill that contradicts the code is worse than no skill. When a convention
changes, the same commit changes the skill — they are short on purpose so that
stays cheap. Anything version-specific about mine-core belongs in
`mine-core-docs`, which resolves the module from `go.mod` rather than hard-coding
a version.
