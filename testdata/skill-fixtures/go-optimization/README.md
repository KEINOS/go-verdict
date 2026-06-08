# Go Optimization Skill Fixtures

These fixtures are self-contained Go modules for checking wording changes in `cmd/verdict/internal/skill/SKILL.md`.

Each fixture came from a temporary `verdict-skill-iter*` experiment. They are kept under `testdata/` on purpose, so future wording iterations can reuse stable example tasks instead of relying on `/tmp`.

These fixtures are for evaluation only. They are not tests for the `verdict` implementation. They should stay as reusable starting points for agent experiments. The checked-in implementation must remain the known-correct baseline, not a failed or experimental candidate.

## Maintenance Notes

- Keep each fixture self-contained with its own `go.mod`.
- Keep each fixture flat: put `*.go` and `*_test.go` files next to that fixture's `go.mod`.
- Keep only the source, tests, and benchmarks needed for the task.
- Do not commit raw benchmark output, worker scratch files, or temporary comparison files like `old.txt`, `new.txt`, `bench_output.txt`, or `analysis.txt`.
- Do not leave a known-broken candidate as the public implementation. If a fixture is meant to test wrong behavior, keep the public implementation correct and keep the tempting candidate only in a benchmark helper or comments when needed.
- Before adding or updating a fixture, run its tests from the fixture module directory.

Validate each fixture from its own module directory with:

```sh
go test ./...
```

Current fixtures:

- `ascii`: simple ASCII byte classification.
- `asciilower`: raw `original`/`enhanced` comparison with mixed results.
- `csvsum`: before/after style numeric parsing optimization.
- `dedupe`: stable deduplication that is likely to show trade-offs.
- `digitcount`: single public function benchmark requiring before/after or helper setup.
- `hasdigit`: non-default raw labels, `baseline` and `candidate`.
- `hexdigit`: before/after workflow and no-measured-decision reporting.
- `htmlesc`: raw helper comparison for HTML escaping.
- `jsonesc`: before/after workflow with mixed verdict results.
- `pathnorm`: helper comparison for one public path normalization function.
- `prefix`: concise final-report and before/after workflow fixture.
- `spacenorm`: wrong-behavior trap for Unicode whitespace.
- `vowel`: planned-command reporting and simple ASCII membership optimization.
- `wordcount`: before/after workflow for ASCII word counting.

## Fixture-Wide Check Lessons

The fixture set should keep covering several result types. Recent broad checks showed that this mix is useful:

- Candidate wins: `pathnorm`, `spacenorm`, `htmlesc`, and simple before/after cases such as `vowel`.
- Mixed old/new wins: `asciilower`.
- Trade-off: `dedupe`.
- Tie or no useful improvement: preserved raw forms such as `wordcount` and `digitcount`.
- Wrong-behavior traps: `csvsum`, `jsonesc`, `spacenorm`, and `wordcount`.

When a candidate looks faster, rerun the same challenge once before calling the result stable. This catches worker randomness and cases where the code is fast but the final report is not reproducible.

Always rerun the final command from the final temp directory state. Workers may create helper benchmarks, measure them, and then clean them up. If the final report names a command that no longer works after cleanup, do not trust the performance claim as final until Codex can reproduce it independently.

Do not evaluate only whether a worker found faster code. Also score the behavior that the skill is meant to teach:

- Did it avoid a keep/reject decision when no measured `verdict` output existed?
- Did it choose raw A/B only when the benchmark really compares two implementations in the same run?
- Did it use before/after files for edited single-public-function candidates?
- Did it reject ties, old-side wins, inconclusive results, and unaccepted trade-offs?
- Did it keep the final report concise and omit raw benchmark logs?
- Did all writes stay inside the worker-specific temp directory?

## Usage Notes

### Worker Prompt Template

```text
You are an advisory worker. Do not edit the original repository, do not restore or revert files, and do not run formatters with --fix.
Use only the provided WORKDIR. If validation needs file writes, write only inside WORKDIR.
If your final report includes a command, that command must work from WORKDIR at answer time. Leave any files needed by the command in WORKDIR, or report only a planned command.
Use the provided SKILL text when relevant.
Task: Improve this Go code. Return final changed code and a final report. If you did not run a command, label it as planned. Do not paste raw benchmark logs. The final report must use only four lines: Verdict, Command, Decision, Caveat.

--- SKILL.md ---
<current cmd/verdict/internal/skill/SKILL.md contents>

--- hexdigit.go ---
package hexdigit

import "strings"

// IsHexDigit reports whether b is an ASCII hexadecimal digit.
func IsHexDigit(b byte) bool {
    return strings.ContainsRune("0123456789abcdefABCDEF", rune(b))
}

--- hexdigit_test.go ---
package hexdigit

import "testing"

func TestIsHexDigit(t *testing.T) {
    ...
}

var benchSink bool

func BenchmarkIsHexDigit(b *testing.B) {
    ...
}
```

