package prototypes

import "github.com/twentyideas/changesaga/internal/currentrecord"

func currentMutationResult(result MutationResult) MutationResult {
	result.URN = currentrecord.StableReference(result.URN)
	result.Path = currentrecord.MemberPath(result.Path)
	return result
}
