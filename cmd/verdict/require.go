package main

import (
	"errors"
	"fmt"
	"strings"

	"github.com/KEINOS/go-verdict/verdict"
)

const requiredOutcomeSep = ", "

var errRequiredOutcome = errors.New("required outcome not met")

type outcomeSet map[verdict.Outcome]struct{}

func parseRequiredOutcomes(input string) (outcomeSet, error) {
	result := make(outcomeSet)
	if input == "" {
		return result, nil
	}

	for field := range strings.SplitSeq(input, ",") {
		outcome := verdict.Outcome(strings.TrimSpace(field))
		if !isValidOutcome(outcome) {
			return nil, fmt.Errorf(
				"%w: %q (expected one of: %s)",
				errUnknownRequired,
				field,
				strings.Join(validOutcomeNames(), requiredOutcomeSep),
			)
		}

		result[outcome] = struct{}{}
	}

	return result, nil
}

func requireReportOutcomes(report verdict.Report, required outcomeSet) error {
	if len(required) == 0 {
		return nil
	}

	missing := make([]string, 0)

	for _, item := range report.Verdicts {
		if _, ok := required[item.Outcome]; ok {
			continue
		}

		missing = append(missing, fmt.Sprintf("%s: %s", item.Benchmark, item.Outcome))
	}

	if len(missing) == 0 {
		return nil
	}

	return fmt.Errorf(
		"%w: %s (required: %s)",
		errRequiredOutcome,
		strings.Join(missing, requiredOutcomeSep),
		strings.Join(required.names(), requiredOutcomeSep),
	)
}

func (set outcomeSet) names() []string {
	names := make([]string, 0, len(set))
	for _, name := range validOutcomeNames() {
		if _, ok := set[verdict.Outcome(name)]; ok {
			names = append(names, name)
		}
	}

	return names
}

func isValidOutcome(outcome verdict.Outcome) bool {
	for _, name := range validOutcomeNames() {
		if outcome == verdict.Outcome(name) {
			return true
		}
	}

	return false
}

func validOutcomeNames() []string {
	return []string{
		string(verdict.NewWins),
		string(verdict.OldWins),
		string(verdict.Tie),
		string(verdict.TradeOff),
		string(verdict.Inconclusive),
	}
}
