package store

import (
	"reflect"
	"testing"
)

func TestAnchorMarshalRoundTrip(t *testing.T) {
	anchors := []Anchor{
		LineAnchor{StartLine: 3, EndLine: 7, ContextBefore: []string{"a", "b"}, ContextAfter: []string{"c"}},
		CellAnchor{StartLine: 4, EndLine: 5, Col: 2, Header: "price", ContextBefore: []string{"a,b"}, ContextAfter: []string{"c,d"}},
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

func TestAnchorLabel(t *testing.T) {
	for _, tt := range []struct {
		a    Anchor
		want string
	}{
		{LineAnchor{StartLine: 3, EndLine: 3}, "line 3"},
		{LineAnchor{StartLine: 3, EndLine: 5}, "line 3-5"},
		{CellAnchor{StartLine: 4, EndLine: 4, Col: 2, Header: "price"}, "line 4, col 2 (price)"},
		{CellAnchor{StartLine: 4, EndLine: 6, Col: 1}, "line 4-6, col 1"},
		{RegionAnchor{}, "region"},
		{TimeAnchor{StartTime: 1.5, EndTime: 3}, "1.5s-3s"},
	} {
		if got := Label(tt.a); got != tt.want {
			t.Errorf("Label(%#v) = %q, want %q", tt.a, got, tt.want)
		}
	}
}
