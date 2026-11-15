package tools

import (
	"context"
	"regexp"
)

type DiffSegment struct {
	Op   string `json:"op"` // equal | add | del
	Text string `json:"text"`
}

var tokenRe = regexp.MustCompile(`\s+|\S+`)

// Differ computes a word-level diff (GitHub-style "before / after").
type Differ struct{}

func (Differ) Name() string { return "diff" }
func (Differ) Description() string {
	return "Word-level diff between the previous and proposed text of a PRD block."
}

func (Differ) Diff(_ context.Context, old, new string) []DiffSegment {
	a, b := tokenRe.FindAllString(old, -1), tokenRe.FindAllString(new, -1)
	if len(a)*len(b) > 4_000_000 { // too big: show as full replace
		return []DiffSegment{{"del", old}, {"add", new}}
	}
	// LCS table
	dp := make([][]int, len(a)+1)
	for i := range dp {
		dp[i] = make([]int, len(b)+1)
	}
	for i := len(a) - 1; i >= 0; i-- {
		for j := len(b) - 1; j >= 0; j-- {
			if a[i] == b[j] {
				dp[i][j] = dp[i+1][j+1] + 1
			} else {
				dp[i][j] = max(dp[i+1][j], dp[i][j+1])
			}
		}
	}
	var segs []DiffSegment
	push := func(op, t string) {
		if n := len(segs); n > 0 && segs[n-1].Op == op {
			segs[n-1].Text += t
			return
		}
		segs = append(segs, DiffSegment{op, t})
	}
	i, j := 0, 0
	for i < len(a) && j < len(b) {
		switch {
		case a[i] == b[j]:
			push("equal", a[i])
			i++
			j++
		case dp[i+1][j] >= dp[i][j+1]:
			push("del", a[i])
			i++
		default:
			push("add", b[j])
			j++
		}
	}
	for ; i < len(a); i++ {
		push("del", a[i])
	}
	for ; j < len(b); j++ {
		push("add", b[j])
	}
	return segs
}
