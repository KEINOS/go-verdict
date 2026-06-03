package verdict

func inconclusiveReport(reason string) Report {
	return labeledInconclusiveReport(reason, "", "")
}

func labeledInconclusiveReport(reason, baselineLabel, candidateLabel string) Report {
	return Report{
		Verdicts: []BenchmarkVerdict{{
			Benchmark:      "all",
			Outcome:        Inconclusive,
			Winner:         "",
			BaselineLabel:  baselineLabel,
			CandidateLabel: candidateLabel,
			Metrics:        nil,
			Reason:         "inconclusive input",
			ReasonCode:     reason,
		}},
	}
}
