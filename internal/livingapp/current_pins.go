package livingapp

import (
	"github.com/twentyideas/changesaga/internal/currentrecord"
	"github.com/twentyideas/changesaga/internal/quality"
	"github.com/twentyideas/changesaga/internal/requirements"
)

func (s *session) currentContentPin(target string, content any) string {
	id := ""
	switch value := content.(type) {
	case *requirements.Revision:
		if value != nil {
			id = value.ID
		}
	case *requirements.PersonaRevision:
		if value != nil {
			id = value.ID
		}
	case *quality.Revision:
		if value != nil {
			id = value.ID
		}
	}
	if id == "" {
		return ""
	}
	pin := requirements.ContentPin(target, id)
	if currentrecord.ValidDigest(pin) {
		return pin
	}
	s.pinMu.Lock()
	defer s.pinMu.Unlock()
	if s.pins == nil && s.saga != nil {
		s.pins, _ = currentrecord.AuditPins(s.saga.Root)
	}
	if s.pins == nil {
		return ""
	}
	digest, _ := s.pins.Resolve(pin)
	return digest
}
