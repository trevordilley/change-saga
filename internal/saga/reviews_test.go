package saga

import (
	"testing"
)

func TestReviewerIdentityValidationDistinguishesHumanAndAI(t *testing.T) {
	valid := []*ReviewerIdentity{nil, {Kind: "human"}, {Kind: "ai", Name: "Codex 1", Agent: "codex", Model: "gpt-5.6-sol"}}
	for _, reviewer := range valid {
		if err := ValidateReviewerIdentity(reviewer); err != nil {
			t.Errorf("valid identity %#v: %v", reviewer, err)
		}
	}
	invalid := []*ReviewerIdentity{
		{Kind: ""},
		{Kind: "robot"},
		{Kind: "human", Model: "gpt-5.6-sol"},
		{Kind: "ai", Agent: "codex", Model: "gpt-5.6-sol"},
		{Kind: "ai", Agent: "codex"},
		{Kind: "ai", Model: "gpt-5.6-sol"},
	}
	for _, reviewer := range invalid {
		if err := ValidateReviewerIdentity(reviewer); err == nil {
			t.Errorf("invalid identity %#v was accepted", reviewer)
		}
	}
}
