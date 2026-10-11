// gloss.js -- small hand-written Alpine.js helpers for the annotation
// UI. Not vendored (it's app code, same status as app.css): served
// same-origin, no build step, no CDN.

// glossCodeView backs the code pane's line selection and the
// gutter highlighting of already-annotated lines. It operates
// generically on [data-line] elements -- the join key any
// line-anchoring plugin's rendered HTML is expected to carry
// (internal/plugins/plaintext.go) -- rather than knowing anything about
// how a given plugin rendered them.
function glossCodeView(threadLines) {
  return {
    selStart: null,
    selEnd: null,
    threadLines: threadLines || [],

    // span returns the [start, end] source lines an element covers.
    // Line-per-element plugins carry only data-line; block-per-element
    // plugins (markdown's rendered view) also carry data-line-end.
    span(el) {
      const start = parseInt(el.dataset.line, 10);
      const end = el.dataset.lineEnd ? parseInt(el.dataset.lineEnd, 10) : start;
      return [start, end];
    },

    init() {
      this.$el.querySelectorAll('[data-line]').forEach((el) => {
        const [start, end] = this.span(el);
        if (this.threadLines.some((n) => n >= start && n <= end)) {
          el.classList.add('bg-amber-50');
        }
      });
      this.$watch('selStart', () => this.paintSelection());
      this.$watch('selEnd', () => this.paintSelection());
    },

    onLineClick(evt) {
      const el = evt.target.closest('[data-line]');
      if (!el) return;
      const [start, end] = this.span(el);
      if (evt.shiftKey && this.selStart !== null) {
        this.selEnd = start >= this.selStart ? end : start;
      } else {
        this.selStart = start;
        this.selEnd = end;
      }
    },

    selRange() {
      if (this.selStart === null) return { start: null, end: null };
      return {
        start: Math.min(this.selStart, this.selEnd),
        end: Math.max(this.selStart, this.selEnd),
      };
    },

    selLabel() {
      const r = this.selRange();
      if (r.start === null) return '';
      return r.start === r.end ? String(r.start) : r.start + '-' + r.end;
    },

    clearSelection() {
      this.selStart = null;
      this.selEnd = null;
    },

    paintSelection() {
      const r = this.selRange();
      this.$el.querySelectorAll('[data-line]').forEach((el) => {
        const [start, end] = this.span(el);
        el.classList.toggle('bg-blue-50', r.start !== null && start <= r.end && end >= r.start);
      });
    },
  };
}
