Your role is a worker for optimizing Go code.

WORKDIR: `<GO_VERDICT_REPO>/testdata/skill-fixtures/go-optimization/digitcount/`

NEVER edit outside WORKDIR. Work only inside WORKDIR.

Use the relevant skill(s).

Task: Optimize the Go code in this directory.

Return the final answer in this run. If you report a measured command, leave files needed to rerun that command in WORKDIR. If you did not run a check, do not claim it passed. If you did not produce a measured result, say:

Result: No measured result available.
Decision: No decision yet.

After you have correctness test results and a measured `verdict` result, stop and report. Do not continue into unrelated lint cleanup, formatting experiments, or extra refactors unless needed for tests or the measured verdict command to run.

Return:

- final answer or changed files
- exact commands and measured results, or planned commands clearly labeled
- concise decision and caveat
