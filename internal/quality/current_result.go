package quality

import "github.com/twentyideas/changesaga/internal/currentrecord"

func currentMutationResult(result MutationResult) MutationResult {
	mutable := currentrecord.StableReference(result.URN) != result.URN || currentrecord.MemberPath(result.Path) != result.Path
	for _, created := range result.Created {
		mutable = mutable || currentrecord.StableReference(created) != created
	}
	for _, path := range result.Paths {
		mutable = mutable || currentrecord.MemberPath(path) != path
	}
	if !mutable {
		return result
	}
	result.URN = currentrecord.StableReference(result.URN)
	result.Path = currentrecord.MemberPath(result.Path)
	result.CurrentHeads = nil
	seen := map[string]bool{}
	paths := []string{}
	for _, path := range result.Paths {
		path = currentrecord.MemberPath(path)
		if !seen[path] {
			seen[path] = true
			paths = append(paths, path)
		}
	}
	result.Paths = paths
	result.Created = []string{result.URN}
	return result
}
