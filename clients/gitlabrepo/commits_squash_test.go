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

package gitlabrepo

import (
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	gitlab "gitlab.com/gitlab-org/api/client-go"

	"github.com/ossf/scorecard/v5/clients"
)

func TestZipSquashCommitAssociation(t *testing.T) {
	t.Parallel()

	mergedAt := time.Date(2026, 11, 6, 16, 1, 39, 0, time.UTC)

	tests := []struct {
		want    map[string]int
		name    string
		commits []*gitlab.Commit
		mrs     []graphqlMergeRequestNode
	}{
		{
			// prev101 is the merge commit's first parent and stays unlinked.
			// feature101 is the pre-squash SHA and is not on the branch.
			want: map[string]int{
				"merge101":  101,
				"squash101": 101,
				"prev101":   0,
			},
			name: "squash commit is the merge commit second parent",
			commits: []*gitlab.Commit{
				glCommit("merge101", "prev101", "squash101"),
				glCommit("squash101", "base101"),
				glCommit("prev101", "base101"),
			},
			mrs: []graphqlMergeRequestNode{
				mergeRequestNode("101", "alice", "carol", "merge101", []string{"feature101"}, []string{"bob"}, mergedAt),
			},
		},
		{
			name: "ordinary merge head stays linked and is not reassigned",
			commits: []*gitlab.Commit{
				glCommit("merge102", "prev102", "head102"),
				glCommit("head102", "base102"),
			},
			mrs: []graphqlMergeRequestNode{
				mergeRequestNode("102", "dave", "frank", "merge102", []string{"head102"}, []string{"eve"}, mergedAt),
			},
			want: map[string]int{
				"merge102": 102,
				"head102":  102,
			},
		},
		{
			name: "second parent already linked to another merge request is kept",
			commits: []*gitlab.Commit{
				glCommit("merge1", "prev", "shared"),
				glCommit("shared", "prev"),
				glCommit("merge2", "prev", "head2"),
			},
			mrs: []graphqlMergeRequestNode{
				mergeRequestNode("1", "alice", "carol", "merge1", nil, []string{"bob"}, mergedAt),
				mergeRequestNode("2", "dave", "frank", "merge2", []string{"shared"}, []string{"eve"}, mergedAt),
			},
			want: map[string]int{
				"merge1": 1,
				"shared": 2,
				"merge2": 2,
			},
		},
		{
			name: "feature branch merge does not pull in an unrelated second parent",
			commits: []*gitlab.Commit{
				glCommit("merge7", "base", "head7"),
				glCommit("featuremerge", "head7", "direct"),
				glCommit("direct", "base"),
				glCommit("head7", "base"),
			},
			mrs: []graphqlMergeRequestNode{
				mergeRequestNode("7", "alice", "carol", "merge7", []string{"head7", "featuremerge"}, []string{"bob"}, mergedAt),
			},
			want: map[string]int{
				"merge7":       7,
				"head7":        7,
				"featuremerge": 7,
				"direct":       0,
			},
		},
		{
			name: "squash commit whose merge commit is outside the window stays unlinked",
			commits: []*gitlab.Commit{
				glCommit("squash-only", "base"),
			},
			mrs: []graphqlMergeRequestNode{
				mergeRequestNode("8", "alice", "carol", "merge-missing", []string{"feature8"}, []string{"bob"}, mergedAt),
			},
			want: map[string]int{
				"squash-only": 0,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var data graphqlData
			data.Project.MergeRequests.Nodes = tt.mrs
			got := (&commitsHandler{}).zip(tt.commits, data)

			linked := map[string]int{}
			for _, c := range got {
				if c.AssociatedMergeRequest.MergedAt.IsZero() {
					linked[c.SHA] = 0
					continue
				}
				linked[c.SHA] = c.AssociatedMergeRequest.Number
			}
			if diff := cmp.Diff(tt.want, linked); diff != "" {
				t.Errorf("merge request numbers mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

// squashCodeReviewFixture is the offline #5447 case: an approved squash merge,
// an approved ordinary merge, and an unapproved self-merge. The pre-squash
// SHA feature101 is not on the branch. zip links squash101 through the merge
// commit's second parent.
func squashCodeReviewFixture() []clients.Commit {
	mergedAt := time.Date(2026, 11, 6, 16, 1, 39, 0, time.UTC)
	rawCommits := []*gitlab.Commit{
		glCommit("merge101", "prev101", "squash101"),
		glCommit("squash101", "base101"),
		glCommit("merge102", "prev102", "head102"),
		glCommit("head102", "base102"),
		glCommit("merge103", "prev103", "head103"),
		glCommit("head103", "base103"),
	}
	var data graphqlData
	data.Project.MergeRequests.Nodes = []graphqlMergeRequestNode{
		mergeRequestNode("101", "alice", "carol", "merge101", []string{"feature101"}, []string{"bob"}, mergedAt),
		mergeRequestNode("102", "dave", "frank", "merge102", []string{"head102"}, []string{"eve"}, mergedAt),
		mergeRequestNode("103", "gina", "gina", "merge103", []string{"head103"}, nil, mergedAt),
	}
	return (&commitsHandler{}).zip(rawCommits, data)
}

func glCommit(sha string, parents ...string) *gitlab.Commit {
	when := time.Date(2026, 11, 6, 16, 0, 0, 0, time.UTC)
	return &gitlab.Commit{
		ID:            sha,
		Message:       sha,
		CommittedDate: &when,
		ParentIDs:     parents,
	}
}

func mergeRequestNode(iid, author, mergedBy, mergeSHA string, commitSHAs, approvers []string, mergedAt time.Time) graphqlMergeRequestNode {
	var node graphqlMergeRequestNode
	node.IID = iid
	node.MergedAt = mergedAt
	node.MergeCommitSHA = mergeSHA
	node.Author.Username = author
	node.MergedBy.Username = mergedBy
	for _, sha := range commitSHAs {
		node.Commits.Nodes = append(node.Commits.Nodes, struct {
			SHA string `graphql:"sha"`
		}{SHA: sha})
	}
	for _, name := range approvers {
		node.Approvers.Nodes = append(node.Approvers.Nodes, struct {
			Username string    `graphql:"username"`
			ID       GitlabGID `graphql:"id"`
		}{Username: name})
	}
	return node
}
