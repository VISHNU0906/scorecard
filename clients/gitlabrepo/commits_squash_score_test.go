// Copyright 2026 OpenSSF Scorecard Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package gitlabrepo_test

import (
	"slices"
	"testing"

	"github.com/google/go-cmp/cmp"
	"go.uber.org/mock/gomock"

	"github.com/ossf/scorecard/v5/checker"
	"github.com/ossf/scorecard/v5/checks/evaluation"
	"github.com/ossf/scorecard/v5/checks/raw"
	"github.com/ossf/scorecard/v5/clients/gitlabrepo"
	mockrepo "github.com/ossf/scorecard/v5/clients/mockclients"
	"github.com/ossf/scorecard/v5/internal/checknames"
	"github.com/ossf/scorecard/v5/probes/codeApproved"
	scut "github.com/ossf/scorecard/v5/utests"
)

// TestSquashCommitLowersCodeReviewScore is the offline fixture from
// ossf/scorecard#5447. It runs zip, then raw.CodeReview, codeApproved, and
// evaluation.CodeReview. Grouping the squash commit with its merge request
// reports 2/3 approved changesets. Leaving it unlinked reports 2/4.
func TestSquashCommitLowersCodeReviewScore(t *testing.T) {
	t.Parallel()

	commits := gitlabrepo.ZipSquashCodeReviewFixture()

	ctrl := gomock.NewController(t)
	repo := mockrepo.NewMockRepoClient(ctrl)
	repo.EXPECT().ListCommits().Return(commits, nil)

	reviewData, err := raw.CodeReview(repo)
	if err != nil {
		t.Fatalf("raw.CodeReview: %v", err)
	}

	byRev := map[string][]string{}
	for _, cs := range reviewData.DefaultBranchChangesets {
		shas := make([]string, 0, len(cs.Commits))
		for _, c := range cs.Commits {
			shas = append(shas, c.SHA)
		}
		slices.Sort(shas)
		byRev[cs.RevisionID] = shas
	}
	wantSets := map[string][]string{
		"101": {"merge101", "squash101"},
		"102": {"head102", "merge102"},
		"103": {"head103", "merge103"},
	}
	if diff := cmp.Diff(wantSets, byRev); diff != "" {
		t.Errorf("changesets mismatch (-want +got):\n%s", diff)
	}

	findings, _, err := codeApproved.Run(&checker.RawResults{CodeReviewResults: reviewData})
	if err != nil {
		t.Fatalf("codeApproved.Run: %v", err)
	}
	result := evaluation.CodeReview(checknames.CodeReview, findings, &scut.TestDetailLogger{})
	if result.Error != nil {
		t.Fatalf("evaluation.CodeReview: %v", result.Error)
	}
	const wantReason = "Found 2/3 approved changesets -- score normalized to 6"
	if result.Score != 6 || result.Reason != wantReason {
		t.Fatalf("score = %d, reason = %q, want 6, %q", result.Score, result.Reason, wantReason)
	}
}
