// gloss.js -- small hand-written Alpine.js helpers for the annotation
// UI. Not vendored (it's app code, same status as app.css): served
// same-origin, no build step, no CDN.

// glossCodeView backs the code pane's line selection and the
// gutter highlighting of already-annotated lines. It operates
// generically on [data-line] elements -- the join key any
// line-anchoring plugin's rendered HTML is expected to carry
// (internal/plugins/lines.go) -- rather than knowing anything about how
// a given plugin rendered them. Elements covering several lines (a
// markdown block, a csv record) also carry data-line-end; csv cells
// carry data-col, which turns a click into a cell selection.
function glossCodeView(threadLines) {
  return {
    selStart: null,
    selEnd: null,
    // selCol is the 1-based column when a single csv cell is selected
    // (elements carrying data-col), else null for a plain line range.
    selCol: null,
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
      this.$watch('selCol', () => this.paintSelection());
    },

    onLineClick(evt) {
      const el = evt.target.closest('[data-line]');
      if (!el) return;
      const [start, end] = this.span(el);
      if (evt.shiftKey && this.selStart !== null) {
        this.selEnd = start >= this.selStart ? end : start;
        this.selCol = null; // a multi-row range is no longer one cell
      } else {
        this.selStart = start;
        this.selEnd = end;
        const cell = evt.target.closest('[data-col]');
        this.selCol = cell && el.contains(cell) ? parseInt(cell.dataset.col, 10) : null;
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
      const lines = r.start === r.end ? String(r.start) : r.start + '-' + r.end;
      return this.selCol === null ? lines : lines + ', column ' + this.selCol;
    },

    clearSelection() {
      this.selStart = null;
      this.selEnd = null;
      this.selCol = null;
    },

    paintSelection() {
      const r = this.selRange();
      this.$el.querySelectorAll('[data-line]').forEach((el) => {
        const [start, end] = this.span(el);
        el.classList.toggle('bg-blue-50', r.start !== null && start <= r.end && end >= r.start);
      });
      this.$el.querySelectorAll('[data-col]').forEach((cell) => {
        const row = cell.closest('[data-line]');
        const picked = this.selCol !== null && row !== null &&
          parseInt(row.dataset.line, 10) === r.start &&
          parseInt(cell.dataset.col, 10) === this.selCol;
        cell.classList.toggle('gloss-cell-selected', picked);
      });
    },
  };
}
