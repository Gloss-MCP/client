package store

import (
	"reflect"
	"testing"
)

func TestAnchorMarshalRoundTrip(t *testing.T) {
	anchors := []Anchor{
		LineAnchor{StartLine: 3, EndLine: 7, ContextBefore: []string{"a", "b"}, ContextAfter: []string{"c"}},
		RegionAnchor{X: 10.5, Y: 20, Width: 30, Height: 40.25},
		TimeAnchor{StartTime: 1.5, EndTime: 90},
		RegionTimeAnchor{X: 1, Y: 2, Width: 3, Height: 4, StartTime: 5.5, EndTime: 6},
	}

	for _, want := range anchors {
		t.Run(string(want.AnchorType()), func(t *testing.T) {
			anchorType, data, err := marshalAnchor(want)
			if err != nil {
				t.Fatalf("marshalAnchor: %v", err)
			}
			if anchorType != want.AnchorType() {
				t.Errorf("type = %q, want %q", anchorType, want.AnchorType())
			}
			got, err := unmarshalAnchor(anchorType, data)
			if err != nil {
				t.Fatalf("unmarshalAnchor: %v", err)
			}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("round trip = %#v, want %#v", got, want)
			}
		})
	}
}

func TestMarshalAnchorNil(t *testing.T) {
	if _, _, err := marshalAnchor(nil); err == nil {
		t.Fatal("marshalAnchor(nil) succeeded, want error")
	}
}

func TestUnmarshalAnchorUnknownType(t *testing.T) {
	if _, err := unmarshalAnchor("hologram", `{}`); err == nil {
		t.Fatal("unmarshalAnchor with unknown type succeeded, want error")
	}
}

func TestUnmarshalAnchorBadJSON(t *testing.T) {
	if _, err := unmarshalAnchor(AnchorTypeLine, `{not json`); err == nil {
		t.Fatal("unmarshalAnchor with bad JSON succeeded, want error")
	}
}
