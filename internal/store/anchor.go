package store

import (
	"encoding/json"
	"fmt"
)

// AnchorType discriminates the polymorphic anchor variants.
type AnchorType string

// Anchor variants (docs/data-model.md#anchor).
const (
	AnchorTypeLine       AnchorType = "line"
	AnchorTypeRegion     AnchorType = "region"
	AnchorTypeTime       AnchorType = "time"
	AnchorTypeRegionTime AnchorType = "region_time"
)

// Anchor is the polymorphic position of a thread within a file — a
// dedicated sum type, not a grab-bag of nullable columns. The rendering
// plugin decides which variant applies to its file type; the store
// persists the variant's fields as opaque JSON alongside a type
// discriminator.
type Anchor interface {
	AnchorType() AnchorType
}

// LineAnchor anchors to a line range; used by text formats (code,
// markdown, json, csv).
type LineAnchor struct {
	StartLine int `json:"start_line"`
	EndLine   int `json:"end_line"`
	// ContextBefore and ContextAfter hold surrounding context lines
	// captured for delta remapping (milestone 7).
	ContextBefore []string `json:"context_before"`
	ContextAfter  []string `json:"context_after"`
}

// AnchorType implements Anchor.
func (LineAnchor) AnchorType() AnchorType { return AnchorTypeLine }

// RegionAnchor anchors to an XY region as percentages of the rendered
// asset; used by images.
type RegionAnchor struct {
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
}

// AnchorType implements Anchor.
func (RegionAnchor) AnchorType() AnchorType { return AnchorTypeRegion }

// TimeAnchor anchors to a time range in seconds; used by audio.
type TimeAnchor struct {
	StartTime float64 `json:"start_time"`
	EndTime   float64 `json:"end_time"`
}

// AnchorType implements Anchor.
func (TimeAnchor) AnchorType() AnchorType { return AnchorTypeTime }

// RegionTimeAnchor anchors to an XY region at a point in time; used by
// video.
type RegionTimeAnchor struct {
	X         float64 `json:"x"`
	Y         float64 `json:"y"`
	Width     float64 `json:"width"`
	Height    float64 `json:"height"`
	StartTime float64 `json:"start_time"`
	EndTime   float64 `json:"end_time"`
}

// AnchorType implements Anchor.
func (RegionTimeAnchor) AnchorType() AnchorType { return AnchorTypeRegionTime }

// marshalAnchor serialises an anchor to its type discriminator and JSON
// payload for storage.
func marshalAnchor(a Anchor) (AnchorType, string, error) {
	if a == nil {
		return "", "", fmt.Errorf("store: anchor is required")
	}
	data, err := json.Marshal(a)
	if err != nil {
		return "", "", fmt.Errorf("store: marshal anchor: %w", err)
	}
	return a.AnchorType(), string(data), nil
}

// unmarshalAnchor reconstructs an anchor variant from its stored type
// discriminator and JSON payload. An unknown type is an error — anchors
// are never silently dropped.
func unmarshalAnchor(anchorType AnchorType, data string) (Anchor, error) {
	unmarshal := func(v Anchor) error {
		if err := json.Unmarshal([]byte(data), v); err != nil {
			return fmt.Errorf("store: unmarshal %s anchor: %w", anchorType, err)
		}
		return nil
	}
	switch anchorType {
	case AnchorTypeLine:
		var a LineAnchor
		if err := unmarshal(&a); err != nil {
			return nil, err
		}
		return a, nil
	case AnchorTypeRegion:
		var a RegionAnchor
		if err := unmarshal(&a); err != nil {
			return nil, err
		}
		return a, nil
	case AnchorTypeTime:
		var a TimeAnchor
		if err := unmarshal(&a); err != nil {
			return nil, err
		}
		return a, nil
	case AnchorTypeRegionTime:
		var a RegionTimeAnchor
		if err := unmarshal(&a); err != nil {
			return nil, err
		}
		return a, nil
	default:
		return nil, fmt.Errorf("store: unknown anchor type %q", anchorType)
	}
}
