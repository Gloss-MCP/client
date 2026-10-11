package plugins

// MaxRenderBytes is the largest file a format-specific plugin renders.
// Beyond it the file falls back to plaintext, which is still
// line-annotatable but skips tokenising, markdown parsing and table
// building that would stall a page load on a huge generated file.
const MaxRenderBytes = 1 << 20

// sizeCapped wraps a format-specific plugin so oversized files render as
// plaintext instead.
type sizeCapped struct {
	Plugin
}

// Render implements Plugin.
func (c sizeCapped) Render(f File) ([]View, error) {
	if len(f.Content) > MaxRenderBytes {
		return NewPlaintext().Render(f)
	}
	return c.Plugin.Render(f)
}

// NewDefaultRegistry returns the registry gloss ships with. Order is
// precedence: media (served from /raw, with its own size cap) and specific formats first, then the broad syntax-highlighting
// plugin (which accepts any extension chroma knows, including the ones
// claimed above it), then plaintext as the catch-all.
func NewDefaultRegistry() *Registry {
	return NewRegistry(
		NewImage(), // before Code, whose lexers claim .svg
		sizeCapped{NewMarkdown()},
		sizeCapped{NewJSON()},
		sizeCapped{NewCSV()},
		sizeCapped{NewCode()},
		NewPlaintext(),
	)
}
