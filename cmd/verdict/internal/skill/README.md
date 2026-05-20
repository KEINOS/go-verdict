# Verdict Skill

This directory contains the canonical AI Agent skill for using the `verdict`
command with Go benchmark results.

Use it when you want an agent to choose the right `verdict` workflow, run the
command, and interpret the outcome without loading the full project README.

To export the skill from an installed `verdict` command:

```sh
verdict skill > SKILL.md
```

The exported text should match [SKILL.md](SKILL.md).
