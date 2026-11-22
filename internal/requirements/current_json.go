package requirements

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
func (value PersonaRevision) MarshalJSON() ([]byte, error) {
	type plain PersonaRevision
	if strings.HasPrefix(value.ID, "sha256-") && currentrecord.ValidDigest("sha256:"+strings.TrimPrefix(value.ID, "sha256-")) {
		return currentrecord.Content(plain(value))
	}
	return json.Marshal(plain(value))
}
func (value FlagRevision) MarshalJSON() ([]byte, error) {
	type plain FlagRevision
	if strings.HasPrefix(value.ID, "sha256-") && currentrecord.ValidDigest("sha256:"+strings.TrimPrefix(value.ID, "sha256-")) {
		return currentrecord.Content(plain(value))
	}
	return json.Marshal(plain(value))
}
func (value TermRevision) MarshalJSON() ([]byte, error) {
	type plain TermRevision
	if strings.HasPrefix(value.ID, "sha256-") && currentrecord.ValidDigest("sha256:"+strings.TrimPrefix(value.ID, "sha256-")) {
		return currentrecord.Content(plain(value))
	}
	return json.Marshal(plain(value))
}
func (value DeprecationRevision) MarshalJSON() ([]byte, error) {
	type plain DeprecationRevision
	if strings.HasPrefix(value.ID, "sha256-") && currentrecord.ValidDigest("sha256:"+strings.TrimPrefix(value.ID, "sha256-")) {
		return currentrecord.Content(plain(value))
	}
	return json.Marshal(plain(value))
}
func (value TechnicalRevision) MarshalJSON() ([]byte, error) {
	type plain TechnicalRevision
	if strings.HasPrefix(value.ID, "sha256-") && currentrecord.ValidDigest("sha256:"+strings.TrimPrefix(value.ID, "sha256-")) {
		return currentrecord.Content(plain(value))
	}
	return json.Marshal(plain(value))
}
func (value WhiteboardRevision) MarshalJSON() ([]byte, error) {
	type plain WhiteboardRevision
	if strings.HasPrefix(value.ID, "sha256-") && currentrecord.ValidDigest("sha256:"+strings.TrimPrefix(value.ID, "sha256-")) {
		return currentrecord.Content(plain(value))
	}
	return json.Marshal(plain(value))
}

func (value LifecycleEvent) MarshalJSON() ([]byte, error) {
	type plain LifecycleEvent
	if value.ID == "current" && len(value.Parents) == 0 {
		return currentrecord.Content(plain(value))
	}
	return json.Marshal(plain(value))
}

func (value PersonaEvent) MarshalJSON() ([]byte, error) {
	type plain PersonaEvent
	if value.ID == "current" && len(value.Parents) == 0 {
		return currentrecord.Content(plain(value))
	}
	return json.Marshal(plain(value))
}

func (value FlagEvent) MarshalJSON() ([]byte, error) {
	type plain FlagEvent
	if value.ID == "current" && len(value.Parents) == 0 {
		return currentrecord.Content(plain(value))
	}
	return json.Marshal(plain(value))
}

func (value TermEvent) MarshalJSON() ([]byte, error) {
	type plain TermEvent
	if value.ID == "current" && len(value.Parents) == 0 {
		return currentrecord.Content(plain(value))
	}
	return json.Marshal(plain(value))
}

func (value DeprecationEvent) MarshalJSON() ([]byte, error) {
	type plain DeprecationEvent
	if value.ID == "current" && len(value.Parents) == 0 {
		return currentrecord.Content(plain(value))
	}
	return json.Marshal(plain(value))
}

func (value TechnicalEvent) MarshalJSON() ([]byte, error) {
	type plain TechnicalEvent
	if value.ID == "current" && len(value.Parents) == 0 {
		return currentrecord.Content(plain(value))
	}
	return json.Marshal(plain(value))
}
