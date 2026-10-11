package store

import (
	"math"
	"reflect"
	"testing"
)

func TestAnchorMarshalRoundTrip(t *testing.T) {
	anchors := []Anchor{
		LineAnchor{StartLine: 3, EndLine: 7, ContextBefore: []string{"a", "b"}, ContextAfter: []string{"c"}},
		CellAnchor{StartLine: 4, EndLine: 5, Col: 2, Header: "price", ContextBefore: []string{"a,b"}, ContextAfter: []string{"c,d"}},
		RegionAnchor{X: 0.105, Y: 0.2, Width: 0.3, Height: 0.4025},
		TimeAnchor{StartTime: 1.5, EndTime: 90},
		RegionTimeAnchor{X: 0.1, Y: 0.2, Width: 0.3, Height: 0.4, StartTime: 5.5, EndTime: 6},
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
		{RegionAnchor{X: 0.125, Y: 0.4, Width: 0.25, Height: 0.1}, "region 12%,40% 25%\u00d710%"},
		{RegionTimeAnchor{X: 0, Y: 0, Width: 0.5, Height: 0.5, StartTime: 2, EndTime: 3}, "region 0%,0% 50%\u00d750% at 2s-3s"},
		{TimeAnchor{StartTime: 1.5, EndTime: 3}, "1.5s-3s"},
	} {
		if got := Label(tt.a); got != tt.want {
			t.Errorf("Label(%#v) = %q, want %q", tt.a, got, tt.want)
		}
	}
}

func TestValidateAnchor(t *testing.T) {
	ok := RegionAnchor{X: 0.5, Y: 0.5, Width: 0.5, Height: 0.5}
	for name, tt := range map[string]struct {
		a       Anchor
		wantErr bool
	}{
		"line is not checked":    {LineAnchor{StartLine: 0}, false},
		"full-frame region":      {RegionAnchor{Width: 1, Height: 1}, false},
		"edge-touching region":   {ok, false},
		"float sum tolerated":    {RegionAnchor{X: 0.7, Width: 0.3, Height: 1}, false},
		"negative x":             {RegionAnchor{X: -0.1, Width: 0.5, Height: 0.5}, true},
		"zero width":             {RegionAnchor{Height: 0.5}, true},
		"overflows right":        {RegionAnchor{X: 0.6, Width: 0.5, Height: 0.5}, true},
		"overflows bottom":       {RegionAnchor{Y: 0.6, Width: 0.5, Height: 0.5}, true},
		"percent-style region":   {RegionAnchor{X: 10, Y: 20, Width: 30, Height: 40}, true},
		"NaN region":             {RegionAnchor{X: math.NaN(), Width: 0.5, Height: 0.5}, true},
		"instant time":           {TimeAnchor{StartTime: 2, EndTime: 2}, false},
		"negative start":         {TimeAnchor{StartTime: -1, EndTime: 2}, true},
		"end before start":       {TimeAnchor{StartTime: 3, EndTime: 2}, true},
		"infinite end":           {TimeAnchor{EndTime: math.Inf(1)}, true},
		"region_time ok":         {RegionTimeAnchor{Width: 0.5, Height: 0.5, EndTime: 1}, false},
		"region_time bad region": {RegionTimeAnchor{Width: 2, Height: 0.5, EndTime: 1}, true},
		"region_time bad time":   {RegionTimeAnchor{Width: 0.5, Height: 0.5, StartTime: 2, EndTime: 1}, true},
	} {
		t.Run(name, func(t *testing.T) {
			err := ValidateAnchor(tt.a)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateAnchor(%#v) = %v, wantErr %v", tt.a, err, tt.wantErr)
			}
			if _, _, merr := marshalAnchor(tt.a); (merr != nil) != tt.wantErr {
				t.Errorf("marshalAnchor err = %v, wantErr %v", merr, tt.wantErr)
			}
		})
	}
}
