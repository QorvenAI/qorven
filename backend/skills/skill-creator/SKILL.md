---
name: skill-creator
description: Create or update Qorven agent skills with structured SKILL.md files, scripts, and references.
---

# Skill Creator

Use this skill when asked to create a new skill, update an existing skill, or extend agent capabilities.

## What is a Skill?

A skill is a directory containing a `SKILL.md` file that teaches an agent how to perform a specific task. Skills are injected into the agent's system prompt.

## Skill Structure

```
skills/<skill-name>/
├── SKILL.md          # Required — instructions for the agent
├── scripts/          # Optional — helper scripts the agent can exec
└── references/       # Optional — reference docs, examples
```

## SKILL.md Format

```yaml
---
name: my-skill-name
description: What this skill does (used for search)
---

# Skill Title

## When to Use
Describe when this skill should be activated.

## Instructions
Step-by-step instructions for the agent.

## Examples
Show example inputs and expected outputs.
```

## Creating a Skill

1. Create the directory: `mkdir -p skills/<name>`
2. Write `SKILL.md` with frontmatter (name, description) and instructions
3. Add helper scripts in `scripts/` if needed
4. Test by asking the agent to use the skill
5. Publish with `publish_skill(path: "skills/<name>")`

## Best Practices

- Keep instructions clear and specific
- Include examples of expected behavior
- Use `{baseDir}` placeholder for paths to skill files
- Test with multiple scenarios before publishing
- Never include secrets or API keys in skill content
