package requests

import (
	"context"
	"log"

	"github.com/klzn-labs/kalzone/app/approvals/entities"
	"github.com/klzn-labs/kalzone/pkg/approval"
)

type previewDTO struct {
	Lines    []lineDTO       `json:"lines,omitempty"`
	Moved    string          `json:"moved,omitempty"`
	Conflict string          `json:"conflict,omitempty"`
	Ships    *approval.Ships `json:"ships,omitempty"`
}

type lineDTO struct {
	Key    string `json:"key"`
	Before string `json:"before"`
	After  string `json:"after"`
}

func (h RequestsHandler) previewOf(ctx context.Context, r entities.Request) *previewDTO {
	p, err := h.requests.Preview(ctx, r)
	if err != nil {
		log.Printf("preview of change request %s: %v", r.ID, err)
	}
	if !r.Pending() || err != nil {
		p = approval.Preview{Ships: p.Ships}
	}
	return toPreviewDTO(p)
}

func toPreviewDTO(p approval.Preview) *previewDTO {
	if len(p.Lines) == 0 && p.Ships == nil && p.Moved == "" && p.Conflict == "" {
		return nil
	}
	dto := &previewDTO{Moved: p.Moved, Conflict: p.Conflict, Ships: p.Ships}
	for _, l := range p.Lines {
		dto.Lines = append(dto.Lines, lineDTO{Key: l.Key, Before: l.Before, After: l.After})
	}
	return dto
}
