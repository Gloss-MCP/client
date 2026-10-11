package store

import (
	"encoding/json"
	"fmt"
	"math"
)

// AnchorType discriminates the polymorphic anchor variants.
type AnchorType string

// Anchor variants (docs/data-model.md#anchor).
const (
	AnchorTypeLine       AnchorType = "line"
	AnchorTypeCell       AnchorType = "cell"
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

// CellAnchor anchors to one cell of a delimited-text file (csv). The row
// is identified by the physical line span of its record (a quoted field
// can make a record span several lines), so the line-based delta
// machinery locates it after edits; the column is a 1-based index kept
// in step with Header, the column's header text at anchor time.
type CellAnchor struct {
	StartLine int    `json:"start_line"`
	EndLine   int    `json:"end_line"`
	Col       int    `json:"col"`
	Header    string `json:"header"`
	// ContextBefore and ContextAfter hold surrounding context lines
	// captured for delta remapping, as for LineAnchor.
	ContextBefore []string `json:"context_before"`
	ContextAfter  []string `json:"context_after"`
}

// AnchorType implements Anchor.
func (CellAnchor) AnchorType() AnchorType { return AnchorTypeCell }

// RegionAnchor anchors to an XY region as fractions (0-1) of the rendered
// asset's width and height; used by images.
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

// ValidateAnchor reports whether a's geometry is well-formed. Text
// anchors are not checked here (the callers that build them validate line
// numbers against the file); region and time anchors are, because nothing
// else can: a region is fractions of the asset, so it must lie within
// [0,1] on both axes, and a time range must be non-negative and ordered.
func ValidateAnchor(a Anchor) error {
	switch v := a.(type) {
	case RegionAnchor:
		return validateRegion(v.X, v.Y, v.Width, v.Height)
	case TimeAnchor:
		return validateTimes(v.StartTime, v.EndTime)
	case RegionTimeAnchor:
		if err := validateRegion(v.X, v.Y, v.Width, v.Height); err != nil {
			return err
		}
		return validateTimes(v.StartTime, v.EndTime)
	}
	return nil
}

func validateRegion(x, y, w, h float64) error {
	const eps = 1e-9 // tolerate float error from x+width sums
	switch {
	case !finite(x, y, w, h):
		return fmt.Errorf("store: region must be finite numbers")
	case x < 0 || y < 0:
		return fmt.Errorf("store: region x and y must be >= 0 (fractions of the asset)")
	case w <= 0 || h <= 0:
		return fmt.Errorf("store: region width and height must be > 0")
	case x+w > 1+eps || y+h > 1+eps:
		return fmt.Errorf("store: region must lie within the asset (x+width <= 1, y+height <= 1)")
	}
	return nil
}

func validateTimes(start, end float64) error {
	switch {
	case !finite(start, end):
		return fmt.Errorf("store: time range must be finite numbers")
	case start < 0:
		return fmt.Errorf("store: start_time must be >= 0")
	case end < start:
		return fmt.Errorf("store: end_time must be >= start_time")
	}
	return nil
}

func finite(vs ...float64) bool {
	for _, v := range vs {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return false
		}
	}
	return true
}

// marshalAnchor serialises an anchor to its type discriminator and JSON
// payload for storage.
func marshalAnchor(a Anchor) (AnchorType, string, error) {
	if a == nil {
		return "", "", fmt.Errorf("store: anchor is required")
	}
	if err := ValidateAnchor(a); err != nil {
		return "", "", err
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
	case AnchorTypeCell:
		var a CellAnchor
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

// Label returns a short human-readable position for UI display, e.g.
// "line 3-5" or "line 4, col 2 (price)".
func Label(a Anchor) string {
	span := func(start, end int) string {
		if start == end {
			return fmt.Sprintf("line %d", start)
		}
		return fmt.Sprintf("line %d-%d", start, end)
	}
	switch v := a.(type) {
	case LineAnchor:
		return span(v.StartLine, v.EndLine)
	case CellAnchor:
		label := fmt.Sprintf("%s, col %d", span(v.StartLine, v.EndLine), v.Col)
		if v.Header != "" {
			label += " (" + v.Header + ")"
		}
		return label
	case RegionAnchor:
		return fmt.Sprintf("region %s,%s %s\u00d7%s", pct(v.X), pct(v.Y), pct(v.Width), pct(v.Height))
	case TimeAnchor:
		return fmt.Sprintf("%gs-%gs", v.StartTime, v.EndTime)
	case RegionTimeAnchor:
		return fmt.Sprintf("region %s,%s %s\u00d7%s at %gs-%gs", pct(v.X), pct(v.Y), pct(v.Width), pct(v.Height), v.StartTime, v.EndTime)
	}
	return ""
}

// pct formats a 0-1 fraction as a whole percentage.
func pct(f float64) string { return fmt.Sprintf("%.0f%%", f*100) }

// AnchorLabel returns the thread's position label (see Label).
func (t *Thread) AnchorLabel() string { return Label(t.Anchor) }