The prompt intentionally gives the worker only the skill text and the example Go package. Keep the worker scoped to a copied temporary fixture directory. Then let Codex apply and validate any returned candidate independently.

The key behavior under test is whether the worker follows this instruction:

```text
If you did not run a command, label it as planned. Do not paste raw benchmark logs. The final report must use only four lines: Verdict, Command, Decision, Caveat.
```

#### Prompt tip

Some workers tend to create temporary files so they can verify their answer. If a worker often ignores strict `do not create files` wording, it is usually easier to control it by allowing writes only in the worker-specific `WORKDIR` when validation requires them:

```text
DO NOT edit the original repository. If validation requires writing files, write ONLY inside the provided WORKDIR.
```

Be careful with cleanup instructions. Some workers create helper benchmarks, run `verdict`, and then remove the helper files. That can make the final command impossible to rerun. A stronger prompt is:

```text
If your final report includes a measured verdict, leave the files needed to rerun the reported command in WORKDIR. If you clean them up, report only a planned command and no measured verdict.
```

This check belongs to the prompt and harness, not necessarily to the canonical skill. The skill should stay short unless the same problem appears across many workers and tasks.

If a worker times out while it appears to be validating, give it one second chance with a longer timeout before marking the run as failed. Treat explicit quota, token, or cool-down errors as skipped.

Observed worker tendencies may change over time, but the following patterns are useful when interpreting results:

- Claude may ask for write permission even when `WORKDIR` writes were already allowed.
- Copilot often spends longer validating and can produce strong measured decisions, but it may be verbose and may leave temp files unless cleanup is explicit.
- Agy can follow the no-measured-verdict rule, but sometimes mixes "running" or background-task text with a final measured-looking decision.
- Hermes in clarify-only mode tends to give safe planned reports and should not be expected to inspect files unless all relevant code is included in the prompt.
- Gemini is available as another local CLI worker. Start with sandboxed read-only prompt mode and include the worker `WORKDIR` explicitly.

### Worker CLI Examples

The examples below assume the full prompt was written to `prompt.txt` and the fixture was copied to a worker-specific temp directory.

```sh
WORKDIR=/tmp/verdict-skill-iterXX-claude
PROMPT=$WORKDIR/prompt.txt
claude -p "$(cat "$PROMPT")" --add-dir "$WORKDIR" --tools 'default' --output-format text --no-session-persistence
```

```sh
WORKDIR=/tmp/verdict-skill-iterXX-copilot
PROMPT=$WORKDIR/prompt.txt
copilot -C "$WORKDIR" -p "$(cat "$PROMPT")" --allow-all-tools --available-tools='' --disallow-temp-dir --no-custom-instructions --no-color --silent
```

```sh
WORKDIR=/tmp/verdict-skill-iterXX-agy
PROMPT=$WORKDIR/prompt.txt
agy --sandbox --print "$(cat "$PROMPT")"
```

```sh
WORKDIR=/tmp/verdict-skill-iterXX-hermes
PROMPT=$WORKDIR/prompt.txt
hermes --ignore-rules -t clarify -z "$(cat "$PROMPT")"
```

Hermes is usually run with `-t clarify`, so include all relevant code and SKILL text directly in the prompt. Do not assume Hermes can inspect the temp fixture unless wider tool access is enabled on purpose.

```sh
WORKDIR=/tmp/verdict-skill-iterXX-gemini
PROMPT=$WORKDIR/prompt.txt
gemini -p "$(cat "$PROMPT")" --skip-trust --sandbox --approval-mode plan --include-directories "$WORKDIR" --output-format text
```

Gemini supports headless prompt mode with `-p`, sandboxing with `--sandbox`, scoped visibility with `--include-directories`, and read-only approval with `--approval-mode plan`. Use `--skip-trust` for copied temp directories in headless runs. Use this read-only form first. If validation writes are needed, try `--approval-mode auto_edit` only in a worker-specific copied `WORKDIR`. Gemini may edit the temp fixture, but shell commands can still be unavailable, so Codex must rerun tests, benchmarks, and `verdict` independently.

After each worker run, check that the worker did not mutate the repository or its temp fixture unexpectedly:

```sh
git status --short
find "$WORKDIR" -maxdepth 3 -type f | sort
```
