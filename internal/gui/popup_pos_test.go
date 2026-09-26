package gui

import (
	"testing"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// TestPopupScreenAnchor pins the top-right work-area maths the Linux bubble
// relies on (macOS anchors natively): right edge minus width and margin, top
// edge plus margin.
func TestPopupScreenAnchor(t *testing.T) {
	wa := application.Rect{X: 0, Y: 32, Width: 1920, Height: 1040}
	cases := []struct {
		name         string
		wa           application.Rect
		wantX, wantY int
	}{
		{"plain", wa, 1920 - popupWidth - popupMargin, 32 + popupMargin},
		{"secondary screen offset", application.Rect{X: 1920, Width: 1280}, 1920 + 1280 - popupWidth - popupMargin, popupMargin},
		{"narrow work area clamps to left margin", application.Rect{X: 0, Width: 200}, popupMargin, popupMargin},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			x, y := popupScreenAnchor(tc.wa, popupWidth)
			if x != tc.wantX || y != tc.wantY {
				t.Errorf("popupScreenAnchor = (%d,%d), want (%d,%d)", x, y, tc.wantX, tc.wantY)
			}
		})
	}
}
