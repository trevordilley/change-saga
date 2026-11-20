package prototypes

import (
	"encoding/json"
	"github.com/twentyideas/changesaga/internal/currentrecord"
	"strings"
)

func (value Revision) MarshalJSON() ([]byte, error) {
	type plain Revision
	if strings.HasPrefix(value.ID, "sha256-") && currentrecord.ValidDigest("sha256:"+strings.TrimPrefix(value.ID, "sha256-")) {
		return currentrecord.Content(plain(value))
	}
	return json.Marshal(plain(value))
}
