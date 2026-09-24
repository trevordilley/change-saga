package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestStoryWithdrawReplaysIdenticalRequestAndStillRefusesAcceptedIntent(t *testing.T) {
	root := newLivingSaga(t)
	withdrawn := addStory(t, root, testFeature, "duplicate").Resource
	args := []string{
		"--story", withdrawn,
		"--parent", withdrawn + ":event:proposed",
		"--event", "withdrawn-as-duplicate",
		"--reason", "The canonical proposal already preserves this intent",
		"--request-id", "withdraw-duplicate-v1",
		"--json", root,
	}
	var output bytes.Buffer
	if err := storyWithdraw(context.Background(), args, &output); err != nil {
		t.Fatal(err)
	}
	first := decodeLivingOutput(t, &output)
	if first.Replayed {
		t.Fatalf("first withdrawal was reported as replayed: %#v", first)
	}
	output.Reset()
	if err := storyWithdraw(context.Background(), args, &output); err != nil {
		t.Fatalf("identical withdrawal retry failed: %v", err)
	}
	second := decodeLivingOutput(t, &output)
	if !second.Replayed || second.Resource != first.Resource {
		t.Fatalf("withdrawal replay = %#v, first = %#v", second, first)
	}

	accepted := addStory(t, root, testFeature, "accepted").Resource
	acceptStory(t, root, accepted)
	output.Reset()
	err := storyWithdraw(context.Background(), []string{
		"--story", accepted,
		"--parent", accepted + ":event:accepted",
		"--event", "withdraw-accepted",
		"--reason", "Must not discard accepted intent",
		"--request-id", "withdraw-accepted-v1",
		root,
	}, &output)
	if err == nil || !strings.Contains(err.Error(), "only a proposed or deferred story") {
		t.Fatalf("accepted withdrawal error = %v", err)
	}
}
